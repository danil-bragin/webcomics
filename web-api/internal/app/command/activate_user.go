package command

import (
	"context"

	"github.com/example/dddcqrs/internal/app/bus"
	"github.com/example/dddcqrs/internal/domain/user"
	"github.com/example/dddcqrs/internal/infrastructure/persistence/uow"
)

type ActivateUser struct {
	UserID string
}

func (ActivateUser) IsCommand() {}

type ActivateUserResult struct{}

type ActivateUserHandler struct {
	uow uow.Manager
}

func NewActivateUserHandler(m uow.Manager) *ActivateUserHandler {
	return &ActivateUserHandler{uow: m}
}

func (h *ActivateUserHandler) Handle(ctx context.Context, cmd ActivateUser) (ActivateUserResult, error) {
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repos := u.Repositories()

		agg, err := repos.Users().GetByID(ctx, user.ID(cmd.UserID))
		if err != nil {
			return err
		}
		if err := agg.Activate(); err != nil {
			return err
		}
		if err := repos.Users().Save(ctx, agg); err != nil {
			return err
		}
		return repos.Outbox().Add(ctx, agg.PullEvents()...)
	})
	return ActivateUserResult{}, err
}

func ActivateUserOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[ActivateUser, ActivateUserResult](r, NewActivateUserHandler(m))
}
