package pipeline

import (
	"context"

	"github.com/example/dddcqrs/internal/app/bus"
)

type GetUploadRecord struct{ ID string }

func (GetUploadRecord) IsQuery() {}

type GetUploadRecordHandler struct{ rm ReadModel }

func NewGetUploadRecordHandler(rm ReadModel) *GetUploadRecordHandler {
	return &GetUploadRecordHandler{rm: rm}
}

func (h *GetUploadRecordHandler) Handle(ctx context.Context, q GetUploadRecord) (UploadRecordView, error) {
	return h.rm.GetUploadRecord(ctx, q.ID)
}

func GetUploadRecordOnBus(r *bus.Registry, rm ReadModel) {
	bus.RegisterQuery[GetUploadRecord, UploadRecordView](r, NewGetUploadRecordHandler(rm))
}

type ListUploadRecordsByRun struct{ RunID string }

func (ListUploadRecordsByRun) IsQuery() {}

type ListUploadRecordsByRunHandler struct{ rm ReadModel }

func NewListUploadRecordsByRunHandler(rm ReadModel) *ListUploadRecordsByRunHandler {
	return &ListUploadRecordsByRunHandler{rm: rm}
}

func (h *ListUploadRecordsByRunHandler) Handle(ctx context.Context, q ListUploadRecordsByRun) ([]UploadRecordView, error) {
	return h.rm.ListUploadRecordsByRun(ctx, q.RunID)
}

func ListUploadRecordsByRunOnBus(r *bus.Registry, rm ReadModel) {
	bus.RegisterQuery[ListUploadRecordsByRun, []UploadRecordView](r, NewListUploadRecordsByRunHandler(rm))
}

type ListUploadRecordsByProject struct {
	ProjectID string
	Limit     int
	Offset    int
}

func (ListUploadRecordsByProject) IsQuery() {}

type ListUploadRecordsByProjectHandler struct{ rm ReadModel }

func NewListUploadRecordsByProjectHandler(rm ReadModel) *ListUploadRecordsByProjectHandler {
	return &ListUploadRecordsByProjectHandler{rm: rm}
}

func (h *ListUploadRecordsByProjectHandler) Handle(ctx context.Context, q ListUploadRecordsByProject) ([]UploadRecordView, error) {
	return h.rm.ListUploadRecordsByProject(ctx, q.ProjectID, q.Limit, q.Offset)
}

func ListUploadRecordsByProjectOnBus(r *bus.Registry, rm ReadModel) {
	bus.RegisterQuery[ListUploadRecordsByProject, []UploadRecordView](r, NewListUploadRecordsByProjectHandler(rm))
}

type AccountUploadStatsQuery struct{ ProjectID string }

func (AccountUploadStatsQuery) IsQuery() {}

type AccountUploadStatsHandler struct{ rm ReadModel }

func NewAccountUploadStatsHandler(rm ReadModel) *AccountUploadStatsHandler {
	return &AccountUploadStatsHandler{rm: rm}
}

func (h *AccountUploadStatsHandler) Handle(ctx context.Context, q AccountUploadStatsQuery) ([]AccountUploadStats, error) {
	return h.rm.AccountUploadStats(ctx, q.ProjectID)
}

func AccountUploadStatsOnBus(r *bus.Registry, rm ReadModel) {
	bus.RegisterQuery[AccountUploadStatsQuery, []AccountUploadStats](r, NewAccountUploadStatsHandler(rm))
}
