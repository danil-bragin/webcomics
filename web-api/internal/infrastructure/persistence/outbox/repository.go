// Package outbox implements the transactional outbox. Domain events are written
// to the outbox table WITHIN the same pgx.Tx as the aggregate change (atomic).
// A separate relay process (cmd/outbox-relay) polls unpublished rows and
// publishes them to Redis Streams, then marks them published.
package outbox

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/example/dddcqrs/internal/domain/shared"
)

// Repository persists events transactionally. Bound to a UoW's tx.
type Repository struct {
	tx pgx.Tx
}

func NewRepository(tx pgx.Tx) *Repository { return &Repository{tx: tx} }

// Add inserts each domain event into the outbox in the current transaction.
func (r *Repository) Add(ctx context.Context, events ...shared.DomainEvent) error {
	const q = `
		INSERT INTO outbox (id, aggregate_id, event_name, payload, occurred_at)
		VALUES ($1, $2, $3, $4, $5)`
	for _, e := range events {
		payload, err := json.Marshal(e)
		if err != nil {
			return err
		}
		if _, err := r.tx.Exec(ctx, q,
			uuid.NewString(),
			e.AggregateID(),
			e.EventName(),
			payload,
			e.OccurredAt(),
		); err != nil {
			return err
		}
	}
	return nil
}

// Record is a row read by the relay.
type Record struct {
	ID          string
	AggregateID string
	EventName   string
	Payload     []byte
	OccurredAt  time.Time
}
