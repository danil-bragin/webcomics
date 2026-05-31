package uploadmetrics

import (
	"context"
	"time"

	"github.com/example/dddcqrs/internal/app/bus"
)

type SnapshotView struct {
	ID         string    `json:"id"`
	FetchedAt  time.Time `json:"fetched_at"`
	Views      int64     `json:"views"`
	Likes      int64     `json:"likes"`
	Comments   int64     `json:"comments"`
	Shares     int64     `json:"shares"`
}

type ReadModel interface {
	ListSnapshots(ctx context.Context, uploadRecordID string, limit int) ([]SnapshotView, error)
}

type ListSnapshots struct {
	UploadRecordID string
	Limit          int
}

func (ListSnapshots) IsQuery() {}

type ListSnapshotsHandler struct{ m ReadModel }

func (h ListSnapshotsHandler) Handle(ctx context.Context, q ListSnapshots) ([]SnapshotView, error) {
	return h.m.ListSnapshots(ctx, q.UploadRecordID, q.Limit)
}

func ListSnapshotsOnBus(r *bus.Registry, m ReadModel) {
	bus.RegisterQuery[ListSnapshots, []SnapshotView](r, ListSnapshotsHandler{m: m})
}
