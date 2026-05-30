package pipeline

import "context"

// ReadModel is the read-side port for pipeline queries. Implementation uses
// the read pool / replica.
type ReadModel interface {
	GetRun(ctx context.Context, id string) (RunView, error)
	ListRuns(ctx context.Context, f ListRunsFilter) ([]RunSummary, error)
	GetTemplate(ctx context.Context, id string) (TemplateView, error)
	ListTemplates(ctx context.Context) ([]TemplateView, error)
	ListTemplatesFiltered(ctx context.Context, f TemplateFilter) ([]TemplateView, error)
	GetAssetRef(ctx context.Context, id string) (AssetRef, error)
	Stats(ctx context.Context) (StatsView, error)

	GetUploadRecord(ctx context.Context, id string) (UploadRecordView, error)
	ListUploadRecordsByRun(ctx context.Context, runID string) ([]UploadRecordView, error)
	ListUploadRecordsByProject(ctx context.Context, projectID string, limit, offset int) ([]UploadRecordView, error)
	ListUploadRecordsByAccount(ctx context.Context, socialAccountID string, limit, offset int) ([]UploadRecordView, error)
	AccountUploadStats(ctx context.Context, projectID string) ([]AccountUploadStats, error)
}
