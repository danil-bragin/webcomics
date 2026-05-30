-- 00012 — review gate flag on pipeline_runs.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE pipeline_runs
    ADD COLUMN require_review_before_upload BOOLEAN NOT NULL DEFAULT false;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE pipeline_runs DROP COLUMN require_review_before_upload;
-- +goose StatementEnd
