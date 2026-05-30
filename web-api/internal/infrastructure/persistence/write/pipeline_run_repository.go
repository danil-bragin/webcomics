package write

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/example/dddcqrs/internal/domain/pipeline"
)

// PipelineRunRepository persists Run aggregates + their steps + step attempts,
// plus the assets and cost entries the aggregate accumulated since load.
type PipelineRunRepository struct{ tx pgx.Tx }

func NewPipelineRunRepository(tx pgx.Tx) *PipelineRunRepository {
	return &PipelineRunRepository{tx: tx}
}

const insRun = `
INSERT INTO pipeline_runs
  (id, template_id, prompt, config_snapshot, auto_assemble, status,
   current_step_index, expected_steps, total_cost_usd, max_cost_usd, error,
   created_at, started_at, finished_at,
   project_id, character_ids, environment_ids, plot_id,
   require_review_before_upload, language)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
ON CONFLICT (id) DO UPDATE SET
  status = EXCLUDED.status,
  current_step_index = EXCLUDED.current_step_index,
  total_cost_usd = EXCLUDED.total_cost_usd,
  error = EXCLUDED.error,
  started_at = EXCLUDED.started_at,
  finished_at = EXCLUDED.finished_at,
  language = EXCLUDED.language`

// upsStepShell creates the step slot WITHOUT setting active_attempt_id, because
// the attempt row may not exist yet at this point. upsStepActive runs after
// the attempts have been persisted and links the slot to the active attempt.
const upsStepShell = `
INSERT INTO pipeline_steps
  (id, run_id, step_index, step_type, current_version, is_stale)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (run_id, step_index) DO UPDATE SET
  current_version = EXCLUDED.current_version,
  is_stale = EXCLUDED.is_stale`

const upsStepActive = `
UPDATE pipeline_steps SET active_attempt_id = $2 WHERE id = $1`

const upsAttempt = `
INSERT INTO pipeline_step_attempts
  (id, step_id, attempt_no, status, input, outputs, params_override,
   upstream_versions, provider, model, cost_usd,
   panels_expected, panels_completed, error, started_at, finished_at, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
ON CONFLICT (step_id, attempt_no) DO UPDATE SET
  status = EXCLUDED.status,
  outputs = EXCLUDED.outputs,
  provider = EXCLUDED.provider,
  model = EXCLUDED.model,
  cost_usd = EXCLUDED.cost_usd,
  panels_completed = EXCLUDED.panels_completed,
  error = EXCLUDED.error,
  finished_at = EXCLUDED.finished_at`

const insAsset = `
INSERT INTO pipeline_assets
  (id, run_id, step_id, attempt_id, kind, bucket, object_key, mime, bytes, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (bucket, object_key) DO NOTHING`

const insCost = `
INSERT INTO pipeline_cost_entries
  (id, run_id, step_id, attempt_id, provider, model, units, unit_label,
   unit_cost_usd, total_cost_usd, occurred_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`

