package pipeline

import (
	"context"

	"github.com/example/dddcqrs/internal/app/bus"
	"github.com/example/dddcqrs/internal/domain/pipeline"
	"github.com/example/dddcqrs/internal/infrastructure/persistence/uow"
)

type CreateTemplate struct {
	Name       string
	Steps      []pipeline.StepConfig
	MaxCostUSD float64
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
		if err := u.Repositories().PipelineTemplates().Save(ctx, tpl); err != nil {
			return err
		}
		out.TemplateID = tpl.ID().String()
		return nil
	})
	return out, err
}

type UpdateTemplate struct {
	TemplateID    string
	Name          string
	Steps         []pipeline.StepConfig
	MaxCostUSD    float64
	UpdateMaxCost bool
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
		if len(cmd.Steps) > 0 {
			if err := tpl.UpdateSteps(cmd.Steps); err != nil {
				return err
			}
		}
		if cmd.UpdateMaxCost {
			tpl.SetMaxCostUSD(cmd.MaxCostUSD)
		}
		return u.Repositories().PipelineTemplates().Save(ctx, tpl)
	})
}

func CreateTemplateOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[CreateTemplate, CreateTemplateResult](r, NewCreateTemplateHandler(m))
}
func UpdateTemplateOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[UpdateTemplate, UpdateTemplateResult](r, NewUpdateTemplateHandler(m))
}
