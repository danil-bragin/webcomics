// Package consumer is an ENTRY POINT driven by the message broker. It consumes
// events from Redis Streams and translates them into commands/queries on the
// bus — exactly like the HTTP entry, just a different transport. Business logic
// stays in handlers; the consumer only maps message → command.
package consumer

import (
	"log/slog"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/message/router/middleware"

	"github.com/example/dddcqrs/internal/app/bus"
	"github.com/example/dddcqrs/internal/app/command"
)

const ConsumerGroup = "dddcqrs-consumer"

type Consumer struct {
	reg *bus.Registry
	log *slog.Logger
}

func New(reg *bus.Registry, log *slog.Logger) *Consumer {
	return &Consumer{reg: reg, log: log}
}

// Register wires handlers onto a Watermill router. The subscriber is created by
// the caller (cmd/consumer) so the consumer group is explicit.
func (c *Consumer) Register(router *message.Router, sub message.Subscriber) {
	router.AddMiddleware(
		middleware.Recoverer,
		middleware.CorrelationID,
		middleware.Retry{MaxRetries: 3}.Middleware,
	)

	// Example: when user.registered is observed, run a follow-up command.
	// (Here we just log; replace with a real command dispatch.)
	router.AddNoPublisherHandler(
		"on_user_registered",
		"user.registered",
		sub,
		c.onUserRegistered,
	)
}

func (c *Consumer) onUserRegistered(msg *message.Message) error {
	ctx := msg.Context()
	c.log.InfoContext(ctx, "consumed event",
		"uuid", msg.UUID,
		"event", msg.Metadata.Get("event_name"),
		"aggregate", msg.Metadata.Get("aggregate_id"),
	)

	// ENTRY → COMMAND example: auto-activate the user after registration.
	// Idempotency is provided by the command/aggregate (Activate is a no-op if
	// already active) plus at-least-once delivery semantics.
	aggregateID := msg.Metadata.Get("aggregate_id")
	if aggregateID != "" {
		if _, err := bus.Dispatch[command.ActivateUserResult](ctx, c.reg, command.ActivateUser{
			UserID: aggregateID,
		}); err != nil {
			// returning err → Nack → retried
			return err
		}
	}
	return nil
}
