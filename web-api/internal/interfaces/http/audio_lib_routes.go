package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/example/dddcqrs/internal/app/bus"
	audiocmd "github.com/example/dddcqrs/internal/app/command/audiolib"
	audioq "github.com/example/dddcqrs/internal/app/query/audiolib"
)

// audioLibHandler holds direct deps the audio library endpoints need beyond
// the bus (Pixabay search runs out-of-band of the command bus because it has
// no side effects on our DB).
type AudioLibHandler struct {
	reg      *bus.Registry
	store    AudioPresigner
	searcher audiocmd.PixabaySearcher
}

// AudioPresigner is the narrow surface we need from the MinIO store for the
// preview URL endpoint.
type AudioPresigner interface {
	PresignGet(ctx context.Context, bucket, key string, ttlSeconds int) (string, error)
}

func (s *Server) MountAudioLibrary(r chi.Router, h *AudioLibHandler) {
	r.Get("/api/audio/tracks", h.list)
	r.Post("/api/audio/tracks", h.upload)
	r.Post("/api/audio/tracks/import-url", h.importURL)
	r.Post("/api/audio/tracks/import-pixabay", h.importPixabay)
	r.Patch("/api/audio/tracks/{id}", h.retag)
	r.Delete("/api/audio/tracks/{id}", h.delete)
	r.Get("/api/audio/tracks/{id}/preview-url", h.previewURL)
	r.Get("/api/audio/pixabay/search", h.pixabaySearch)
}

func NewAudioLibHandler(reg *bus.Registry, store AudioPresigner, p audiocmd.PixabaySearcher) *AudioLibHandler {
	return &AudioLibHandler{reg: reg, store: store, searcher: p}
}

func (h *AudioLibHandler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	f := audioq.ListTracksFilter{
		Kind:      q.Get("kind"),
		Scope:     q.Get("scope"),
		ProjectID: q.Get("project_id"),
		Mood:      q.Get("mood"),
		Search:    q.Get("q"),
		Limit:     limit,
		Offset:    offset,
	}
	res, err := bus.Ask[[]audioq.TrackView](r.Context(), h.reg, audioq.ListTracks{Filter: f})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

const maxUploadBytes = 32 << 20

func (h *AudioLibHandler) upload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid multipart body: "+err.Error())
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "missing 'file' field: "+err.Error())
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxUploadBytes+1))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(data) > maxUploadBytes {
		writeErr(w, http.StatusRequestEntityTooLarge, "file exceeds 32MB")
		return
	}
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "audio/mpeg"
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		title = strings.TrimSuffix(header.Filename, ".mp3")
		title = strings.TrimSuffix(title, ".wav")
	}
	durationMs, _ := strconv.Atoi(r.FormValue("duration_ms"))
	cmd := audiocmd.UploadTrack{
		Kind:        r.FormValue("kind"),
		Title:       title,
		Tags:        splitCSV(r.FormValue("tags")),
		Mood:        r.FormValue("mood"),
		DurationMs:  durationMs,
		Data:        data,
		ContentType: contentType,
		Scope:       r.FormValue("scope"),
		ProjectID:   r.FormValue("project_id"),
	}
	res, err := bus.Dispatch[audiocmd.UploadTrackResult](r.Context(), h.reg, cmd)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (h *AudioLibHandler) importURL(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind        string   `json:"kind"`
		URL         string   `json:"url"`
		Title       string   `json:"title"`
		Tags        []string `json:"tags"`
		Mood        string   `json:"mood"`
		Scope       string   `json:"scope"`
		ProjectID   string   `json:"project_id"`
		Attribution string   `json:"attribution"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	cmd := audiocmd.ImportFromURL{
		Kind: body.Kind, URL: body.URL, Title: body.Title, Tags: body.Tags,
		Mood: body.Mood, Scope: body.Scope, ProjectID: body.ProjectID,
		Attribution: body.Attribution,
	}
	res, err := bus.Dispatch[audiocmd.ImportFromURLResult](r.Context(), h.reg, cmd)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (h *AudioLibHandler) importPixabay(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind      string                 `json:"kind"`
		Result    audiocmd.PixabayResult `json:"result"`
		Mood      string                 `json:"mood"`
		Scope     string                 `json:"scope"`
		ProjectID string                 `json:"project_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	cmd := audiocmd.ImportFromPixabay{
		Kind: body.Kind, Result: body.Result, Mood: body.Mood,
		Scope: body.Scope, ProjectID: body.ProjectID,
	}
	res, err := bus.Dispatch[audiocmd.ImportFromPixabayResult](r.Context(), h.reg, cmd)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (h *AudioLibHandler) retag(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Tags []string `json:"tags"`
		Mood string   `json:"mood"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if _, err := bus.Dispatch[audiocmd.RetagTrackResult](r.Context(), h.reg, audiocmd.RetagTrack{
		ID: id, Tags: body.Tags, Mood: body.Mood,
	}); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AudioLibHandler) delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := bus.Dispatch[audiocmd.DeleteTrackResult](r.Context(), h.reg, audiocmd.DeleteTrack{ID: id}); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AudioLibHandler) previewURL(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	track, err := bus.Ask[audioq.TrackView](r.Context(), h.reg, audioq.GetTrack{ID: id})
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	url, err := h.store.PresignGet(r.Context(), track.Bucket, track.ObjectKey, 600)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}

func (h *AudioLibHandler) pixabaySearch(w http.ResponseWriter, r *http.Request) {
	if h.searcher == nil {
		writeErr(w, http.StatusServiceUnavailable, "pixabay search not configured")
		return
	}
	q := r.URL.Query()
	kind := q.Get("kind")
	if kind == "" {
		kind = "music"
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	results, err := h.searcher.Search(r.Context(), kind, q.Get("q"), limit)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, results)
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
