package pipeline

import (
	"strings"
	"testing"

	"github.com/example/dddcqrs/internal/domain/shared"
)

func newTestTemplate(t *testing.T, panelCount int) *Template {
	t.Helper()
	steps := []StepConfig{
		{Type: StepScript, Model: "gpt-4o-mini", Params: map[string]any{"panel_count": panelCount}},
		{Type: StepImage, Model: "fal/flux-schnell"},
		{Type: StepAssemble, Params: map[string]any{"width": 1080, "height": 1080, "fps": 30}},
	}
	tpl, err := NewTemplate("test", steps)
	if err != nil {
		t.Fatalf("NewTemplate: %v", err)
	}
	return tpl
}

func pullEventNames(events []shared.DomainEvent) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		out = append(out, e.EventName())
	}
	return out
}

func TestNewRun_RejectsEmptyPrompt(t *testing.T) {
	tpl := newTestTemplate(t, 3)
	if _, err := NewRun("", tpl); err != ErrRunPromptEmpty {
		t.Fatalf("expected ErrRunPromptEmpty, got %v", err)
	}
}

func TestNewRun_RejectsNilTemplate(t *testing.T) {
	if _, err := NewRun("hello", nil); err != ErrTemplateNotFound {
		t.Fatalf("expected ErrTemplateNotFound, got %v", err)
	}
}

func TestNewRun_RejectsNonScriptFirstStep(t *testing.T) {
	tpl, err := NewTemplate("bad", []StepConfig{{Type: StepImage}})
	if err != nil {
		t.Fatalf("NewTemplate: %v", err)
	}
	if _, err := NewRun("hi", tpl); err != ErrFirstStepMustBeScript {
		t.Fatalf("expected ErrFirstStepMustBeScript, got %v", err)
	}
}

func TestStart_EmitsScriptRequested(t *testing.T) {
	tpl := newTestTemplate(t, 3)
	run, err := NewRun("a cat", tpl)
	if err != nil {
		t.Fatalf("NewRun: %v", err)
	}
	if got := run.Status(); got != RunStatusQueued {
		t.Fatalf("status: want queued got %s", got)
	}
	if err := run.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := run.Status(); got != RunStatusRunning {
		t.Fatalf("status: want running got %s", got)
	}
	names := pullEventNames(run.PullEvents())
	if len(names) != 1 || names[0] != "pipeline.script.requested" {
		t.Fatalf("events: want [pipeline.script.requested] got %v", names)
	}
}

func TestStart_RejectsDoubleStart(t *testing.T) {
	tpl := newTestTemplate(t, 1)
	run, _ := NewRun("x", tpl)
	if err := run.Start(); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	if err := run.Start(); err != ErrRunNotQueued {
		t.Fatalf("expected ErrRunNotQueued on second Start, got %v", err)
	}
}

func TestRecordScriptCompleted_AdvancesAndFansOut(t *testing.T) {
	tpl := newTestTemplate(t, 3)
	run, _ := NewRun("x", tpl)
	_ = run.Start()
	_ = run.PullEvents() // discard initial events

	panels := []PanelDef{
		{Index: 0, Prompt: "p1"},
		{Index: 1, Prompt: "p2"},
		{Index: 2, Prompt: "p3"},
	}
	cost := CostInfo{Provider: "openrouter", Model: "x", Units: 100, UnitLabel: "tokens", UnitCostUSD: 0.0001, TotalCostUSD: 0.01}
	if err := run.RecordScriptCompleted(0, "runs/x/0/script.json", panels, cost, 50); err != nil {
		t.Fatalf("RecordScriptCompleted: %v", err)
	}
	if got := run.Steps()[0].Status(); got != StepCompleted {
		t.Fatalf("script step status: %s", got)
	}
	if run.TotalCostUSD() != 0.01 {
		t.Fatalf("total cost: want 0.01 got %v", run.TotalCostUSD())
	}
	if got := run.Steps()[1].Status(); got != StepRunning {
		t.Fatalf("image step status: %s", got)
	}
	if got := run.Steps()[1].PanelsExpected(); got != 3 {
		t.Fatalf("panels_expected: want 3 got %d", got)
	}
	names := pullEventNames(run.PullEvents())
	if len(names) != 3 {
		t.Fatalf("want 3 image.requested events got %d (%v)", len(names), names)
	}
	for _, n := range names {
		if n != "pipeline.image.requested" {
			t.Fatalf("unexpected event %s", n)
		}
	}
	if len(run.NewAssets()) != 1 {
		t.Fatalf("want 1 new asset (script), got %d", len(run.NewAssets()))
	}
}

