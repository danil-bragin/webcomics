package pipeline

import (
	"context"
	"errors"

	"github.com/example/dddcqrs/internal/app/bus"
	"github.com/example/dddcqrs/internal/domain/pipeline"
	"github.com/example/dddcqrs/internal/infrastructure/persistence/uow"
)

// ---- EditUploadMetadata --------------------------------------------------
//
// Operator-side manual edit. Wipes the LLM-generated metadata and locks the
// row so subsequent caption retries don't clobber the manual values.

type EditUploadMetadata struct {
	ID       string
	Metadata pipeline.UploadMetadata
}

func (EditUploadMetadata) IsCommand() {}

type EditUploadMetadataResult struct{ ID string }

type EditUploadMetadataHandler struct{ uow uow.Manager }

func NewEditUploadMetadataHandler(m uow.Manager) *EditUploadMetadataHandler {
	return &EditUploadMetadataHandler{uow: m}
}

func (h *EditUploadMetadataHandler) Handle(ctx context.Context, c EditUploadMetadata) (EditUploadMetadataResult, error) {
	var out EditUploadMetadataResult
	if c.ID == "" {
		return out, errors.New("upload record id required")
	}
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		rec, err := u.Repositories().UploadRecords().GetByID(ctx, pipeline.UploadRecordID(c.ID))
		if err != nil {
			return err
		}
		rec.OverrideMetadata(c.Metadata)
		if err := u.Repositories().UploadRecords().Save(ctx, rec); err != nil {
			return err
		}
		out.ID = rec.ID().String()
		return nil
	})
	return out, err
}

func EditUploadMetadataOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[EditUploadMetadata, EditUploadMetadataResult](r, NewEditUploadMetadataHandler(m))
}

// ---- ApproveUploadRecord ------------------------------------------------
//
// Transitions pending_review → approved. The upload worker watches for
// approved rows and emits the actual pipeline.upload.requested message.

type ApproveUploadRecord struct {
	ID string
}

func (ApproveUploadRecord) IsCommand() {}

type ApproveUploadRecordResult struct{ ID string }

type ApproveUploadRecordHandler struct{ uow uow.Manager }

func NewApproveUploadRecordHandler(m uow.Manager) *ApproveUploadRecordHandler {
	return &ApproveUploadRecordHandler{uow: m}
}

func (h *ApproveUploadRecordHandler) Handle(ctx context.Context, c ApproveUploadRecord) (ApproveUploadRecordResult, error) {
	var out ApproveUploadRecordResult
	if c.ID == "" {
		return out, errors.New("upload record id required")
	}
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repos := u.Repositories()
		rec, err := repos.UploadRecords().GetByID(ctx, pipeline.UploadRecordID(c.ID))
		if err != nil {
			return err
		}
		rec.Approve()
		if err := repos.UploadRecords().Save(ctx, rec); err != nil {
			return err
		}
		out.ID = rec.ID().String()

		// Resume the run if it's waiting on a review gate AND all upload
		// records on the run are no longer in pending_review/metadata_ready.
		run, err := repos.PipelineRuns().GetByID(ctx, pipeline.RunID(rec.RunID()))
		if err != nil {
			return nil // run gone — record updated, don't crash the approval
		}
		if run.Status() != pipeline.RunStatusAwaitingAction || !run.RequireReviewBeforeUpload() {
			return nil
		}
		// Are there other records still awaiting review?
		all, err := repos.UploadRecords().ListByRun(ctx, rec.RunID())
		if err != nil {
			return nil
		}
		for _, r2 := range all {
			st := r2.Status()
			if st == pipeline.UploadStatusPendingReview || st == pipeline.UploadStatusMetadataReady {
				return nil // still waiting on user
			}
		}
		if err := run.ResumeFromReview(nil); err != nil {
			return nil // can't resume — non-fatal for the approve
		}
		if err := repos.PipelineRuns().Save(ctx, run); err != nil {
			return err
		}
		return repos.Outbox().Add(ctx, run.PullEvents()...)
	})
	return out, err
}

func ApproveUploadRecordOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[ApproveUploadRecord, ApproveUploadRecordResult](r, NewApproveUploadRecordHandler(m))
}

// ---- RejectUploadRecord -------------------------------------------------

type RejectUploadRecord struct {
	ID string
}

func (RejectUploadRecord) IsCommand() {}

type RejectUploadRecordResult struct{ ID string }

type RejectUploadRecordHandler struct{ uow uow.Manager }

func NewRejectUploadRecordHandler(m uow.Manager) *RejectUploadRecordHandler {
	return &RejectUploadRecordHandler{uow: m}
}

func (h *RejectUploadRecordHandler) Handle(ctx context.Context, c RejectUploadRecord) (RejectUploadRecordResult, error) {
	var out RejectUploadRecordResult
	if c.ID == "" {
		return out, errors.New("upload record id required")
	}
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		rec, err := u.Repositories().UploadRecords().GetByID(ctx, pipeline.UploadRecordID(c.ID))
		if err != nil {
			return err
		}
		rec.Reject()
		if err := u.Repositories().UploadRecords().Save(ctx, rec); err != nil {
			return err
		}
		out.ID = rec.ID().String()
		return nil
	})
	return out, err
}

func RejectUploadRecordOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[RejectUploadRecord, RejectUploadRecordResult](r, NewRejectUploadRecordHandler(m))
}
