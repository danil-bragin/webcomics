// Package command holds command handlers (the write side). Each handler opens
// a Unit of Work, performs domain work through transactional repositories,
// records domain events into the outbox, and commits — all within ONE
// explicit transaction owned by the handler.
package command

import (
	"context"
	"errors"

	"github.com/example/dddcqrs/internal/app/bus"
	"github.com/example/dddcqrs/internal/domain/user"
	"github.com/example/dddcqrs/internal/infrastructure/persistence/uow"
)

// RegisterUser is the command. It carries already-validated primitive input;
// the bus validation middleware checks it before the handler runs.
type RegisterUser struct {
	Email        string
	PasswordHash string // app pre-hashes; domain never sees plaintext
}

func (RegisterUser) IsCommand() {}

// RegisterUserResult is returned to the entry point.
type RegisterUserResult struct {
	UserID string
}

var ErrEmailTaken = errors.New("email already taken")

// RegisterUserHandler depends only on the UoW Manager (port), not on concrete
// infrastructure. The transaction is opened, used, and committed right here —
// the boundary is explicit and reviewable.
type RegisterUserHandler struct {
	uow uow.Manager
}

func NewRegisterUserHandler(m uow.Manager) *RegisterUserHandler {
	return &RegisterUserHandler{uow: m}
}

func (h *RegisterUserHandler) Handle(ctx context.Context, cmd RegisterUser) (RegisterUserResult, error) {
	var result RegisterUserResult

	email, err := user.NewEmail(cmd.Email)
	if err != nil {
		return result, err
	}

	// Explicit Unit of Work. Everything below shares one pgx.Tx.
	txErr := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repos := u.Repositories()

		exists, err := repos.Users().ExistsByEmail(ctx, email)
		if err != nil {
			return err
		}
		if exists {
			return ErrEmailTaken
		}

		agg, err := user.Register(email, cmd.PasswordHash)
		if err != nil {
			return err
		}

		// Persist aggregate AND its domain events in the same transaction.
		if err := repos.Users().Save(ctx, agg); err != nil {
			return err
		}
		if err := repos.Outbox().Add(ctx, agg.PullEvents()...); err != nil {
			return err
		}

		result.UserID = agg.ID().String()
		return nil
		// WithinTx commits on nil, rolls back on error.
	})
	if txErr != nil {
		return RegisterUserResult{}, txErr
	}
	return result, nil
}

// Register wires this handler into the bus.
func RegisterUserOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[RegisterUser, RegisterUserResult](r, NewRegisterUserHandler(m))
}
