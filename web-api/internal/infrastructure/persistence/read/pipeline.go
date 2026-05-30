package read

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	pipelineq "github.com/example/dddcqrs/internal/app/query/pipeline"
)

// PipelineModel implements pipeline query.ReadModel against the read pool.
type PipelineModel struct{ pool *pgxpool.Pool }

func NewPipelineModel(readPool *pgxpool.Pool) *PipelineModel {
	return &PipelineModel{pool: readPool}
}

const selRunFull = `
SELECT r.id, COALESCE(r.template_id,''), COALESCE(r.project_id,''), COALESCE(p.name,''),
       r.prompt, r.status, r.current_step_index,
       r.expected_steps, r.auto_assemble, r.total_cost_usd, r.max_cost_usd, COALESCE(r.error,''),
       COALESCE(r.language,'en'),
       r.config_snapshot, r.created_at, r.started_at, r.finished_at
FROM pipeline_runs r
LEFT JOIN projects p ON p.id = r.project_id
WHERE r.id = $1`

const selStepsForRun = `
SELECT id, step_index, step_type, current_version, is_stale, COALESCE(active_attempt_id,'')
FROM pipeline_steps
WHERE run_id = $1
ORDER BY step_index ASC`

const selAttemptsForRun = `
SELECT a.id, a.step_id, a.attempt_no, a.status,
       COALESCE(a.input,'null'::jsonb), a.outputs,
       COALESCE(a.params_override,'null'::jsonb),
       COALESCE(a.upstream_versions,'{}'::jsonb),
       COALESCE(a.provider,''), COALESCE(a.model,''),
       a.cost_usd, a.panels_expected, a.panels_completed,
       COALESCE(a.error,''), a.started_at, a.finished_at, a.created_at
FROM pipeline_step_attempts a
JOIN pipeline_steps s ON s.id = a.step_id
WHERE s.run_id = $1
ORDER BY a.step_id, a.attempt_no ASC`

const selAssetsForRun = `
SELECT id, COALESCE(step_id,''), COALESCE(attempt_id,''), kind, bucket, object_key, mime, bytes, created_at
FROM pipeline_assets
WHERE run_id = $1
ORDER BY created_at ASC`

const selCostsForRun = `
SELECT id, COALESCE(step_id,''), COALESCE(attempt_id,''), provider, COALESCE(model,''),
       units, unit_label, unit_cost_usd, total_cost_usd, occurred_at
FROM pipeline_cost_entries
WHERE run_id = $1
ORDER BY occurred_at ASC`

