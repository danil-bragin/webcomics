//go:build integration

// Persistence integration tests. Run with:
//
//	PG_TEST_DSN=postgres://app:app@localhost:5433/app?sslmode=disable \
//	  go test -tags=integration ./internal/infrastructure/persistence/write/...
package write_test

import (
	"context"
	"testing"

	"github.com/example/dddcqrs/internal/domain/pipeline"
	"github.com/example/dddcqrs/internal/domain/projects"
	"github.com/example/dddcqrs/internal/infrastructure/persistence/uow"
	"github.com/example/dddcqrs/internal/testhelpers"
)

// ----- Run persistence -----

func TestRun_SaveAndReload_RoundTrip(t *testing.T) {
	pool := testhelpers.MustPool(t)
	testhelpers.Wipe(t, pool)
	tplID := testhelpers.SeedDefaultTemplate(t, pool)

	mgr := uow.NewManager(pool)
	ctx := context.Background()

	var runID pipeline.RunID
	if err := mgr.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		tpl, err := u.Repositories().PipelineTemplates().GetByID(ctx, pipeline.TemplateID(tplID))
		if err != nil {
			return err
		}
		run, err := pipeline.NewRunWithOptions("hello", tpl, pipeline.RunOptions{AutoAssemble: false})
		if err != nil {
			return err
		}
		if err := run.Start(); err != nil {
			return err
		}
		runID = run.ID()
		return u.Repositories().PipelineRuns().Save(ctx, run)
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Reload in a fresh tx.
	if err := mgr.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		run, err := u.Repositories().PipelineRuns().GetByID(ctx, runID)
		if err != nil {
			return err
		}
		if run.Prompt() != "hello" {
			t.Errorf("prompt: %q", run.Prompt())
		}
		if run.AutoAssemble() != false {
			t.Errorf("auto_assemble flag lost: %v", run.AutoAssemble())
		}
		if len(run.Steps()) != 3 {
			t.Errorf("steps: %d", len(run.Steps()))
		}
		// First step should have an attempt v1 (script.requested fired).
		script := run.Steps()[0]
		if script.CurrentVersion() != 1 {
			t.Errorf("script currentVersion: %d", script.CurrentVersion())
		}
		if len(script.Attempts()) != 1 {
			t.Errorf("script attempts: %d", len(script.Attempts()))
		}
		return nil
	}); err != nil {
		t.Fatalf("reload: %v", err)
	}
}

func TestRun_RegenerateMarksDownstreamStaleOnReload(t *testing.T) {
	pool := testhelpers.MustPool(t)
	testhelpers.Wipe(t, pool)
	tplID := testhelpers.SeedDefaultTemplate(t, pool)

	mgr := uow.NewManager(pool)
	ctx := context.Background()

	// Create + drive a full run through.
	var runID pipeline.RunID
	_ = mgr.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		tpl, _ := u.Repositories().PipelineTemplates().GetByID(ctx, pipeline.TemplateID(tplID))
		run, _ := pipeline.NewRunWithOptions("hi", tpl, pipeline.RunOptions{AutoAssemble: true})
		_ = run.Start()
		_ = run.RecordScriptCompleted(0, "k.json", []pipeline.PanelDef{{Index: 0}, {Index: 1}, {Index: 2}}, pipeline.CostInfo{}, 100)
		runID = run.ID()
		return u.Repositories().PipelineRuns().Save(ctx, run)
	})

	// Reload + regenerate the script step.
	_ = mgr.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		run, _ := u.Repositories().PipelineRuns().GetByID(ctx, runID)
		if err := run.RegenerateStep(0, map[string]any{"system_prompt": "v2"}); err != nil {
			t.Fatalf("regen: %v", err)
		}
		return u.Repositories().PipelineRuns().Save(ctx, run)
	})

	// Reload — image step should be marked stale.
	_ = mgr.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		run, _ := u.Repositories().PipelineRuns().GetByID(ctx, runID)
		if !run.Steps()[1].IsStale() {
			t.Errorf("expected image step is_stale=true after upstream regen")
		}
		if run.Steps()[0].CurrentVersion() != 2 {
			t.Errorf("script version: %d", run.Steps()[0].CurrentVersion())
		}
		if len(run.Steps()[0].Attempts()) != 2 {
			t.Errorf("expected 2 script attempts, got %d", len(run.Steps()[0].Attempts()))
		}
		return nil
	})
}

