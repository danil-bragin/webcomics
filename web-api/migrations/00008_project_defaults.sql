-- Project-level defaults that auto-fill the Studio override form. Stored as
-- a free-form jsonb map so we can add new tunables without a schema bump.
--
-- Schema (all keys optional):
--   panel_count, target_duration_ms, enable_audio, auto_assemble,
--   script_model, system_prompt, image_model, style_reference,
--   audio: {voice_id, model, speed},
--   assemble: {fps, width, height, codec}

-- +goose Up
-- +goose StatementBegin
ALTER TABLE projects ADD COLUMN defaults JSONB NOT NULL DEFAULT '{}'::jsonb;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE projects DROP COLUMN defaults;
-- +goose StatementEnd
