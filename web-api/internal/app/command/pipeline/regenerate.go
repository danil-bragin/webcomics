package pipeline

import (
	"context"

	"github.com/example/dddcqrs/internal/app/bus"
	"github.com/example/dddcqrs/internal/domain/pipeline"
	"github.com/example/dddcqrs/internal/infrastructure/persistence/uow"
)

// RegenerateStep creates a new attempt for an existing step inside a run.
// Downstream steps are flagged stale and stay that way until the user
// regenerates them too — no auto cascade by design (lets the user inspect
// intermediate output before paying for the next leg).
type RegenerateStep struct {
	RunID          string
	StepIndex      int
	ParamsOverride map[string]any
}

func (RegenerateStep) IsCommand() {}

type RegenerateStepResult struct {
	RunID        string
	StepIndex    int
	NewAttemptID string
	NewVersion   int
}

type RegenerateStepHandler struct{ uow uow.Manager }

func NewRegenerateStepHandler(m uow.Manager) *RegenerateStepHandler {
	return &RegenerateStepHandler{uow: m}
}

func (h *RegenerateStepHandler) Handle(ctx context.Context, cmd RegenerateStep) (RegenerateStepResult, error) {
	var out RegenerateStepResult
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repos := u.Repositories()
		run, err := repos.PipelineRuns().GetByID(ctx, pipeline.RunID(cmd.RunID))
		if err != nil {
			return err
		}
		// Re-resolve project linkage so the regenerate event carries the
		// CURRENT character/environment/plot data (the user may have edited
		// descriptions or added reference images since the run was created).
		if run.ProjectID() != "" {
			linked, _, err := resolveLinkedContext(ctx, repos, run.ProjectID(),
				run.CharacterIDs(), run.EnvironmentIDs(), run.PlotID() != "")
			if err != nil {
				return err
			}
			run.SetLinkedContext(linked)
		}
		if err := run.RegenerateStep(cmd.StepIndex, cmd.ParamsOverride); err != nil {
			return err
		}
		if err := repos.PipelineRuns().Save(ctx, run); err != nil {
			return err
		}
		if err := repos.Outbox().Add(ctx, run.PullEvents()...); err != nil {
			return err
		}
		step := run.Steps()[cmd.StepIndex]
		out.RunID = cmd.RunID
		out.StepIndex = cmd.StepIndex
		if a := step.ActiveAttempt(); a != nil {
			out.NewAttemptID = a.ID().String()
		}
		out.NewVersion = step.CurrentVersion()
		return nil
	})
	return out, err
}

func RegenerateStepOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[RegenerateStep, RegenerateStepResult](r, NewRegenerateStepHandler(m))
}

// RequestAssemble triggers the assemble step when a run is in awaiting_action
// state (auto_assemble=false). Optional params_override lets the caller change
// fps / panel_duration_ms / transition / the full timeline JSON at trigger time.
type RequestAssemble struct {
	RunID          string
	ParamsOverride map[string]any
}

func (RequestAssemble) IsCommand() {}

type RequestAssembleResult struct {
	RunID     string
	StepIndex int
}

type RequestAssembleHandler struct{ uow uow.Manager }

func NewRequestAssembleHandler(m uow.Manager) *RequestAssembleHandler {
	return &RequestAssembleHandler{uow: m}
}

func (h *RequestAssembleHandler) Handle(ctx context.Context, cmd RequestAssemble) (RequestAssembleResult, error) {
	var out RequestAssembleResult
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repos := u.Repositories()
		run, err := repos.PipelineRuns().GetByID(ctx, pipeline.RunID(cmd.RunID))
		if err != nil {
			return err
		}
		if run.ProjectID() != "" {
			linked, _, err := resolveLinkedContext(ctx, repos, run.ProjectID(),
				run.CharacterIDs(), run.EnvironmentIDs(), run.PlotID() != "")
			if err != nil {
				return err
			}
			run.SetLinkedContext(linked)
		}
		if err := run.RequestAssemble(cmd.ParamsOverride); err != nil {
			return err
		}
		if err := repos.PipelineRuns().Save(ctx, run); err != nil {
			return err
		}
		if err := repos.Outbox().Add(ctx, run.PullEvents()...); err != nil {
			return err
		}
		out.RunID = cmd.RunID
		out.StepIndex = run.CurrentStepIndex()
		return nil
	})
	return out, err
}

func RequestAssembleOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[RequestAssemble, RequestAssembleResult](r, NewRequestAssembleHandler(m))
}
