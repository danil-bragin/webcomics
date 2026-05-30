package user

import (
	"context"

	"github.com/example/dddcqrs/internal/domain/shared"
)

// UserRegistered is emitted when a new user is registered.
type UserRegistered struct {
	shared.BaseEvent
	Email string
}

func (UserRegistered) EventName() string { return "user.registered" }

// UserActivated is emitted when a pending user is activated.
type UserActivated struct {
	shared.BaseEvent
}

func (UserActivated) EventName() string { return "user.activated" }

// WriteRepository is the WRITE-side port for the User aggregate. It is bound to
// a Unit of Work's transaction (obtained via uow.Repositories().Users()).
// It deals in domain aggregates, never DTOs.
//
// NOTE: this interface is referenced by the uow package via an alias to avoid
// an import cycle (uow -> domain). See uow.UserWriteRepository.
type WriteRepository interface {
	Save(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id ID) (*User, error)
	GetByEmail(ctx context.Context, email Email) (*User, error)
	ExistsByEmail(ctx context.Context, email Email) (bool, error)
}
