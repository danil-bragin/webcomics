// Package middleware provides the cross-cutting pipeline applied to every
// command/query dispatched through the bus. Command and query sides can use
// different pipelines (e.g. only commands get transactional concerns).
package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/example/dddcqrs/internal/app/bus"
)

// Recover converts panics in handlers into errors so one bad message can't
// crash the process. Outermost in the pipeline.
func Recover() bus.Middleware {
	return func(next bus.Handler) bus.Handler {
		return func(ctx context.Context, msg any) (result any, err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("panic in handler %T: %v", msg, r)
				}
			}()
			return next(ctx, msg)
		}
	}
}

// HasRunID is implemented by pipeline commands so the Logging middleware
// can surface run_id (the unified pipeline field) without reflection.
type HasRunID interface {
	GetRunID() string
}

// Logging records each dispatch with timing and outcome.
func Logging(log *slog.Logger) bus.Middleware {
	return func(next bus.Handler) bus.Handler {
		return func(ctx context.Context, msg any) (any, error) {
			start := time.Now()
			res, err := next(ctx, msg)
			attrs := []any{
				"message", fmt.Sprintf("%T", msg),
				"duration_ms", time.Since(start).Milliseconds(),
			}
			if hr, ok := msg.(HasRunID); ok {
				if id := hr.GetRunID(); id != "" {
					attrs = append(attrs, "run_id", id)
				}
			}
			if err != nil {
				log.ErrorContext(ctx, "dispatch failed", append(attrs, "err", err)...)
			} else {
				log.InfoContext(ctx, "dispatch ok", attrs...)
			}
			return res, err
		}
	}
}

// Validatable is implemented by commands/queries that can self-validate.
type Validatable interface {
	Validate() error
}

// Validation runs Validate() on messages that support it, before the handler.
func Validation() bus.Middleware {
	return func(next bus.Handler) bus.Handler {
		return func(ctx context.Context, msg any) (any, error) {
			if v, ok := msg.(Validatable); ok {
				if err := v.Validate(); err != nil {
					return nil, err
				}
			}
			return next(ctx, msg)
		}
	}
}
