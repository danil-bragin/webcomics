-- Auto-upload support.
--
-- social_accounts: one row per (project, platform, label) representing a
-- pre-authenticated Firefox profile (or future API credential set). Used by
-- the selenium upload workers to know which browser profile to launch.
--
-- pipeline_runs.scheduled_at: when set in the future, the run pauses in
-- awaiting_action after caption step; a cron-ish loop in the upload worker
-- picks it up at the scheduled time.

-- +goose Up

-- +goose StatementBegin
CREATE TABLE social_accounts (
    id                   TEXT PRIMARY KEY,
    project_id           TEXT REFERENCES projects(id) ON DELETE CASCADE,
    platform             TEXT NOT NULL,           -- 'youtube_selenium' | 'twitter_selenium' | ...
    label                TEXT NOT NULL DEFAULT '',
    firefox_profile_path TEXT NOT NULL DEFAULT '',
    extra                JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_social_accounts_project ON social_accounts (project_id);
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE pipeline_runs
    ADD COLUMN scheduled_at TIMESTAMPTZ;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE pipeline_runs DROP COLUMN scheduled_at;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE social_accounts;
-- +goose StatementEnd
