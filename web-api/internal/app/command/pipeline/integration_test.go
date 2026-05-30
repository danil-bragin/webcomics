//go:build integration

// Integration tests exercise the command handlers against a real Postgres
// (via pgx + the production UoW). Run with:
//
//	PG_TEST_DSN=postgres://app:app@localhost:5433/app?sslmode=disable \
//	  go test -tags=integration ./internal/app/command/pipeline/...
//
// Requires `make migrate-up` to have been run first against the same DSN.
package pipeline

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/dddcqrs/internal/domain/pipeline"
	"github.com/example/dddcqrs/internal/infrastructure/persistence/uow"
)

func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("PG_TEST_DSN")
	if dsn == "" {
		t.Skip("PG_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func wipe(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	const sql = `TRUNCATE
		pipeline_cost_entries,
		pipeline_assets,
		pipeline_step_attempts,
		pipeline_steps,
		pipeline_runs,
		plots,
		characters,
		environments,
		projects,
		outbox,
		processed_messages
		RESTART IDENTITY CASCADE`
	if _, err := pool.Exec(context.Background(), sql); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	// Also collect old test-fixture templates so they don't accumulate across
	// runs and pollute the Studio dropdown. is_test=true rows are hidden from
	// the marketplace anyway, but cleanup keeps the table small.
	if _, err := pool.Exec(context.Background(), `DELETE FROM pipeline_templates WHERE is_test = TRUE`); err != nil {
		t.Fatalf("delete test templates: %v", err)
	}
}

func ensureTemplate(t *testing.T, mgr uow.Manager) pipeline.TemplateID {
	t.Helper()
	steps := []pipeline.StepConfig{
		{Type: pipeline.StepScript, Params: map[string]any{"panel_count": 3}},
		{Type: pipeline.StepImage},
		{Type: pipeline.StepAssemble},
	}
	tpl, err := pipeline.NewTemplate("integration-3panel", steps)
	if err != nil {
		t.Fatalf("NewTemplate: %v", err)
	}
	tpl.SetIsTest(true) // hidden from marketplace + cleaned on next wipe()
	if err := mgr.WithinTx(context.Background(), func(ctx context.Context, u uow.UnitOfWork) error {
		return u.Repositories().PipelineTemplates().Save(ctx, tpl)
	}); err != nil {
		t.Fatalf("save template: %v", err)
	}
	return tpl.ID()
}

func TestCreateRun_WritesRunStepsAndOutbox(t *testing.T) {
	pool := newPool(t)
	wipe(t, pool)
	mgr := uow.NewManager(pool)
	tplID := ensureTemplate(t, mgr)

	h := NewCreateRunHandler(mgr)
	res, err := h.Handle(context.Background(), CreateRun{Prompt: "a cat", TemplateID: tplID.String()})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	var status string
	var expected int
	if err := pool.QueryRow(context.Background(),
		`SELECT status, expected_steps FROM pipeline_runs WHERE id=$1`, res.RunID,
	).Scan(&status, &expected); err != nil {
		t.Fatalf("read run: %v", err)
	}
	if status != "running" {
		t.Fatalf("status: want running got %s", status)
	}
	if expected != 3 {
		t.Fatalf("expected_steps: want 3 got %d", expected)
	}

	var stepCount int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM pipeline_steps WHERE run_id=$1`, res.RunID,
	).Scan(&stepCount); err != nil {
		t.Fatalf("count steps: %v", err)
	}
	if stepCount != 3 {
		t.Fatalf("steps: want 3 got %d", stepCount)
	}

	var outboxCount int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM outbox WHERE aggregate_id=$1 AND event_name='pipeline.script.requested'`, res.RunID,
	).Scan(&outboxCount); err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	if outboxCount != 1 {
		t.Fatalf("outbox script.requested: want 1 got %d", outboxCount)
	}
}

func TestRecordScriptCompleted_AdvancesAndFansOut(t *testing.T) {
	pool := newPool(t)
	wipe(t, pool)
	mgr := uow.NewManager(pool)
	tplID := ensureTemplate(t, mgr)

	create := NewCreateRunHandler(mgr)
	cr, err := create.Handle(context.Background(), CreateRun{Prompt: "cat", TemplateID: tplID.String()})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	rsc := NewRecordScriptCompletedHandler(mgr)
	cost := pipeline.CostInfo{Provider: "or", Model: "gpt-4o-mini", TotalCostUSD: 0.01}
	if _, err := rsc.Handle(context.Background(), RecordScriptCompleted{
		RunID: cr.RunID, StepIndex: 0,
		ScriptKey: "k", Panels: []pipeline.PanelDef{
			{Index: 0, Prompt: "p1"}, {Index: 1, Prompt: "p2"}, {Index: 2, Prompt: "p3"},
		},
		Cost: cost, DurationMs: 10,
	}); err != nil {
		t.Fatalf("RecordScriptCompleted: %v", err)
	}

	var imageReqs int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM outbox WHERE aggregate_id=$1 AND event_name='pipeline.image.requested'`, cr.RunID,
	).Scan(&imageReqs); err != nil {
		t.Fatalf("count image reqs: %v", err)
	}
	if imageReqs != 3 {
		t.Fatalf("image.requested: want 3 got %d", imageReqs)
	}

	var costRows int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM pipeline_cost_entries WHERE run_id=$1`, cr.RunID,
	).Scan(&costRows); err != nil {
		t.Fatalf("count cost rows: %v", err)
	}
	if costRows != 1 {
		t.Fatalf("cost_entries: want 1 got %d", costRows)
	}
}

func TestRecordImageCompleted_ConcurrentFanInIsSerialized(t *testing.T) {
	// Verifies that FOR UPDATE in the run repo prevents the race where 3
	// concurrent RecordImageCompleted commands each read panels_completed=0
	// and all save 1 — the bug we hit during the e2e debugging.
	pool := newPool(t)
	wipe(t, pool)
	mgr := uow.NewManager(pool)
	tplID := ensureTemplate(t, mgr)

	create := NewCreateRunHandler(mgr)
	cr, _ := create.Handle(context.Background(), CreateRun{Prompt: "x", TemplateID: tplID.String()})

	rsc := NewRecordScriptCompletedHandler(mgr)
	_, err := rsc.Handle(context.Background(), RecordScriptCompleted{
		RunID: cr.RunID, StepIndex: 0,
		ScriptKey: "k", Panels: []pipeline.PanelDef{
			{Index: 0, Prompt: "p1"}, {Index: 1, Prompt: "p2"}, {Index: 2, Prompt: "p3"},
		},
		Cost: pipeline.CostInfo{Provider: "or"},
	})
	if err != nil {
		t.Fatalf("script: %v", err)
	}

	ric := NewRecordImageCompletedHandler(mgr)
	var wg sync.WaitGroup
	errs := make(chan error, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			_, err := ric.Handle(context.Background(), RecordImageCompleted{
				RunID: cr.RunID, StepIndex: 1, PanelIndex: i,
				ObjectKey: "k" + string(rune('0'+i)),
				Cost:      pipeline.CostInfo{Provider: "fal", TotalCostUSD: 0.003},
			})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatalf("concurrent: %v", e)
		}
	}

	var pc int
	if err := pool.QueryRow(context.Background(),
		`SELECT a.panels_completed
		   FROM pipeline_steps s
		   JOIN pipeline_step_attempts a ON a.id = s.active_attempt_id
		  WHERE s.run_id = $1 AND s.step_index = 1`, cr.RunID,
	).Scan(&pc); err != nil {
		t.Fatalf("read step: %v", err)
	}
	if pc != 3 {
		t.Fatalf("panels_completed after 3 concurrent dispatches: want 3 got %d", pc)
	}

	var assembleReqs int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM outbox WHERE aggregate_id=$1 AND event_name='pipeline.assemble.requested'`, cr.RunID,
	).Scan(&assembleReqs); err != nil {
		t.Fatalf("count assemble reqs: %v", err)
	}
	if assembleReqs != 1 {
		t.Fatalf("assemble.requested: want 1 got %d", assembleReqs)
	}
}
