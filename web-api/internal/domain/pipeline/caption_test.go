package pipeline

import (
	"encoding/json"
	"strings"
	"testing"
)

// newCaptionUploadTemplate returns a 5-step template with caption + upload
// already attached. Used to exercise Run state-machine for the upload tail.
func newCaptionUploadTemplate(t *testing.T) *Template {
	t.Helper()
	steps := []StepConfig{
		{Type: StepScript, Model: "gpt-4o-mini", Params: map[string]any{"panel_count": 2}},
		{Type: StepImage, Model: "fal/flux-schnell"},
		{Type: StepAssemble, Params: map[string]any{"width": 1080, "height": 1920, "fps": 30}},
		{Type: StepCaption, Model: "openai/gpt-4o-mini", Params: map[string]any{
			"platforms": []any{"youtube"},
		}},
		{Type: StepUpload, Provider: "youtube_selenium", Params: map[string]any{
			"firefox_profile_path": "/tmp/ff-profile",
			"social_account_id":    "acc-1",
			"scheduled_at":         "2026-12-01T10:00:00Z",
		}},
	}
	tpl, err := NewTemplate("with-caption", steps)
	if err != nil {
		t.Fatalf("NewTemplate: %v", err)
	}
	return tpl
}

func driveToCaption(t *testing.T, run *Run) {
	t.Helper()
	if err := run.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := run.RecordScriptCompleted(0, "k.json", panelsOf(2), CostInfo{TotalCostUSD: 0.001}, 50); err != nil {
		t.Fatalf("script: %v", err)
	}
	for i := range 2 {
		key := "runs/x/1/panel-" + itoa(i) + ".png"
		if err := run.RecordImageCompleted(1, i, key, CostInfo{TotalCostUSD: 0.003}, 30, 0); err != nil {
			t.Fatalf("image %d: %v", i, err)
		}
	}
	if err := run.RecordAssembleCompleted(2, "runs/x/2/video.mp4", CostInfo{}, 100); err != nil {
		t.Fatalf("assemble: %v", err)
	}
}

func TestRun_CaptionStep_EmitsRequestWithLinkedContext(t *testing.T) {
	tpl := newCaptionUploadTemplate(t)
	linked := &LinkedContext{
		ProjectID:  "p1",
		Characters: []CharacterContext{{Name: "Hero", Description: "tall"}},
		Plot:       &PlotContext{Premise: "epic"},
	}
	run, _ := NewRunWithOptions("test", tpl, RunOptions{AutoAssemble: true, LinkedContext: linked})
	driveToCaption(t, run)

	// Caption step should now be running with v1 attempt.
	caption := run.Steps()[3]
	att := caption.ActiveAttempt()
	if att == nil {
		t.Fatal("caption step has no attempt")
	}
	// Pull the CaptionRequested event and verify linkage flowed in.
	events := run.PullEvents()
	var captionEvt *CaptionRequested
	for _, e := range events {
		if ce, ok := e.(CaptionRequested); ok {
			captionEvt = &ce
		}
	}
	if captionEvt == nil {
		t.Fatal("CaptionRequested not emitted")
	}
	if len(captionEvt.Characters) != 1 || captionEvt.Characters[0].Name != "Hero" {
		t.Errorf("characters not propagated: %+v", captionEvt.Characters)
	}
	if captionEvt.Plot == nil || captionEvt.Plot.Premise != "epic" {
		t.Errorf("plot not propagated: %+v", captionEvt.Plot)
	}
	if len(captionEvt.Platforms) != 1 || captionEvt.Platforms[0] != "youtube" {
		t.Errorf("platforms: %v", captionEvt.Platforms)
	}
	if len(captionEvt.Panels) != 2 {
		t.Errorf("panels not forwarded: %d", len(captionEvt.Panels))
	}
}

func TestRun_RecordCaptionCompleted_AdvancesToUpload(t *testing.T) {
	tpl := newCaptionUploadTemplate(t)
	run, _ := NewRunWithOptions("test", tpl, RunOptions{AutoAssemble: true})
	driveToCaption(t, run)

	captions := map[string]any{
		"youtube": map[string]any{
			"title":    "Cool",
			"caption":  "description",
			"hashtags": []any{"ai", "shorts"},
		},
	}
	if err := run.RecordCaptionCompleted(3, captions, CostInfo{TotalCostUSD: 0.001}, 80); err != nil {
		t.Fatalf("RecordCaptionCompleted: %v", err)
	}
	if run.Status() != RunStatusRunning {
		t.Errorf("expected running (upload queued), got %s", run.Status())
	}
	upload := run.Steps()[4]
	if upload.ActiveAttempt() == nil {
		t.Fatal("upload step missing attempt after caption advance")
	}
}

