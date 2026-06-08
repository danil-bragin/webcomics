package pipeline

import (
	"context"

	"github.com/example/dddcqrs/internal/app/bus"
	"github.com/example/dddcqrs/internal/domain/pipeline"
	"github.com/example/dddcqrs/internal/infrastructure/persistence/uow"
)

// RecordScriptCompleted is dispatched by the consumer when a Python script
// worker publishes pipeline.script.completed.
type RecordScriptCompleted struct {
	RunID      string
	StepIndex  int
	ScriptKey  string
	Bucket     string
	Bytes      int64
	Panels     []pipeline.PanelDef
	Cost       pipeline.CostInfo
	DurationMs int
}

func (RecordScriptCompleted) IsCommand()            {}
func (c RecordScriptCompleted) GetRunID() string    { return c.RunID }
func (c RecordScriptCompleted) GetStepType() string { return "script" }
func (c RecordScriptCompleted) GetProvider() string { return c.Cost.Provider }
func (c RecordScriptCompleted) GetCostUSD() float64 { return c.Cost.TotalCostUSD }

type RecordStepResult struct{}

type RecordScriptCompletedHandler struct{ uow uow.Manager }

func NewRecordScriptCompletedHandler(m uow.Manager) *RecordScriptCompletedHandler {
	return &RecordScriptCompletedHandler{uow: m}
}

func (h *RecordScriptCompletedHandler) Handle(ctx context.Context, cmd RecordScriptCompleted) (RecordStepResult, error) {
	return RecordStepResult{}, h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repos := u.Repositories()
		run, err := repos.PipelineRuns().GetByID(ctx, pipeline.RunID(cmd.RunID))
		if err != nil {
			return err
		}
		if err := run.RecordScriptCompleted(cmd.StepIndex, cmd.ScriptKey, cmd.Panels, cmd.Cost, cmd.DurationMs, cmd.Bytes); err != nil {
			return err
		}
		run.FillEmptyBucketsOnNewAssets(cmd.Bucket)
		if err := repos.PipelineRuns().Save(ctx, run); err != nil {
			return err
		}
		return repos.Outbox().Add(ctx, run.PullEvents()...)
	})
}

// RecordImageCompleted — one panel completed.
type RecordImageCompleted struct {
	RunID      string
	StepIndex  int
	PanelIndex int
	ObjectKey  string
	Bucket     string
	Bytes      int64
	Cost       pipeline.CostInfo
	DurationMs int
}

func (RecordImageCompleted) IsCommand()            {}
func (c RecordImageCompleted) GetRunID() string    { return c.RunID }
func (c RecordImageCompleted) GetStepType() string { return "image" }
func (c RecordImageCompleted) GetProvider() string { return c.Cost.Provider }
func (c RecordImageCompleted) GetCostUSD() float64 { return c.Cost.TotalCostUSD }

type RecordImageCompletedHandler struct{ uow uow.Manager }

func NewRecordImageCompletedHandler(m uow.Manager) *RecordImageCompletedHandler {
	return &RecordImageCompletedHandler{uow: m}
}

func (h *RecordImageCompletedHandler) Handle(ctx context.Context, cmd RecordImageCompleted) (RecordStepResult, error) {
	return RecordStepResult{}, h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repos := u.Repositories()
		run, err := repos.PipelineRuns().GetByID(ctx, pipeline.RunID(cmd.RunID))
		if err != nil {
			return err
		}
		if err := run.RecordImageCompleted(cmd.StepIndex, cmd.PanelIndex, cmd.ObjectKey, cmd.Cost, cmd.DurationMs, cmd.Bytes); err != nil {
			return err
		}
		run.FillEmptyBucketsOnNewAssets(cmd.Bucket)
		if err := repos.PipelineRuns().Save(ctx, run); err != nil {
			return err
		}
		return repos.Outbox().Add(ctx, run.PullEvents()...)
	})
}

