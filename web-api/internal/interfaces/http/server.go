// Package http is an ENTRY POINT. HTTP routes are spec-first: openapi.yaml
// in api/openapi/ → gen.ServerInterface via oapi-codegen. This file wires the
// generated chi server to the *Server implementation.
package http

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/minio/minio-go/v7"

	"github.com/example/dddcqrs/internal/app/bus"
	"github.com/example/dddcqrs/internal/interfaces/http/gen"
)

type Server struct {
	reg          *bus.Registry
	store        AssetStore
	metrics      http.Handler
	apiKey       string
	ready        func(context.Context) error
	balances     BalancesProvider
	uploadsPool  uploadsPool
	assetsBucket string
	fxLogin      *fxLoginMgr
	musicClient  *minio.Client
	audioLib     *AudioLibHandler
}

// uploadsPool is the narrow surface we need from pgxpool — kept as an
// interface so server tests can stub it.
type uploadsPool interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// AssetStore is the minimal surface this transport needs from MinIO/S3.
type AssetStore interface {
	PresignGet(ctx context.Context, bucket, key string, ttlSeconds int) (string, error)
}

// BalancesProvider — surface needed for GET /api/balances.
type BalancesProvider interface {
	Snapshot(ctx context.Context) any
}

func NewServer(reg *bus.Registry, store AssetStore, metricsHandler http.Handler, apiKey string, ready func(context.Context) error, bal BalancesProvider) *Server {
	return &Server{reg: reg, store: store, metrics: metricsHandler, apiKey: apiKey, ready: ready, balances: bal}
}

// WithUploads enables the /api/uploads/presign endpoint by wiring an asset
// table writer (pgxpool) and the bucket name used for uploaded refs.
func (s *Server) WithUploads(pool uploadsPool, bucket string) *Server {
	s.uploadsPool = pool
	s.assetsBucket = bucket
	return s
}

// WithMusicLibrary attaches the MinIO client used to read the music manifest
// behind /api/music-library.
func (s *Server) WithMusicLibrary(c *minio.Client) *Server {
	s.musicClient = c
	return s
}

// WithAudioLibrary mounts /api/audio/* routes if a handler is configured.
func (s *Server) WithAudioLibrary(h *AudioLibHandler) *Server {
	s.audioLib = h
	return s
}

// WithFirefoxLogin enables the /api/firefox-login/* orchestration endpoints
// that spin up jlesage/firefox containers for interactive social account
// authentication. Disabled if HostProfilesDir is empty.
func (s *Server) WithFirefoxLogin(cfg FirefoxLoginConfig) *Server {
	if cfg.HostProfilesDir != "" {
		s.fxLogin = newFxLoginMgr(cfg)
	}
	return s
}

// apiKeyAuth gates /api/* and /metrics behind X-API-Key. SPA paths bypass auth
// (assets are public; the UI bundles the key at build time if you want it
// scoped). When apiKey is empty the middleware is a no-op.
func (s *Server) apiKeyAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.apiKey == "" {
			next.ServeHTTP(w, r)
			return
		}
		path := r.URL.Path
		needsAuth := false
		if len(path) >= 5 && path[:5] == "/api/" {
			needsAuth = true
		}
		if path == "/metrics" {
			needsAuth = true
		}
		if needsAuth && r.Header.Get("X-API-Key") != s.apiKey {
			writeErr(w, http.StatusUnauthorized, "missing or invalid X-API-Key")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Compile-time assertion that *Server satisfies the generated interface.
var _ gen.ServerInterface = (*Server)(nil)

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID, chimw.RealIP, chimw.Recoverer, corsMiddleware, s.apiKeyAuth)
	// Liveness/readiness — bypassed by apiKeyAuth (paths don't start with /api/ or = /metrics).
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/ready", func(w http.ResponseWriter, req *http.Request) {
		if s.ready != nil {
			if err := s.ready(req.Context()); err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"ready": "false", "err": err.Error()})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"ready": "true"})
	})
	// Non-OpenAPI proxy endpoints.
	r.Get("/api/elevenlabs/voices", s.VoicesProxy)
	r.Get("/api/music-library", s.MusicLibrary)
	r.Get("/api/formats", s.ListFormats)
	r.Post("/api/uploads/presign", s.PresignUploadRef)
	s.MountFirefoxLogin(r)
	s.MountUploadRecords(r)
	s.MountPresets(r)
	s.MountFormats(r)
	s.MountRunDelete(r)
	s.MountSocial(r)
	if s.audioLib != nil {
		s.MountAudioLibrary(r, s.audioLib)
	}
	// Generated API routes (registered on r in place).
	gen.HandlerFromMux(s, r)
	// Prometheus.
	if s.metrics != nil {
		r.Handle("/metrics", s.metrics)
	}
	// SPA — served last so any unmatched path falls through to the bundle.
	mountSPA(r)
	return r
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, gen.Error{Error: msg})
}
