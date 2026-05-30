package pipeline

import (
	"context"
	"errors"

	"github.com/example/dddcqrs/internal/app/bus"
	"github.com/example/dddcqrs/internal/domain/pipeline"
	"github.com/example/dddcqrs/internal/infrastructure/persistence/uow"
)

// ---- CreateUploadRecord -------------------------------------------------

type CreateUploadRecord struct {
	RunID           string
	ProjectID       string
	SocialAccountID string
	Provider        string
	StepIndex       int
	Metadata        pipeline.UploadMetadata
}

func (CreateUploadRecord) IsCommand() {}

type CreateUploadRecordResult struct{ ID string }

type CreateUploadRecordHandler struct{ uow uow.Manager }

func NewCreateUploadRecordHandler(m uow.Manager) *CreateUploadRecordHandler {
	return &CreateUploadRecordHandler{uow: m}
}

func (h *CreateUploadRecordHandler) Handle(ctx context.Context, c CreateUploadRecord) (CreateUploadRecordResult, error) {
	var out CreateUploadRecordResult
	if c.RunID == "" || c.Provider == "" {
		return out, errors.New("create upload record: run_id and provider required")
	}
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		rec := pipeline.NewUploadRecord(c.RunID, c.ProjectID, c.SocialAccountID, c.Provider, c.StepIndex, c.Metadata)
		if err := u.Repositories().UploadRecords().Save(ctx, rec); err != nil {
			return err
		}
		out.ID = rec.ID().String()
		return nil
	})
	return out, err
}

func CreateUploadRecordOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[CreateUploadRecord, CreateUploadRecordResult](r, NewCreateUploadRecordHandler(m))
}

// ---- MarkUploadRecordCompleted ------------------------------------------
//
// The pipeline step's own RecordUploadCompleted (record_step.go) advances the
// Run aggregate. This one updates the UploadRecord row attached to the run so
// the UI / metrics surface the actual video URL + final visibility.

type MarkUploadRecordCompleted struct {
	ID              string // optional explicit record id
	RunID           string // fallback: latest pending record for run
	ExternalRef     string
	ExternalID      string
	FinalVisibility string
	ScreenshotTrail []pipeline.ScreenshotEntry
}

func (MarkUploadRecordCompleted) IsCommand() {}

type MarkUploadRecordCompletedResult struct{ ID string }

type MarkUploadRecordCompletedHandler struct{ uow uow.Manager }

func NewMarkUploadRecordCompletedHandler(m uow.Manager) *MarkUploadRecordCompletedHandler {
	return &MarkUploadRecordCompletedHandler{uow: m}
}

func (h *MarkUploadRecordCompletedHandler) Handle(ctx context.Context, c MarkUploadRecordCompleted) (MarkUploadRecordCompletedResult, error) {
	var out MarkUploadRecordCompletedResult
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		rec, err := loadUploadRecordForUpdate(ctx, u, c.ID, c.RunID)
		if err != nil {
			return err
		}
		rec.MarkUploaded(c.ExternalRef, c.ExternalID, c.FinalVisibility)
		if len(c.ScreenshotTrail) > 0 {
			rec.SetScreenshotTrail(c.ScreenshotTrail)
		}
		if err := u.Repositories().UploadRecords().Save(ctx, rec); err != nil {
			return err
		}
		out.ID = rec.ID().String()
		return nil
	})
	return out, err
}

func MarkUploadRecordCompletedOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[MarkUploadRecordCompleted, MarkUploadRecordCompletedResult](r, NewMarkUploadRecordCompletedHandler(m))
}

// ---- MarkUploadRecordFailed ---------------------------------------------

type MarkUploadRecordFailed struct {
	ID                     string
	RunID                  string
	Error                  string
	ErrorScreenshotAssetID string
	ScreenshotTrail        []pipeline.ScreenshotEntry
}

func (MarkUploadRecordFailed) IsCommand() {}

type MarkUploadRecordFailedResult struct{ ID string }

type MarkUploadRecordFailedHandler struct{ uow uow.Manager }

func NewMarkUploadRecordFailedHandler(m uow.Manager) *MarkUploadRecordFailedHandler {
	return &MarkUploadRecordFailedHandler{uow: m}
}

func (h *MarkUploadRecordFailedHandler) Handle(ctx context.Context, c MarkUploadRecordFailed) (MarkUploadRecordFailedResult, error) {
	var out MarkUploadRecordFailedResult
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		rec, err := loadUploadRecordForUpdate(ctx, u, c.ID, c.RunID)
		if err != nil {
			return err
		}
		rec.MarkFailed(c.Error, c.ErrorScreenshotAssetID)
		if len(c.ScreenshotTrail) > 0 {
			rec.SetScreenshotTrail(c.ScreenshotTrail)
		}
		if err := u.Repositories().UploadRecords().Save(ctx, rec); err != nil {
			return err
		}
		out.ID = rec.ID().String()
		return nil
	})
	return out, err
}

func MarkUploadRecordFailedOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[MarkUploadRecordFailed, MarkUploadRecordFailedResult](r, NewMarkUploadRecordFailedHandler(m))
}

// ---- PromoteUploadToPublished -------------------------------------------

type PromoteUploadToPublished struct {
	ID string
}

func (PromoteUploadToPublished) IsCommand() {}

type PromoteUploadToPublishedResult struct{ ID string }

type PromoteUploadToPublishedHandler struct{ uow uow.Manager }

func NewPromoteUploadToPublishedHandler(m uow.Manager) *PromoteUploadToPublishedHandler {
	return &PromoteUploadToPublishedHandler{uow: m}
}

func (h *PromoteUploadToPublishedHandler) Handle(ctx context.Context, c PromoteUploadToPublished) (PromoteUploadToPublishedResult, error) {
	var out PromoteUploadToPublishedResult
	if c.ID == "" {
		return out, errors.New("upload record id required")
	}
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		rec, err := u.Repositories().UploadRecords().GetByID(ctx, pipeline.UploadRecordID(c.ID))
		if err != nil {
			return err
		}
		rec.PromoteToPublished()
		if err := u.Repositories().UploadRecords().Save(ctx, rec); err != nil {
			return err
		}
		out.ID = rec.ID().String()
		return nil
	})
	return out, err
}

func PromoteUploadToPublishedOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[PromoteUploadToPublished, PromoteUploadToPublishedResult](
		r, NewPromoteUploadToPublishedHandler(m))
}

// ---- helpers ------------------------------------------------------------

// loadUploadRecordForUpdate resolves the upload record either by explicit ID
// or by finding the most recently created pending row on the run.
func loadUploadRecordForUpdate(ctx context.Context, u uow.UnitOfWork, id, runID string) (*pipeline.UploadRecord, error) {
	repo := u.Repositories().UploadRecords()
	if id != "" {
		return repo.GetByID(ctx, pipeline.UploadRecordID(id))
	}
	recs, err := repo.ListByRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	for i := len(recs) - 1; i >= 0; i-- {
		if recs[i].Status() == pipeline.UploadStatusPending {
			return recs[i], nil
		}
	}
	if len(recs) == 0 {
		return nil, pipeline.ErrUploadRecordNotFound
	}
	return recs[len(recs)-1], nil
}