// RecordAudioCompleted — audio step done.
type RecordAudioCompleted struct {
	RunID      string
	StepIndex  int
	ObjectKey  string
	Bucket     string
	Bytes      int64
	Cost       pipeline.CostInfo
	DurationMs int
}

func (RecordAudioCompleted) IsCommand()            {}
func (c RecordAudioCompleted) GetRunID() string    { return c.RunID }
func (c RecordAudioCompleted) GetStepType() string { return "audio" }
func (c RecordAudioCompleted) GetProvider() string { return c.Cost.Provider }
func (c RecordAudioCompleted) GetCostUSD() float64 { return c.Cost.TotalCostUSD }

type RecordAudioCompletedHandler struct{ uow uow.Manager }

func NewRecordAudioCompletedHandler(m uow.Manager) *RecordAudioCompletedHandler {
	return &RecordAudioCompletedHandler{uow: m}
}

func (h *RecordAudioCompletedHandler) Handle(ctx context.Context, cmd RecordAudioCompleted) (RecordStepResult, error) {
	return RecordStepResult{}, h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repos := u.Repositories()
		run, err := repos.PipelineRuns().GetByID(ctx, pipeline.RunID(cmd.RunID))
		if err != nil {
			return err
		}
		if err := run.RecordAudioCompleted(cmd.StepIndex, cmd.ObjectKey, cmd.Cost, cmd.DurationMs, cmd.Bytes); err != nil {
			return err
		}
		run.FillEmptyBucketsOnNewAssets(cmd.Bucket)
		if err := repos.PipelineRuns().Save(ctx, run); err != nil {
			return err
		}
		return repos.Outbox().Add(ctx, run.PullEvents()...)
	})
}

// RecordMusicCompleted — background music ready.
type RecordMusicCompleted struct {
	RunID      string
	StepIndex  int
	ObjectKey  string
	Bucket     string
	Bytes      int64
	Cost       pipeline.CostInfo
	DurationMs int
}

func (RecordMusicCompleted) IsCommand()            {}
func (c RecordMusicCompleted) GetRunID() string    { return c.RunID }
func (c RecordMusicCompleted) GetStepType() string { return "music" }
func (c RecordMusicCompleted) GetProvider() string { return c.Cost.Provider }
func (c RecordMusicCompleted) GetCostUSD() float64 { return c.Cost.TotalCostUSD }

type RecordMusicCompletedHandler struct{ uow uow.Manager }

func NewRecordMusicCompletedHandler(m uow.Manager) *RecordMusicCompletedHandler {
	return &RecordMusicCompletedHandler{uow: m}
}

func (h *RecordMusicCompletedHandler) Handle(ctx context.Context, cmd RecordMusicCompleted) (RecordStepResult, error) {
	return RecordStepResult{}, h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repos := u.Repositories()
		run, err := repos.PipelineRuns().GetByID(ctx, pipeline.RunID(cmd.RunID))
		if err != nil {
			return err
		}
		if err := run.RecordMusicCompleted(cmd.StepIndex, cmd.ObjectKey, cmd.Cost, cmd.DurationMs, cmd.Bytes); err != nil {
			return err
		}
		run.FillEmptyBucketsOnNewAssets(cmd.Bucket)
		if err := repos.PipelineRuns().Save(ctx, run); err != nil {
			return err
		}
		return repos.Outbox().Add(ctx, run.PullEvents()...)
	})
}

// RecordUploadCompleted — social-upload step done.
type RecordUploadCompleted struct {
	RunID       string
	StepIndex   int
	ExternalRef string
	Cost        pipeline.CostInfo
	DurationMs  int
}

func (RecordUploadCompleted) IsCommand()            {}
func (c RecordUploadCompleted) GetRunID() string    { return c.RunID }
func (c RecordUploadCompleted) GetStepType() string { return "upload" }
func (c RecordUploadCompleted) GetProvider() string { return c.Cost.Provider }
func (c RecordUploadCompleted) GetCostUSD() float64 { return c.Cost.TotalCostUSD }

type RecordUploadCompletedHandler struct{ uow uow.Manager }