// ----- Project persistence -----

func TestProject_SaveLoad_WithDefaultsAndCharacters(t *testing.T) {
	pool := testhelpers.MustPool(t)
	testhelpers.Wipe(t, pool)
	mgr := uow.NewManager(pool)
	ctx := context.Background()

	var pid projects.ProjectID
	_ = mgr.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repo := u.Repositories().Projects()
		p, _ := projects.NewProject("Saga", "test")
		p.SetDefaults(map[string]any{"format_id": "manga", "panel_count": float64(5)})
		if err := repo.SaveProject(ctx, p); err != nil {
			return err
		}
		pid = p.ID()
		c, _ := projects.NewCharacter(pid, "Hero", "tall", map[string]any{"hair": "red"})
		c.AddRefAsset("asset1")
		return repo.SaveCharacter(ctx, c)
	})

	_ = mgr.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repo := u.Repositories().Projects()
		p, err := repo.GetProject(ctx, pid)
		if err != nil {
			t.Fatalf("GetProject: %v", err)
		}
		if p.Name() != "Saga" || p.Description() != "test" {
			t.Errorf("project fields: %+v", p)
		}
		if p.Defaults()["format_id"] != "manga" {
			t.Errorf("defaults format_id missing: %v", p.Defaults())
		}
		chars, err := repo.ListCharacters(ctx, pid)
		if err != nil {
			t.Fatalf("ListCharacters: %v", err)
		}
		if len(chars) != 1 {
			t.Fatalf("characters count: %d", len(chars))
		}
		if chars[0].Name() != "Hero" || chars[0].Traits()["hair"] != "red" {
			t.Errorf("character data: %+v traits=%v", chars[0], chars[0].Traits())
		}
		if len(chars[0].RefAssetIDs()) != 1 || chars[0].RefAssetIDs()[0] != "asset1" {
			t.Errorf("ref assets: %+v", chars[0].RefAssetIDs())
		}
		return nil
	})
}

func TestSocialAccount_RoundTrip(t *testing.T) {
	pool := testhelpers.MustPool(t)
	testhelpers.Wipe(t, pool)
	mgr := uow.NewManager(pool)
	ctx := context.Background()

	var pid projects.ProjectID
	var aid projects.SocialAccountID
	_ = mgr.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repo := u.Repositories().Projects()
		p, _ := projects.NewProject("Saga", "")
		if err := repo.SaveProject(ctx, p); err != nil {
			return err
		}
		pid = p.ID()
		acct, _ := projects.NewSocialAccount(pid, "youtube_selenium", "Main", "/path/to/profile", map[string]any{"region": "us"})
		if err := repo.SaveSocialAccount(ctx, acct); err != nil {
			return err
		}
		aid = acct.ID()
		return nil
	})

	_ = mgr.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repo := u.Repositories().Projects()
		got, err := repo.GetSocialAccount(ctx, aid)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Platform() != "youtube_selenium" || got.Label() != "Main" {
			t.Errorf("fields lost: %+v", got)
		}
		if got.FirefoxProfilePath() != "/path/to/profile" {
			t.Errorf("path: %s", got.FirefoxProfilePath())
		}
		if got.Extra()["region"] != "us" {
			t.Errorf("extra: %v", got.Extra())
		}
		list, err := repo.ListSocialAccounts(ctx, pid)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(list) != 1 {
			t.Errorf("list len: %d", len(list))
		}
		return nil
	})
}

