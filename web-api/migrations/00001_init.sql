-- +goose Up
-- +goose StatementBegin
CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'pending',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
-- Transactional outbox: events are written here in the SAME tx as the
-- aggregate change; the relay publishes unpublished rows to the broker.
CREATE TABLE outbox (
    id           TEXT PRIMARY KEY,
    aggregate_id TEXT NOT NULL,
    event_name   TEXT NOT NULL,
    payload      JSONB NOT NULL,
    occurred_at  TIMESTAMPTZ NOT NULL,
    published_at TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_outbox_unpublished ON outbox (occurred_at) WHERE published_at IS NULL;
-- +goose StatementEnd

-- +goose StatementBegin
-- Dedup for at-least-once consumers (idempotency).
CREATE TABLE processed_messages (
    message_id   TEXT NOT NULL,
    handler      TEXT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (message_id, handler)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE processed_messages;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE outbox;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE users;
-- +goose StatementEnd
