package http

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/minio/minio-go/v7"
)

// MusicLibrary proxies the music library manifest from MinIO so the UI can
// render a track picker. Cached on first read since the file is tiny.
type musicTrack struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Artist      string   `json:"artist"`
	ObjectKey   string   `json:"object_key"`
	DurationS   int      `json:"duration_s"`
	Mood        []string `json:"mood"`
	Genre       []string `json:"genre"`
	Tempo       string   `json:"tempo"`
	License     string   `json:"license"`
	Attribution string   `json:"attribution,omitempty"`
}

func (s *Server) MusicLibrary(w http.ResponseWriter, r *http.Request) {
	if s.musicClient == nil {
		writeErr(w, http.StatusServiceUnavailable, "music library not configured")
		return
	}
	obj, err := s.musicClient.GetObject(r.Context(), s.assetsBucket,
		"library/music/manifest.json", minio.GetObjectOptions{})
	if err != nil {
		writeErr(w, http.StatusNotFound, "manifest missing — run `make refresh-music`")
		return
	}
	defer obj.Close()
	body, err := io.ReadAll(obj)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var tracks []musicTrack
	if err := json.Unmarshal(body, &tracks); err != nil {
		writeErr(w, http.StatusInternalServerError, "manifest invalid: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tracks)
}
