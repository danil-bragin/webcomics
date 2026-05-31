// Package uploadmetrics command-handlers.
package uploadmetrics

import (
	"context"
	"time"

	"github.com/example/dddcqrs/internal/app/bus"
	domum "github.com/example/dddcqrs/internal/domain/uploadmetrics"
	"github.com/example/dddcqrs/internal/infrastructure/persistence/uow"
)

// RecordMetricsSnapshot persists one snapshot row + denormalises last-known
// counters. Used by both the in-process YT fetcher and the Python-worker
// completion handler for IG/TT/FB.
type RecordMetricsSnapshot struct {
	UploadRecordID string
	Views          int64
	Likes          int64
	Comments       int64
	Shares         int64
	Raw            map[string]any
}

func (RecordMetricsSnapshot) IsCommand() {}

type RecordMetricsSnapshotResult struct{ ID string }

type RecordMetricsSnapshotHandler struct{ uow uow.Manager }

func NewRecordMetricsSnapshotHandler(m uow.Manager) *RecordMetricsSnapshotHandler {
	return &RecordMetricsSnapshotHandler{uow: m}
}

func (h *RecordMetricsSnapshotHandler) Handle(ctx context.Context, cmd RecordMetricsSnapshot) (RecordMetricsSnapshotResult, error) {
	var out RecordMetricsSnapshotResult
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		snap := domum.NewSnapshot(cmd.UploadRecordID, cmd.Views, cmd.Likes, cmd.Comments, cmd.Shares, cmd.Raw)
		if err := u.Repositories().Metrics().InsertSnapshot(ctx, snap); err != nil {
			return err
		}
		out.ID = snap.ID.String()
		return nil
	})
	return out, err
}

// RecordMetricsFailure marks an upload row as failed-to-fetch without an
// actual snapshot. Bumps the attempt counter so the UI can show "stuck".
type RecordMetricsFailure struct {
	UploadRecordID string
	Error          string
}

func (RecordMetricsFailure) IsCommand() {}

type RecordMetricsFailureResult struct{}

type RecordMetricsFailureHandler struct{ uow uow.Manager }

func NewRecordMetricsFailureHandler(m uow.Manager) *RecordMetricsFailureHandler {
	return &RecordMetricsFailureHandler{uow: m}
}

func (h *RecordMetricsFailureHandler) Handle(ctx context.Context, cmd RecordMetricsFailure) (RecordMetricsFailureResult, error) {
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		return u.Repositories().Metrics().MarkFetchFailed(ctx, cmd.UploadRecordID, cmd.Error, time.Now().UTC())
	})
	return RecordMetricsFailureResult{}, err
}

func RecordMetricsSnapshotOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[RecordMetricsSnapshot, RecordMetricsSnapshotResult](r, NewRecordMetricsSnapshotHandler(m))
}
func RecordMetricsFailureOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[RecordMetricsFailure, RecordMetricsFailureResult](r, NewRecordMetricsFailureHandler(m))
}
