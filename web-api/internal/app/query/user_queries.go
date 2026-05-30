// Package query holds query handlers (the READ side). Queries NEVER touch the
// domain aggregates or open write transactions. They read flat DTOs from the
// read-model store (sqlc against the READ pool), which can point at a replica
// in production. This is the CQRS read/write split that enables master-slave.
package query

import (
	"context"
	"time"

	"github.com/example/dddcqrs/internal/app/bus"
)

// --- Read DTOs (flat projections, NOT domain objects) ---

type UserView struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// ReadModel is the READ-side port. Its implementation uses the read pool /
// replica. It returns DTOs directly — no mapping through the domain.
type ReadModel interface {
	UserByID(ctx context.Context, id string) (UserView, error)
	UserByEmailWithSecret(ctx context.Context, email string) (UserAuthView, error)
	ListUsers(ctx context.Context, limit, offset int32) ([]UserView, error)
}

// UserAuthView is a read DTO that includes the password hash, used only by the
// login flow to verify credentials. Kept separate from UserView on purpose.
type UserAuthView struct {
	ID           string
	Email        string
	PasswordHash string
	Status       string
}

// --- GetUser query ---

type GetUser struct {
	UserID string
}

func (GetUser) IsQuery() {}

type GetUserHandler struct {
	read ReadModel
}

func NewGetUserHandler(r ReadModel) *GetUserHandler { return &GetUserHandler{read: r} }

func (h *GetUserHandler) Handle(ctx context.Context, q GetUser) (UserView, error) {
	return h.read.UserByID(ctx, q.UserID)
}

func GetUserOnBus(r *bus.Registry, rm ReadModel) {
	bus.RegisterQuery[GetUser, UserView](r, NewGetUserHandler(rm))
}

// --- ListUsers query ---

type ListUsers struct {
	Limit  int32
	Offset int32
}

func (ListUsers) IsQuery() {}

type ListUsersHandler struct {
	read ReadModel
}

func NewListUsersHandler(r ReadModel) *ListUsersHandler { return &ListUsersHandler{read: r} }

func (h *ListUsersHandler) Handle(ctx context.Context, q ListUsers) ([]UserView, error) {
	if q.Limit <= 0 || q.Limit > 100 {
		q.Limit = 20
	}
	return h.read.ListUsers(ctx, q.Limit, q.Offset)
}

func ListUsersOnBus(r *bus.Registry, rm ReadModel) {
	bus.RegisterQuery[ListUsers, []UserView](r, NewListUsersHandler(rm))
}
