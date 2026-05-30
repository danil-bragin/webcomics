package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/example/dddcqrs/internal/app/bus"
	projcmd "github.com/example/dddcqrs/internal/app/command/projects"
)

// FirefoxLoginConfig — host paths needed to orchestrate jlesage/firefox
// containers and to compute the in-worker profile path stored on
// SocialAccount.firefox_profile_path.
type FirefoxLoginConfig struct {
	// Host (the machine running docker) directory mounted into worker-upload
	// containers under /profiles. Per-session subdir is created here.
	HostProfilesDir string
	// In-container mount point on worker-upload; usually "/profiles".
	WorkerMountPoint string
	// Docker image to spawn for one-shot login.
	Image string
	// Allowed host port range.
	BasePort int
	MaxPort  int
}

func (c FirefoxLoginConfig) enabled() bool { return c.HostProfilesDir != "" }

type fxSession struct {
	ID           string    `json:"id"`
	Port         int       `json:"port"`
	Container    string    `json:"container"`
	HostDir      string    `json:"host_dir"`
	VNCURL       string    `json:"vnc_url"`
	Status       string    `json:"status"` // starting | ready | finished | error
	ProjectID    string    `json:"project_id"`
	Platform     string    `json:"platform"`
	Label        string    `json:"label"`
	ProfileInner string    `json:"profile_inner,omitempty"` // computed at finish
	CreatedAt    time.Time `json:"created_at"`
	ErrMsg       string    `json:"error,omitempty"`
}

type fxLoginMgr struct {
	cfg      FirefoxLoginConfig
	mu       sync.Mutex
	sessions map[string]*fxSession
	usedPort map[int]bool
}

func newFxLoginMgr(cfg FirefoxLoginConfig) *fxLoginMgr {
	if cfg.Image == "" {
		cfg.Image = "jlesage/firefox:latest"
	}
	if cfg.WorkerMountPoint == "" {
		cfg.WorkerMountPoint = "/profiles"
	}
	if cfg.BasePort == 0 {
		cfg.BasePort = 5810
	}
	if cfg.MaxPort == 0 {
		cfg.MaxPort = 5899
	}
	m := &fxLoginMgr{cfg: cfg, sessions: map[string]*fxSession{}, usedPort: map[int]bool{}}
	// Stale containers from a previous API process hold ports that net.Listen
	// can't see (docker-proxy binds them). Reap orphaned wcm-fx-* on startup.
	_ = exec.Command("sh", "-c", "docker ps -aq --filter name=wcm-fx- | xargs -r docker rm -f").Run()
	return m
}

func (m *fxLoginMgr) allocPort() (int, error) {
	for p := m.cfg.BasePort; p <= m.cfg.MaxPort; p++ {
		if m.usedPort[p] {
			continue
		}
		if isPortFree(p) {
			m.usedPort[p] = true
			return p, nil
		}
	}
	return 0, errors.New("no free port in firefox-login range")
}

