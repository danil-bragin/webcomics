// Package grpc is an ENTRY POINT. Like the HTTP and consumer entries, it only
// translates gRPC calls into commands/queries dispatched through the bus.
//
// NOTE: this is a transport skeleton. Generate your *.pb.go from api/proto via
// buf/protoc, then implement the generated service interface by delegating each
// RPC to bus.Dispatch / bus.Ask. The pattern is shown below in pseudo-form.
package grpc

import (
	"context"

	"github.com/example/dddcqrs/internal/app/bus"
	"github.com/example/dddcqrs/internal/app/command"
	"github.com/example/dddcqrs/internal/app/query"
)

// Service holds the bus; the generated UserServiceServer embeds/uses it.
type Service struct {
	reg *bus.Registry
}

func NewService(reg *bus.Registry) *Service { return &Service{reg: reg} }

// RegisterUser is the shape a generated RPC method would take. Replace the
// plain structs with the generated *pb.RegisterUserRequest / Response.
func (s *Service) RegisterUser(ctx context.Context, email, passwordHash string) (string, error) {
	res, err := bus.Dispatch[command.RegisterUserResult](ctx, s.reg, command.RegisterUser{
		Email:        email,
		PasswordHash: passwordHash,
	})
	if err != nil {
		return "", err
	}
	return res.UserID, nil
}

// GetUser delegates to the read side.
func (s *Service) GetUser(ctx context.Context, id string) (query.UserView, error) {
	return bus.Ask[query.UserView](ctx, s.reg, query.GetUser{UserID: id})
}
