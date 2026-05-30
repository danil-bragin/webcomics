-- 00013 — per-upload screenshot trail for the UI debug strip.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE pipeline_upload_records
    ADD COLUMN screenshot_trail JSONB NOT NULL DEFAULT '[]'::jsonb;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE pipeline_upload_records DROP COLUMN screenshot_trail;
-- +goose StatementEnd
