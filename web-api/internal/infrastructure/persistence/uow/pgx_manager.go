package uow

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/dddcqrs/internal/infrastructure/persistence/outbox"
	writerepo "github.com/example/dddcqrs/internal/infrastructure/persistence/write"
)

// pgxManager implements uow.Manager backed by the write (master) pool.
type pgxManager struct {
	pool *pgxpool.Pool
}

// NewManager constructs the UoW manager. Pass the WRITE pool here.
func NewManager(pool *pgxpool.Pool) Manager {
	return &pgxManager{pool: pool}
}

func (m *pgxManager) Begin(ctx context.Context) (UnitOfWork, error) {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("uow begin: %w", err)
	}
	return newPgxUoW(tx), nil
}

func (m *pgxManager) WithinTx(ctx context.Context, fn func(ctx context.Context, u UnitOfWork) error) (err error) {
	u, err := m.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = u.Rollback(ctx)
			panic(p)
		}
	}()
	if err = fn(ctx, u); err != nil {
		_ = u.Rollback(ctx)
		return err
	}
	return u.Commit(ctx)
}

// pgxUoW binds repositories to a single transaction.
type pgxUoW struct {
	tx        pgx.Tx
	committed bool
	repos     *repositories
}

func newPgxUoW(tx pgx.Tx) *pgxUoW {
	return &pgxUoW{
		tx: tx,
		repos: &repositories{
			users:             writerepo.NewUserRepository(tx),
			pipelineRuns:      writerepo.NewPipelineRunRepository(tx),
			pipelineTemplates: writerepo.NewPipelineTemplateRepository(tx),
			uploadRecords:     writerepo.NewUploadRecordRepository(tx),
			projects:          writerepo.NewProjectsRepository(tx),
			audioLib:          writerepo.NewAudioRepository(tx),
			formats:           writerepo.NewFormatsRepository(tx),
			outbox:            outbox.NewRepository(tx),
		},
	}
}

func (u *pgxUoW) Repositories() Repositories { return u.repos }

func (u *pgxUoW) Commit(ctx context.Context) error {
	if u.committed {
		return nil
	}
	u.committed = true
	return u.tx.Commit(ctx)
}

func (u *pgxUoW) Rollback(ctx context.Context) error {
	if u.committed {
		return nil
	}
	return u.tx.Rollback(ctx)
}

// repositories satisfies uow.Repositories. All share u.tx.
type repositories struct {
	users             UserWriteRepository
	pipelineRuns      PipelineRunWriteRepository
	pipelineTemplates PipelineTemplateWriteRepository
	uploadRecords     UploadRecordWriteRepository
	projects          ProjectsWriteRepository
	audioLib          AudioLibWriteRepository
	formats           FormatsWriteRepository
	outbox            OutboxRepository
}

func (r *repositories) Users() UserWriteRepository               { return r.users }
func (r *repositories) PipelineRuns() PipelineRunWriteRepository { return r.pipelineRuns }
func (r *repositories) PipelineTemplates() PipelineTemplateWriteRepository {
	return r.pipelineTemplates
}
func (r *repositories) UploadRecords() UploadRecordWriteRepository { return r.uploadRecords }
func (r *repositories) Projects() ProjectsWriteRepository          { return r.projects }
func (r *repositories) AudioLib() AudioLibWriteRepository          { return r.audioLib }
func (r *repositories) Formats() FormatsWriteRepository            { return r.formats }
func (r *repositories) Outbox() OutboxRepository                   { return r.outbox }