func (r *PipelineRunRepository) Save(ctx context.Context, run *pipeline.Run) error {
	cfgJSON, err := json.Marshal(run.ConfigSnapshot())
	if err != nil {
		return err
	}
	charIDs := run.CharacterIDs()
	if charIDs == nil {
		charIDs = []string{}
	}
	envIDs := run.EnvironmentIDs()
	if envIDs == nil {
		envIDs = []string{}
	}
	if _, err := r.tx.Exec(ctx, insRun,
		run.ID().String(),
		nullString(run.TemplateID().String()),
		run.Prompt(),
		cfgJSON,
		run.AutoAssemble(),
		string(run.Status()),
		run.CurrentStepIndex(),
		run.ExpectedSteps(),
		run.TotalCostUSD(),
		run.MaxCostUSD(),
		nullString(run.Error()),
		run.CreatedAt(),
		run.StartedAt(),
		run.FinishedAt(),
		nullString(run.ProjectID()),
		charIDs,
		envIDs,
		nullString(run.PlotID()),
		run.RequireReviewBeforeUpload(),
		run.Language(),
	); err != nil {
		return err
	}

	// Two-phase: insert step slots without active_attempt_id, insert attempts,
	// then link slot → active attempt. Step.active_attempt_id has a FK on
	// pipeline_step_attempts(id) and the attempts haven't been written yet.
	for _, s := range run.Steps() {
		if _, err := r.tx.Exec(ctx, upsStepShell,
			s.ID().String(),
			run.ID().String(),
			s.Index(),
			string(s.Type()),
			s.CurrentVersion(),
			s.IsStale(),
		); err != nil {
			return err
		}
		for _, a := range s.Attempts() {
			if _, err := r.tx.Exec(ctx, upsAttempt,
				a.ID().String(),
				s.ID().String(),
				a.AttemptNo(),
				string(a.Status()),
				emptyJSONIfNil(a.Input()),
				emptyJSONIfNil(a.Outputs()),
				emptyJSONIfNil(a.ParamsOverride()),
				marshalUpstream(a.UpstreamVersions()),
				a.Provider(),
				a.Model(),
				a.CostUSD(),
				a.PanelsExpected(),
				a.PanelsCompleted(),
				nullString(a.Error()),
				a.StartedAt(),
				a.FinishedAt(),
				a.CreatedAt(),
			); err != nil {
				return err
			}
		}
		activeID := s.ActiveAttemptID().String()
		var activeAny any
		if activeID != "" {
			activeAny = activeID
		}
		if _, err := r.tx.Exec(ctx, upsStepActive, s.ID().String(), activeAny); err != nil {
			return err
		}
	}

	for _, a := range run.NewAssets() {
		if _, err := r.tx.Exec(ctx, insAsset,
			a.ID.String(),
			a.RunID.String(),
			nullString(a.StepID.String()),
			nullString(a.AttemptID.String()),
			string(a.Kind),
			a.Bucket,
			a.ObjectKey,
			a.Mime,
			a.Bytes,
			a.CreatedAt,
		); err != nil {
			return err
		}
	}
	for _, c := range run.NewCosts() {
		if _, err := r.tx.Exec(ctx, insCost,
			c.ID,
			c.RunID.String(),
			nullString(c.StepID.String()),
			nullString(c.AttemptID.String()),
			c.Provider,
			c.Model,
			c.Units,
			c.UnitLabel,
			c.UnitCostUSD,
			c.TotalCostUSD,
			c.OccurredAt,
		); err != nil {
			return err
		}
	}
	run.ResetSideEffects()
	return nil
}

// SELECT FOR UPDATE locks the run row for the lifetime of the UoW transaction.
// Required because image step completions arrive concurrently (one per panel)
// and each one mutates the active attempt's panels_completed counter.
const selRun = `
SELECT id, template_id, prompt, config_snapshot, auto_assemble, status,
       current_step_index, expected_steps, total_cost_usd, max_cost_usd, error,
       created_at, started_at, finished_at,
       project_id, character_ids, environment_ids, plot_id,
       COALESCE(require_review_before_upload, false),
       COALESCE(language, 'en')
FROM pipeline_runs WHERE id = $1 FOR UPDATE`

const selSteps = `
SELECT id, step_index, step_type, current_version, is_stale, COALESCE(active_attempt_id,'')
FROM pipeline_steps
WHERE run_id = $1
ORDER BY step_index ASC`

const selAttempts = `
SELECT a.id, a.step_id, a.attempt_no, a.status, a.input, a.outputs,
       a.params_override, a.upstream_versions, COALESCE(a.provider,''),
       COALESCE(a.model,''), a.cost_usd, a.panels_expected, a.panels_completed,
       COALESCE(a.error,''), a.started_at, a.finished_at, a.created_at
FROM pipeline_step_attempts a
JOIN pipeline_steps s ON s.id = a.step_id
WHERE s.run_id = $1
ORDER BY a.step_id, a.attempt_no ASC`

