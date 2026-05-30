package pipeline

import (
	"context"

	"github.com/example/dddcqrs/internal/app/bus"
	"github.com/example/dddcqrs/internal/domain/pipeline"
	"github.com/example/dddcqrs/internal/infrastructure/persistence/uow"
)

// BackfillUploadMetadata applies the LLM-generated caption metadata to any
// pending/uploaded UploadRecord rows on the run. Fired by the caption-completed
// consumer so the review UI shows real titles/descriptions/tags right after
// the LLM finishes — without overwriting user-supplied run overrides (those
// were baked into the row at create time).
type BackfillUploadMetadata struct {
	RunID         string
	Metadata      *pipeline.CaptionMetadata
	RequireReview bool // true → row transitions to pending_review; false → metadata_ready
}

func (BackfillUploadMetadata) IsCommand() {}

type BackfillUploadMetadataResult struct{ UpdatedCount int }

type BackfillUploadMetadataHandler struct{ uow uow.Manager }

func NewBackfillUploadMetadataHandler(m uow.Manager) *BackfillUploadMetadataHandler {
	return &BackfillUploadMetadataHandler{uow: m}
}

func (h *BackfillUploadMetadataHandler) Handle(ctx context.Context, c BackfillUploadMetadata) (BackfillUploadMetadataResult, error) {
	var out BackfillUploadMetadataResult
	if c.Metadata == nil || len(c.Metadata.Platforms) == 0 {
		return out, nil
	}
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		recs, err := u.Repositories().UploadRecords().ListByRun(ctx, c.RunID)
		if err != nil {
			return err
		}
		// Resolve project review_mode (auto | review) from the first record's
		// project ID. Default = review (safer).
		requireReview := c.RequireReview
		for _, rec := range recs {
			if rec.ProjectID() == "" {
				continue
			}
			proj, err := u.Repositories().Projects().GetProject(ctx, projectIDFromString(rec.ProjectID()))
			if err != nil || proj == nil {
				continue
			}
			if mode, ok := readUploadReviewMode(proj.Defaults()); ok {
				requireReview = mode == "review"
			}
			break
		}
		audienceKids := c.Metadata.Audience.MadeForKids
		for _, rec := range recs {
			platformKey := platformKeyFor(rec.Provider())
			meta, ok := c.Metadata.Platforms[platformKey]
			if !ok {
				// Fallback to youtube_shorts entry when provider-specific entry missing.
				meta = c.Metadata.Platforms["youtube_shorts"]
			}
			if meta.Title == "" && meta.Description == "" && len(meta.Tags) == 0 && len(meta.Hashtags) == 0 {
				continue
			}
			rec.ApplyLLMMetadata(
				meta.Title, meta.Description, c.Metadata.Hook,
				meta.Tags, meta.Hashtags,
				&audienceKids,
				c.Metadata.Audience.Confidence,
				c.Metadata.Audience.Reasoning,
				requireReview,
			)
			if err := u.Repositories().UploadRecords().Save(ctx, rec); err != nil {
				return err
			}
			out.UpdatedCount++
		}
		return nil
	})
	return out, err
}

// readUploadReviewMode pulls project.defaults.upload.review_mode. Returns
// (value, true) when defined.
func readUploadReviewMode(defaults map[string]any) (string, bool) {
	if defaults == nil {
		return "", false
	}
	up, ok := defaults["upload"].(map[string]any)
	if !ok {
		return "", false
	}
	mode, ok := up["review_mode"].(string)
	if !ok || mode == "" {
		return "", false
	}
	return mode, true
}

func BackfillUploadMetadataOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[BackfillUploadMetadata, BackfillUploadMetadataResult](r, NewBackfillUploadMetadataHandler(m))
}

// platformKeyFor maps an upload provider string to the caption schema key.
func platformKeyFor(provider string) string {
	switch provider {
	case "youtube_selenium", "youtube_api", "youtube_shorts_selenium":
		return "youtube_shorts"
	case "youtube_long_selenium":
		return "youtube_long"
	case "instagram_selenium":
		return "instagram_reels"
	case "tiktok_selenium":
		return "tiktok"
	case "twitter_selenium":
		return "twitter"
	default:
		return "youtube_shorts"
	}
}
