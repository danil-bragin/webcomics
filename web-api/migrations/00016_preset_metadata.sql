-- Presets (renamed concept from Templates in API/UI; DB table name stays).
-- Adds discoverability fields so users can pick a starting point without
-- reading raw step JSON.
--
-- - description: paragraph explaining what the preset produces and who it's for
-- - category: meme | shorts | story | demo | custom — drives marketplace tabs
-- - icon: single emoji shown on preset cards
-- - sample_prompts: array of ready-made prompts the user can click to populate
--   the Studio prompt field
-- - format_id: optional default render aspect (square|portrait|landscape) —
--   pulled from formats library
-- - defaults: JSON bag of project defaults (voice/music/ambient/style/system)
--   so a preset can be a one-click setup
-- - is_test: hides spec/integration/e2e test rows from the marketplace
--
-- Also wipes the polluted rows created by integration tests (50+ duplicates of
-- "integration-3panel" / "e2e-1panel" / "spec-test" / "metrics-test" that
-- accumulated in the UI dropdown).

-- +goose Up

-- +goose StatementBegin
ALTER TABLE pipeline_templates
    ADD COLUMN description    TEXT NOT NULL DEFAULT '',
    ADD COLUMN category       TEXT NOT NULL DEFAULT 'custom',
    ADD COLUMN icon           TEXT NOT NULL DEFAULT '',
    ADD COLUMN sample_prompts JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN format_id      TEXT,
    ADD COLUMN defaults       JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN is_test        BOOLEAN NOT NULL DEFAULT FALSE;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE pipeline_templates
    ADD CONSTRAINT pipeline_templates_category_chk
    CHECK (category IN ('meme','shorts','story','demo','custom'));
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_pipeline_templates_category ON pipeline_templates (category);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_pipeline_templates_is_test ON pipeline_templates (is_test) WHERE is_test IS TRUE;
-- +goose StatementEnd

-- Mark all current test-pollution rows so listing endpoints can exclude them
-- without breaking existing runs that still reference them by id.
-- +goose StatementBegin
UPDATE pipeline_templates
SET is_test = TRUE
WHERE name IN ('integration-3panel','e2e-1panel','spec-test','metrics-test','default-test');
-- +goose StatementEnd

-- Hard delete the test-pollution rows that have NO referencing runs. Runs that
-- still reference them get to keep them (foreign key + history preservation).
-- +goose StatementBegin
DELETE FROM pipeline_templates t
WHERE t.is_test = TRUE
  AND NOT EXISTS (SELECT 1 FROM pipeline_runs r WHERE r.template_id = t.id);
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP INDEX IF EXISTS idx_pipeline_templates_is_test;
-- +goose StatementEnd

-- +goose StatementBegin
DROP INDEX IF EXISTS idx_pipeline_templates_category;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE pipeline_templates DROP CONSTRAINT IF EXISTS pipeline_templates_category_chk;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE pipeline_templates
    DROP COLUMN IF EXISTS is_test,
    DROP COLUMN IF EXISTS defaults,
    DROP COLUMN IF EXISTS format_id,
    DROP COLUMN IF EXISTS sample_prompts,
    DROP COLUMN IF EXISTS icon,
    DROP COLUMN IF EXISTS category,
    DROP COLUMN IF EXISTS description;
-- +goose StatementEnd