func NewRecordUploadCompletedHandler(m uow.Manager) *RecordUploadCompletedHandler {
	return &RecordUploadCompletedHandler{uow: m}
}

func (h *RecordUploadCompletedHandler) Handle(ctx context.Context, cmd RecordUploadCompleted) (RecordStepResult, error) {
	return RecordStepResult{}, h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repos := u.Repositories()
		run, err := repos.PipelineRuns().GetByID(ctx, pipeline.RunID(cmd.RunID))
		if err != nil {
			return err
		}
		if err := run.RecordUploadCompleted(cmd.StepIndex, cmd.ExternalRef, cmd.Cost, cmd.DurationMs); err != nil {
			return err
		}
		if err := repos.PipelineRuns().Save(ctx, run); err != nil {
			return err
		}
		return repos.Outbox().Add(ctx, run.PullEvents()...)
	})
}

// RecordAssembleCompleted — final video uploaded.
type RecordAssembleCompleted struct {
	RunID      string
	StepIndex  int
	ObjectKey  string
	Bucket     string
	Bytes      int64
	Cost       pipeline.CostInfo
	DurationMs int
}

func (RecordAssembleCompleted) IsCommand()                  {}
func (c RecordAssembleCompleted) GetRunID() string          { return c.RunID }
func (c RecordAssembleCompleted) GetStepType() string       { return "assemble" }
func (c RecordAssembleCompleted) GetProvider() string       { return c.Cost.Provider }
func (c RecordAssembleCompleted) GetCostUSD() float64       { return c.Cost.TotalCostUSD }
func (c RecordAssembleCompleted) GetTerminalStatus() string { return "completed" }

type RecordAssembleCompletedHandler struct{ uow uow.Manager }

func NewRecordAssembleCompletedHandler(m uow.Manager) *RecordAssembleCompletedHandler {
	return &RecordAssembleCompletedHandler{uow: m}
}

func (h *RecordAssembleCompletedHandler) Handle(ctx context.Context, cmd RecordAssembleCompleted) (RecordStepResult, error) {
	return RecordStepResult{}, h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repos := u.Repositories()
		run, err := repos.PipelineRuns().GetByID(ctx, pipeline.RunID(cmd.RunID))
		if err != nil {
			return err
		}
		if err := run.RecordAssembleCompleted(cmd.StepIndex, cmd.ObjectKey, cmd.Cost, cmd.DurationMs, cmd.Bytes); err != nil {
			return err
		}
		run.FillEmptyBucketsOnNewAssets(cmd.Bucket)
		if err := repos.PipelineRuns().Save(ctx, run); err != nil {
			return err
		}
		return repos.Outbox().Add(ctx, run.PullEvents()...)
	})
}

// RecordCaptionCompleted — caption step done; carries per-platform map.
type RecordCaptionCompleted struct {
	RunID      string
	StepIndex  int
	Captions   map[string]any
	Bucket     string
	Cost       pipeline.CostInfo
	DurationMs int
}

func (RecordCaptionCompleted) IsCommand()            {}
func (c RecordCaptionCompleted) GetRunID() string    { return c.RunID }
func (c RecordCaptionCompleted) GetStepType() string { return "caption" }
func (c RecordCaptionCompleted) GetProvider() string { return c.Cost.Provider }
func (c RecordCaptionCompleted) GetCostUSD() float64 { return c.Cost.TotalCostUSD }

type RecordCaptionCompletedHandler struct{ uow uow.Manager }

func NewRecordCaptionCompletedHandler(m uow.Manager) *RecordCaptionCompletedHandler {
	return &RecordCaptionCompletedHandler{uow: m}
}

func (h *RecordCaptionCompletedHandler) Handle(ctx context.Context, cmd RecordCaptionCompleted) (RecordStepResult, error) {
	return RecordStepResult{}, h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repos := u.Repositories()
		run, err := repos.PipelineRuns().GetByID(ctx, pipeline.RunID(cmd.RunID))
		if err != nil {
			return err
		}
		if err := run.RecordCaptionCompleted(cmd.StepIndex, cmd.Captions, cmd.Cost, cmd.DurationMs); err != nil {
			return err
		}
		run.FillEmptyBucketsOnNewAssets(cmd.Bucket)
		if err := repos.PipelineRuns().Save(ctx, run); err != nil {
			return err
		}
		return repos.Outbox().Add(ctx, run.PullEvents()...)
	})
}

