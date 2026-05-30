package pipeline

import (
	"testing"

	"github.com/example/dddcqrs/internal/domain/pipeline"
)

func TestSetCaptionAndUploadSteps_AddsBothWhenEnabled(t *testing.T) {
	steps := []pipeline.StepConfig{
		{Type: pipeline.StepScript},
		{Type: pipeline.StepImage},
		{Type: pipeline.StepAssemble},
	}
	on := true
	got := setCaptionAndUploadSteps(steps, &UploadOverride{
		Enabled:          &on,
		Provider:         "youtube_selenium",
		SocialAccountIDs: []string{"acc1"},
		ScheduledAt:      "2026-06-01T12:00:00Z",
		CaptionModel:     "openai/gpt-4o-mini",
		Platforms:        []string{"youtube", "twitter"},
	})
	if len(got) != 5 {
		t.Fatalf("expected 5 steps, got %d", len(got))
	}
	if got[3].Type != pipeline.StepCaption {
		t.Errorf("step[3] should be caption, got %s", got[3].Type)
	}
	if got[4].Type != pipeline.StepUpload {
		t.Errorf("step[4] should be upload, got %s", got[4].Type)
	}
	if got[3].Model != "openai/gpt-4o-mini" {
		t.Errorf("caption model not applied: %q", got[3].Model)
	}
	pl, ok := got[3].Params["platforms"].([]string)
	if !ok || len(pl) != 2 {
		t.Errorf("platforms not applied to caption step: %v", got[3].Params)
	}
	if got[4].Provider != "youtube_selenium" {
		t.Errorf("upload provider lost: %q", got[4].Provider)
	}
	if got[4].Params["scheduled_at"] != "2026-06-01T12:00:00Z" {
		t.Errorf("scheduled_at not applied: %v", got[4].Params)
	}
	ids, ok := got[4].Params["social_account_ids"].([]string)
	if !ok || len(ids) != 1 || ids[0] != "acc1" {
		t.Errorf("social_account_ids: %v", got[4].Params)
	}
}

func TestSetCaptionAndUploadSteps_RemovesExistingWhenDisabled(t *testing.T) {
	steps := []pipeline.StepConfig{
		{Type: pipeline.StepScript},
		{Type: pipeline.StepImage},
		{Type: pipeline.StepAssemble},
		{Type: pipeline.StepCaption},
		{Type: pipeline.StepUpload},
	}
	off := false
	got := setCaptionAndUploadSteps(steps, &UploadOverride{Enabled: &off})
	if len(got) != 3 {
		t.Fatalf("expected 3 steps after removal, got %d", len(got))
	}
	for _, s := range got {
		if s.Type == pipeline.StepCaption || s.Type == pipeline.StepUpload {
			t.Errorf("residual step %s", s.Type)
		}
	}
}

func TestSetCaptionAndUploadSteps_NilNoOp(t *testing.T) {
	steps := []pipeline.StepConfig{{Type: pipeline.StepScript}}
	got := setCaptionAndUploadSteps(steps, nil)
	if len(got) != 1 {
		t.Errorf("nil override should not change steps, got %d", len(got))
	}
}

func TestSetCaptionAndUploadSteps_EnabledFalseExplicitNoOp(t *testing.T) {
	steps := []pipeline.StepConfig{{Type: pipeline.StepScript}}
	off := false
	got := setCaptionAndUploadSteps(steps, &UploadOverride{Enabled: &off})
	if len(got) != 1 {
		t.Errorf("disabled override with no existing caption/upload should not change, got %d", len(got))
	}
}

func TestSetCaptionAndUploadSteps_CaptionOverrideOnCaptionStep(t *testing.T) {
	steps := []pipeline.StepConfig{
		{Type: pipeline.StepScript},
		{Type: pipeline.StepImage},
		{Type: pipeline.StepAssemble},
	}
	on := true
	got := setCaptionAndUploadSteps(steps, &UploadOverride{
		Enabled:         &on,
		CaptionOverride: "manual caption",
	})
	caption := got[3]
	if caption.Type != pipeline.StepCaption {
		t.Fatalf("step[3] not caption")
	}
	if caption.Params["caption_override"] != "manual caption" {
		t.Errorf("caption_override not set: %v", caption.Params)
	}
}

func TestMergeProjectDefaults_UploadBlock(t *testing.T) {
	defaults := map[string]any{
		"upload": map[string]any{
			"enabled":            true,
			"provider":           "youtube_selenium",
			"caption_model":      "openai/gpt-4o-mini",
			"social_account_ids": []any{"acc-a", "acc-b"},
			"platforms":          []any{"youtube"},
		},
	}
	got := mergeProjectDefaults(nil, defaults)
	if got.Upload == nil || got.Upload.Enabled == nil || *got.Upload.Enabled != true {
		t.Fatalf("upload.enabled not propagated: %+v", got.Upload)
	}
	if got.Upload.Provider != "youtube_selenium" {
		t.Errorf("provider: %s", got.Upload.Provider)
	}
	if len(got.Upload.SocialAccountIDs) != 2 || got.Upload.SocialAccountIDs[1] != "acc-b" {
		t.Errorf("social_account_ids: %v", got.Upload.SocialAccountIDs)
	}
	if got.Upload.CaptionModel != "openai/gpt-4o-mini" {
		t.Errorf("caption_model: %s", got.Upload.CaptionModel)
	}
	if len(got.Upload.Platforms) != 1 || got.Upload.Platforms[0] != "youtube" {
		t.Errorf("platforms: %v", got.Upload.Platforms)
	}
}

func TestMergeProjectDefaults_UploadUserOverrideWins(t *testing.T) {
	defaults := map[string]any{
		"upload": map[string]any{
			"provider":           "youtube_selenium",
			"social_account_ids": []any{"default-acc"},
		},
	}
	user := &RunOverrides{
		Upload: &UploadOverride{
			Provider:         "twitter_selenium",
			SocialAccountIDs: []string{"user-acc"},
		},
	}
	got := mergeProjectDefaults(user, defaults)
	if got.Upload.Provider != "twitter_selenium" {
		t.Errorf("user provider clobbered: %s", got.Upload.Provider)
	}
	if got.Upload.SocialAccountIDs[0] != "user-acc" {
		t.Errorf("user accounts clobbered: %v", got.Upload.SocialAccountIDs)
	}
}
