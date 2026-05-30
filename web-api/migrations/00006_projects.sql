-- Projects + character / environment / plot builders.
--
-- A project loosely groups runs. Characters and environments are reusable
-- prompt + ref-image bundles. A plot captures recurring story state. Everything
-- here is OPTIONAL on a run — runs without project_id keep working unchanged.
--
-- ref_asset_ids points at pipeline_assets rows so the same image can power
-- both a character (style anchor) and the run that produced it.

-- +goose Up

-- +goose StatementBegin
CREATE TABLE projects (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    archived    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_projects_updated_at ON projects (updated_at DESC);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE characters (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    traits          JSONB NOT NULL DEFAULT '{}'::jsonb,
    ref_asset_ids   TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_characters_project ON characters (project_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE environments (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    traits          JSONB NOT NULL DEFAULT '{}'::jsonb,
    ref_asset_ids   TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_environments_project ON environments (project_id);
-- +goose StatementEnd

-- +goose StatementBegin
-- Single plot per project for the MVP. `beats` is a jsonb array of arc points
-- like [{name, description, order}, ...]. premise is the long-form setup.
CREATE TABLE plots (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name        TEXT NOT NULL DEFAULT 'Main',
    premise     TEXT NOT NULL DEFAULT '',
    beats       JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id)
);
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE pipeline_runs
    ADD COLUMN project_id      TEXT REFERENCES projects(id) ON DELETE SET NULL,
    ADD COLUMN character_ids   TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    ADD COLUMN environment_ids TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    ADD COLUMN plot_id         TEXT REFERENCES plots(id) ON DELETE SET NULL;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_pipeline_runs_project ON pipeline_runs (project_id);
-- +goose StatementEnd

-- Mark assets attached to characters / environments / plots so we can filter
-- them out of run dashboards and clean them up independently.
-- +goose StatementBegin
DO $$ BEGIN
    -- These new kinds are stored as plain TEXT in pipeline_assets.kind; no enum
    -- to alter. Adding rows for documentation only.
    INSERT INTO pipeline_templates (id, name, steps, max_cost_usd)
    VALUES ('asset-kinds-comment', 'asset_kinds_doc',
            '[]'::jsonb, 0)
    ON CONFLICT (id) DO NOTHING;
    DELETE FROM pipeline_templates WHERE id = 'asset-kinds-comment';
END $$;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE pipeline_runs
    DROP COLUMN project_id,
    DROP COLUMN character_ids,
    DROP COLUMN environment_ids,
    DROP COLUMN plot_id;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE plots;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE environments;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE characters;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE projects;
-- +goose StatementEnd
