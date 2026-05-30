package pipeline

import (
	"reflect"
	"strings"
	"testing"

	"github.com/example/dddcqrs/internal/domain/formats"
	"github.com/example/dddcqrs/internal/domain/pipeline"
)

// ----- applyFormatToOverrides -----

func TestApplyFormatToOverrides_NilUserOverridesGetFormatDefaults(t *testing.T) {
	f := formats.ByID("manga")
	if f == nil {
		t.Fatal("manga not in catalog")
	}
	got := applyFormatToOverrides(nil, f)
	if got.ImageModel != f.ImageModel {
		t.Errorf("ImageModel: got %q want %q", got.ImageModel, f.ImageModel)
	}
	if got.StyleReference != f.StyleReference {
		t.Errorf("StyleReference: got %q want %q", got.StyleReference, f.StyleReference)
	}
	if !strings.Contains(got.ImagePromptPrefix, "manga style") {
		t.Errorf("ImagePromptPrefix missing manga style cue: %q", got.ImagePromptPrefix)
	}
	if got.PanelDurationMs != f.PanelDurationMs {
		t.Errorf("PanelDurationMs: got %d want %d", got.PanelDurationMs, f.PanelDurationMs)
	}
	if got.Transition != f.Transition {
		t.Errorf("Transition: got %q want %q", got.Transition, f.Transition)
	}
	if got.Assemble == nil || got.Assemble.FPS != f.FPS ||
		got.Assemble.Width != f.Width || got.Assemble.Height != f.Height ||
		got.Assemble.Codec != f.Codec {
		t.Errorf("Assemble preset not applied: %+v", got.Assemble)
	}
}

func TestApplyFormatToOverrides_UserOverridesWin(t *testing.T) {
	f := formats.ByID("noir")
	ov := &RunOverrides{
		ImageModel:        "user-picked-model",
		StyleReference:    "previous",
		ImagePromptPrefix: "user prefix, ",
		PanelDurationMs:   1234,
		Transition:        "zoom",
		Assemble: &AssembleOverride{
			FPS: 60, Width: 4000, Height: 4000, Codec: "h265",
		},
	}
	got := applyFormatToOverrides(ov, f)
	if got.ImageModel != "user-picked-model" {
		t.Errorf("user ImageModel got clobbered: %q", got.ImageModel)
	}
	if got.StyleReference != "previous" {
		t.Errorf("user StyleReference clobbered: %q", got.StyleReference)
	}
	if got.ImagePromptPrefix != "user prefix, " {
		t.Errorf("user ImagePromptPrefix clobbered: %q", got.ImagePromptPrefix)
	}
	if got.PanelDurationMs != 1234 {
		t.Errorf("user PanelDurationMs clobbered: %d", got.PanelDurationMs)
	}
	if got.Transition != "zoom" {
		t.Errorf("user Transition clobbered: %q", got.Transition)
	}
	if got.Assemble.FPS != 60 || got.Assemble.Width != 4000 ||
		got.Assemble.Height != 4000 || got.Assemble.Codec != "h265" {
		t.Errorf("user assemble clobbered: %+v", got.Assemble)
	}
}

func TestApplyFormatToOverrides_SystemPromptConcatJSONTail(t *testing.T) {
	f := formats.ByID("manga")
	ov := &RunOverrides{SystemPrompt: "USER PRELUDE."}
	got := applyFormatToOverrides(ov, f)
	if !strings.Contains(got.SystemPrompt, "USER PRELUDE.") {
		t.Errorf("user system prompt missing: %q", got.SystemPrompt)
	}
	if !strings.Contains(strings.ToLower(got.SystemPrompt), "manga") {
		t.Errorf("format style guidance missing: %q", got.SystemPrompt)
	}
	if !strings.Contains(strings.ToLower(got.SystemPrompt), "json") {
		t.Errorf("JSON schema tail missing — OpenRouter response_format will reject: %q", got.SystemPrompt)
	}
}

// ----- mergeProjectDefaults -----

