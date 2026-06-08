package http

import (
	"net/http"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"
)

// uploadStepIndex is the synthetic step index the scheduler uses for uploads.
// Live screenshots land under runs/{run_id}/upload/{uploadStepIndex}/live/.
const uploadStepIndex = "99"

type liveFrame struct {
	Idx   int    `json:"idx"`
	Kind  string `json:"kind"`  // "step" | "fail"
	Stage string `json:"stage"` // human label, e.g. "04-metadata-filled"
	URL   string `json:"url"`   // presigned GET
}

// MountUploadProgress registers GET /api/runs/{id}/upload-progress, which lists
// the live screenshot frames a selenium upload mirrors into MinIO as it walks
// through YT Studio. The UI polls this while an upload is in flight so the user
// can watch the current stage instead of waiting blind.
func (s *Server) MountUploadProgress(r chi.Router) {
	r.Get("/api/runs/{id}/upload-progress", func(w http.ResponseWriter, req *http.Request) {
		runID := chi.URLParam(req, "id")
		bucket := s.assetsBucket
		prefix := "runs/" + runID + "/upload/" + uploadStepIndex + "/live/"
		keys, err := s.store.ListPrefix(req.Context(), bucket, prefix)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		frames := make([]liveFrame, 0, len(keys))
		for _, key := range keys {
			name := strings.TrimSuffix(path.Base(key), ".png")
			// name shape: "NN-kind-stage..." e.g. "04-step-03-metadata-filled".
			idx, kind, stage := parseFrameName(name)
			url, perr := s.store.PresignGet(req.Context(), bucket, key, 300)
			if perr != nil {
				continue
			}
			frames = append(frames, liveFrame{Idx: idx, Kind: kind, Stage: stage, URL: url})
		}
		current := ""
		if len(frames) > 0 {
			current = frames[len(frames)-1].Stage
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"frames":  frames,
			"current": current,
			"count":   len(frames),
		})
	})
}

// parseFrameName splits "NN-kind-rest" into (idx, kind, stageLabel). Falls back
// gracefully when the name doesn't match the expected shape.
func parseFrameName(name string) (int, string, string) {
	parts := strings.SplitN(name, "-", 3)
	idx := 0
	kind := "step"
	stage := name
	if len(parts) >= 1 {
		idx = atoiSafe(parts[0])
	}
	if len(parts) >= 2 && (parts[1] == "step" || parts[1] == "fail") {
		kind = parts[1]
		if len(parts) >= 3 {
			stage = parts[2]
		} else {
			stage = parts[1]
		}
	} else if len(parts) >= 2 {
		stage = strings.Join(parts[1:], "-")
	}
	return idx, kind, stage
}

func atoiSafe(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + int(c-'0')
	}
	return n
}