func TestRun_UploadStep_InheritsCaptionAndProfile(t *testing.T) {
	tpl := newCaptionUploadTemplate(t)
	run, _ := NewRunWithOptions("test", tpl, RunOptions{AutoAssemble: true})
	driveToCaption(t, run)
	captions := map[string]any{
		"youtube": map[string]any{"title": "T", "caption": "C", "hashtags": []any{"ai"}},
	}
	_ = run.RecordCaptionCompleted(3, captions, CostInfo{TotalCostUSD: 0.001}, 80)

	upload := run.Steps()[4]
	att := upload.ActiveAttempt()
	if att == nil {
		t.Fatal("upload step has no attempt")
	}
	var inp map[string]any
	if err := json.Unmarshal(att.input, &inp); err != nil {
		t.Fatalf("input not JSON: %v", err)
	}
	if inp["firefox_profile_path"] != "/tmp/ff-profile" {
		t.Errorf("profile not inherited: %v", inp["firefox_profile_path"])
	}
	if inp["social_account_id"] != "acc-1" {
		t.Errorf("social_account_id not inherited: %v", inp["social_account_id"])
	}
	if inp["scheduled_at"] != "2026-12-01T10:00:00Z" {
		t.Errorf("scheduled_at not inherited: %v", inp["scheduled_at"])
	}
	got, ok := inp["captions"].(map[string]any)
	if !ok {
		t.Fatalf("captions block missing: %v", inp)
	}
	yt, _ := got["youtube"].(map[string]any)
	if yt["title"] != "T" {
		t.Errorf("caption.youtube.title not inherited: %v", got)
	}
}

func TestStringsFromParam_HandlesAnyAndStringSlices(t *testing.T) {
	// []any{string,...}
	out := stringsFromParam(map[string]any{"x": []any{"a", "b", 99, "c"}}, "x")
	if len(out) != 3 || out[0] != "a" || out[1] != "b" || out[2] != "c" {
		t.Errorf("any-slice path: %v", out)
	}
	// []string
	out = stringsFromParam(map[string]any{"x": []string{"d", "e"}}, "x")
	if len(out) != 2 || out[1] != "e" {
		t.Errorf("string-slice path: %v", out)
	}
	// Missing key.
	if got := stringsFromParam(map[string]any{}, "x"); got != nil {
		t.Errorf("missing key: %v", got)
	}
	// Nil params.
	if got := stringsFromParam(nil, "x"); got != nil {
		t.Errorf("nil params: %v", got)
	}
	// Non-array.
	if got := stringsFromParam(map[string]any{"x": "string-not-array"}, "x"); got != nil {
		t.Errorf("non-array: %v", got)
	}
}

func TestRun_CaptionInputIncludesPanelsAndPlatforms(t *testing.T) {
	tpl := newCaptionUploadTemplate(t)
	run, _ := NewRunWithOptions("test", tpl, RunOptions{AutoAssemble: true})
	driveToCaption(t, run)
	caption := run.Steps()[3]
	att := caption.ActiveAttempt()
	if att == nil {
		t.Fatal("caption step has no attempt")
	}
	var inp map[string]any
	_ = json.Unmarshal(att.input, &inp)
	panels, _ := inp["panels"].([]any)
	if len(panels) != 2 {
		t.Errorf("expected 2 panels in input: %v", panels)
	}
	platforms, _ := inp["platforms"].([]any)
	if len(platforms) != 1 || platforms[0] != "youtube" {
		t.Errorf("platforms: %v", platforms)
	}
}

func TestApplyFormatPromptCues_Idempotent(t *testing.T) {
	// Direct unit covering applyFormatPromptCues — protects against future
	// edits that might double-apply prefix/suffix.
	params := map[string]any{
		"image_prompt_prefix": "noir style, ",
		"image_prompt_suffix": ", dramatic",
	}
	got := applyFormatPromptCues("hero in alley", params)
	if !strings.HasPrefix(got, "noir style, ") {
		t.Errorf("prefix not applied: %q", got)
	}
	if !strings.HasSuffix(got, ", dramatic") {
		t.Errorf("suffix not applied: %q", got)
	}
}
