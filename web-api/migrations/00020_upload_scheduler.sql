-- Per-account rate-limit config + scheduled_uploads table.
--
-- daily_upload_limit + limit_window_hours model platform caps:
--   YT unverified  → 15 / 24h  (default)
--   YT verified    → 100 / 24h
--   IG / TikTok    → varies
-- min_gap_seconds is the floor between two scheduled uploads on the same
-- account (e.g. 60s) so Selenium/profile lock doesn't race.

-- +goose Up

-- +goose StatementBegin
ALTER TABLE social_accounts
    ADD COLUMN daily_upload_limit INT  NOT NULL DEFAULT 15,
    ADD COLUMN limit_window_hours INT  NOT NULL DEFAULT 24,
    ADD COLUMN is_verified        BOOL NOT NULL DEFAULT FALSE,
    ADD COLUMN min_gap_seconds    INT  NOT NULL DEFAULT 60;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE scheduled_uploads (
    id                TEXT PRIMARY KEY,
    run_id            TEXT NOT NULL REFERENCES pipeline_runs(id)  ON DELETE CASCADE,
    social_account_id TEXT NOT NULL REFERENCES social_accounts(id) ON DELETE CASCADE,
    scheduled_at      TIMESTAMPTZ NOT NULL,
    status            TEXT NOT NULL DEFAULT 'pending',
        -- pending | in_flight | completed | failed | cancelled
    external_ref      TEXT NOT NULL DEFAULT '',
    error             TEXT NOT NULL DEFAULT '',
    metadata          JSONB NOT NULL DEFAULT '{}'::jsonb,
        -- snapshot of upload params (visibility, tags, title, description …)
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    fired_at          TIMESTAMPTZ,
    completed_at      TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_sched_due ON scheduled_uploads (scheduled_at) WHERE status = 'pending';
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_sched_account_at ON scheduled_uploads (social_account_id, scheduled_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_sched_run ON scheduled_uploads (run_id);
-- +goose StatementEnd


-- +goose Down

-- +goose StatementBegin
DROP TABLE scheduled_uploads;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE social_accounts
    DROP COLUMN daily_upload_limit,
    DROP COLUMN limit_window_hours,
    DROP COLUMN is_verified,
    DROP COLUMN min_gap_seconds;
-- +goose StatementEnd
