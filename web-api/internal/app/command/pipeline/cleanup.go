package pipeline

import (
	"context"
	"strings"

	"github.com/example/dddcqrs/internal/app/bus"
	"github.com/example/dddcqrs/internal/infrastructure/persistence/uow"
)

// CleanupRuns bulk-deletes terminal runs older than N days. Cascading FKs
// remove their steps, assets, and cost entries. Outbox rows for these runs
// are not touched (they may already be published or about to be; the relay
// handles missing aggregate gracefully).
type CleanupRuns struct {
	OlderThanDays int
	Statuses      []string
}

func (CleanupRuns) IsCommand() {}

type CleanupRunsResult struct {
	Deleted int
}

type CleanupRunsHandler struct{ uow uow.Manager }

func NewCleanupRunsHandler(m uow.Manager) *CleanupRunsHandler { return &CleanupRunsHandler{uow: m} }

func (h *CleanupRunsHandler) Handle(ctx context.Context, cmd CleanupRuns) (CleanupRunsResult, error) {
	if cmd.OlderThanDays <= 0 {
		cmd.OlderThanDays = 30
	}
	statuses := cmd.Statuses
	if len(statuses) == 0 {
		statuses = []string{"completed", "failed", "cancelled"}
	}
	var deleted int
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		n, err := u.Repositories().PipelineRuns().DeleteOlderThan(ctx, cmd.OlderThanDays, statuses)
		if err != nil {
			return err
		}
		deleted = n
		return nil
	})
	return CleanupRunsResult{Deleted: deleted}, err
}

func CleanupRunsOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[CleanupRuns, CleanupRunsResult](r, NewCleanupRunsHandler(m))
}

// Trim trailing comment to avoid future stale-import noise.
var _ = strings.TrimSpace