func (m *PipelineModel) GetRun(ctx context.Context, id string) (pipelineq.RunView, error) {
	var v pipelineq.RunView
	var (
		cfgRaw                []byte
		startedAt, finishedAt *time.Time
	)
	err := m.pool.QueryRow(ctx, selRunFull, id).Scan(
		&v.ID, &v.TemplateID, &v.ProjectID, &v.ProjectName,
		&v.Prompt, &v.Status, &v.CurrentStepIndex,
		&v.ExpectedSteps, &v.AutoAssemble, &v.TotalCostUSD, &v.MaxCostUSD, &v.Error,
		&v.Language,
		&cfgRaw, &v.CreatedAt, &startedAt, &finishedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if err != nil {
		return v, err
	}
	v.ConfigSnapshot = json.RawMessage(cfgRaw)
	v.StartedAt = startedAt
	v.FinishedAt = finishedAt

	attemptsByStep, err := m.fetchAttempts(ctx, id)
	if err != nil {
		return v, err
	}

	steps, err := m.fetchSteps(ctx, id, attemptsByStep)
	if err != nil {
		return v, err
	}
	v.Steps = steps

	assets, err := m.fetchAssets(ctx, id)
	if err != nil {
		return v, err
	}
	v.Assets = assets
	costs, err := m.fetchCosts(ctx, id)
	if err != nil {
		return v, err
	}
	v.CostEntries = costs
	return v, nil
}

func (m *PipelineModel) fetchAttempts(ctx context.Context, runID string) (map[string][]pipelineq.AttemptView, error) {
	rows, err := m.pool.Query(ctx, selAttemptsForRun, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]pipelineq.AttemptView{}
	for rows.Next() {
		var a pipelineq.AttemptView
		var input, outputs, paramsOverride, upstream []byte
		var startedAt, finishedAt *time.Time
		if err := rows.Scan(&a.ID, &a.StepID, &a.AttemptNo, &a.Status,
			&input, &outputs, &paramsOverride, &upstream,
			&a.Provider, &a.Model, &a.CostUSD,
			&a.PanelsExpected, &a.PanelsCompleted, &a.Error,
			&startedAt, &finishedAt, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.Input = json.RawMessage(input)
		a.Outputs = json.RawMessage(outputs)
		if len(paramsOverride) > 0 && string(paramsOverride) != "null" {
			a.ParamsOverride = json.RawMessage(paramsOverride)
		}
		if len(upstream) > 0 && string(upstream) != "{}" {
			a.UpstreamVersions = json.RawMessage(upstream)
		}
		a.StartedAt = startedAt
		a.FinishedAt = finishedAt
		out[a.StepID] = append(out[a.StepID], a)
	}
	return out, rows.Err()
}

func (m *PipelineModel) fetchSteps(ctx context.Context, runID string, attemptsByStep map[string][]pipelineq.AttemptView) ([]pipelineq.StepView, error) {
	rows, err := m.pool.Query(ctx, selStepsForRun, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []pipelineq.StepView{}
	for rows.Next() {
		var s pipelineq.StepView
		if err := rows.Scan(&s.ID, &s.Index, &s.Type, &s.CurrentVersion, &s.IsStale, &s.ActiveAttemptID); err != nil {
			return nil, err
		}
		s.Attempts = attemptsByStep[s.ID]
		if active := s.FindActiveAttempt(); active != nil {
			s.Status = active.Status
			s.Input = active.Input
			s.Outputs = active.Outputs
			s.Provider = active.Provider
			s.Model = active.Model
			s.CostUSD = active.CostUSD
			s.PanelsExpected = active.PanelsExpected
			s.PanelsCompleted = active.PanelsCompleted
			s.Error = active.Error
			s.StartedAt = active.StartedAt
			s.FinishedAt = active.FinishedAt
		} else {
			s.Status = "pending"
			s.PanelsExpected = 1
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (m *PipelineModel) fetchAssets(ctx context.Context, runID string) ([]pipelineq.AssetView, error) {
	rows, err := m.pool.Query(ctx, selAssetsForRun, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []pipelineq.AssetView{}
	for rows.Next() {
		var a pipelineq.AssetView
		if err := rows.Scan(&a.ID, &a.StepID, &a.AttemptID, &a.Kind, &a.Bucket, &a.ObjectKey,
			&a.Mime, &a.Bytes, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (m *PipelineModel) fetchCosts(ctx context.Context, runID string) ([]pipelineq.CostEntryView, error) {
	rows, err := m.pool.Query(ctx, selCostsForRun, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []pipelineq.CostEntryView{}
	for rows.Next() {
		var c pipelineq.CostEntryView
		if err := rows.Scan(&c.ID, &c.StepID, &c.AttemptID, &c.Provider, &c.Model,
			&c.Units, &c.UnitLabel, &c.UnitCostUSD, &c.TotalCostUSD,
			&c.OccurredAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (m *PipelineModel) ListRuns(ctx context.Context, f pipelineq.ListRunsFilter) ([]pipelineq.RunSummary, error) {
	q := `SELECT r.id, COALESCE(r.template_id,''), r.prompt, r.status, r.total_cost_usd,
	             r.created_at, r.finished_at, COALESCE(v.id,'') AS video_asset_id
	      FROM pipeline_runs r
	      LEFT JOIN LATERAL (
	          SELECT id FROM pipeline_assets
	          WHERE run_id = r.id AND kind = 'video'
	          ORDER BY created_at DESC LIMIT 1
	      ) v ON true`
	args := []any{f.Limit, f.Offset}
	conds := []string{}
	if len(f.Statuses) > 0 {
		pos := len(args) + 1
		conds = append(conds, fmt.Sprintf("r.status = ANY($%d)", pos))
		args = append(args, f.Statuses)
	}
	if f.Search != "" {
		pos := len(args) + 1
		conds = append(conds, fmt.Sprintf("r.prompt ILIKE $%d", pos))
		args = append(args, "%"+f.Search+"%")
	}
	if f.ProjectID != "" {
		pos := len(args) + 1
		conds = append(conds, fmt.Sprintf("r.project_id = $%d", pos))
		args = append(args, f.ProjectID)
	}
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY r.created_at DESC LIMIT $1 OFFSET $2"
	return m.scanSummaries(ctx, q, args...)
}

func (m *PipelineModel) scanSummaries(ctx context.Context, q string, args ...any) ([]pipelineq.RunSummary, error) {
	rows, err := m.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// Initialize to empty slice so JSON marshals as [] not null when no rows.
	out := []pipelineq.RunSummary{}
	for rows.Next() {
		var s pipelineq.RunSummary
		var finishedAt *time.Time
		if err := rows.Scan(&s.ID, &s.TemplateID, &s.Prompt, &s.Status,
			&s.TotalCostUSD, &s.CreatedAt, &finishedAt, &s.VideoAssetID); err != nil {
			return nil, err
		}
		s.FinishedAt = finishedAt
		out = append(out, s)
	}
	return out, rows.Err()
}

const templateCols = `
SELECT id, name, COALESCE(description,''), COALESCE(category,'custom'), COALESCE(icon,''),
       steps, COALESCE(sample_prompts,'[]'::jsonb), COALESCE(format_id,''),
       COALESCE(defaults,'{}'::jsonb), max_cost_usd, COALESCE(is_test,false),
       created_at, updated_at
FROM pipeline_templates`

func scanTemplate(scan func(...any) error) (pipelineq.TemplateView, error) {
	var t pipelineq.TemplateView
	var stepsRaw, sampleRaw, defaultsRaw []byte
	err := scan(&t.ID, &t.Name, &t.Description, &t.Category, &t.Icon,
		&stepsRaw, &sampleRaw, &t.FormatID, &defaultsRaw, &t.MaxCostUSD, &t.IsTest,
		&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return t, err
	}
	t.Steps = json.RawMessage(stepsRaw)
	t.Defaults = json.RawMessage(defaultsRaw)
	_ = json.Unmarshal(sampleRaw, &t.SamplePrompts)
	if t.SamplePrompts == nil {
		t.SamplePrompts = []string{}
	}
	return t, nil
}

func (m *PipelineModel) GetTemplate(ctx context.Context, id string) (pipelineq.TemplateView, error) {
	row := m.pool.QueryRow(ctx, templateCols+" WHERE id = $1", id)
	t, err := scanTemplate(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

func (m *PipelineModel) ListTemplates(ctx context.Context) ([]pipelineq.TemplateView, error) {
	return m.ListTemplatesFiltered(ctx, pipelineq.TemplateFilter{})
}

func (m *PipelineModel) ListTemplatesFiltered(ctx context.Context, f pipelineq.TemplateFilter) ([]pipelineq.TemplateView, error) {
	q := templateCols
	args := []any{}
	conds := []string{}
	if !f.IncludeTest {
		conds = append(conds, "is_test = false")
	}
	if f.Category != "" {
		args = append(args, f.Category)
		conds = append(conds, "category = $"+itoa(len(args)))
	}
	if len(conds) > 0 {
		q += " WHERE " + joinAnd(conds)
	}
	q += " ORDER BY category, name"
	rows, err := m.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []pipelineq.TemplateView{}
	for rows.Next() {
		t, err := scanTemplate(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func itoa(n int) string {
	if n < 10 {
		return string('0' + byte(n))
	}
	return string('0'+byte(n/10)) + string('0'+byte(n%10))
}

func joinAnd(s []string) string {
	out := s[0]
	for _, x := range s[1:] {
		out += " AND " + x
	}
	return out
}

func (m *PipelineModel) Stats(ctx context.Context) (pipelineq.StatsView, error) {
	var v pipelineq.StatsView
	v.RunsByStatus = map[string]int{}
	v.CostByProvider = []pipelineq.ProviderCost{}
	v.CostByDay = []pipelineq.DayCost{}

	const qRuns = `SELECT status, count(*) FROM pipeline_runs GROUP BY status`
	rows, err := m.pool.Query(ctx, qRuns)
	if err != nil {
		return v, err
	}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			rows.Close()
			return v, err
		}
		v.RunsByStatus[st] = n
	}
	rows.Close()

	const qTotal = `SELECT COALESCE(SUM(total_cost_usd),0) FROM pipeline_runs`
	if err := m.pool.QueryRow(ctx, qTotal).Scan(&v.TotalCostUSD); err != nil {
		return v, err
	}

	const qProv = `
		SELECT provider, COALESCE(SUM(total_cost_usd),0)
		FROM pipeline_cost_entries
		GROUP BY provider
		ORDER BY 2 DESC`
	rows, err = m.pool.Query(ctx, qProv)
	if err != nil {
		return v, err
	}
	for rows.Next() {
		var p pipelineq.ProviderCost
		if err := rows.Scan(&p.Provider, &p.TotalCostUSD); err != nil {
			rows.Close()
			return v, err
		}
		v.CostByProvider = append(v.CostByProvider, p)
	}
	rows.Close()

	const qDay = `
		SELECT to_char(date_trunc('day', occurred_at), 'YYYY-MM-DD') AS d,
		       COALESCE(SUM(total_cost_usd),0)
		FROM pipeline_cost_entries
		WHERE occurred_at >= now() - interval '14 days'
		GROUP BY d
		ORDER BY d ASC`
	rows, err = m.pool.Query(ctx, qDay)
	if err != nil {
		return v, err
	}
	for rows.Next() {
		var d pipelineq.DayCost
		if err := rows.Scan(&d.Date, &d.TotalCostUSD); err != nil {
			rows.Close()
			return v, err
		}
		v.CostByDay = append(v.CostByDay, d)
	}
	rows.Close()
	return v, nil
}

func (m *PipelineModel) GetAssetRef(ctx context.Context, id string) (pipelineq.AssetRef, error) {
	const q = `SELECT id, bucket, object_key, mime FROM pipeline_assets WHERE id = $1`
	var a pipelineq.AssetRef
	err := m.pool.QueryRow(ctx, q, id).Scan(&a.ID, &a.Bucket, &a.ObjectKey, &a.Mime)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}
