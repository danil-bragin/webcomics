//go:build integration

// Full run-lifecycle integration test: drive CreateRun → RecordScript →
// RecordImage (fan-in) → RecordAssemble through the command bus. Workers are
// stubbed: we publish completion commands directly instead of round-tripping
// through Redis + python.
package pipeline_test

import (
	"context"
	"testing"
	"time"

	"github.com/example/dddcqrs/internal/app/bus"
	pipecmd "github.com/example/dddcqrs/internal/app/command/pipeline"
	projcmd "github.com/example/dddcqrs/internal/app/command/projects"
	"github.com/example/dddcqrs/internal/app/middleware"
	"github.com/example/dddcqrs/internal/domain/pipeline"
	"github.com/example/dddcqrs/internal/domain/projects"
	"github.com/example/dddcqrs/internal/infrastructure/persistence/uow"
	"github.com/example/dddcqrs/internal/testhelpers"
)

func newRegistry(t *testing.T, mgr uow.Manager) *bus.Registry {
	t.Helper()
	reg := bus.NewRegistry()
	reg.UseCommandMiddleware(middleware.Recover(), middleware.Validation())
	reg.UseQueryMiddleware(middleware.Recover(), middleware.Validation())
	pipecmd.CreateRunOnBus(reg, mgr)
	pipecmd.RecordScriptCompletedOnBus(reg, mgr)
	pipecmd.RecordImageCompletedOnBus(reg, mgr)
	pipecmd.RecordAssembleCompletedOnBus(reg, mgr)
	pipecmd.RecordAudioCompletedOnBus(reg, mgr)
	pipecmd.RegenerateStepOnBus(reg, mgr)
	pipecmd.RequestAssembleOnBus(reg, mgr)
	pipecmd.CancelRunOnBus(reg, mgr)
	projcmd.CreateProjectOnBus(reg, mgr)
	projcmd.UpsertCharacterOnBus(reg, mgr)
	return reg
}

func loadRun(t *testing.T, mgr uow.Manager, id pipeline.RunID) *pipeline.Run {
	t.Helper()
	var got *pipeline.Run
	if err := mgr.WithinTx(context.Background(), func(ctx context.Context, u uow.UnitOfWork) error {
		r, err := u.Repositories().PipelineRuns().GetByID(ctx, id)
		got = r
		return err
	}); err != nil {
		t.Fatalf("load run: %v", err)
	}
	return got
}