// RecordStepFailed — any worker failed.
type RecordStepFailed struct {
	RunID     string
	StepIndex int
	Error     string
}

func (RecordStepFailed) IsCommand()         {}
func (c RecordStepFailed) GetRunID() string { return c.RunID }

// StepType isn't carried on the failure payload; we can't tag the metric by
// type, so report "unknown". (Could be tagged by reading the step row first.)
func (c RecordStepFailed) GetStepType() string     { return "unknown" }
func (RecordStepFailed) GetTerminalStatus() string { return "failed" }

type RecordStepFailedHandler struct{ uow uow.Manager }

func NewRecordStepFailedHandler(m uow.Manager) *RecordStepFailedHandler {
	return &RecordStepFailedHandler{uow: m}
}

func (h *RecordStepFailedHandler) Handle(ctx context.Context, cmd RecordStepFailed) (RecordStepResult, error) {
	return RecordStepResult{}, h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repos := u.Repositories()
		run, err := repos.PipelineRuns().GetByID(ctx, pipeline.RunID(cmd.RunID))
		if err != nil {
			return err
		}
		if err := run.RecordStepFailed(cmd.StepIndex, cmd.Error); err != nil {
			return err
		}
		if err := repos.PipelineRuns().Save(ctx, run); err != nil {
			return err
		}
		return repos.Outbox().Add(ctx, run.PullEvents()...)
	})
}

// CancelRun cancels a queued or running run.
type CancelRun struct{ RunID string }

func (CancelRun) IsCommand()                {}
func (c CancelRun) GetRunID() string        { return c.RunID }
func (CancelRun) GetTerminalStatus() string { return "cancelled" }

type CancelRunResult struct{}

type CancelRunHandler struct{ uow uow.Manager }

func NewCancelRunHandler(m uow.Manager) *CancelRunHandler { return &CancelRunHandler{uow: m} }

func (h *CancelRunHandler) Handle(ctx context.Context, cmd CancelRun) (CancelRunResult, error) {
	return CancelRunResult{}, h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repos := u.Repositories()
		run, err := repos.PipelineRuns().GetByID(ctx, pipeline.RunID(cmd.RunID))
		if err != nil {
			return err
		}
		if err := run.Cancel(); err != nil {
			return err
		}
		if err := repos.PipelineRuns().Save(ctx, run); err != nil {
			return err
		}
		return repos.Outbox().Add(ctx, run.PullEvents()...)
	})
}

func RecordScriptCompletedOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[RecordScriptCompleted, RecordStepResult](r, NewRecordScriptCompletedHandler(m))
}
func RecordImageCompletedOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[RecordImageCompleted, RecordStepResult](r, NewRecordImageCompletedHandler(m))
}
func RecordAssembleCompletedOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[RecordAssembleCompleted, RecordStepResult](r, NewRecordAssembleCompletedHandler(m))
}
func RecordAudioCompletedOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[RecordAudioCompleted, RecordStepResult](r, NewRecordAudioCompletedHandler(m))
}
func RecordUploadCompletedOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[RecordUploadCompleted, RecordStepResult](r, NewRecordUploadCompletedHandler(m))
}
func RecordMusicCompletedOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[RecordMusicCompleted, RecordStepResult](r, NewRecordMusicCompletedHandler(m))
}
func RecordStepFailedOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[RecordStepFailed, RecordStepResult](r, NewRecordStepFailedHandler(m))
}
func CancelRunOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[CancelRun, CancelRunResult](r, NewCancelRunHandler(m))
}
func RecordCaptionCompletedOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[RecordCaptionCompleted, RecordStepResult](r, NewRecordCaptionCompletedHandler(m))
}