func TestRecordImageCompleted_FanInAndAdvance(t *testing.T) {
	tpl := newTestTemplate(t, 3)
	run, _ := NewRun("x", tpl)
	_ = run.Start()
	_ = run.PullEvents()

	panels := []PanelDef{{Index: 0, Prompt: "a"}, {Index: 1, Prompt: "b"}, {Index: 2, Prompt: "c"}}
	cost := CostInfo{Provider: "or", Units: 1, UnitLabel: "tokens", TotalCostUSD: 0.001}
	_ = run.RecordScriptCompleted(0, "k", panels, cost, 10)
	_ = run.PullEvents()
	run.ResetSideEffects()

	imgCost := CostInfo{Provider: "fal", Model: "flux", Units: 1, UnitLabel: "images", UnitCostUSD: 0.003, TotalCostUSD: 0.003}

	// First two panels: step remains running, no new outbox events.
	if err := run.RecordImageCompleted(1, 0, "runs/x/1/0.png", imgCost, 100, 0); err != nil {
		t.Fatalf("panel 0: %v", err)
	}
	if got := run.Steps()[1].Status(); got != StepRunning {
		t.Fatalf("after panel 0 status: %s", got)
	}
	if events := pullEventNames(run.PullEvents()); len(events) != 0 {
		t.Fatalf("after panel 0: want no events, got %v", events)
	}
	if err := run.RecordImageCompleted(1, 1, "runs/x/1/1.png", imgCost, 100, 0); err != nil {
		t.Fatalf("panel 1: %v", err)
	}
	if got := run.Steps()[1].Status(); got != StepRunning {
		t.Fatalf("after panel 1 status: %s", got)
	}

	// Third panel completes the step and emits assemble.requested.
	if err := run.RecordImageCompleted(1, 2, "runs/x/1/2.png", imgCost, 100, 0); err != nil {
		t.Fatalf("panel 2: %v", err)
	}
	if got := run.Steps()[1].Status(); got != StepCompleted {
		t.Fatalf("after panel 2 status: %s", got)
	}
	if got := run.Steps()[1].PanelsCompleted(); got != 3 {
		t.Fatalf("panels_completed: want 3 got %d", got)
	}
	names := pullEventNames(run.PullEvents())
	if len(names) != 1 || names[0] != "pipeline.assemble.requested" {
		t.Fatalf("want [pipeline.assemble.requested] got %v", names)
	}
	if len(run.NewAssets()) != 3 {
		t.Fatalf("want 3 panel assets, got %d", len(run.NewAssets()))
	}
	// Cost: 1×0.001 script + 3×0.003 image = 0.010
	if want, got := 0.010, run.TotalCostUSD(); !floatEq(want, got) {
		t.Fatalf("total cost: want %v got %v", want, got)
	}
}

func TestRecordImageCompleted_IdempotentSamePanelKey(t *testing.T) {
	tpl := newTestTemplate(t, 2)
	run, _ := NewRun("x", tpl)
	_ = run.Start()
	_ = run.PullEvents()
	_ = run.RecordScriptCompleted(0, "k", []PanelDef{{Index: 0, Prompt: "a"}, {Index: 1, Prompt: "b"}}, CostInfo{}, 0)
	_ = run.PullEvents()

	imgCost := CostInfo{Provider: "fal", TotalCostUSD: 0.003}
	_ = run.RecordImageCompleted(1, 0, "k0", imgCost, 0, 0)
	// Same panel + same key — should be a no-op.
	_ = run.RecordImageCompleted(1, 0, "k0", imgCost, 0, 0)
	if got := run.Steps()[1].PanelsCompleted(); got != 1 {
		t.Fatalf("panels_completed: want 1 (idempotent), got %d", got)
	}
}