func TestMergeProjectDefaults_FillsEmptyFromMap(t *testing.T) {
	defaults := map[string]any{
		"panel_count":        float64(5),
		"target_duration_ms": float64(15000),
		"enable_audio":       true,
		"auto_assemble":      false,
		"system_prompt":      "stay terse",
		"script_model":       "anthropic/claude-3.5-sonnet",
		"image_model":        "fal-ai/flux-2/edit",
		"style_reference":    "anchor",
		"audio":              map[string]any{"voice_id": "v1", "model": "eleven_flash_v2_5", "speed": float64(1.1)},
		"assemble":           map[string]any{"fps": float64(60), "width": float64(1080), "height": float64(1920), "codec": "h265"},
	}
	got := mergeProjectDefaults(nil, defaults)
	if got.PanelCount != 5 {
		t.Errorf("PanelCount=%d", got.PanelCount)
	}
	if got.TargetDurationMs != 15000 {
		t.Errorf("TargetDurationMs=%d", got.TargetDurationMs)
	}
	if got.EnableAudio == nil || *got.EnableAudio != true {
		t.Errorf("EnableAudio not set")
	}
	if got.AutoAssemble == nil || *got.AutoAssemble != false {
		t.Errorf("AutoAssemble not set false")
	}
	if got.SystemPrompt != "stay terse" {
		t.Errorf("SystemPrompt=%q", got.SystemPrompt)
	}
	if got.ScriptModel != "anthropic/claude-3.5-sonnet" {
		t.Errorf("ScriptModel=%q", got.ScriptModel)
	}
	if got.ImageModel != "fal-ai/flux-2/edit" {
		t.Errorf("ImageModel=%q", got.ImageModel)
	}
	if got.StyleReference != "anchor" {
		t.Errorf("StyleReference=%q", got.StyleReference)
	}
	if got.Audio == nil || got.Audio.VoiceID != "v1" || got.Audio.Model != "eleven_flash_v2_5" || got.Audio.Speed != 1.1 {
		t.Errorf("Audio=%+v", got.Audio)
	}
	if got.Assemble == nil || got.Assemble.FPS != 60 || got.Assemble.Width != 1080 ||
		got.Assemble.Height != 1920 || got.Assemble.Codec != "h265" {
		t.Errorf("Assemble=%+v", got.Assemble)
	}
}

func TestMergeProjectDefaults_UserFieldsWin(t *testing.T) {
	defaults := map[string]any{
		"panel_count":  float64(5),
		"image_model":  "default-model",
		"enable_audio": true,
		"audio":        map[string]any{"voice_id": "default-voice"},
	}
	t1 := true // user override different from default
	user := &RunOverrides{
		PanelCount:  10,
		ImageModel:  "user-model",
		EnableAudio: &t1,
		Audio:       &AudioOverride{VoiceID: "user-voice"},
	}
	got := mergeProjectDefaults(user, defaults)
	if got.PanelCount != 10 {
		t.Errorf("PanelCount: user value lost (%d)", got.PanelCount)
	}
	if got.ImageModel != "user-model" {
		t.Errorf("ImageModel: user value lost (%q)", got.ImageModel)
	}
	if got.Audio.VoiceID != "user-voice" {
		t.Errorf("Audio.VoiceID: user value lost (%q)", got.Audio.VoiceID)
	}
}

// ----- applyOverrides on template steps -----

func TestApplyOverrides_PanelDurationFromOverrideOnAssemble(t *testing.T) {
	steps := []pipeline.StepConfig{
		{Type: pipeline.StepScript},
		{Type: pipeline.StepImage},
		{Type: pipeline.StepAssemble},
	}
	got := applyOverrides(steps, &RunOverrides{PanelDurationMs: 4321})
	asm := got[2].Params
	if asm["panel_duration_ms"] != 4321 {
		t.Errorf("panel_duration_ms not applied to assemble: %v", asm)
	}
}

func TestApplyOverrides_TargetDurationTakesPrecedenceOverPanelDuration(t *testing.T) {
	steps := []pipeline.StepConfig{
		{Type: pipeline.StepScript},
		{Type: pipeline.StepImage},
		{Type: pipeline.StepAssemble},
	}
	got := applyOverrides(steps, &RunOverrides{
		PanelCount: 5, TargetDurationMs: 10000, PanelDurationMs: 9999,
	})
	asm := got[2].Params
	if asm["panel_duration_ms"] != 2000 {
		t.Errorf("expected target/panels = 2000, got %v", asm["panel_duration_ms"])
	}
}

