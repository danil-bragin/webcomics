package uow

import (
	"context"
	"time"

	"github.com/example/dddcqrs/internal/domain/audiolib"
	"github.com/example/dddcqrs/internal/domain/formats"
	"github.com/example/dddcqrs/internal/domain/pipeline"
	"github.com/example/dddcqrs/internal/domain/projects"
	"github.com/example/dddcqrs/internal/domain/scheduler"
	"github.com/example/dddcqrs/internal/domain/shared"
	"github.com/example/dddcqrs/internal/domain/user"
)

// UserWriteRepository is the write-side port for the User aggregate, surfaced
// through the UoW. It aliases the domain's WriteRepository so the application
// layer depends on uow.Repositories() while the contract lives in the domain.
type UserWriteRepository = user.WriteRepository

// PipelineRunWriteRepository aliases the pipeline.Run write port.
type PipelineRunWriteRepository = pipeline.RunWriteRepository

// PipelineTemplateWriteRepository aliases the pipeline.Template write port.
type PipelineTemplateWriteRepository = pipeline.TemplateWriteRepository

// UploadRecordWriteRepository aliases the pipeline.UploadRecord write port.
type UploadRecordWriteRepository = pipeline.UploadRecordWriteRepository

// ProjectsWriteRepository aliases the projects write port.
type ProjectsWriteRepository = projects.WriteRepo

// AudioLibWriteRepository aliases the audiolib write port.
type AudioLibWriteRepository = audiolib.WriteRepo

// FormatsWriteRepository aliases the formats write port.
type FormatsWriteRepository interface {
	Save(ctx context.Context, f *formats.Format) error
	GetByID(ctx context.Context, id string) (*formats.Format, error)
	Delete(ctx context.Context, id string) error
}

// SchedulerWriteRepository persists scheduled_uploads inside a UoW tx.
type SchedulerWriteRepository interface {
	Save(ctx context.Context, s *scheduler.ScheduledUpload) error
	Get(ctx context.Context, id scheduler.ID) (*scheduler.ScheduledUpload, error)
	Delete(ctx context.Context, id scheduler.ID) error
	ListSlotsInWindow(ctx context.Context, accountID string, targetAt time.Time, window time.Duration) ([]scheduler.SlotPoint, error)
	ListPendingDue(ctx context.Context, now time.Time, limit int) ([]*scheduler.ScheduledUpload, error)
	FindByRunPending(ctx context.Context, runID string) (*scheduler.ScheduledUpload, error)
}

// OutboxRepository persists domain events into the transactional outbox table
// WITHIN the same UoW transaction as the aggregate change. A separate relay
// process publishes them to the message broker (Redis Streams) afterwards.
// This guarantees atomicity: state change and event emission commit together.
type OutboxRepository interface {
	Add(ctx context.Context, events ...shared.DomainEvent) error
}
