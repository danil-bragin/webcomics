package pipeline

import (
	"github.com/example/dddcqrs/internal/domain/pipeline"
	"github.com/example/dddcqrs/internal/domain/projects"
)

// resolveUploadMeta builds the flat metadata map that goes into the upload
// step's Params. Precedence (high → low):
//  1. RunOverrides.Upload.* (run-level, explicit user input)
//  2. projectDefaults["upload"].* (project settings)
//  3. SocialAccount per-account defaults
//  4. Pipeline-wide defaults (hardcoded)
//
// Caption-LLM output is NOT consulted here — it lives in the caption step's
// payload and is merged on the worker side at upload time.
func resolveUploadMeta(ov *UploadOverride, acct *projects.SocialAccount, projectDefaults map[string]any) map[string]any {
	out := map[string]any{}

	// Layer 4 — global pipeline defaults.
	out["visibility"] = "unlisted"
	out["made_for_kids"] = false
	out["age_restriction"] = "none"
	out["category_id"] = "22"
	out["category_label"] = "People & Blogs"
	out["comments_enabled"] = true
	out["tags"] = []string{}
	out["playlist_names"] = []string{}
	out["headless"] = true

	// Layer 3 — per-account defaults.
	if acct != nil {
		if acct.DefaultVisibility() != "" {
			out["visibility"] = acct.DefaultVisibility()
		}
		out["made_for_kids"] = acct.DefaultMadeForKids()
		if acct.DefaultCategoryID() != "" {
			out["category_id"] = acct.DefaultCategoryID()
		}
		if acct.DefaultCategoryLabel() != "" {
			out["category_label"] = acct.DefaultCategoryLabel()
		}
	}

	// Layer 2 — project defaults (jsonb).
	if projectDefaults != nil {
		if up, ok := projectDefaults["upload"].(map[string]any); ok {
			if v, ok := up["visibility"].(string); ok && v != "" {
				out["visibility"] = v
			}
			if v, ok := up["made_for_kids"].(bool); ok {
				out["made_for_kids"] = v
			}
			if v, ok := up["age_restriction"].(string); ok && v != "" {
				out["age_restriction"] = v
			}
			if v, ok := up["category_id"].(string); ok && v != "" {
				out["category_id"] = v
			}
			if v, ok := up["category_label"].(string); ok && v != "" {
				out["category_label"] = v
			}
			if v, ok := up["comments_enabled"].(bool); ok {
				out["comments_enabled"] = v
			}
			if arr, ok := up["tags"].([]any); ok {
				tags := []string{}
				for _, x := range arr {
					if s, ok := x.(string); ok {
						tags = append(tags, s)
					}
				}
				out["tags"] = tags
			}
			if arr, ok := up["playlist_names"].([]any); ok {
				names := []string{}
				for _, x := range arr {
					if s, ok := x.(string); ok {
						names = append(names, s)
					}
				}
				out["playlist_names"] = names
			}
			if v, ok := up["title"].(string); ok && v != "" {
				out["title"] = v
			}
			if v, ok := up["description"].(string); ok && v != "" {
				out["description"] = v
			}
			if v, ok := up["thumbnail_asset_id"].(string); ok && v != "" {
				out["thumbnail_asset_id"] = v
			}
		}
	}

	// Layer 1 — run-level explicit overrides.
	if ov != nil {
		if ov.Visibility != "" {
			out["visibility"] = ov.Visibility
		}
		if ov.MadeForKids != nil {
			out["made_for_kids"] = *ov.MadeForKids
		}
		if ov.AgeRestriction != "" {
			out["age_restriction"] = ov.AgeRestriction
		}
		if ov.CategoryID != "" {
			out["category_id"] = ov.CategoryID
		}
		if ov.CategoryLabel != "" {
			out["category_label"] = ov.CategoryLabel
		}
		if ov.CommentsEnabled != nil {
			out["comments_enabled"] = *ov.CommentsEnabled
		}
		if len(ov.Tags) > 0 {
			out["tags"] = append([]string{}, ov.Tags...)
		}
		if len(ov.PlaylistNames) > 0 {
			out["playlist_names"] = append([]string{}, ov.PlaylistNames...)
		}
		if ov.Title != "" {
			out["title"] = ov.Title
		}
		if ov.Description != "" {
			out["description"] = ov.Description
		}
		if ov.ThumbnailKey != "" {
			out["thumbnail_asset_id"] = ov.ThumbnailKey
		}
		if ov.Headless != nil {
			out["headless"] = *ov.Headless
		}
		if ov.ScheduledAt != "" {
			out["scheduled_at"] = ov.ScheduledAt
		}
	}
	return out
}

// applySubtitlesDefaults reads project.defaults.subtitles and forces it onto
// every assemble step's params. Run-level overrides are applied later through
// the override merger, so this just plugs the defaults.
func applySubtitlesDefaults(steps []pipeline.StepConfig, projectDefaults map[string]any) {
	if projectDefaults == nil {
		return
	}
	sub, ok := projectDefaults["subtitles"].(map[string]any)
	if !ok || len(sub) == 0 {
		return
	}
	for i, s := range steps {
		if s.Type != pipeline.StepAssemble {
			continue
		}
		if steps[i].Params == nil {
			steps[i].Params = map[string]any{}
		}
		if _, has := steps[i].Params["subtitles"]; has {
			continue
		}
		steps[i].Params["subtitles"] = sub
	}
}

// applyPrimaryFormat reads project.defaults.upload.primary_format and forces
// matching assemble dimensions. Skips if the assemble step already has explicit
// width/height (Run override wins).
//
// Mapping:
//
//	shorts | reels | tiktok  → 1080 × 1920 (9:16)
//	long                     → 1920 × 1080 (16:9)
//	square                   → 1080 × 1080 (1:1)
func applyPrimaryFormat(steps []pipeline.StepConfig, projectDefaults map[string]any) {
	if projectDefaults == nil {
		return
	}
	up, ok := projectDefaults["upload"].(map[string]any)
	if !ok {
		return
	}
	pf, _ := up["primary_format"].(string)
	if pf == "" {
		return
	}
	var w, h int
	switch pf {
	case "shorts", "reels", "tiktok", "vertical":
		w, h = 1080, 1920
	case "long", "horizontal":
		w, h = 1920, 1080
	case "square":
		w, h = 1080, 1080
	default:
		return
	}
	for i, s := range steps {
		if s.Type != pipeline.StepAssemble {
			continue
		}
		if steps[i].Params == nil {
			steps[i].Params = map[string]any{}
		}
		if _, hasW := steps[i].Params["width"]; !hasW {
			steps[i].Params["width"] = w
		}
		if _, hasH := steps[i].Params["height"]; !hasH {
			steps[i].Params["height"] = h
		}
	}
}

// --- map helpers used by create_run.go to materialise UploadMetadata.

func stringFromMap(m map[string]any, key, def string) string {
	if m == nil {
		return def
	}
	if v, ok := m[key].(string); ok && v != "" {
		return v
	}
	return def
}

func boolFromMap(m map[string]any, key string, def bool) bool {
	if m == nil {
		return def
	}
	if v, ok := m[key].(bool); ok {
		return v
	}
	return def
}

func stringSliceFromMap(m map[string]any, key string) []string {
	if m == nil {
		return nil
	}
	if v, ok := m[key].([]string); ok {
		return append([]string{}, v...)
	}
	if v, ok := m[key].([]any); ok {
		out := []string{}
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
