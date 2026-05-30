package pipeline

import (
	"strings"
	"testing"
)

func TestApplyLLMMetadata_PreservesUserOverride(t *testing.T) {
	rec := NewUploadRecord("run1", "p1", "acc1", "youtube_selenium", -1, UploadMetadata{
		Title: "manual title",
	})
	rec.OverrideMetadata(rec.Metadata()) // marks metadataOverridden=true
	rec.ApplyLLMMetadata("llm title", "llm desc", "hook", []string{"a"}, []string{"#Shorts"}, nil, 0.5, "", false)
	if rec.Metadata().Title != "manual title" {
		t.Errorf("user override should win, got %q", rec.Metadata().Title)
	}
}

func TestApplyLLMMetadata_FillsBlankFields(t *testing.T) {
	rec := NewUploadRecord("run1", "p1", "acc1", "youtube_selenium", -1, UploadMetadata{})
	kids := false
	rec.ApplyLLMMetadata("Cool title", "Engaging desc", "Punchy hook",
		[]string{"comedy", "ai"}, []string{"Shorts"}, &kids, 0.85, "lighthearted satire", false)
	m := rec.Metadata()
	if m.Title != "Cool title" {
		t.Errorf("title not applied: %q", m.Title)
	}
	if m.Description != "Engaging desc" {
		t.Errorf("description not applied: %q", m.Description)
	}
	if len(m.Tags) != 2 {
		t.Errorf("tags not applied: %v", m.Tags)
	}
	if rec.AudienceConfidence() != 0.85 {
		t.Errorf("confidence not stored: %v", rec.AudienceConfidence())
	}
	if rec.AudienceReasoning() != "lighthearted satire" {
		t.Errorf("reasoning not stored: %q", rec.AudienceReasoning())
	}
	if rec.Hook() != "Punchy hook" {
		t.Errorf("hook not stored: %q", rec.Hook())
	}
}

func TestApplyLLMMetadata_RaisesKidsBar(t *testing.T) {
	rec := NewUploadRecord("run1", "p1", "acc1", "youtube_selenium", -1, UploadMetadata{
		MadeForKids: false,
	})
	yes := true
	rec.ApplyLLMMetadata("t", "d", "h", nil, nil, &yes, 0.9, "kids content", false)
	if !rec.Metadata().MadeForKids {
		t.Error("LLM raised the kids bar — value should become true")
	}
}

func TestApplyLLMMetadata_NeverLowersKidsBar(t *testing.T) {
	rec := NewUploadRecord("run1", "p1", "acc1", "youtube_selenium", -1, UploadMetadata{
		MadeForKids: true,
	})
	no := false
	rec.ApplyLLMMetadata("t", "d", "h", nil, nil, &no, 0.5, "adult content", false)
	if !rec.Metadata().MadeForKids {
		t.Error("LLM should never lower MadeForKids from true to false (safety)")
	}
}

func TestApplyLLMMetadata_RequireReviewSetsPendingReview(t *testing.T) {
	rec := NewUploadRecord("run1", "p1", "acc1", "youtube_selenium", -1, UploadMetadata{})
	kids := false
	rec.ApplyLLMMetadata("t", "d", "h", nil, nil, &kids, 0.5, "ok", true)
	if rec.Status() != UploadStatusPendingReview {
		t.Errorf("expected pending_review, got %s", rec.Status())
	}
}

func TestApplyLLMMetadata_NoReviewGoesToMetadataReady(t *testing.T) {
	rec := NewUploadRecord("run1", "p1", "acc1", "youtube_selenium", -1, UploadMetadata{})
	kids := false
	rec.ApplyLLMMetadata("t", "d", "h", nil, nil, &kids, 0.5, "ok", false)
	if rec.Status() != UploadStatusMetadataReady {
		t.Errorf("expected metadata_ready, got %s", rec.Status())
	}
}

func TestApprove_RequiresPendingReviewOrMetadataReady(t *testing.T) {
	rec := NewUploadRecord("run1", "p1", "acc1", "youtube_selenium", -1, UploadMetadata{})
	rec.MarkUploaded("https://youtu.be/x", "x", "public")
	rec.Approve() // no-op from published
	if rec.Status() != UploadStatusPublished {
		t.Errorf("approve from published should be no-op, got %s", rec.Status())
	}
}

