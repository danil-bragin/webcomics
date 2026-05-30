-- +goose Up
-- +goose StatementBegin
ALTER TABLE pipeline_templates ADD COLUMN max_cost_usd NUMERIC(12,6) NOT NULL DEFAULT 0;
ALTER TABLE pipeline_runs ADD COLUMN max_cost_usd NUMERIC(12,6) NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE pipeline_runs DROP COLUMN max_cost_usd;
ALTER TABLE pipeline_templates DROP COLUMN max_cost_usd;
-- +goose StatementEnd
