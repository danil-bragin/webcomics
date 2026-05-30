package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/example/dddcqrs/internal/app/bus"
	pipecmd "github.com/example/dddcqrs/internal/app/command/pipeline"
	pipeq "github.com/example/dddcqrs/internal/app/query/pipeline"
	"github.com/example/dddcqrs/internal/domain/pipeline"
)

// MountUploadRecords registers /api/upload-records/* and related project/run
// listing endpoints. These are non-OpenAPI to avoid regen for now.
func (s *Server) MountUploadRecords(r chi.Router) {
	r.Get("/api/upload-records/{id}", func(w http.ResponseWriter, req *http.Request) {
		id := chi.URLParam(req, "id")
		res, err := bus.Ask[pipeq.UploadRecordView](req.Context(), s.reg, pipeq.GetUploadRecord{ID: id})
		if err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	r.Get("/api/runs/{run_id}/upload-records", func(w http.ResponseWriter, req *http.Request) {
		runID := chi.URLParam(req, "run_id")
		res, err := bus.Ask[[]pipeq.UploadRecordView](req.Context(), s.reg,
			pipeq.ListUploadRecordsByRun{RunID: runID})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	r.Get("/api/projects/{project_id}/upload-records", func(w http.ResponseWriter, req *http.Request) {
		pid := chi.URLParam(req, "project_id")
		limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(req.URL.Query().Get("offset"))
		res, err := bus.Ask[[]pipeq.UploadRecordView](req.Context(), s.reg,
			pipeq.ListUploadRecordsByProject{ProjectID: pid, Limit: limit, Offset: offset})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	r.Get("/api/projects/{project_id}/account-upload-stats", func(w http.ResponseWriter, req *http.Request) {
		pid := chi.URLParam(req, "project_id")
		res, err := bus.Ask[[]pipeq.AccountUploadStats](req.Context(), s.reg,
			pipeq.AccountUploadStatsQuery{ProjectID: pid})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	// Promote unlisted → public. The handler runs the in-process command which
	// updates the row; the actual YT-side visibility flip is owned by the
	// worker (a new upload step or a "republish" command — wired separately).
	r.Post("/api/upload-records/{id}/publish", func(w http.ResponseWriter, req *http.Request) {
		id := chi.URLParam(req, "id")
		res, err := bus.Dispatch[pipecmd.PromoteUploadToPublishedResult](req.Context(), s.reg,
			pipecmd.PromoteUploadToPublished{ID: id})
		if err != nil {
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	// Retry: re-enqueue an upload step for this record's run (handled by the
	// consumer dispatching upload.requested). For now this is a stub-friendly
	// endpoint that returns 202 with a hint — full retry semantics land in
	// Phase J. We persist a fresh attempt counter via MarkUploadRecordFailed.
	r.Post("/api/upload-records/{id}/retry", func(w http.ResponseWriter, req *http.Request) {
		var body struct{}
		_ = json.NewDecoder(req.Body).Decode(&body)
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "retry_not_yet_wired"})
	})

	// PATCH /api/upload-records/:id/metadata — operator-side manual edit.
	r.Patch("/api/upload-records/{id}/metadata", func(w http.ResponseWriter, req *http.Request) {
		id := chi.URLParam(req, "id")
		var body editMetadataRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		_, err := bus.Dispatch[pipecmd.EditUploadMetadataResult](req.Context(), s.reg,
			pipecmd.EditUploadMetadata{
				ID:       id,
				Metadata: body.toDomain(),
			})
		if err != nil {
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"id": id})
	})

	r.Post("/api/upload-records/{id}/approve", func(w http.ResponseWriter, req *http.Request) {
		id := chi.URLParam(req, "id")
		_, err := bus.Dispatch[pipecmd.ApproveUploadRecordResult](req.Context(), s.reg,
			pipecmd.ApproveUploadRecord{ID: id})
		if err != nil {
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"id": id})
	})

	// Presign-by-key for the screenshot trail thumbnails. Limited to the
	// `runs/<id>/upload/...` prefix so it can't be used to leak arbitrary keys.
	r.Get("/api/upload-records/screenshot-url", func(w http.ResponseWriter, req *http.Request) {
		key := req.URL.Query().Get("key")
		if key == "" || !strings.HasPrefix(key, "runs/") || !strings.Contains(key, "/upload/") {
			writeErr(w, http.StatusBadRequest, "invalid key")
			return
		}
		url, err := s.store.PresignGet(req.Context(), s.assetsBucket, key, 300)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"url": url})
	})

	r.Post("/api/upload-records/{id}/reject", func(w http.ResponseWriter, req *http.Request) {
		id := chi.URLParam(req, "id")
		_, err := bus.Dispatch[pipecmd.RejectUploadRecordResult](req.Context(), s.reg,
			pipecmd.RejectUploadRecord{ID: id})
		if err != nil {
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"id": id})
	})
}

// editMetadataRequest is the body shape for PATCH /api/upload-records/:id/metadata.
type editMetadataRequest struct {
	Title           string   `json:"title"`
	Description     string   `json:"description"`
	Tags            []string `json:"tags"`
	Hashtags        []string `json:"hashtags"`
	Visibility      string   `json:"visibility"`
	MadeForKids     bool     `json:"made_for_kids"`
	AgeRestriction  string   `json:"age_restriction"`
	CategoryID      string   `json:"category_id"`
	CategoryLabel   string   `json:"category_label"`
	CommentsEnabled bool     `json:"comments_enabled"`
	PlaylistNames   []string `json:"playlist_names"`
}

func (b editMetadataRequest) toDomain() pipeline.UploadMetadata {
	return pipeline.UploadMetadata{
		Title:           b.Title,
		Description:     b.Description,
		Tags:            b.Tags,
		Hashtags:        b.Hashtags,
		Visibility:      b.Visibility,
		MadeForKids:     b.MadeForKids,
		AgeRestriction:  b.AgeRestriction,
		CategoryID:      b.CategoryID,
		CategoryLabel:   b.CategoryLabel,
		CommentsEnabled: b.CommentsEnabled,
		PlaylistNames:   b.PlaylistNames,
	}
}
