package outbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Relay polls the outbox on the WRITE pool and publishes unpublished events to
// the broker, then marks them published. Run as its own process/goroutine.
// At-least-once: a crash between publish and mark re-publishes — consumers
// must be idempotent.
type Relay struct {
	pool      *pgxpool.Pool
	publisher message.Publisher
	log       *slog.Logger
	batch     int32
	interval  time.Duration
}

func NewRelay(pool *pgxpool.Pool, pub message.Publisher, log *slog.Logger) *Relay {
	return &Relay{
		pool:      pool,
		publisher: pub,
		log:       log,
		batch:     100,
		interval:  time.Second,
	}
}

// Run loops until ctx is cancelled.
func (r *Relay) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := r.drain(ctx); err != nil {
				r.log.Error("outbox drain failed", "err", err)
			}
		}
	}
}

func (r *Relay) drain(ctx context.Context) error {
	// Lock unpublished rows so multiple relay instances don't double-publish.
	const sel = `
		SELECT id, aggregate_id, event_name, payload, occurred_at
		FROM outbox
		WHERE published_at IS NULL
		ORDER BY occurred_at
		LIMIT $1
		FOR UPDATE SKIP LOCKED`

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, sel, r.batch)
	if err != nil {
		return err
	}
	var recs []Record
	for rows.Next() {
		var rec Record
		if err := rows.Scan(&rec.ID, &rec.AggregateID, &rec.EventName, &rec.Payload, &rec.OccurredAt); err != nil {
			rows.Close()
			return err
		}
		recs = append(recs, rec)
	}
	rows.Close()
	if len(recs) == 0 {
		return tx.Commit(ctx)
	}

	ids := make([]string, 0, len(recs))
	for _, rec := range recs {
		msg := message.NewMessage(rec.ID, rec.Payload)
		msg.Metadata.Set("event_name", rec.EventName)
		msg.Metadata.Set("aggregate_id", rec.AggregateID)
		if err := r.publisher.Publish(rec.EventName, msg); err != nil {
			return err // rollback; retry next tick
		}
		ids = append(ids, rec.ID)
	}

	const upd = `UPDATE outbox SET published_at = now() WHERE id = ANY($1)`
	if _, err := tx.Exec(ctx, upd, ids); err != nil {
		return err
	}
	r.log.Info("outbox published", "count", len(ids))
	return tx.Commit(ctx)
}