func isPortFree(p int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func dockerRun(args ...string) error {
	cmd := exec.Command("docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func waitPortReady(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("http://127.0.0.1:%d/", port)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		// noVNC returns the launcher HTML once jlesage's nginx is up; before
		// that, port may accept TCP but close immediately. HTTP probe avoids
		// races where the iframe loads on a half-ready socket.
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	_ = net.Dialer{Timeout: 100 * time.Millisecond}
	return fmt.Errorf("port %d not serving HTTP 200 within %s", port, timeout)
}

func (m *fxLoginMgr) start(ctx context.Context, projectID, platform, label string) (*fxSession, error) {
	if !m.cfg.enabled() {
		return nil, errors.New("firefox-login disabled: set FIREFOX_PROFILES_DIR env to an absolute host path")
	}
	m.mu.Lock()
	port, err := m.allocPort()
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	id := uuid.NewString()
	hostDir := filepath.Join(m.cfg.HostProfilesDir, id)
	container := "wcm-fx-" + id[:8]
	sess := &fxSession{
		ID: id, Port: port, Container: container,
		HostDir: hostDir, VNCURL: fmt.Sprintf("http://localhost:%d", port),
		Status: "starting", ProjectID: projectID, Platform: platform, Label: label,
		CreatedAt: time.Now(),
	}
	m.sessions[id] = sess
	m.mu.Unlock()

	if err := os.MkdirAll(hostDir, 0o755); err != nil {
		m.fail(sess, err)
		return sess, err
	}
	// Spin up jlesage/firefox with the per-session profile volume.
	err = dockerRun(
		"run", "-d", "--name", container,
		"-p", fmt.Sprintf("%d:5800", port),
		"-v", hostDir+":/config:rw",
		"-e", "DISPLAY_WIDTH=1280", "-e", "DISPLAY_HEIGHT=900",
		"-e", "KEEP_APP_RUNNING=1",
		m.cfg.Image,
	)
	if err != nil {
		m.fail(sess, err)
		return sess, err
	}
	if err := waitPortReady(port, 40*time.Second); err != nil {
		m.fail(sess, err)
		return sess, err
	}
	m.mu.Lock()
	sess.Status = "ready"
	m.mu.Unlock()
	return sess, nil
}

func (m *fxLoginMgr) fail(s *fxSession, err error) {
	m.mu.Lock()
	s.Status = "error"
	s.ErrMsg = err.Error()
	m.mu.Unlock()
	_ = exec.Command("docker", "rm", "-f", s.Container).Run()
}

// detectProfile returns the *relative* profile sub-path inside hostDir whose
// contents the worker should pass to `firefox -profile`. Returns "" when the
// profile dir is hostDir itself (legacy layout).
func (m *fxLoginMgr) detectProfile(hostDir string) (string, error) {
	// jlesage/firefox layout: <hostDir>/profile/{cookies.sqlite,prefs.js,...}
	flatProfile := filepath.Join(hostDir, "profile")
	if hasFirefoxProfile(flatProfile) {
		return "profile", nil
	}
	// Standard layout: <hostDir>/.mozilla/firefox/<random>.default-release/
	mozDir := filepath.Join(hostDir, ".mozilla", "firefox")
	if entries, err := os.ReadDir(mozDir); err == nil {
		for _, e := range entries {
			if e.IsDir() && (strings.HasSuffix(e.Name(), ".default-release") || strings.HasSuffix(e.Name(), ".default")) {
				return filepath.Join(".mozilla", "firefox", e.Name()), nil
			}
		}
	}
	// Fallback: hostDir itself contains the profile.
	if hasFirefoxProfile(hostDir) {
		return "", nil
	}
	return "", errors.New("no firefox profile directory found")
}

func hasFirefoxProfile(dir string) bool {
	// A real Firefox profile has prefs.js or cookies.sqlite — much more reliable
	// than checking for a specific subdirectory name.
	for _, marker := range []string{"prefs.js", "cookies.sqlite"} {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}

func (m *fxLoginMgr) finish(ctx context.Context, id string) (*fxSession, string, error) {
	m.mu.Lock()
	sess, ok := m.sessions[id]
	m.mu.Unlock()
	if !ok {
		return nil, "", errors.New("session not found")
	}
	// Stop the container so the profile is flushed to disk.
	_ = exec.Command("docker", "stop", sess.Container).Run()
	_ = exec.Command("docker", "rm", "-f", sess.Container).Run()
	m.mu.Lock()
	m.usedPort[sess.Port] = false
	m.mu.Unlock()

	inner, err := m.detectProfile(sess.HostDir)
	if err != nil {
		m.fail(sess, err)
		return sess, "", err
	}
	workerPath := filepath.Join(m.cfg.WorkerMountPoint, sess.ID, inner)
	m.mu.Lock()
	sess.Status = "finished"
	sess.ProfileInner = workerPath
	m.mu.Unlock()
	return sess, workerPath, nil
}

func (m *fxLoginMgr) cancel(_ context.Context, id string) error {
	m.mu.Lock()
	sess, ok := m.sessions[id]
	m.mu.Unlock()
	if !ok {
		return errors.New("session not found")
	}
	_ = exec.Command("docker", "rm", "-f", sess.Container).Run()
	m.mu.Lock()
	m.usedPort[sess.Port] = false
	delete(m.sessions, id)
	m.mu.Unlock()
	// Remove the profile directory — login was abandoned, no point keeping it.
	_ = os.RemoveAll(sess.HostDir)
	return nil
}

func (m *fxLoginMgr) get(id string) (*fxSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	return s, ok
}

// ---- HTTP handlers ----

type fxStartRequest struct {
	ProjectID string `json:"project_id"`
	Platform  string `json:"platform"`
	Label     string `json:"label"`
}

type fxFinishRequest struct {
	Label string `json:"label"`
}

// MountFirefoxLogin registers /api/firefox-login/* routes on the provided
// chi router. It is called from server.Router() after NewServer is constructed
// with the appropriate config.
func (s *Server) MountFirefoxLogin(r chi.Router) {
	if s.fxLogin == nil {
		// Disabled — return 503 stubs so the UI can detect.
		r.Route("/api/firefox-login", func(r chi.Router) {
			r.HandleFunc("/*", func(w http.ResponseWriter, _ *http.Request) {
				writeErr(w, http.StatusServiceUnavailable, "firefox-login disabled: set FIREFOX_PROFILES_DIR env")
			})
		})
		return
	}
	mgr := s.fxLogin
	r.Route("/api/firefox-login", func(r chi.Router) {
		r.Get("/status", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]any{"enabled": true})
		})
		r.Post("/sessions", func(w http.ResponseWriter, req *http.Request) {
			var body fxStartRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				writeErr(w, http.StatusBadRequest, "invalid json")
				return
			}
			if body.Platform == "" {
				writeErr(w, http.StatusBadRequest, "platform required")
				return
			}
			sess, err := mgr.start(req.Context(), body.ProjectID, body.Platform, body.Label)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, sess)
				return
			}
			writeJSON(w, http.StatusCreated, sess)
		})
		r.Get("/sessions/{id}", func(w http.ResponseWriter, req *http.Request) {
			sess, ok := mgr.get(chi.URLParam(req, "id"))
			if !ok {
				writeErr(w, http.StatusNotFound, "session not found")
				return
			}
			writeJSON(w, http.StatusOK, sess)
		})
		r.Post("/sessions/{id}/finish", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			var body fxFinishRequest
			_ = json.NewDecoder(req.Body).Decode(&body)
			sess, workerPath, err := mgr.finish(req.Context(), id)
			if err != nil {
				writeErr(w, http.StatusUnprocessableEntity, err.Error())
				return
			}
			label := body.Label
			if label == "" {
				label = sess.Label
			}
			// Persist as SocialAccount on the originating project.
			cmd := projcmd.UpsertSocialAccount{
				ProjectID:          sess.ProjectID,
				Platform:           sess.Platform,
				Label:              label,
				FirefoxProfilePath: workerPath,
			}
			res, derr := bus.Dispatch[projcmd.UpsertSocialAccountResult](req.Context(), s.reg, cmd)
			if derr != nil {
				writeErr(w, http.StatusUnprocessableEntity, derr.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"session":              sess,
				"social_account_id":    res.ID,
				"firefox_profile_path": workerPath,
			})
		})
		r.Delete("/sessions/{id}", func(w http.ResponseWriter, req *http.Request) {
			if err := mgr.cancel(req.Context(), chi.URLParam(req, "id")); err != nil {
				writeErr(w, http.StatusNotFound, err.Error())
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	})
}