func TestLifecycle_FullPipeline_CompletesEndToEnd(t *testing.T) {
	pool := testhelpers.MustPool(t)
	testhelpers.Wipe(t, pool)
	tplID := testhelpers.SeedDefaultTemplate(t, pool)

	mgr := uow.NewManager(pool)
	reg := newRegistry(t, mgr)
	ctx := context.Background()

	cr, err := bus.Dispatch[pipecmd.CreateRunResult](ctx, reg, pipecmd.CreateRun{
		Prompt:     "lifecycle",
		TemplateID: tplID,
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	// Simulate the script worker completing.
	if _, err := bus.Dispatch[pipecmd.RecordStepResult](ctx, reg, pipecmd.RecordScriptCompleted{
		RunID: cr.RunID, StepIndex: 0,
		ScriptKey: "runs/x/0/script.json",
		Panels: []pipeline.PanelDef{
			{Index: 0, Prompt: "p0"},
			{Index: 1, Prompt: "p1"},
			{Index: 2, Prompt: "p2"},
		},
		Cost: pipeline.CostInfo{TotalCostUSD: 0.001},
	}); err != nil {
		t.Fatalf("RecordScript: %v", err)
	}

	// Simulate the 3 image panels completing.
	for i := range 3 {
		if _, err := bus.Dispatch[pipecmd.RecordStepResult](ctx, reg, pipecmd.RecordImageCompleted{
			RunID: cr.RunID, StepIndex: 1, PanelIndex: i,
			ObjectKey: "runs/x/1/v1/panel-" + itoaTest(i) + ".png",
			Cost:      pipeline.CostInfo{TotalCostUSD: 0.003},
		}); err != nil {
			t.Fatalf("RecordImage(%d): %v", i, err)
		}
	}

	// Assemble step should now be running. Complete it.
	if _, err := bus.Dispatch[pipecmd.RecordStepResult](ctx, reg, pipecmd.RecordAssembleCompleted{
		RunID: cr.RunID, StepIndex: 2,
		ObjectKey: "runs/x/2/v1/video.mp4",
		Cost:      pipeline.CostInfo{TotalCostUSD: 0},
	}); err != nil {
		t.Fatalf("RecordAssemble: %v", err)
	}

	run := loadRun(t, mgr, pipeline.RunID(cr.RunID))
	if run.Status() != pipeline.RunStatusCompleted {
		t.Errorf("status: got %s want completed", run.Status())
	}
	if run.TotalCostUSD() < 0.001 {
		t.Errorf("total cost too low: %f", run.TotalCostUSD())
	}
	for i, s := range run.Steps() {
		if s.ActiveAttempt() == nil || s.ActiveAttempt().Status() != pipeline.AttemptCompleted {
			t.Errorf("step %d active attempt not completed: %+v", i, s.ActiveAttempt())
		}
	}
}

func TestLifecycle_AutoAssembleFalse_PausesUntilRequest(t *testing.T) {
	pool := testhelpers.MustPool(t)
	testhelpers.Wipe(t, pool)
	tplID := testhelpers.SeedDefaultTemplate(t, pool)

	mgr := uow.NewManager(pool)
	reg := newRegistry(t, mgr)
	ctx := context.Background()
	off := false
	cr, err := bus.Dispatch[pipecmd.CreateRunResult](ctx, reg, pipecmd.CreateRun{
		Prompt: "gated", TemplateID: tplID,
		Overrides: &pipecmd.RunOverrides{AutoAssemble: &off},
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	// Script + images complete.
	_, _ = bus.Dispatch[pipecmd.RecordStepResult](ctx, reg, pipecmd.RecordScriptCompleted{
		RunID: cr.RunID, StepIndex: 0, ScriptKey: "k",
		Panels: []pipeline.PanelDef{{Index: 0}, {Index: 1}, {Index: 2}},
		Cost:   pipeline.CostInfo{TotalCostUSD: 0.001},
	})
	for i := range 3 {
		_, _ = bus.Dispatch[pipecmd.RecordStepResult](ctx, reg, pipecmd.RecordImageCompleted{
			RunID: cr.RunID, StepIndex: 1, PanelIndex: i,
			ObjectKey: "k-" + itoaTest(i),
			Cost:      pipeline.CostInfo{TotalCostUSD: 0.003},
		})
	}
	run := loadRun(t, mgr, pipeline.RunID(cr.RunID))
	if run.Status() != pipeline.RunStatusAwaitingAction {
		t.Errorf("expected awaiting_action, got %s", run.Status())
	}

	// User triggers assemble manually.
	if _, err := bus.Dispatch[pipecmd.RequestAssembleResult](ctx, reg, pipecmd.RequestAssemble{
		RunID: cr.RunID, ParamsOverride: map[string]any{"transition": "wipe"},
	}); err != nil {
		t.Fatalf("RequestAssemble: %v", err)
	}
	_, _ = bus.Dispatch[pipecmd.RecordStepResult](ctx, reg, pipecmd.RecordAssembleCompleted{
		RunID: cr.RunID, StepIndex: 2, ObjectKey: "video.mp4",
		Cost: pipeline.CostInfo{TotalCostUSD: 0},
	})
	run = loadRun(t, mgr, pipeline.RunID(cr.RunID))
	if run.Status() != pipeline.RunStatusCompleted {
		t.Errorf("after manual assemble expected completed, got %s", run.Status())
	}
}

func TestLifecycle_FullPipelineThroughCaptionAndUpload(t *testing.T) {
	pool := testhelpers.MustPool(t)
	testhelpers.Wipe(t, pool)
	tplID := testhelpers.SeedDefaultTemplate(t, pool)

	mgr := uow.NewManager(pool)
	reg := newRegistry(t, mgr)
	pipecmd.RecordCaptionCompletedOnBus(reg, mgr)
	pipecmd.RecordUploadCompletedOnBus(reg, mgr)
	projcmd.UpsertSocialAccountOnBus(reg, mgr)
	ctx := context.Background()

	// Seed project + social account.
	pr, _ := bus.Dispatch[projcmd.CreateProjectResult](ctx, reg, projcmd.CreateProject{Name: "P"})
	ar, err := bus.Dispatch[projcmd.UpsertSocialAccountResult](ctx, reg, projcmd.UpsertSocialAccount{
		ProjectID: pr.ID, Platform: "youtube_selenium", Label: "main", FirefoxProfilePath: "/tmp/ff",
	})
	if err != nil {
		t.Fatalf("UpsertSocialAccount: %v", err)
	}

	on := true
	cr, err := bus.Dispatch[pipecmd.CreateRunResult](ctx, reg, pipecmd.CreateRun{
		Prompt:     "shorts test",
		TemplateID: tplID,
		ProjectID:  pr.ID,
		Overrides: &pipecmd.RunOverrides{
			Upload: &pipecmd.UploadOverride{
				Enabled:          &on,
				SocialAccountIDs: []string{ar.ID},
				Platforms:        []string{"youtube"},
				CaptionModel:     "openai/gpt-4o-mini",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	// Walk the run through every step.
	_, _ = bus.Dispatch[pipecmd.RecordStepResult](ctx, reg, pipecmd.RecordScriptCompleted{
		RunID: cr.RunID, StepIndex: 0, ScriptKey: "k.json",
		Panels: []pipeline.PanelDef{{Index: 0}, {Index: 1}, {Index: 2}},
		Cost:   pipeline.CostInfo{TotalCostUSD: 0.001},
	})
	for i := range 3 {
		_, _ = bus.Dispatch[pipecmd.RecordStepResult](ctx, reg, pipecmd.RecordImageCompleted{
			RunID: cr.RunID, StepIndex: 1, PanelIndex: i,
			ObjectKey: "p" + itoaTest(i), Cost: pipeline.CostInfo{TotalCostUSD: 0.003},
		})
	}
	_, _ = bus.Dispatch[pipecmd.RecordStepResult](ctx, reg, pipecmd.RecordAssembleCompleted{
		RunID: cr.RunID, StepIndex: 2, ObjectKey: "video.mp4",
		Cost: pipeline.CostInfo{},
	})

	run := loadRun(t, mgr, pipeline.RunID(cr.RunID))
	if len(run.Steps()) != 5 {
		t.Fatalf("expected 5 steps (script+image+assemble+caption+upload), got %d", len(run.Steps()))
	}
	if run.Steps()[3].Type() != pipeline.StepCaption {
		t.Errorf("step[3] type: %s", run.Steps()[3].Type())
	}
	if run.Steps()[4].Type() != pipeline.StepUpload {
		t.Errorf("step[4] type: %s", run.Steps()[4].Type())
	}

	_, _ = bus.Dispatch[pipecmd.RecordStepResult](ctx, reg, pipecmd.RecordCaptionCompleted{
		RunID: cr.RunID, StepIndex: 3,
		Captions: map[string]any{
			"youtube": map[string]any{"title": "T", "caption": "C", "hashtags": []any{"shorts"}},
		},
		Cost: pipeline.CostInfo{TotalCostUSD: 0.001},
	})

	run = loadRun(t, mgr, pipeline.RunID(cr.RunID))
	upload := run.Steps()[4]
	if upload.ActiveAttempt() == nil {
		t.Fatal("upload step has no attempt after caption advance")
	}

	_, _ = bus.Dispatch[pipecmd.RecordStepResult](ctx, reg, pipecmd.RecordUploadCompleted{
		RunID: cr.RunID, StepIndex: 4, ExternalRef: "https://youtube.com/watch?v=stub",
		Cost: pipeline.CostInfo{},
	})

	run = loadRun(t, mgr, pipeline.RunID(cr.RunID))
	if run.Status() != pipeline.RunStatusCompleted {
		t.Errorf("expected completed after upload, got %s", run.Status())
	}
}

func TestLifecycle_ProjectLinkagePersists(t *testing.T) {
	pool := testhelpers.MustPool(t)
	testhelpers.Wipe(t, pool)
	tplID := testhelpers.SeedDefaultTemplate(t, pool)

	mgr := uow.NewManager(pool)
	reg := newRegistry(t, mgr)
	ctx := context.Background()

	pr, err := bus.Dispatch[projcmd.CreateProjectResult](ctx, reg, projcmd.CreateProject{Name: "P"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	chr, err := bus.Dispatch[projcmd.UpsertCharacterResult](ctx, reg, projcmd.UpsertCharacter{
		ProjectID: pr.ID, Name: "Hero", Description: "tall",
	})
	if err != nil {
		t.Fatalf("UpsertCharacter: %v", err)
	}

	cr, err := bus.Dispatch[pipecmd.CreateRunResult](ctx, reg, pipecmd.CreateRun{
		Prompt:       "linked",
		TemplateID:   tplID,
		ProjectID:    pr.ID,
		CharacterIDs: []string{chr.ID},
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	run := loadRun(t, mgr, pipeline.RunID(cr.RunID))
	if run.ProjectID() != pr.ID {
		t.Errorf("project_id lost: %q", run.ProjectID())
	}
	if len(run.CharacterIDs()) != 1 || run.CharacterIDs()[0] != chr.ID {
		t.Errorf("character_ids: %v", run.CharacterIDs())
	}
}

// helpers ---

func itoaTest(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// silence the unused-time import in case the file shrinks later.
var _ = time.Time{}

// silence imports projects unused warning if compiler ever does so.
var _ = projects.NewProjectID
