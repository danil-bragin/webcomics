-- Content language per run (captions + voice + social copy). Image prompts
-- stay English regardless — diffusion models train on English captions and
-- localised prompts hurt quality.
--
-- Default 'en' for legacy rows. Project defaults may override per project.

-- +goose Up

-- +goose StatementBegin
ALTER TABLE pipeline_runs ADD COLUMN language TEXT NOT NULL DEFAULT 'en';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE pipeline_runs
  ADD CONSTRAINT pipeline_runs_language_chk CHECK (language IN ('en','ru','fr'));
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE pipeline_runs DROP CONSTRAINT IF EXISTS pipeline_runs_language_chk;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE pipeline_runs DROP COLUMN IF EXISTS language;
-- +goose StatementEnd
