package pipeline

import "context"

// RunWriteRepository is the write-side port for the Run aggregate.
// Bound to a UoW transaction. Persists the full Run state plus any new
// assets and cost entries the aggregate accumulated during the call.
type RunWriteRepository interface {
	Save(ctx context.Context, r *Run) error
	GetByID(ctx context.Context, id RunID) (*Run, error)
	// DeleteOlderThan deletes runs created more than `days` days ago whose
	// status is in the given list. Returns the number of rows deleted.
	// Cascading FKs handle steps/assets/cost_entries.
	DeleteOlderThan(ctx context.Context, days int, statuses []string) (int, error)
	// GetAssetObjectKeys batches asset ID → MinIO object_key lookups so the
	// command layer can resolve character/environment ref images without N+1.
	GetAssetObjectKeys(ctx context.Context, ids []string) (map[string]string, error)
}

// TemplateWriteRepository is the write-side port for templates (simple CRUD).
type TemplateWriteRepository interface {
	Save(ctx context.Context, t *Template) error
	GetByID(ctx context.Context, id TemplateID) (*Template, error)
}

// UploadRecordWriteRepository persists upload attempts. One row per attempt;
// retries make new rows (so failure trail is preserved verbatim).
type UploadRecordWriteRepository interface {
	Save(ctx context.Context, r *UploadRecord) error
	GetByID(ctx context.Context, id UploadRecordID) (*UploadRecord, error)
	ListByRun(ctx context.Context, runID string) ([]*UploadRecord, error)
}

// AssetStore is the port for binary object storage (MinIO/S3).
// Lives here because the domain emits asset metadata; the app layer uses this
// port to mint presigned URLs and (in workers) to upload bytes.
type AssetStore interface {
	PresignGet(ctx context.Context, bucket, key string, ttlSeconds int) (string, error)
	PresignPut(ctx context.Context, bucket, key string, ttlSeconds int) (string, error)
}
