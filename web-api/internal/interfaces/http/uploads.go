package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UploadsHandler issues presigned PUT URLs for user-uploaded reference images
// and records a pipeline_assets row pointing at the future MinIO key. The
// frontend uploads directly to MinIO with the URL; the asset_id returned here
// is what gets attached to characters / environments.
type UploadsHandler struct {
	store AssetStore
	pool  *pgxpool.Pool
}

func NewUploadsHandler(store AssetStore, pool *pgxpool.Pool) *UploadsHandler {
	return &UploadsHandler{store: store, pool: pool}
}

// AssetStore here extends the read-side AssetStore with a PresignPut method.
// We use the existing minio.Store directly via a type assertion when needed.

type presignBody struct {
	Kind        string `json:"kind"` // "character_ref" | "environment_ref" | "panel_image"
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	ProjectID   string `json:"project_id,omitempty"`
}

type presignResp struct {
	AssetID   string `json:"asset_id"`
	URL       string `json:"url"`
	ObjectKey string `json:"object_key"`
	Bucket    string `json:"bucket"`
	Mime      string `json:"mime"`
}

// PresignPutter is the minimal store surface needed here. The MinIO store
// implements it; injected through the *Server.
type PresignPutter interface {
	PresignPut(ctx context.Context, bucket, key string, ttlSeconds int) (string, error)
}

// PresignUploadRef issues a presigned PUT and creates the asset row. Used by
// the character / environment builders to attach uploaded images.
func (s *Server) PresignUploadRef(w http.ResponseWriter, r *http.Request) {
	var b presignBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if b.Kind == "" {
		b.Kind = "character_ref"
	}
	if b.ContentType == "" {
		b.ContentType = "image/png"
	}
	ext := ".png"
	if strings.HasSuffix(strings.ToLower(b.Filename), ".jpg") || strings.HasSuffix(strings.ToLower(b.Filename), ".jpeg") {
		ext = ".jpg"
	}
	if strings.HasSuffix(strings.ToLower(b.Filename), ".webp") {
		ext = ".webp"
	}
	bucket := "comics"
	if s.assetsBucket != "" {
		bucket = s.assetsBucket
	}
	assetID := uuid.NewString()
	objectKey := "refs/" + b.Kind + "/" + assetID + ext

	pp, ok := s.store.(PresignPutter)
	if !ok {
		writeErr(w, http.StatusServiceUnavailable, "asset store does not support uploads")
		return
	}
	url, err := pp.PresignPut(r.Context(), bucket, objectKey, 600)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.uploadsPool != nil {
		_, err := s.uploadsPool.Exec(r.Context(),
			`INSERT INTO pipeline_assets (id, run_id, kind, bucket, object_key, mime, bytes, created_at)
			 VALUES ($1, NULL, $2, $3, $4, $5, 0, $6)`,
			assetID, b.Kind, bucket, objectKey, b.ContentType, time.Now().UTC(),
		)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, presignResp{
		AssetID: assetID, URL: url, ObjectKey: objectKey,
		Bucket: bucket, Mime: b.ContentType,
	})
}

// AssetIDFromAssetURL — convenience for the "lift" workflow. The frontend
// already knows asset_id from the runs list, so all it needs to do is include
// it in characters.ref_asset_ids. No endpoint required.

// Suppress unused-import.
var _ = chi.NewRouter
