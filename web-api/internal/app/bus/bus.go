// Package bus is the CQRS core. Every entry point (HTTP, gRPC, consumer, CLI)
// converts its input into a Command or Query and dispatches it through here.
// Handlers never know which entry invoked them.
package bus

import (
	"context"
	"fmt"
	"reflect"
)

// Command mutates state. It must declare its result type via the generic
// CommandHandler. Commands are dispatched on the WRITE side (transactional).
type Command interface {
	IsCommand()
}

// Query reads state. Queries are dispatched on the READ side and must never
// open a write transaction. They hit read-models (potentially a replica).
type Query interface {
	IsQuery()
}

// CommandHandler handles a single command type C producing result R.
type CommandHandler[C Command, R any] interface {
	Handle(ctx context.Context, cmd C) (R, error)
}

// QueryHandler handles a single query type Q producing result R.
type QueryHandler[Q Query, R any] interface {
	Handle(ctx context.Context, q Q) (R, error)
}

// HandlerFunc adapters so plain funcs can be handlers.
type CommandHandlerFunc[C Command, R any] func(context.Context, C) (R, error)

func (f CommandHandlerFunc[C, R]) Handle(ctx context.Context, c C) (R, error) { return f(ctx, c) }

type QueryHandlerFunc[Q Query, R any] func(context.Context, Q) (R, error)

func (f QueryHandlerFunc[Q, R]) Handle(ctx context.Context, q Q) (R, error) { return f(ctx, q) }

// Middleware wraps a generic dispatch step. It operates on the erased
// (any, any) boundary so a single pipeline applies to every cmd/query.
// next returns the result of the inner handler.
type Middleware func(next Handler) Handler

// Handler is the type-erased unit the middleware pipeline wraps.
type Handler func(ctx context.Context, msg any) (any, error)

// Registry stores erased handlers keyed by the concrete message type, and the
// middleware pipelines for the command and query sides separately (commands
// get the transactional middleware; queries do not).
type Registry struct {
	handlers map[reflect.Type]Handler
	cmdMW    []Middleware
	queryMW  []Middleware
}

func NewRegistry() *Registry {
	return &Registry{handlers: make(map[reflect.Type]Handler)}
}

// UseCommandMiddleware sets the pipeline applied to every command (order:
// first listed runs outermost). Typically: recover, logging, validation, tx.
func (r *Registry) UseCommandMiddleware(mw ...Middleware) { r.cmdMW = mw }

// UseQueryMiddleware sets the pipeline applied to every query (no tx).
func (r *Registry) UseQueryMiddleware(mw ...Middleware) { r.queryMW = mw }

func chain(h Handler, mw []Middleware) Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}

// RegisterCommand wires a typed command handler into the registry with the
// command middleware pipeline applied.
func RegisterCommand[C Command, R any](r *Registry, h CommandHandler[C, R]) {
	t := reflect.TypeFor[C]()
	erased := func(ctx context.Context, msg any) (any, error) {
		return h.Handle(ctx, msg.(C))
	}
	r.handlers[t] = chain(erased, r.cmdMW)
}

// RegisterQuery wires a typed query handler with the query middleware pipeline.
func RegisterQuery[Q Query, R any](r *Registry, h QueryHandler[Q, R]) {
	t := reflect.TypeFor[Q]()
	erased := func(ctx context.Context, msg any) (any, error) {
		return h.Handle(ctx, msg.(Q))
	}
	r.handlers[t] = chain(erased, r.queryMW)
}

// Dispatch sends a command and returns its typed result. This is the single
// call every entry point uses for writes.
func Dispatch[R any](ctx context.Context, r *Registry, cmd Command) (R, error) {
	var zero R
	h, ok := r.handlers[reflect.TypeOf(cmd)]
	if !ok {
		return zero, fmt.Errorf("bus: no handler for command %T", cmd)
	}
	out, err := h(ctx, cmd)
	if err != nil {
		return zero, err
	}
	res, ok := out.(R)
	if !ok {
		return zero, fmt.Errorf("bus: command %T returned %T, want %T", cmd, out, zero)
	}
	return res, nil
}

// Ask sends a query and returns its typed result. Read side, no transaction.
func Ask[R any](ctx context.Context, r *Registry, q Query) (R, error) {
	var zero R
	h, ok := r.handlers[reflect.TypeOf(q)]
	if !ok {
		return zero, fmt.Errorf("bus: no handler for query %T", q)
	}
	out, err := h(ctx, q)
	if err != nil {
		return zero, err
	}
	res, ok := out.(R)
	if !ok {
		return zero, fmt.Errorf("bus: query %T returned %T, want %T", q, out, zero)
	}
	return res, nil
}