func TestRecordAssembleCompleted_FinalizesRun(t *testing.T) {
	tpl := newTestTemplate(t, 1)
	run, _ := NewRun("x", tpl)
	_ = run.Start()
	_ = run.PullEvents()
	_ = run.RecordScriptCompleted(0, "s", []PanelDef{{Index: 0, Prompt: "p"}}, CostInfo{Provider: "or", TotalCostUSD: 0.001}, 0)
	_ = run.PullEvents()
	_ = run.RecordImageCompleted(1, 0, "img", CostInfo{Provider: "fal", TotalCostUSD: 0.003}, 0, 0)
	_ = run.PullEvents()

	if err := run.RecordAssembleCompleted(2, "runs/x/2/video.mp4", CostInfo{Provider: "local"}, 200); err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if got := run.Status(); got != RunStatusCompleted {
		t.Fatalf("run status: want completed got %s", got)
	}
	if run.FinishedAt() == nil {
		t.Fatalf("finished_at not set")
	}
	names := pullEventNames(run.PullEvents())
	if len(names) != 1 || names[0] != "pipeline.run.completed" {
		t.Fatalf("want [pipeline.run.completed] got %v", names)
	}
}

func TestRecordStepFailed_FailsRun(t *testing.T) {
	tpl := newTestTemplate(t, 3)
	run, _ := NewRun("x", tpl)
	_ = run.Start()
	_ = run.PullEvents()
	if err := run.RecordStepFailed(0, "openrouter rate-limited"); err != nil {
		t.Fatalf("RecordStepFailed: %v", err)
	}
	if got := run.Status(); got != RunStatusFailed {
		t.Fatalf("status: want failed got %s", got)
	}
	if !strings.Contains(run.Error(), "rate-limited") {
		t.Fatalf("error: want substring, got %q", run.Error())
	}
	names := pullEventNames(run.PullEvents())
	if len(names) != 1 || names[0] != "pipeline.run.failed" {
		t.Fatalf("want [pipeline.run.failed] got %v", names)
	}
}

func TestCancel_FromQueued(t *testing.T) {
	tpl := newTestTemplate(t, 1)
	run, _ := NewRun("x", tpl)
	if err := run.Cancel(); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if got := run.Status(); got != RunStatusCancelled {
		t.Fatalf("status: want cancelled got %s", got)
	}
}

func TestCancel_FromRunning(t *testing.T) {
	tpl := newTestTemplate(t, 1)
	run, _ := NewRun("x", tpl)
	_ = run.Start()
	if err := run.Cancel(); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if got := run.Status(); got != RunStatusCancelled {
		t.Fatalf("status: want cancelled got %s", got)
	}
}

func TestCancel_RejectsTerminal(t *testing.T) {
	tpl := newTestTemplate(t, 1)
	run, _ := NewRun("x", tpl)
	_ = run.Start()
	_ = run.PullEvents()
	_ = run.RecordScriptCompleted(0, "s", []PanelDef{{Index: 0, Prompt: "p"}}, CostInfo{}, 0)
	_ = run.PullEvents()
	_ = run.RecordImageCompleted(1, 0, "k", CostInfo{}, 0, 0)
	_ = run.PullEvents()
	_ = run.RecordAssembleCompleted(2, "v", CostInfo{}, 0)
	if err := run.Cancel(); err == nil {
		t.Fatalf("expected error cancelling a completed run")
	}
}

func TestRecordScriptCompleted_RejectsWrongStep(t *testing.T) {
	tpl := newTestTemplate(t, 1)
	run, _ := NewRun("x", tpl)
	_ = run.Start()
	if err := run.RecordScriptCompleted(2, "k", []PanelDef{{Prompt: "p"}}, CostInfo{}, 0); err != ErrStepTypeMismatch {
		t.Fatalf("want ErrStepTypeMismatch, got %v", err)
	}
}

func TestRecordImageCompleted_RejectsUnknownPanel(t *testing.T) {
	tpl := newTestTemplate(t, 1)
	run, _ := NewRun("x", tpl)
	_ = run.Start()
	_ = run.RecordScriptCompleted(0, "k", []PanelDef{{Index: 0, Prompt: "p"}}, CostInfo{}, 0)
	if err := run.RecordImageCompleted(1, 42, "x", CostInfo{}, 0, 0); err != ErrUnknownPanel {
		t.Fatalf("want ErrUnknownPanel, got %v", err)
	}
}

