package write

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/example/dddcqrs/internal/domain/pipeline"
)

type PipelineTemplateRepository struct{ tx pgx.Tx }

func NewPipelineTemplateRepository(tx pgx.Tx) *PipelineTemplateRepository {
	return &PipelineTemplateRepository{tx: tx}
}

const upsTemplate = `
INSERT INTO pipeline_templates (id, name, steps, max_cost_usd, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name,
  steps = EXCLUDED.steps,
  max_cost_usd = EXCLUDED.max_cost_usd,
  updated_at = EXCLUDED.updated_at`

func (r *PipelineTemplateRepository) Save(ctx context.Context, t *pipeline.Template) error {
	stepsJSON, err := t.StepsJSON()
	if err != nil {
		return err
	}
	_, err = r.tx.Exec(ctx, upsTemplate,
		t.ID().String(), t.Name(), stepsJSON, t.MaxCostUSD(), t.CreatedAt(), t.UpdatedAt(),
	)
	return err
}

const selTemplate = `
SELECT id, name, steps, max_cost_usd, created_at, updated_at
FROM pipeline_templates WHERE id = $1`

func (r *PipelineTemplateRepository) GetByID(ctx context.Context, id pipeline.TemplateID) (*pipeline.Template, error) {
	var (
		tid, name            string
		stepsRaw             []byte
		maxCost              float64
		createdAt, updatedAt time.Time
	)
	row := r.tx.QueryRow(ctx, selTemplate, id.String())
	if err := row.Scan(&tid, &name, &stepsRaw, &maxCost, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pipeline.ErrTemplateNotFound
		}
		return nil, err
	}
	steps, err := pipeline.UnmarshalSteps(stepsRaw)
	if err != nil {
		return nil, err
	}
	return pipeline.ReconstituteTemplateWithCap(
		pipeline.TemplateID(tid), name, steps, maxCost, createdAt, updatedAt,
	), nil
}
