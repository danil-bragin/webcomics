// Package uow defines the Unit of Work: the explicit transaction boundary for
// the command (write) side. A command handler opens a UoW, obtains
// transactional repositories from it, does its work, and commits. The handler
// OWNS the transaction lifecycle — it is visible, not hidden.
package uow

import "context"

// UnitOfWork represents a single business transaction on the write side.
// Everything obtained via Repositories() participates in the same pgx.Tx.
type UnitOfWork interface {
	// Repositories returns the write repositories bound to this UoW's tx.
	Repositories() Repositories

	// Commit persists all changes. After Commit the UoW is done.
	Commit(ctx context.Context) error

	// Rollback discards all changes. Safe to call after Commit (no-op).
	Rollback(ctx context.Context) error
}

// Manager starts new Units of Work. Injected into command handlers.
type Manager interface {
	// Begin opens a transaction and returns a UoW bound to it.
	Begin(ctx context.Context) (UnitOfWork, error)

	// WithinTx is a convenience helper that begins a UoW, runs fn, and commits
	// on success or rolls back on error/panic. Handlers may use this instead of
	// manual Begin/Commit when they don't need fine-grained control.
	WithinTx(ctx context.Context, fn func(ctx context.Context, uow UnitOfWork) error) error
}

// Repositories is the set of write-side repositories available within a UoW.
// Extend this as aggregates are added. All share the same transaction.
type Repositories interface {
	Users() UserWriteRepository
	PipelineRuns() PipelineRunWriteRepository
	PipelineTemplates() PipelineTemplateWriteRepository
	UploadRecords() UploadRecordWriteRepository
	Projects() ProjectsWriteRepository
	AudioLib() AudioLibWriteRepository
	Formats() FormatsWriteRepository
	Scheduler() SchedulerWriteRepository
	Outbox() OutboxRepository
}