func TestExtensibility_ScriptImageAudioAssemble(t *testing.T) {
	// Proves a new step type slots in without touching orchestrator code:
	// 4-step template (script → image → audio → assemble) advances correctly,
	// and the assemble step's request carries the audio key.
	steps := []StepConfig{
		{Type: StepScript, Params: map[string]any{"panel_count": 1}},
		{Type: StepImage},
		{Type: StepAudio, Model: "fal-ai/tts"},
		{Type: StepAssemble, Params: map[string]any{"fps": 30, "width": 1080, "height": 1080}},
	}
	tpl, err := NewTemplate("4step", steps)
	if err != nil {
		t.Fatalf("NewTemplate: %v", err)
	}
	run, _ := NewRun("hi", tpl)
	_ = run.Start()
	_ = run.PullEvents()

	// script
	_ = run.RecordScriptCompleted(0, "k", []PanelDef{{Index: 0, Prompt: "p", Caption: "hi"}}, CostInfo{Provider: "or", TotalCostUSD: 0.001}, 0)
	if got := pullEventNames(run.PullEvents()); len(got) != 1 || got[0] != "pipeline.image.requested" {
		t.Fatalf("after script want image.requested, got %v", got)
	}

	// image
	_ = run.RecordImageCompleted(1, 0, "img", CostInfo{Provider: "fal", TotalCostUSD: 0.003}, 0, 0)
	got := pullEventNames(run.PullEvents())
	if len(got) != 1 || got[0] != "pipeline.audio.requested" {
		t.Fatalf("after image want audio.requested, got %v", got)
	}

	// audio
	_ = run.RecordAudioCompleted(2, "runs/x/2/audio.mp3", CostInfo{Provider: "fal", Model: "tts", TotalCostUSD: 0.003}, 0)
	got = pullEventNames(run.PullEvents())
	if len(got) != 1 || got[0] != "pipeline.assemble.requested" {
		t.Fatalf("after audio want assemble.requested, got %v", got)
	}
	// The assemble step's recorded input should carry the audio_key.
	assembleStep := run.Steps()[3]
	if !contains(string(assembleStep.Input()), "runs/x/2/audio.mp3") {
		t.Fatalf("assemble input missing audio_key: %s", string(assembleStep.Input()))
	}

	// assemble
	_ = run.RecordAssembleCompleted(3, "vid", CostInfo{Provider: "local"}, 0)
	if run.Status() != RunStatusCompleted {
		t.Fatalf("run not completed: %s", run.Status())
	}
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

func TestCancel_EmitsRunCancelled(t *testing.T) {
	tpl := newTestTemplate(t, 1)
	run, _ := NewRun("x", tpl)
	_ = run.Start()
	_ = run.PullEvents()
	if err := run.Cancel(); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	names := pullEventNames(run.PullEvents())
	if len(names) != 1 || names[0] != "pipeline.run.cancelled" {
		t.Fatalf("want [pipeline.run.cancelled], got %v", names)
	}
}

func TestRecordAudioCompleted_Standalone(t *testing.T) {
	steps := []StepConfig{
		{Type: StepScript, Params: map[string]any{"panel_count": 1}},
		{Type: StepImage},
		{Type: StepAudio, Model: "fal-ai/tts"},
		{Type: StepAssemble},
	}
	tpl, err := NewTemplate("audio", steps)
	if err != nil {
		t.Fatalf("NewTemplate: %v", err)
	}
	run, _ := NewRun("x", tpl)
	_ = run.Start()
	_ = run.PullEvents()
	_ = run.RecordScriptCompleted(0, "k", []PanelDef{{Index: 0, Prompt: "p", Caption: "c"}}, CostInfo{Provider: "or"}, 0)
	_ = run.PullEvents()
	_ = run.RecordImageCompleted(1, 0, "img", CostInfo{Provider: "fal"}, 0, 0)
	_ = run.PullEvents()

	// Audio step now in flight. Pre-state: pending → running on requestStep.
	if got := run.Steps()[2].Status(); got != StepRunning {
		t.Fatalf("audio step status: %s", got)
	}

	if err := run.RecordAudioCompleted(2, "runs/x/2/audio.mp3", CostInfo{Provider: "fal", Model: "tts", TotalCostUSD: 0.001}, 250); err != nil {
		t.Fatalf("RecordAudioCompleted: %v", err)
	}
	if got := run.Steps()[2].Status(); got != StepCompleted {
		t.Fatalf("audio status after record: %s", got)
	}
	got := pullEventNames(run.PullEvents())
	if len(got) != 1 || got[0] != "pipeline.assemble.requested" {
		t.Fatalf("after audio want assemble.requested, got %v", got)
	}
}

func TestUploadStep_PassesAssembleVideoKey(t *testing.T) {
	steps := []StepConfig{
		{Type: StepScript, Params: map[string]any{"panel_count": 1}},
		{Type: StepImage},
		{Type: StepAssemble},
		{Type: StepUpload, Provider: "telegram", Params: map[string]any{"chat_id": "12345"}},
	}
	tpl, err := NewTemplate("upload", steps)
	if err != nil {
		t.Fatalf("NewTemplate: %v", err)
	}
	run, _ := NewRun("x", tpl)
	_ = run.Start()
	_ = run.PullEvents()
	_ = run.RecordScriptCompleted(0, "k", []PanelDef{{Index: 0, Prompt: "p"}}, CostInfo{Provider: "or"}, 0)
	_ = run.PullEvents()
	_ = run.RecordImageCompleted(1, 0, "img", CostInfo{Provider: "fal"}, 0, 0)
	_ = run.PullEvents()

	// After assemble completes, upload step is requested with the video key.
	const videoKey = "runs/x/2/video.mp4"
	if err := run.RecordAssembleCompleted(2, videoKey, CostInfo{Provider: "local"}, 0); err != nil {
		t.Fatalf("assemble: %v", err)
	}
	got := pullEventNames(run.PullEvents())
	if len(got) != 1 || got[0] != "pipeline.upload.requested" {
		t.Fatalf("after assemble want upload.requested, got %v", got)
	}
	upStep := run.Steps()[3]
	if !contains(string(upStep.Input()), videoKey) {
		t.Fatalf("upload input missing video_key: %s", string(upStep.Input()))
	}
	if !contains(string(upStep.Input()), "telegram") {
		t.Fatalf("upload input missing provider: %s", string(upStep.Input()))
	}

	if err := run.RecordUploadCompleted(3, "tg://12345/42", CostInfo{Provider: "telegram"}, 0); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if run.Status() != RunStatusCompleted {
		t.Fatalf("run not completed: %s", run.Status())
	}
}

func TestMusicStep_PassesMusicKeyToAssemble(t *testing.T) {
	// 5-step template proving music slots in alongside audio.
	steps := []StepConfig{
		{Type: StepScript, Params: map[string]any{"panel_count": 1}},
		{Type: StepImage},
		{Type: StepMusic, Model: "stable-audio"},
		{Type: StepAssemble},
	}
	tpl, err := NewTemplate("music", steps)
	if err != nil {
		t.Fatalf("NewTemplate: %v", err)
	}
	run, _ := NewRun("a vibey cat", tpl)
	_ = run.Start()
	_ = run.PullEvents()
	_ = run.RecordScriptCompleted(0, "k", []PanelDef{{Index: 0, Prompt: "p"}}, CostInfo{Provider: "or"}, 0)
	_ = run.PullEvents()
	_ = run.RecordImageCompleted(1, 0, "img", CostInfo{Provider: "fal"}, 0, 0)
	got := pullEventNames(run.PullEvents())
	if len(got) != 1 || got[0] != "pipeline.music.requested" {
		t.Fatalf("after image want music.requested, got %v", got)
	}

	const musicKey = "runs/x/2/music.mp3"
	if err := run.RecordMusicCompleted(2, musicKey, CostInfo{Provider: "stable", TotalCostUSD: 0.05}, 0); err != nil {
		t.Fatalf("music: %v", err)
	}
	got = pullEventNames(run.PullEvents())
	if len(got) != 1 || got[0] != "pipeline.assemble.requested" {
		t.Fatalf("after music want assemble.requested, got %v", got)
	}
	asm := run.Steps()[3]
	if !contains(string(asm.Input()), musicKey) {
		t.Fatalf("assemble input missing music_key: %s", string(asm.Input()))
	}
}

func TestUploadStep_FailsIfNoAssembleBefore(t *testing.T) {
	// Upload before assemble is meaningless — the aggregate rejects it.
	steps := []StepConfig{
		{Type: StepScript, Params: map[string]any{"panel_count": 1}},
		{Type: StepUpload, Provider: "telegram"},
	}
	tpl, err := NewTemplate("bad-upload", steps)
	if err != nil {
		t.Fatalf("NewTemplate: %v", err)
	}
	run, _ := NewRun("x", tpl)
	_ = run.Start()
	_ = run.PullEvents()
	err = run.RecordScriptCompleted(0, "k", []PanelDef{{Index: 0, Prompt: "p"}}, CostInfo{Provider: "or"}, 0)
	if err == nil {
		t.Fatalf("expected error advancing into upload step without an assemble in front")
	}
}

func TestCostCap_FailsRunWhenExceeded(t *testing.T) {
	// Cap of $0.005. Script costs $0.001, three image panels at $0.003 each = $0.010 total.
	// After the FIRST image panel completes ($0.001 + $0.003 = $0.004) we're still under cap.
	// After the SECOND ($0.007) we cross the cap → run fails before any further work.
	steps := []StepConfig{
		{Type: StepScript, Params: map[string]any{"panel_count": 3}},
		{Type: StepImage},
		{Type: StepAssemble},
	}
	tpl, err := NewTemplateWithCap("cheap", steps, 0.005)
	if err != nil {
		t.Fatalf("NewTemplate: %v", err)
	}
	run, _ := NewRun("x", tpl)
	_ = run.Start()
	_ = run.PullEvents()

	panels := []PanelDef{{Index: 0, Prompt: "a"}, {Index: 1, Prompt: "b"}, {Index: 2, Prompt: "c"}}
	_ = run.RecordScriptCompleted(0, "k", panels, CostInfo{Provider: "or", TotalCostUSD: 0.001}, 0)
	_ = run.PullEvents()
	// First panel: under cap.
	_ = run.RecordImageCompleted(1, 0, "k0", CostInfo{Provider: "fal", TotalCostUSD: 0.003}, 0, 0)
	if run.Status() != RunStatusRunning {
		t.Fatalf("after panel 0 status: %s", run.Status())
	}
	// Second panel: crosses cap. advance() should mark run failed + emit run.failed.
	_ = run.RecordImageCompleted(1, 1, "k1", CostInfo{Provider: "fal", TotalCostUSD: 0.003}, 0, 0)
	// 2nd panel still doesn't trigger advance() because all panels not done. Trigger by completing 3rd.
	_ = run.RecordImageCompleted(1, 2, "k2", CostInfo{Provider: "fal", TotalCostUSD: 0.003}, 0, 0)
	if run.Status() != RunStatusFailed {
		t.Fatalf("after cost cap exceeded want failed, got %s", run.Status())
	}
	if !contains(run.Error(), "cost cap") {
		t.Fatalf("expected cost-cap error, got %q", run.Error())
	}
	names := pullEventNames(run.PullEvents())
	if len(names) != 1 || names[0] != "pipeline.run.failed" {
		t.Fatalf("want run.failed, got %v", names)
	}
}

func TestCostCap_UnlimitedByDefault(t *testing.T) {
	tpl := newTestTemplate(t, 1)
	if tpl.MaxCostUSD() != 0 {
		t.Fatalf("default cap should be 0 (unlimited), got %v", tpl.MaxCostUSD())
	}
	run, _ := NewRun("x", tpl)
	if run.MaxCostUSD() != 0 {
		t.Fatalf("run inherited non-zero cap: %v", run.MaxCostUSD())
	}
}

func floatEq(a, b float64) bool {
	const eps = 1e-9
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < eps
}
