-- Make social_accounts global (not tied to a single project) + add a
-- project_social_account_links many-to-many table so one YouTube channel
-- (or Telegram bot, etc.) can be linked to multiple projects.
--
-- Existing rows: the audit query showed 0 rows in social_accounts at
-- migration time; this also wipes any future env to a known state. If a
-- prod env ever has rows, write a backfill migration first that creates
-- {project_id, social_account_id, is_default=true} rows for each existing
-- account before dropping project_id.
--
-- Resolution chain for uploads now: explicit step param → run override
-- → project default (per platform) → fail. Project default lives on the
-- link row (is_default).

-- +goose Up

-- +goose StatementBegin
DELETE FROM social_accounts;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE social_accounts DROP COLUMN project_id;
-- +goose StatementEnd

-- +goose StatementBegin
DROP INDEX IF EXISTS idx_social_accounts_project;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE project_social_account_links (
    project_id        TEXT NOT NULL REFERENCES projects(id)        ON DELETE CASCADE,
    social_account_id TEXT NOT NULL REFERENCES social_accounts(id) ON DELETE CASCADE,
    is_default        BOOLEAN NOT NULL DEFAULT FALSE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, social_account_id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_psal_account ON project_social_account_links (social_account_id);
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP TABLE project_social_account_links;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE social_accounts ADD COLUMN project_id TEXT REFERENCES projects(id) ON DELETE CASCADE;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_social_accounts_project ON social_accounts (project_id);
-- +goose StatementEnd
