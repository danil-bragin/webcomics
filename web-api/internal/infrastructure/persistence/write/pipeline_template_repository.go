package write

import (
	"context"
	"encoding/json"
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
INSERT INTO pipeline_templates
  (id, name, description, category, icon, steps, sample_prompts,
   format_id, defaults, max_cost_usd, is_test, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
ON CONFLICT (id) DO UPDATE SET
  name           = EXCLUDED.name,
  description    = EXCLUDED.description,
  category       = EXCLUDED.category,
  icon           = EXCLUDED.icon,
  steps          = EXCLUDED.steps,
  sample_prompts = EXCLUDED.sample_prompts,
  format_id      = EXCLUDED.format_id,
  defaults       = EXCLUDED.defaults,
  max_cost_usd   = EXCLUDED.max_cost_usd,
  is_test        = EXCLUDED.is_test,
  updated_at     = EXCLUDED.updated_at`

func (r *PipelineTemplateRepository) Save(ctx context.Context, t *pipeline.Template) error {
	stepsJSON, err := t.StepsJSON()
	if err != nil {
		return err
	}
	samplePrompts := t.SamplePrompts()
	if samplePrompts == nil {
		samplePrompts = []string{}
	}
	sampleJSON, err := json.Marshal(samplePrompts)
	if err != nil {
		return err
	}
	defaultsJSON, err := json.Marshal(t.Defaults())
	if err != nil {
		return err
	}
	var formatID any
	if t.FormatID() != "" {
		formatID = t.FormatID()
	}
	_, err = r.tx.Exec(ctx, upsTemplate,
		t.ID().String(), t.Name(), t.Description(), t.Category(), t.Icon(),
		stepsJSON, sampleJSON, formatID, defaultsJSON, t.MaxCostUSD(), t.IsTest(),
		t.CreatedAt(), t.UpdatedAt(),
	)
	return err
}

const selTemplate = `
SELECT id, name, COALESCE(description,''), COALESCE(category,'custom'), COALESCE(icon,''),
       steps, COALESCE(sample_prompts,'[]'::jsonb), COALESCE(format_id,''),
       COALESCE(defaults,'{}'::jsonb), max_cost_usd, COALESCE(is_test,false),
       created_at, updated_at
FROM pipeline_templates WHERE id = $1`

func (r *PipelineTemplateRepository) GetByID(ctx context.Context, id pipeline.TemplateID) (*pipeline.Template, error) {
	var (
		tid, name, description, category, icon, formatID string
		stepsRaw, sampleRaw, defaultsRaw                 []byte
		maxCost                                          float64
		isTest                                           bool
		createdAt, updatedAt                             time.Time
	)
	row := r.tx.QueryRow(ctx, selTemplate, id.String())
	if err := row.Scan(&tid, &name, &description, &category, &icon,
		&stepsRaw, &sampleRaw, &formatID, &defaultsRaw, &maxCost, &isTest,
		&createdAt, &updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pipeline.ErrTemplateNotFound
		}
		return nil, err
	}
	steps, err := pipeline.UnmarshalSteps(stepsRaw)
	if err != nil {
		return nil, err
	}
	var sample []string
	_ = json.Unmarshal(sampleRaw, &sample)
	defaults := map[string]any{}
	_ = json.Unmarshal(defaultsRaw, &defaults)
	return pipeline.ReconstituteTemplateFull(
		pipeline.TemplateID(tid), name, description, category, icon,
		steps, sample, formatID, defaults, maxCost, isTest,
		createdAt, updatedAt,
	), nil
}