func (r *PipelineRunRepository) GetByID(ctx context.Context, id pipeline.RunID) (*pipeline.Run, error) {
	var (
		rid, prompt, status       string
		templateID                *string
		cfgRaw                    []byte
		autoAssemble              bool
		currentIdx, expectedSteps int
		totalCost, maxCost        float64
		errMsg                    *string
		createdAt                 time.Time
		startedAt, finishedAt     *time.Time
		projectID, plotID         *string
		charIDs, envIDs           []string
		requireReviewBeforeUpload bool
		language                  string
	)
	row := r.tx.QueryRow(ctx, selRun, id.String())
	if err := row.Scan(&rid, &templateID, &prompt, &cfgRaw, &autoAssemble, &status,
		&currentIdx, &expectedSteps, &totalCost, &maxCost, &errMsg,
		&createdAt, &startedAt, &finishedAt,
		&projectID, &charIDs, &envIDs, &plotID,
		&requireReviewBeforeUpload, &language); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pipeline.ErrRunNotFound
		}
		return nil, err
	}
	cfg, err := pipeline.UnmarshalSteps(cfgRaw)
	if err != nil {
		return nil, err
	}

	attemptsByStep, err := r.loadAttempts(ctx, id.String())
	if err != nil {
		return nil, err
	}

	rows, err := r.tx.Query(ctx, selSteps, id.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var steps []*pipeline.Step
	for rows.Next() {
		var (
			stID, sType, activeID string
			sIdx, currentVersion  int
			isStale               bool
		)
		if err := rows.Scan(&stID, &sIdx, &sType, &currentVersion, &isStale, &activeID); err != nil {
			return nil, err
		}
		steps = append(steps, pipeline.ReconstituteStep(
			pipeline.StepID(stID), sIdx, pipeline.StepType(sType),
			currentVersion, isStale, pipeline.AttemptID(activeID),
			attemptsByStep[stID],
		))
	}
	tplID := pipeline.TemplateID("")
	if templateID != nil {
		tplID = pipeline.TemplateID(*templateID)
	}
	errStr := ""
	if errMsg != nil {
		errStr = *errMsg
	}
	run := pipeline.ReconstituteRunFull(
		pipeline.RunID(rid), tplID, prompt, cfg, autoAssemble,
		pipeline.RunStatus(status),
		currentIdx, totalCost, maxCost, errStr,
		createdAt, startedAt, finishedAt,
		steps,
	)
	pidStr := ""
	if projectID != nil {
		pidStr = *projectID
	}
	plStr := ""
	if plotID != nil {
		plStr = *plotID
	}
	pipeline.AttachLinkage(run, pidStr, charIDs, envIDs, plStr)
	run.SetRequireReviewBeforeUpload(requireReviewBeforeUpload)
	run.SetLanguage(language)
	return run, nil
}

func (r *PipelineRunRepository) loadAttempts(ctx context.Context, runID string) (map[string][]*pipeline.StepAttempt, error) {
	rows, err := r.tx.Query(ctx, selAttempts, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]*pipeline.StepAttempt{}
	for rows.Next() {
		var (
			id, stepID, status, provider, model, errStr string
			attemptNo                                   int
			input, outputs, paramsOverride              []byte
			upstreamRaw                                 []byte
			costUSD                                     float64
			panelsExpected, panelsCompleted             int
			startedAt, finishedAt                       *time.Time
			createdAt                                   time.Time
		)
		if err := rows.Scan(&id, &stepID, &attemptNo, &status, &input, &outputs,
			&paramsOverride, &upstreamRaw, &provider, &model, &costUSD,
			&panelsExpected, &panelsCompleted, &errStr,
			&startedAt, &finishedAt, &createdAt); err != nil {
			return nil, err
		}
		upstream := map[int]int{}
		if len(upstreamRaw) > 0 {
			var raw map[string]int
			if err := json.Unmarshal(upstreamRaw, &raw); err == nil {
				for k, v := range raw {
					upstream[atoiSafe(k)] = v
				}
			}
		}
		a := pipeline.ReconstituteAttempt(
			pipeline.AttemptID(id), pipeline.StepID(stepID), attemptNo,
			pipeline.AttemptStatus(status),
			input, outputs, paramsOverride, upstream,
			provider, model, costUSD,
			panelsExpected, panelsCompleted, errStr,
			startedAt, finishedAt, createdAt,
		)
		out[stepID] = append(out[stepID], a)
	}
	return out, rows.Err()
}

// GetAssetObjectKeys returns id → object_key for the requested asset IDs.
// Missing IDs are omitted. Bulk-fetched in one query.
func (r *PipelineRunRepository) GetAssetObjectKeys(ctx context.Context, ids []string) (map[string]string, error) {
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	const q = `SELECT id, object_key FROM pipeline_assets WHERE id = ANY($1)`
	rows, err := r.tx.Query(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, key string
		if err := rows.Scan(&id, &key); err != nil {
			return nil, err
		}
		out[id] = key
	}
	return out, rows.Err()
}

func (r *PipelineRunRepository) DeleteOlderThan(ctx context.Context, days int, statuses []string) (int, error) {
	const q = `DELETE FROM pipeline_runs
		WHERE status = ANY($1) AND created_at < now() - ($2 || ' days')::interval`
	tag, err := r.tx.Exec(ctx, q, statuses, days)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func emptyJSONIfNil(b []byte) []byte {
	if len(b) == 0 {
		return []byte("null")
	}
	return b
}

func marshalUpstream(m map[int]int) []byte {
	if len(m) == 0 {
		return []byte("{}")
	}
	out := map[string]int{}
	for k, v := range m {
		out[itoa(k)] = v
	}
	b, _ := json.Marshal(out)
	return b
}

func itoa(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = digits[i%10]
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

func atoiSafe(s string) int {
	n := 0
	neg := false
	for i, c := range s {
		if i == 0 && c == '-' {
			neg = true
			continue
		}
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	if neg {
		return -n
	}
	return n
}