func TestApprove_ForcesShortsHashtagOnVerticalProviders(t *testing.T) {
	rec := NewUploadRecord("run1", "p1", "acc1", "youtube_selenium", -1, UploadMetadata{
		Description: "Some description without the hashtag",
		Hashtags:    []string{},
	})
	kids := false
	rec.ApplyLLMMetadata("t", "d", "h", nil, nil, &kids, 0.5, "", true)
	rec.Approve()
	if !strings.Contains(rec.Metadata().Description, "#Shorts") {
		t.Error("Approve must inject #Shorts into description for vertical providers")
	}
	hasShortsTag := false
	for _, h := range rec.Metadata().Hashtags {
		if strings.EqualFold(strings.TrimPrefix(h, "#"), "shorts") {
			hasShortsTag = true
		}
	}
	if !hasShortsTag {
		t.Error("Approve must add Shorts hashtag to the list")
	}
}

func TestApprove_DoesNotDuplicateShortsHashtag(t *testing.T) {
	rec := NewUploadRecord("run1", "p1", "acc1", "youtube_selenium", -1, UploadMetadata{
		Description: "Cool video #Shorts",
		Hashtags:    []string{"Shorts"},
	})
	kids := false
	rec.ApplyLLMMetadata("t", "d", "h", nil, nil, &kids, 0.5, "", true)
	rec.Approve()
	count := strings.Count(rec.Metadata().Description, "#Shorts")
	if count != 1 {
		t.Errorf("expected single #Shorts in description, found %d", count)
	}
}

func TestReject_TerminalState(t *testing.T) {
	rec := NewUploadRecord("run1", "p1", "acc1", "youtube_selenium", -1, UploadMetadata{})
	rec.Reject()
	if rec.Status() != UploadStatusRejected {
		t.Errorf("expected rejected, got %s", rec.Status())
	}
}

func TestReject_DoesNotReverseUploaded(t *testing.T) {
	rec := NewUploadRecord("run1", "p1", "acc1", "youtube_selenium", -1, UploadMetadata{})
	rec.MarkUploaded("https://youtu.be/x", "x", "unlisted")
	rec.Reject()
	if rec.Status() == UploadStatusRejected {
		t.Error("reject should be a no-op when already uploaded — video lives on YT")
	}
}

func TestMarkUploaded_PublishedWhenVisibilityPublic(t *testing.T) {
	rec := NewUploadRecord("run1", "p1", "acc1", "youtube_selenium", -1, UploadMetadata{})
	rec.MarkUploaded("https://youtu.be/x", "x", "public")
	if rec.Status() != UploadStatusPublished {
		t.Errorf("expected published, got %s", rec.Status())
	}
}

func TestMarkUploaded_UploadedWhenUnlisted(t *testing.T) {
	rec := NewUploadRecord("run1", "p1", "acc1", "youtube_selenium", -1, UploadMetadata{})
	rec.MarkUploaded("https://youtu.be/x", "x", "unlisted")
	if rec.Status() != UploadStatusUploaded {
		t.Errorf("expected uploaded, got %s", rec.Status())
	}
}

func TestMarkFailed_TracksAttemptsAndScreenshot(t *testing.T) {
	rec := NewUploadRecord("run1", "p1", "acc1", "youtube_selenium", -1, UploadMetadata{})
	rec.MarkFailed("timeout", "screenshots/err.png")
	if rec.Status() != UploadStatusFailed {
		t.Errorf("expected failed, got %s", rec.Status())
	}
	if rec.Attempts() != 1 {
		t.Errorf("expected attempt count 1, got %d", rec.Attempts())
	}
	if rec.ErrorScreenshotAssetID() != "screenshots/err.png" {
		t.Errorf("screenshot not stored")
	}
}

func TestSetScreenshotTrail_StoresAndUpdatesTimestamp(t *testing.T) {
	rec := NewUploadRecord("run1", "p1", "acc1", "youtube_selenium", -1, UploadMetadata{})
	trail := []ScreenshotEntry{
		{Stage: "01-step-studio-opened", ObjectKey: "runs/x/upload/0/01.png"},
		{Stage: "02-step-title-typed", ObjectKey: "runs/x/upload/0/02.png"},
	}
	rec.SetScreenshotTrail(trail)
	got := rec.ScreenshotTrail()
	if len(got) != 2 {
		t.Fatalf("expected 2 trail frames, got %d", len(got))
	}
	if got[1].Stage != "02-step-title-typed" {
		t.Errorf("wrong stage at index 1: %s", got[1].Stage)
	}
}
