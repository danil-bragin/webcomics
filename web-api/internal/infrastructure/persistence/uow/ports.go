package uow

import (
	"context"

	"github.com/example/dddcqrs/internal/domain/audiolib"
	"github.com/example/dddcqrs/internal/domain/pipeline"
	"github.com/example/dddcqrs/internal/domain/projects"
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

// OutboxRepository persists domain events into the transactional outbox table
// WITHIN the same UoW transaction as the aggregate change. A separate relay
// process publishes them to the message broker (Redis Streams) afterwards.
// This guarantees atomicity: state change and event emission commit together.
type OutboxRepository interface {
	Add(ctx context.Context, events ...shared.DomainEvent) error
}
