package pipeline

import (
	"context"

	"github.com/example/dddcqrs/internal/app/bus"
)

// --- GetRun ---

type GetRun struct{ RunID string }

func (GetRun) IsQuery() {}

type GetRunHandler struct{ rm ReadModel }

func NewGetRunHandler(rm ReadModel) *GetRunHandler { return &GetRunHandler{rm: rm} }
func (h *GetRunHandler) Handle(ctx context.Context, q GetRun) (RunView, error) {
	return h.rm.GetRun(ctx, q.RunID)
}
func GetRunOnBus(r *bus.Registry, rm ReadModel) {
	bus.RegisterQuery[GetRun, RunView](r, NewGetRunHandler(rm))
}

// --- ListRuns ---

type ListRuns struct {
	Limit     int
	Offset    int
	Statuses  []string // empty = no filter
	Search    string   // ILIKE %search% on prompt; empty = no filter
	ProjectID string   // empty = no filter
}

func (ListRuns) IsQuery() {}

type ListRunsHandler struct{ rm ReadModel }

func NewListRunsHandler(rm ReadModel) *ListRunsHandler { return &ListRunsHandler{rm: rm} }
func (h *ListRunsHandler) Handle(ctx context.Context, q ListRuns) ([]RunSummary, error) {
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 50
	}
	return h.rm.ListRuns(ctx, ListRunsFilter{
		Limit: q.Limit, Offset: q.Offset, Statuses: q.Statuses, Search: q.Search, ProjectID: q.ProjectID,
	})
}

// ListRunsFilter is a value type so the read-model signature stays stable as
// new filters are added.
type ListRunsFilter struct {
	Limit     int
	Offset    int
	Statuses  []string
	Search    string
	ProjectID string
}

func ListRunsOnBus(r *bus.Registry, rm ReadModel) {
	bus.RegisterQuery[ListRuns, []RunSummary](r, NewListRunsHandler(rm))
}

// --- GetTemplate ---

type GetTemplate struct{ TemplateID string }

func (GetTemplate) IsQuery() {}

type GetTemplateHandler struct{ rm ReadModel }

func NewGetTemplateHandler(rm ReadModel) *GetTemplateHandler { return &GetTemplateHandler{rm: rm} }
func (h *GetTemplateHandler) Handle(ctx context.Context, q GetTemplate) (TemplateView, error) {
	return h.rm.GetTemplate(ctx, q.TemplateID)
}
func GetTemplateOnBus(r *bus.Registry, rm ReadModel) {
	bus.RegisterQuery[GetTemplate, TemplateView](r, NewGetTemplateHandler(rm))
}

// --- ListTemplates ---

// TemplateFilter narrows the marketplace listing by category and whether to
// include test-fixture rows (hidden from the UI by default).
type TemplateFilter struct {
	Category    string
	IncludeTest bool
}

type ListTemplates struct {
	Filter TemplateFilter
}

func (ListTemplates) IsQuery() {}

type ListTemplatesHandler struct{ rm ReadModel }

func NewListTemplatesHandler(rm ReadModel) *ListTemplatesHandler {
	return &ListTemplatesHandler{rm: rm}
}
func (h *ListTemplatesHandler) Handle(ctx context.Context, q ListTemplates) ([]TemplateView, error) {
	if (q.Filter == TemplateFilter{}) {
		// Default: exclude test fixtures from the marketplace.
		return h.rm.ListTemplatesFiltered(ctx, TemplateFilter{IncludeTest: false})
	}
	return h.rm.ListTemplatesFiltered(ctx, q.Filter)
}
func ListTemplatesOnBus(r *bus.Registry, rm ReadModel) {
	bus.RegisterQuery[ListTemplates, []TemplateView](r, NewListTemplatesHandler(rm))
}

// --- GetAssetRef ---

type GetAssetRef struct{ AssetID string }

func (GetAssetRef) IsQuery() {}

type GetAssetRefHandler struct{ rm ReadModel }

func NewGetAssetRefHandler(rm ReadModel) *GetAssetRefHandler { return &GetAssetRefHandler{rm: rm} }
func (h *GetAssetRefHandler) Handle(ctx context.Context, q GetAssetRef) (AssetRef, error) {
	return h.rm.GetAssetRef(ctx, q.AssetID)
}
func GetAssetRefOnBus(r *bus.Registry, rm ReadModel) {
	bus.RegisterQuery[GetAssetRef, AssetRef](r, NewGetAssetRefHandler(rm))
}

// --- GetStats ---

type GetStats struct{}

func (GetStats) IsQuery() {}

type GetStatsHandler struct{ rm ReadModel }

func NewGetStatsHandler(rm ReadModel) *GetStatsHandler { return &GetStatsHandler{rm: rm} }
func (h *GetStatsHandler) Handle(ctx context.Context, _ GetStats) (StatsView, error) {
	return h.rm.Stats(ctx)
}
func GetStatsOnBus(r *bus.Registry, rm ReadModel) {
	bus.RegisterQuery[GetStats, StatsView](r, NewGetStatsHandler(rm))
}
