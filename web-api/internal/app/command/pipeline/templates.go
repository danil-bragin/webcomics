package pipeline

import (
	"context"

	"github.com/example/dddcqrs/internal/app/bus"
	"github.com/example/dddcqrs/internal/domain/pipeline"
	"github.com/example/dddcqrs/internal/infrastructure/persistence/uow"
)

// CreateTemplate accepts the rich preset metadata so the marketplace UI can
// build a fully-formed preset in one POST.
type CreateTemplate struct {
	Name          string
	Description   string
	Category      string
	Icon          string
	Steps         []pipeline.StepConfig
	SamplePrompts []string
	FormatID      string
	Defaults      map[string]any
	MaxCostUSD    float64
}

func (CreateTemplate) IsCommand() {}

type CreateTemplateResult struct{ TemplateID string }

type CreateTemplateHandler struct{ uow uow.Manager }

func NewCreateTemplateHandler(m uow.Manager) *CreateTemplateHandler {
	return &CreateTemplateHandler{uow: m}
}

func (h *CreateTemplateHandler) Handle(ctx context.Context, cmd CreateTemplate) (CreateTemplateResult, error) {
	var out CreateTemplateResult
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		tpl, err := pipeline.NewTemplateWithCap(cmd.Name, cmd.Steps, cmd.MaxCostUSD)
		if err != nil {
			return err
		}
		tpl.SetDescription(cmd.Description)
		if cmd.Category != "" {
			tpl.SetCategory(cmd.Category)
		}
		tpl.SetIcon(cmd.Icon)
		if cmd.SamplePrompts != nil {
			tpl.SetSamplePrompts(cmd.SamplePrompts)
		}
		tpl.SetFormatID(cmd.FormatID)
		if cmd.Defaults != nil {
			tpl.SetDefaults(cmd.Defaults)
		}
		if err := u.Repositories().PipelineTemplates().Save(ctx, tpl); err != nil {
			return err
		}
		out.TemplateID = tpl.ID().String()
		return nil
	})
	return out, err
}

type UpdateTemplate struct {
	TemplateID     string
	Name           string
	Description    *string
	Category       string
	Icon           *string
	Steps          []pipeline.StepConfig
	SamplePrompts  *[]string
	FormatID       *string
	Defaults       map[string]any
	UpdateDefaults bool
	MaxCostUSD     float64
	UpdateMaxCost  bool
}

func (UpdateTemplate) IsCommand() {}

type UpdateTemplateResult struct{}

type UpdateTemplateHandler struct{ uow uow.Manager }

func NewUpdateTemplateHandler(m uow.Manager) *UpdateTemplateHandler {
	return &UpdateTemplateHandler{uow: m}
}

func (h *UpdateTemplateHandler) Handle(ctx context.Context, cmd UpdateTemplate) (UpdateTemplateResult, error) {
	return UpdateTemplateResult{}, h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		tpl, err := u.Repositories().PipelineTemplates().GetByID(ctx, pipeline.TemplateID(cmd.TemplateID))
		if err != nil {
			return err
		}
		if cmd.Name != "" {
			if err := tpl.UpdateName(cmd.Name); err != nil {
				return err
			}
		}
		if cmd.Description != nil {
			tpl.SetDescription(*cmd.Description)
		}
		if cmd.Category != "" {
			tpl.SetCategory(cmd.Category)
		}
		if cmd.Icon != nil {
			tpl.SetIcon(*cmd.Icon)
		}
		if len(cmd.Steps) > 0 {
			if err := tpl.UpdateSteps(cmd.Steps); err != nil {
				return err
			}
		}
		if cmd.SamplePrompts != nil {
			tpl.SetSamplePrompts(*cmd.SamplePrompts)
		}
		if cmd.FormatID != nil {
			tpl.SetFormatID(*cmd.FormatID)
		}
		if cmd.UpdateDefaults {
			tpl.SetDefaults(cmd.Defaults)
		}
		if cmd.UpdateMaxCost {
			tpl.SetMaxCostUSD(cmd.MaxCostUSD)
		}
		return u.Repositories().PipelineTemplates().Save(ctx, tpl)
	})
}

// DeleteTemplate removes a preset from the catalogue. Runs that already
// snapshotted the preset still keep their history (template_id column has FK
// but DB enforces ON DELETE SET NULL — see pipeline migration).
type DeleteTemplate struct{ TemplateID string }

func (DeleteTemplate) IsCommand() {}

type DeleteTemplateResult struct{}

type DeleteTemplateHandler struct{ uow uow.Manager }

func NewDeleteTemplateHandler(m uow.Manager) *DeleteTemplateHandler {
	return &DeleteTemplateHandler{uow: m}
}

func (h *DeleteTemplateHandler) Handle(ctx context.Context, cmd DeleteTemplate) (DeleteTemplateResult, error) {
	return DeleteTemplateResult{}, h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		// Soft-delete by marking is_test=true so the marketplace hides it but
		// any pipeline_runs.template_id FK keeps validating. Hard DELETE would
		// require ON DELETE SET NULL on the runs FK; not worth the migration.
		tpl, err := u.Repositories().PipelineTemplates().GetByID(ctx, pipeline.TemplateID(cmd.TemplateID))
		if err != nil {
			return err
		}
		tpl.SetIsTest(true)
		return u.Repositories().PipelineTemplates().Save(ctx, tpl)
	})
}

func CreateTemplateOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[CreateTemplate, CreateTemplateResult](r, NewCreateTemplateHandler(m))
}
func UpdateTemplateOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[UpdateTemplate, UpdateTemplateResult](r, NewUpdateTemplateHandler(m))
}
func DeleteTemplateOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[DeleteTemplate, DeleteTemplateResult](r, NewDeleteTemplateHandler(m))
}
