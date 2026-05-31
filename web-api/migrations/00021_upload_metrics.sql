-- Upload analytics: per-upload metrics over time.
--
-- pipeline_upload_records gains denormalised last-known counters so the
-- list-of-uploads view doesn't have to JOIN the snapshots table for every
-- row. upload_metrics_snapshots is the append-only time-series the UI uses
-- to render charts + trend deltas.

-- +goose Up

-- +goose StatementBegin
ALTER TABLE pipeline_upload_records
    ADD COLUMN last_known_views    BIGINT      NOT NULL DEFAULT 0,
    ADD COLUMN last_known_likes    BIGINT      NOT NULL DEFAULT 0,
    ADD COLUMN last_known_comments BIGINT      NOT NULL DEFAULT 0,
    ADD COLUMN last_known_shares   BIGINT      NOT NULL DEFAULT 0,
    ADD COLUMN duration_seconds    INT         NOT NULL DEFAULT 0,
    ADD COLUMN published_at        TIMESTAMPTZ,
    ADD COLUMN last_fetched_at     TIMESTAMPTZ,
    ADD COLUMN fetch_error         TEXT        NOT NULL DEFAULT '',
    ADD COLUMN fetch_attempt_count INT         NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE upload_metrics_snapshots (
    id                TEXT PRIMARY KEY,
    upload_record_id  TEXT NOT NULL REFERENCES pipeline_upload_records(id) ON DELETE CASCADE,
    fetched_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    views             BIGINT NOT NULL DEFAULT 0,
    likes             BIGINT NOT NULL DEFAULT 0,
    comments          BIGINT NOT NULL DEFAULT 0,
    shares            BIGINT NOT NULL DEFAULT 0,
    raw_json          JSONB NOT NULL DEFAULT '{}'::jsonb
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_metrics_upload_time
    ON upload_metrics_snapshots (upload_record_id, fetched_at DESC);
-- +goose StatementEnd

-- +goose StatementBegin
-- Index for the metrics ticker: "give me rows due for a fetch".
CREATE INDEX idx_upload_records_fetch_due
    ON pipeline_upload_records (last_fetched_at NULLS FIRST)
    WHERE external_ref <> '';
-- +goose StatementEnd


-- +goose Down

-- +goose StatementBegin
DROP TABLE upload_metrics_snapshots;
-- +goose StatementEnd

-- +goose StatementBegin
DROP INDEX IF EXISTS idx_upload_records_fetch_due;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE pipeline_upload_records
    DROP COLUMN last_known_views,
    DROP COLUMN last_known_likes,
    DROP COLUMN last_known_comments,
    DROP COLUMN last_known_shares,
    DROP COLUMN duration_seconds,
    DROP COLUMN published_at,
    DROP COLUMN last_fetched_at,
    DROP COLUMN fetch_error,
    DROP COLUMN fetch_attempt_count;
-- +goose StatementEnd
