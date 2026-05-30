-- Branch-style step versioning.
--
-- Before: pipeline_steps was the unit of execution — it held status, inputs,
-- outputs, cost and timestamps. There was one row per (run_id, step_index)
-- and a re-run meant overwriting it.
--
-- After: pipeline_steps becomes a slot (run_id, step_index, step_type) that
-- points at the *current* attempt and tracks staleness. Every execution —
-- the initial one and every regenerate — lands in pipeline_step_attempts.
-- Assets and cost entries hang off attempt_id so history is preserved when
-- the user iterates.
--
-- Stale flag means an upstream attempt changed after this step last ran and
-- its outputs no longer match. Set by Run.RegenerateStep on N+1..M.

-- +goose Up

-- +goose StatementBegin
CREATE TABLE pipeline_step_attempts (
    id                  TEXT PRIMARY KEY,
    step_id             TEXT NOT NULL REFERENCES pipeline_steps(id) ON DELETE CASCADE,
    attempt_no          INT  NOT NULL,
    status              TEXT NOT NULL DEFAULT 'pending',
    input               JSONB,
    outputs             JSONB NOT NULL DEFAULT '[]'::jsonb,
    params_override     JSONB,
    upstream_versions   JSONB NOT NULL DEFAULT '{}'::jsonb,
    provider            TEXT,
    model               TEXT,
    cost_usd            NUMERIC(12,6) NOT NULL DEFAULT 0,
    panels_expected     INT  NOT NULL DEFAULT 1,
    panels_completed    INT  NOT NULL DEFAULT 0,
    error               TEXT,
    started_at          TIMESTAMPTZ,
    finished_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (step_id, attempt_no)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_pipeline_step_attempts_step ON pipeline_step_attempts (step_id, attempt_no);
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE pipeline_steps
    ADD COLUMN active_attempt_id TEXT REFERENCES pipeline_step_attempts(id),
    ADD COLUMN current_version   INT  NOT NULL DEFAULT 1,
    ADD COLUMN is_stale          BOOLEAN NOT NULL DEFAULT FALSE;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE pipeline_runs
    ADD COLUMN auto_assemble BOOLEAN NOT NULL DEFAULT TRUE;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE pipeline_assets
    ADD COLUMN attempt_id TEXT REFERENCES pipeline_step_attempts(id) ON DELETE CASCADE;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE pipeline_cost_entries
    ADD COLUMN attempt_id TEXT REFERENCES pipeline_step_attempts(id) ON DELETE CASCADE;
-- +goose StatementEnd

-- Backfill: every existing pipeline_steps row becomes attempt_no=1.
-- +goose StatementBegin
INSERT INTO pipeline_step_attempts
    (id, step_id, attempt_no, status, input, outputs, provider, model,
     cost_usd, panels_expected, panels_completed, error, started_at, finished_at, created_at)
SELECT
    s.id || '-a1', s.id, 1, s.status, s.input, s.outputs, s.provider, s.model,
    s.cost_usd, s.panels_expected, s.panels_completed, s.error, s.started_at, s.finished_at,
    COALESCE(s.started_at, now())
FROM pipeline_steps s;
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE pipeline_steps SET active_attempt_id = id || '-a1', current_version = 1;
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE pipeline_assets a
SET attempt_id = (SELECT id FROM pipeline_step_attempts WHERE step_id = a.step_id ORDER BY attempt_no DESC LIMIT 1)
WHERE step_id IS NOT NULL;
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE pipeline_cost_entries c
SET attempt_id = (SELECT id FROM pipeline_step_attempts WHERE step_id = c.step_id ORDER BY attempt_no DESC LIMIT 1)
WHERE step_id IS NOT NULL;
-- +goose StatementEnd

-- Drop columns that moved to pipeline_step_attempts. Keep step_index, step_type,
-- run_id — those are slot identity, not per-attempt.
-- +goose StatementBegin
ALTER TABLE pipeline_steps
    DROP COLUMN status,
    DROP COLUMN input,
    DROP COLUMN outputs,
    DROP COLUMN provider,
    DROP COLUMN model,
    DROP COLUMN cost_usd,
    DROP COLUMN panels_expected,
    DROP COLUMN panels_completed,
    DROP COLUMN error,
    DROP COLUMN started_at,
    DROP COLUMN finished_at;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE pipeline_steps
    ADD COLUMN status            TEXT NOT NULL DEFAULT 'pending',
    ADD COLUMN input             JSONB,
    ADD COLUMN outputs           JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN provider          TEXT,
    ADD COLUMN model             TEXT,
    ADD COLUMN cost_usd          NUMERIC(12,6) NOT NULL DEFAULT 0,
    ADD COLUMN panels_expected   INT  NOT NULL DEFAULT 1,
    ADD COLUMN panels_completed  INT  NOT NULL DEFAULT 0,
    ADD COLUMN error             TEXT,
    ADD COLUMN started_at        TIMESTAMPTZ,
    ADD COLUMN finished_at       TIMESTAMPTZ;
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE pipeline_steps s SET
    status = a.status, input = a.input, outputs = a.outputs,
    provider = a.provider, model = a.model, cost_usd = a.cost_usd,
    panels_expected = a.panels_expected, panels_completed = a.panels_completed,
    error = a.error, started_at = a.started_at, finished_at = a.finished_at
FROM pipeline_step_attempts a
WHERE a.id = s.active_attempt_id;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE pipeline_cost_entries DROP COLUMN attempt_id;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE pipeline_assets DROP COLUMN attempt_id;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE pipeline_runs DROP COLUMN auto_assemble;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE pipeline_steps
    DROP COLUMN active_attempt_id,
    DROP COLUMN current_version,
    DROP COLUMN is_stale;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE pipeline_step_attempts;
-- +goose StatementEnd
