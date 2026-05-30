-- 00010 — upload records + extended social account fields.
--
-- pipeline_upload_records: one row per upload attempt. Persists the resolved
-- metadata snapshot (so we can reproduce/retry) plus the eventual YT URL,
-- error trail, and screenshot asset id.
--
-- social_accounts.*: scheduling + per-account defaults so the scheduler can
-- skip an account in cooldown and so a project that doesn't override metadata
-- inherits sensible defaults from the bound channel.

-- +goose Up

-- +goose StatementBegin
CREATE TABLE pipeline_upload_records (
    id                          TEXT PRIMARY KEY,
    run_id                      TEXT NOT NULL REFERENCES pipeline_runs(id) ON DELETE CASCADE,
    project_id                  TEXT REFERENCES projects(id) ON DELETE SET NULL,
    social_account_id           TEXT REFERENCES social_accounts(id) ON DELETE SET NULL,
    step_index                  INT  NOT NULL DEFAULT -1,

    status                      TEXT NOT NULL DEFAULT 'pending',
        -- pending | uploaded | published | failed
    provider                    TEXT NOT NULL,
        -- youtube_selenium | telegram | ...

    -- Resolved metadata snapshot (what we actually sent to YT).
    title                       TEXT NOT NULL DEFAULT '',
    description                 TEXT NOT NULL DEFAULT '',
    tags                        TEXT[] NOT NULL DEFAULT '{}',
    hashtags                    TEXT[] NOT NULL DEFAULT '{}',
    visibility                  TEXT NOT NULL DEFAULT 'unlisted',
        -- public | unlisted | private
    made_for_kids               BOOLEAN NOT NULL DEFAULT false,
    age_restriction             TEXT NOT NULL DEFAULT 'none',
        -- none | 18plus
    category_id                 TEXT NOT NULL DEFAULT '',
    category_label              TEXT NOT NULL DEFAULT '',
    comments_enabled            BOOLEAN NOT NULL DEFAULT true,
    playlist_names              TEXT[] NOT NULL DEFAULT '{}',
    scheduled_at                TIMESTAMPTZ,

    -- Results.
    external_ref                TEXT NOT NULL DEFAULT '',
        -- canonical youtu.be/<id> URL when known
    external_id                 TEXT NOT NULL DEFAULT '',
        -- bare video id
    thumbnail_asset_id          TEXT REFERENCES pipeline_assets(id) ON DELETE SET NULL,

    -- Failure trail.
    attempts                    INT NOT NULL DEFAULT 0,
    error                       TEXT NOT NULL DEFAULT '',
    error_screenshot_asset_id   TEXT REFERENCES pipeline_assets(id) ON DELETE SET NULL,

    started_at                  TIMESTAMPTZ,
    finished_at                 TIMESTAMPTZ,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_upload_records_run ON pipeline_upload_records (run_id);
CREATE INDEX idx_upload_records_project ON pipeline_upload_records (project_id);
CREATE INDEX idx_upload_records_account ON pipeline_upload_records (social_account_id);
CREATE INDEX idx_upload_records_status ON pipeline_upload_records (status);
-- +goose StatementEnd

-- Extend social_accounts with status + scheduling + defaults.
-- +goose StatementBegin
ALTER TABLE social_accounts
    ADD COLUMN status                 TEXT        NOT NULL DEFAULT 'active',
        -- active | needs_relogin | banned | disabled
    ADD COLUMN last_used_at           TIMESTAMPTZ,
    ADD COLUMN cooldown_until         TIMESTAMPTZ,
    ADD COLUMN failure_streak         INT         NOT NULL DEFAULT 0,
    ADD COLUMN default_visibility     TEXT        NOT NULL DEFAULT 'unlisted',
    ADD COLUMN default_made_for_kids  BOOLEAN     NOT NULL DEFAULT false,
    ADD COLUMN default_category_id    TEXT        NOT NULL DEFAULT '22',
    ADD COLUMN default_category_label TEXT        NOT NULL DEFAULT 'People & Blogs';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE pipeline_upload_records;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE social_accounts
    DROP COLUMN status,
    DROP COLUMN last_used_at,
    DROP COLUMN cooldown_until,
    DROP COLUMN failure_streak,
    DROP COLUMN default_visibility,
    DROP COLUMN default_made_for_kids,
    DROP COLUMN default_category_id,
    DROP COLUMN default_category_label;
-- +goose StatementEnd
