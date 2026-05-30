package pipeline

import (
	"context"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/example/dddcqrs/internal/app/bus"
	"github.com/example/dddcqrs/internal/infrastructure/persistence/uow"
)

// DeleteRun removes a run + all child rows (steps, attempts, assets, costs,
// uploads) via FK CASCADE, and also wipes its MinIO prefix runs/{id}/* so the
// storage doesn't accumulate orphan blobs. Hard delete — no soft archive.
type DeleteRun struct{ RunID string }

func (DeleteRun) IsCommand() {}

type DeleteRunResult struct{}

// MinIOPrefixDeleter is the minimal interface the handler needs from the
// MinIO store: recursively wipe everything under a prefix.
type MinIOPrefixDeleter interface {
	RemovePrefix(ctx context.Context, bucket, prefix string) error
	Bucket() string
}

type DeleteRunHandler struct {
	uow      uow.Manager
	store    MinIOPrefixDeleter
	writePoolExec func(ctx context.Context, sql string, args ...any) error
}

// NewDeleteRunHandler wires the UoW + MinIO + a raw pool exec for the actual
// DELETE statement (no domain aggregate involved; cascade does the work).
func NewDeleteRunHandler(m uow.Manager, store MinIOPrefixDeleter, exec func(ctx context.Context, sql string, args ...any) error) *DeleteRunHandler {
	return &DeleteRunHandler{uow: m, store: store, writePoolExec: exec}
}

func (h *DeleteRunHandler) Handle(ctx context.Context, cmd DeleteRun) (DeleteRunResult, error) {
	// Run a single statement on the WRITE pool — FK CASCADE handles all child
	// rows. Using the bare pool keeps the SQL trivial; the UoW would force us
	// to drag the run aggregate through Reconstitute just to delete it.
	if err := h.writePoolExec(ctx, `DELETE FROM pipeline_runs WHERE id = $1`, cmd.RunID); err != nil {
		return DeleteRunResult{}, err
	}
	// MinIO cleanup is best-effort: the row is already gone, an orphan blob
	// is just wasted bytes and not a correctness bug. Log+ignore failures.
	if h.store != nil {
		_ = h.store.RemovePrefix(ctx, h.store.Bucket(), "runs/"+cmd.RunID+"/")
	}
	return DeleteRunResult{}, nil
}

// PoolExecAdapter wraps a *pgxpool.Pool so the handler can stay decoupled.
func PoolExecAdapter(pool interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}) func(ctx context.Context, sql string, args ...any) error {
	return func(ctx context.Context, sql string, args ...any) error {
		_, err := pool.Exec(ctx, sql, args...)
		return err
	}
}

func DeleteRunOnBus(r *bus.Registry, m uow.Manager, store MinIOPrefixDeleter, exec func(ctx context.Context, sql string, args ...any) error) {
	bus.RegisterCommand[DeleteRun, DeleteRunResult](r, NewDeleteRunHandler(m, store, exec))
}
