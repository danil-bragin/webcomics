-- +goose Up
-- +goose StatementBegin
-- Templates describe an ordered list of steps a run will execute.
-- `steps` is a jsonb array: [{type, system_prompt?, model?, params}, ...]
CREATE TABLE pipeline_templates (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    steps       JSONB NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
-- A run snapshots its template into `config_snapshot` at creation so later
-- template edits don't change history. `expected_steps` is the length of
-- config_snapshot.steps. `current_step_index` is the 0-based index of the
-- step that has been requested but not yet completed (or = expected_steps when done).
CREATE TABLE pipeline_runs (
    id                  TEXT PRIMARY KEY,
    template_id         TEXT REFERENCES pipeline_templates(id),
    prompt              TEXT NOT NULL,
    config_snapshot     JSONB NOT NULL,
    status              TEXT NOT NULL DEFAULT 'queued',
    current_step_index  INT  NOT NULL DEFAULT 0,
    expected_steps      INT  NOT NULL,
    total_cost_usd      NUMERIC(12,6) NOT NULL DEFAULT 0,
    error               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at          TIMESTAMPTZ,
    finished_at         TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_pipeline_runs_created_at ON pipeline_runs (created_at DESC);
CREATE INDEX idx_pipeline_runs_status ON pipeline_runs (status);
-- +goose StatementEnd

-- +goose StatementBegin
-- One row per step in the run's pipeline. For image fan-out steps,
-- panels_expected > 1 and panels_completed tracks how many have finished.
-- For single-output steps (script, assemble), panels_expected = 1.
CREATE TABLE pipeline_steps (
    id                TEXT PRIMARY KEY,
    run_id            TEXT NOT NULL REFERENCES pipeline_runs(id) ON DELETE CASCADE,
    step_index        INT  NOT NULL,
    step_type         TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'pending',
    input             JSONB,
    outputs           JSONB NOT NULL DEFAULT '[]'::jsonb,
    provider          TEXT,
    model             TEXT,
    cost_usd          NUMERIC(12,6) NOT NULL DEFAULT 0,
    panels_expected   INT  NOT NULL DEFAULT 1,
    panels_completed  INT  NOT NULL DEFAULT 0,
    error             TEXT,
    started_at        TIMESTAMPTZ,
    finished_at       TIMESTAMPTZ,
    UNIQUE (run_id, step_index)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_pipeline_steps_run ON pipeline_steps (run_id, step_index);
-- +goose StatementEnd

-- +goose StatementBegin
-- Binary artifacts (script.json, panel images, video) live in MinIO;
-- this table is just metadata + key. Look up presigned URL via key.
CREATE TABLE pipeline_assets (
    id          TEXT PRIMARY KEY,
    run_id      TEXT NOT NULL REFERENCES pipeline_runs(id) ON DELETE CASCADE,
    step_id     TEXT REFERENCES pipeline_steps(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    bucket      TEXT NOT NULL,
    object_key  TEXT NOT NULL,
    mime        TEXT NOT NULL,
    bytes       BIGINT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (bucket, object_key)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_pipeline_assets_run ON pipeline_assets (run_id);
-- +goose StatementEnd

-- +goose StatementBegin
-- One row per provider invocation. Sums to pipeline_runs.total_cost_usd.
CREATE TABLE pipeline_cost_entries (
    id               TEXT PRIMARY KEY,
    run_id           TEXT NOT NULL REFERENCES pipeline_runs(id) ON DELETE CASCADE,
    step_id          TEXT REFERENCES pipeline_steps(id) ON DELETE CASCADE,
    provider         TEXT NOT NULL,
    model            TEXT,
    units            NUMERIC(14,4) NOT NULL DEFAULT 0,
    unit_label       TEXT NOT NULL DEFAULT 'units',
    unit_cost_usd    NUMERIC(12,8) NOT NULL DEFAULT 0,
    total_cost_usd   NUMERIC(12,6) NOT NULL DEFAULT 0,
    occurred_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_pipeline_cost_entries_run ON pipeline_cost_entries (run_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE pipeline_cost_entries;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE pipeline_assets;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE pipeline_steps;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE pipeline_runs;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE pipeline_templates;
-- +goose StatementEnd