func TestApplyOverrides_TransitionAppliedToAssemble(t *testing.T) {
	steps := []pipeline.StepConfig{
		{Type: pipeline.StepScript},
		{Type: pipeline.StepImage},
		{Type: pipeline.StepAssemble},
	}
	got := applyOverrides(steps, &RunOverrides{Transition: "wipe"})
	if got[2].Params["transition"] != "wipe" {
		t.Errorf("transition not applied: %v", got[2].Params)
	}
}

func TestApplyOverrides_ImagePromptCuesLandOnImageStep(t *testing.T) {
	steps := []pipeline.StepConfig{
		{Type: pipeline.StepScript},
		{Type: pipeline.StepImage},
		{Type: pipeline.StepAssemble},
	}
	got := applyOverrides(steps, &RunOverrides{
		ImagePromptPrefix: "noir style, ",
		ImagePromptSuffix: ", dramatic composition",
	})
	imgParams := got[1].Params
	if imgParams["image_prompt_prefix"] != "noir style, " {
		t.Errorf("prefix not applied: %v", imgParams)
	}
	if imgParams["image_prompt_suffix"] != ", dramatic composition" {
		t.Errorf("suffix not applied: %v", imgParams)
	}
}

func TestApplyOverrides_StyleReferenceLandsOnImageStep(t *testing.T) {
	steps := []pipeline.StepConfig{
		{Type: pipeline.StepScript},
		{Type: pipeline.StepImage},
	}
	got := applyOverrides(steps, &RunOverrides{StyleReference: "anchor"})
	if got[1].Params["style_reference"] != "anchor" {
		t.Errorf("style_reference not applied: %v", got[1].Params)
	}
}

func TestApplyOverrides_NilOverridesYieldsCloneOfTemplate(t *testing.T) {
	steps := []pipeline.StepConfig{
		{Type: pipeline.StepScript, SystemPrompt: "from template"},
	}
	got := applyOverrides(steps, nil)
	if !reflect.DeepEqual(got[0], steps[0]) {
		t.Errorf("template step mutated: %+v", got)
	}
	// Confirm it's a clone, not aliased.
	got[0].SystemPrompt = "mutated"
	if steps[0].SystemPrompt == "mutated" {
		t.Errorf("original template mutated through clone")
	}
}

func TestApplyOverrides_EnableAudioInsertsAudioStep(t *testing.T) {
	steps := []pipeline.StepConfig{
		{Type: pipeline.StepScript},
		{Type: pipeline.StepImage},
		{Type: pipeline.StepAssemble},
	}
	on := true
	got := applyOverrides(steps, &RunOverrides{EnableAudio: &on})
	if len(got) != 4 {
		t.Fatalf("expected 4 steps after audio insert, got %d", len(got))
	}
	if got[2].Type != pipeline.StepAudio {
		t.Errorf("audio not inserted before assemble; got %+v", typesOf(got))
	}
}

func TestApplyOverrides_EnableAudioFalseRemovesExistingAudio(t *testing.T) {
	steps := []pipeline.StepConfig{
		{Type: pipeline.StepScript},
		{Type: pipeline.StepImage},
		{Type: pipeline.StepAudio},
		{Type: pipeline.StepAssemble},
	}
	off := false
	got := applyOverrides(steps, &RunOverrides{EnableAudio: &off})
	if len(got) != 3 {
		t.Fatalf("expected 3 steps after audio removal, got %d (%v)", len(got), typesOf(got))
	}
	for _, s := range got {
		if s.Type == pipeline.StepAudio {
			t.Errorf("audio still present after removal: %v", typesOf(got))
		}
	}
}

func typesOf(s []pipeline.StepConfig) []string {
	out := make([]string, len(s))
	for i, st := range s {
		out[i] = string(st.Type)
	}
	return out
}
