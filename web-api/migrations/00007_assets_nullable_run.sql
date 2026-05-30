-- Allow standalone assets that aren't owned by a run. Character / environment
-- reference uploads are global to a project, not a run.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE pipeline_assets ALTER COLUMN run_id DROP NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE pipeline_assets ALTER COLUMN run_id SET NOT NULL;
-- +goose StatementEnd