func TestSocialAccount_CascadesOnProjectDelete(t *testing.T) {
	pool := testhelpers.MustPool(t)
	testhelpers.Wipe(t, pool)
	mgr := uow.NewManager(pool)
	ctx := context.Background()

	var pid projects.ProjectID
	var aid projects.SocialAccountID
	_ = mgr.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repo := u.Repositories().Projects()
		p, _ := projects.NewProject("S", "")
		_ = repo.SaveProject(ctx, p)
		pid = p.ID()
		acct, _ := projects.NewSocialAccount(pid, "youtube_selenium", "", "/p", nil)
		_ = repo.SaveSocialAccount(ctx, acct)
		aid = acct.ID()
		return nil
	})

	_ = mgr.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		return u.Repositories().Projects().DeleteProject(ctx, pid)
	})

	_ = mgr.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		_, err := u.Repositories().Projects().GetSocialAccount(ctx, aid)
		if err != projects.ErrSocialAccountNotFound {
			t.Errorf("expected ErrSocialAccountNotFound after cascade, got %v", err)
		}
		return nil
	})
}

func TestSocialAccount_UpdateReplacesFields(t *testing.T) {
	pool := testhelpers.MustPool(t)
	testhelpers.Wipe(t, pool)
	mgr := uow.NewManager(pool)
	ctx := context.Background()

	var pid projects.ProjectID
	var aid projects.SocialAccountID
	_ = mgr.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repo := u.Repositories().Projects()
		p, _ := projects.NewProject("S", "")
		_ = repo.SaveProject(ctx, p)
		pid = p.ID()
		acct, _ := projects.NewSocialAccount(pid, "youtube_selenium", "old", "/p1", nil)
		_ = repo.SaveSocialAccount(ctx, acct)
		aid = acct.ID()
		return nil
	})

	_ = mgr.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repo := u.Repositories().Projects()
		acct, _ := repo.GetSocialAccount(ctx, aid)
		acct.Update("twitter_selenium", "new", "/p2", map[string]any{"k": "v"})
		return repo.SaveSocialAccount(ctx, acct)
	})

	_ = mgr.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		got, _ := u.Repositories().Projects().GetSocialAccount(ctx, aid)
		if got.Platform() != "twitter_selenium" || got.Label() != "new" || got.FirefoxProfilePath() != "/p2" {
			t.Errorf("update failed: %+v", got)
		}
		if got.Extra()["k"] != "v" {
			t.Errorf("extra: %v", got.Extra())
		}
		return nil
	})
}

func TestProject_PlotUpsertReplaces(t *testing.T) {
	pool := testhelpers.MustPool(t)
	testhelpers.Wipe(t, pool)
	mgr := uow.NewManager(pool)
	ctx := context.Background()

	var pid projects.ProjectID
	_ = mgr.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repo := u.Repositories().Projects()
		p, _ := projects.NewProject("S", "")
		if err := repo.SaveProject(ctx, p); err != nil {
			return err
		}
		pid = p.ID()
		plot := projects.NewPlot(pid, "v1", "premise A", []projects.PlotBeat{{Name: "b1", Order: 0}})
		return repo.SavePlot(ctx, plot)
	})

	// Upsert: same project, different content.
	_ = mgr.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repo := u.Repositories().Projects()
		existing, err := repo.GetPlotByProject(ctx, pid)
		if err != nil {
			return err
		}
		existing.Update("v2", "premise B", []projects.PlotBeat{{Name: "X", Order: 0}, {Name: "Y", Order: 1}})
		return repo.SavePlot(ctx, existing)
	})

	_ = mgr.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		plot, err := u.Repositories().Projects().GetPlotByProject(ctx, pid)
		if err != nil {
			t.Fatalf("get plot: %v", err)
		}
		if plot.Name() != "v2" || plot.Premise() != "premise B" {
			t.Errorf("upsert content: %+v", plot)
		}
		if len(plot.Beats()) != 2 {
			t.Errorf("upsert beats count: %d", len(plot.Beats()))
		}
		return nil
	})
}
