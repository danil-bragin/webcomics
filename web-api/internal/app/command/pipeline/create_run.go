// Package pipeline holds write-side command handlers for the pipeline context.
// Each handler opens a UoW and persists the aggregate + its outbox events
// inside one tx — same pattern as command/register_user.go.
package pipeline

import (
	"context"
	"maps"
	"strings"

	"github.com/example/dddcqrs/internal/app/bus"
	"github.com/example/dddcqrs/internal/domain/formats"
	"github.com/example/dddcqrs/internal/domain/pipeline"
	"github.com/example/dddcqrs/internal/domain/projects"
	"github.com/example/dddcqrs/internal/infrastructure/persistence/uow"
)

// applyFormatToOverrides fills RunOverrides fields from a Format preset.
// User-supplied fields in ov take precedence; format fills empties only.
func applyFormatToOverrides(ov *RunOverrides, f *formats.Format) *RunOverrides {
	out := &RunOverrides{}
	if ov != nil {
		*out = *ov
	}
	if out.ImageModel == "" {
		out.ImageModel = f.ImageModel
	}
	if out.StyleReference == "" {
		out.StyleReference = f.StyleReference
	}
	if out.ImagePromptPrefix == "" {
		out.ImagePromptPrefix = f.ImagePromptPrefix
	}
	if out.ImagePromptSuffix == "" {
		out.ImagePromptSuffix = f.ImagePromptSuffix
	}
	if out.PanelDurationMs == 0 {
		out.PanelDurationMs = f.PanelDurationMs
	}
	if out.Transition == "" {
		out.Transition = f.Transition
	}
	// Append format's script-style guidance to user's system prompt. Always
	// terminate with the JSON response schema instruction so OpenRouter's
	// response_format=json_object filter accepts the request.
	const jsonTail = "\n\nRespond strictly in JSON with the schema {\"panels\":[{\"prompt\":\"…\",\"caption\":\"…\"}]}."
	if f.ScriptSystemSuffix != "" {
		if out.SystemPrompt == "" {
			out.SystemPrompt = f.ScriptSystemSuffix + jsonTail
		} else {
			out.SystemPrompt = out.SystemPrompt + "\n\n" + f.ScriptSystemSuffix + jsonTail
		}
	}
	// Assemble dims default from format unless user already specified.
	if out.Assemble == nil {
		out.Assemble = &AssembleOverride{}
	}
	if out.Assemble.FPS == 0 {
		out.Assemble.FPS = f.FPS
	}
	if out.Assemble.Width == 0 {
		out.Assemble.Width = f.Width
	}
	if out.Assemble.Height == 0 {
		out.Assemble.Height = f.Height
	}
	if out.Assemble.Codec == "" {
		out.Assemble.Codec = f.Codec
	}
	return out
}

func projectIDFromString(s string) projects.ProjectID { return projects.ProjectID(s) }

// mergeProjectDefaults overlays a project's saved defaults onto the user's
// RunOverrides. User-supplied fields win; missing fields take the default.
// Returns a fresh RunOverrides — caller's input isn't mutated.
func mergeProjectDefaults(ov *RunOverrides, def map[string]any) *RunOverrides {
	out := &RunOverrides{}
	if ov != nil {
		*out = *ov
	}
	if out.PanelCount == 0 {
		if v, ok := def["panel_count"].(float64); ok {
			out.PanelCount = int(v)
		}
	}
	if out.TargetDurationMs == 0 {
		if v, ok := def["target_duration_ms"].(float64); ok {
			out.TargetDurationMs = int(v)
		}
	}
	if out.EnableAudio == nil {
		if v, ok := def["enable_audio"].(bool); ok {
			out.EnableAudio = &v
		}
	}
	if out.AutoAssemble == nil {
		if v, ok := def["auto_assemble"].(bool); ok {
			out.AutoAssemble = &v
		}
	}
	if out.SystemPrompt == "" {
		if v, ok := def["system_prompt"].(string); ok {
			out.SystemPrompt = v
		}
	}
	if out.ScriptModel == "" {
		if v, ok := def["script_model"].(string); ok {
			out.ScriptModel = v
		}
	}
	if out.ImageModel == "" {
		if v, ok := def["image_model"].(string); ok {
			out.ImageModel = v
		}
	}
	if out.StyleReference == "" {
		if v, ok := def["style_reference"].(string); ok {
			out.StyleReference = v
		}
	}
	// Nested audio block — apply per field.
	if defAudio, ok := def["audio"].(map[string]any); ok {
		if out.Audio == nil {
			out.Audio = &AudioOverride{}
		}
		if out.Audio.VoiceID == "" {
			if v, ok := defAudio["voice_id"].(string); ok {
				out.Audio.VoiceID = v
			}
		}
		if out.Audio.Model == "" {
			if v, ok := defAudio["model"].(string); ok {
				out.Audio.Model = v
			}
		}
		if out.Audio.Speed == 0 {
			if v, ok := defAudio["speed"].(float64); ok {
				out.Audio.Speed = v
			}
		}
	}
	if defUp, ok := def["upload"].(map[string]any); ok {
		if out.Upload == nil {
			out.Upload = &UploadOverride{}
		}
		if out.Upload.Enabled == nil {
			if v, ok := defUp["enabled"].(bool); ok {
				out.Upload.Enabled = &v
			}
		}
		if out.Upload.Provider == "" {
			if v, ok := defUp["provider"].(string); ok {
				out.Upload.Provider = v
			}
		}
		if out.Upload.CaptionModel == "" {
			if v, ok := defUp["caption_model"].(string); ok {
				out.Upload.CaptionModel = v
			}
		}
		if len(out.Upload.SocialAccountIDs) == 0 {
			if arr, ok := defUp["social_account_ids"].([]any); ok {
				for _, v := range arr {
					if s, ok := v.(string); ok {
						out.Upload.SocialAccountIDs = append(out.Upload.SocialAccountIDs, s)
					}
				}
			}
		}
		if len(out.Upload.Platforms) == 0 {
			if arr, ok := defUp["platforms"].([]any); ok {
				for _, v := range arr {
					if s, ok := v.(string); ok {
						out.Upload.Platforms = append(out.Upload.Platforms, s)
					}
				}
			}
		}
	}
	if defAsm, ok := def["assemble"].(map[string]any); ok {
		if out.Assemble == nil {
			out.Assemble = &AssembleOverride{}
		}
		if out.Assemble.FPS == 0 {
			if v, ok := defAsm["fps"].(float64); ok {
				out.Assemble.FPS = int(v)
			}
		}
		if out.Assemble.Width == 0 {
			if v, ok := defAsm["width"].(float64); ok {
				out.Assemble.Width = int(v)
			}
		}
		if out.Assemble.Height == 0 {
			if v, ok := defAsm["height"].(float64); ok {
				out.Assemble.Height = int(v)
			}
		}
		if out.Assemble.Codec == "" {
			if v, ok := defAsm["codec"].(string); ok {
				out.Assemble.Codec = v
			}
		}
	}
	// Project defaults can also carry an `ambient.object_key` field that maps
	// to the assemble step (renderer loops it under music + voice).
	if defAmb, ok := def["ambient"].(map[string]any); ok {
		if out.Assemble == nil {
			out.Assemble = &AssembleOverride{}
		}
		if out.Assemble.AmbientObjectKey == "" {
			if v, ok := defAmb["object_key"].(string); ok {
				out.Assemble.AmbientObjectKey = v
			}
		}
	}
	return out
}

// CreateRun creates a new run from a template + prompt and starts it.
// Start emits the first step request via the outbox in the same tx.
type CreateRun struct {
	Prompt         string
	TemplateID     string
	ProjectID      string
	CharacterIDs   []string
	EnvironmentIDs []string
	UsePlot        bool
	FormatID       string // overrides project.defaults.format_id
	Language       string // "en" | "ru" | "fr" (default "en") — caption + voice language
	Overrides      *RunOverrides
}

func (CreateRun) IsCommand() {}

// RunOverrides — per-run tweaks merged into the chosen template's steps.
// All fields optional. `Steps`, when non-empty, replaces the template snapshot.
type RunOverrides struct {
	PanelCount       int
	TargetDurationMs int
	EnableAudio      *bool
	AutoAssemble     *bool
	SystemPrompt     string
	ScriptModel      string
	ImageModel       string
	StyleReference   string // "none" | "anchor" | "previous"
	// Format-driven image prompt cues applied per panel by Run.requestStep.
	ImagePromptPrefix string
	ImagePromptSuffix string
	PanelDurationMs   int
	Transition        string
	Audio             *AudioOverride
	Assemble          *AssembleOverride
	Upload            *UploadOverride
	Music             *MusicOverride
	Steps             []pipeline.StepConfig
}

// UploadOverride drives the upload step + insert/remove of caption+upload
// steps from the template snapshot. Fields here win over project defaults
// which win over per-account defaults which win over caption-LLM output.
type UploadOverride struct {
	Enabled          *bool
	Provider         string   // youtube_selenium | twitter_selenium | ...
	SocialAccountIDs []string // accounts to post from (one upload step per account in v2)
	ScheduledAt      string   // RFC3339, empty = immediate
	CaptionModel     string   // LLM model for caption gen
	CaptionOverride  string   // user-supplied final caption (skips LLM)
	Platforms        []string // override platform set for caption LLM

	// Per-publication metadata. Any empty / zero value means "fall through to
	// project default → account default → caption LLM output".
	Title           string
	Description     string
	Tags            []string
	Visibility      string // public | unlisted | private
	MadeForKids     *bool
	AgeRestriction  string // none | 18plus
	CategoryID      string
	CategoryLabel   string
	CommentsEnabled *bool
	PlaylistNames   []string
	ThumbnailKey    string // MinIO key of pre-uploaded thumbnail
	Headless        *bool  // worker-only, defaults true
}

// AudioOverride applied to the audio step's params when enable_audio=true.
type AudioOverride struct {
	VoiceID string
	Model   string
	Speed   float64
}

// MusicOverride applied to the music step's params. Lets the operator pin a
// mood or specific track id and skip the LLM picker.
type MusicOverride struct {
	PreferredMood string `json:"preferred_mood"`
	TrackID       string `json:"track_id"`
}

// AssembleOverride applied to the assemble step's params.
type AssembleOverride struct {
	FPS             int
	Width           int
	Height          int
	Codec           string
	PanelDurationMs int    `json:"panel_duration_ms"`
	Transition      string `json:"transition"`
	// AmbientObjectKey is the MinIO key of a looped ambient bed mixed under
	// music + voice during render. Empty = no ambient track.
	AmbientObjectKey string `json:"ambient_object_key"`
}

type CreateRunResult struct {
	RunID string
}

type CreateRunHandler struct{ uow uow.Manager }

func NewCreateRunHandler(m uow.Manager) *CreateRunHandler { return &CreateRunHandler{uow: m} }

func (h *CreateRunHandler) Handle(ctx context.Context, cmd CreateRun) (CreateRunResult, error) {
	var out CreateRunResult
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repos := u.Repositories()
		tpl, err := repos.PipelineTemplates().GetByID(ctx, pipeline.TemplateID(cmd.TemplateID))
		if err != nil {
			return err
		}
		// Layer (low→high precedence): format → project defaults → user overrides.
		// Each layer only fills fields the next layer left empty.
		mergedOverrides := cmd.Overrides
		var projectDefaults map[string]any
		if cmd.ProjectID != "" {
			proj, err := repos.Projects().GetProject(ctx, projectIDFromString(cmd.ProjectID))
			if err == nil && proj != nil && len(proj.Defaults()) > 0 {
				projectDefaults = proj.Defaults()
			}
		}
		formatID := cmd.FormatID
		if formatID == "" && projectDefaults != nil {
			if v, ok := projectDefaults["format_id"].(string); ok {
				formatID = v
			}
		}
		if formatID != "" {
			if f := formats.ByID(formatID); f != nil {
				mergedOverrides = applyFormatToOverrides(mergedOverrides, f)
			}
		}
		if projectDefaults != nil {
			mergedOverrides = mergeProjectDefaults(mergedOverrides, projectDefaults)
		}
		// Resolve social account for upload step. Order:
		//   1. Explicit overrides.upload.social_account_ids (from run create body)
		//   2. Project's default linked account on the upload platform
		//   3. (later) any active linked account on the platform
		// If still none and upload is enabled, the run will fail at upload time
		// with a clear "no social account available" error.
		if mergedOverrides != nil && mergedOverrides.Upload != nil {
			if len(mergedOverrides.Upload.SocialAccountIDs) == 0 && cmd.ProjectID != "" {
				links, lerr := repos.Projects().ListLinkedSocialAccounts(ctx, projects.ProjectID(cmd.ProjectID))
				if lerr == nil {
					wantPlatform := strings.TrimSpace(mergedOverrides.Upload.Provider)
					var picked *projects.SocialAccount
					// Pass 1: explicit-platform default.
					for _, l := range links {
						if !l.IsDefault {
							continue
						}
						if wantPlatform == "" || l.Account.Platform() == wantPlatform {
							picked = l.Account
							break
						}
					}
					// Pass 2: any default link (if no platform constraint).
					if picked == nil && wantPlatform == "" {
						for _, l := range links {
							if l.IsDefault {
								picked = l.Account
								break
							}
						}
					}
					// Pass 3: any active link matching platform (no default set).
					if picked == nil {
						for _, l := range links {
							if l.Account.Status() != projects.SocialAccountStatusActive {
								continue
							}
							if wantPlatform == "" || l.Account.Platform() == wantPlatform {
								picked = l.Account
								break
							}
						}
					}
					if picked != nil {
						mergedOverrides.Upload.SocialAccountIDs = []string{picked.ID().String()}
					}
				}
			}
			if len(mergedOverrides.Upload.SocialAccountIDs) > 0 {
				acct, err := repos.Projects().GetSocialAccount(ctx, projects.SocialAccountID(mergedOverrides.Upload.SocialAccountIDs[0]))
				if err == nil && acct != nil {
					if mergedOverrides.Upload.Provider == "" {
						mergedOverrides.Upload.Provider = acct.Platform()
					}
				}
			}
		}
		effective := applyOverrides(tpl.Steps(), mergedOverrides)
		// After step list built, enrich upload step's params with resolved
		// social account info + full metadata snapshot so the worker doesn't
		// need a DB hit and so each UploadRecord can reproduce exactly what
		// went out.
		var resolvedUploadAcct *projects.SocialAccount
		var resolvedUploadMeta map[string]any
		if mergedOverrides != nil && mergedOverrides.Upload != nil && len(mergedOverrides.Upload.SocialAccountIDs) > 0 {
			acct, err := repos.Projects().GetSocialAccount(ctx, projects.SocialAccountID(mergedOverrides.Upload.SocialAccountIDs[0]))
			if err == nil && acct != nil {
				resolvedUploadAcct = acct
				resolvedUploadMeta = resolveUploadMeta(mergedOverrides.Upload, acct, projectDefaults)
				for i, s := range effective {
					if s.Type != pipeline.StepUpload {
						continue
					}
					if effective[i].Params == nil {
						effective[i].Params = map[string]any{}
					}
					effective[i].Params["social_account_id"] = acct.ID().String()
					effective[i].Params["firefox_profile_path"] = acct.FirefoxProfilePath()
					for k, v := range resolvedUploadMeta {
						effective[i].Params[k] = v
					}
					break
				}
			}
		}
		synth := pipeline.ReconstituteTemplateWithCap(
			tpl.ID(), tpl.Name(), effective, tpl.MaxCostUSD(),
			tpl.CreatedAt(), tpl.UpdatedAt(),
		)
		opts := pipeline.RunOptions{AutoAssemble: true}
		if mergedOverrides != nil && mergedOverrides.AutoAssemble != nil {
			opts.AutoAssemble = *mergedOverrides.AutoAssemble
		}
		// Language precedence: explicit cmd.Language → project.defaults.language → "en".
		lang := cmd.Language
		if lang == "" && projectDefaults != nil {
			if v, ok := projectDefaults["language"].(string); ok {
				lang = v
			}
		}
		opts.Language = lang
		// Project-side review_mode: when "review" and upload is enabled, the
		// run will pause in awaiting_action right before the upload step.
		if projectDefaults != nil {
			if mode, ok := readUploadReviewMode(projectDefaults); ok && mode == "review" {
				opts.RequireReviewBeforeUpload = true
			}
		}
		// Primary publish format drives the assemble dimensions so the rendered
		// master matches the destination platform out of the box. Per-platform
		// re-encode comes later.
		applyPrimaryFormat(effective, projectDefaults)
		// Burned-in subtitle defaults from project flow into the assemble step.
		applySubtitlesDefaults(effective, projectDefaults)
		// Resolve project linkage if a project was attached.
		if cmd.ProjectID != "" {
			linked, plotID, err := resolveLinkedContext(ctx, repos, cmd.ProjectID, cmd.CharacterIDs, cmd.EnvironmentIDs, cmd.UsePlot)
			if err != nil {
				return err
			}
			opts.ProjectID = cmd.ProjectID
			opts.CharacterIDs = cmd.CharacterIDs
			opts.EnvironmentIDs = cmd.EnvironmentIDs
			opts.PlotID = plotID
			opts.LinkedContext = linked
		}
		run, err := pipeline.NewRunWithOptions(cmd.Prompt, synth, opts)
		if err != nil {
			return err
		}
		if err := run.Start(); err != nil {
			return err
		}
		if err := repos.PipelineRuns().Save(ctx, run); err != nil {
			return err
		}
		if err := repos.Outbox().Add(ctx, run.PullEvents()...); err != nil {
			return err
		}

		// Persist a pending UploadRecord for the bound account so the UI has a
		// row to render before the worker fires. Done after the run save so the
		// FK is valid.
		_ = projectDefaults // kept for review_mode resolution below
		if resolvedUploadAcct != nil {
			meta := pipeline.UploadMetadata{
				Visibility:       stringFromMap(resolvedUploadMeta, "visibility", "unlisted"),
				MadeForKids:      boolFromMap(resolvedUploadMeta, "made_for_kids", false),
				AgeRestriction:   stringFromMap(resolvedUploadMeta, "age_restriction", "none"),
				CategoryID:       stringFromMap(resolvedUploadMeta, "category_id", "22"),
				CategoryLabel:    stringFromMap(resolvedUploadMeta, "category_label", "People & Blogs"),
				CommentsEnabled:  boolFromMap(resolvedUploadMeta, "comments_enabled", true),
				Tags:             stringSliceFromMap(resolvedUploadMeta, "tags"),
				PlaylistNames:    stringSliceFromMap(resolvedUploadMeta, "playlist_names"),
				Title:            stringFromMap(resolvedUploadMeta, "title", ""),
				Description:      stringFromMap(resolvedUploadMeta, "description", ""),
				ThumbnailAssetID: stringFromMap(resolvedUploadMeta, "thumbnail_asset_id", ""),
			}
			rec := pipeline.NewUploadRecord(
				run.ID().String(), cmd.ProjectID, resolvedUploadAcct.ID().String(),
				resolvedUploadAcct.Platform(), -1, meta,
			)
			if err := repos.UploadRecords().Save(ctx, rec); err != nil {
				return err
			}
		}

		out.RunID = run.ID().String()
		return nil
	})
	return out, err
}

// applyOverrides merges per-run tweaks into the template's step list.
// Returns a fresh slice; template's own steps are not mutated.
func applyOverrides(templateSteps []pipeline.StepConfig, ov *RunOverrides) []pipeline.StepConfig {
	if ov == nil {
		return cloneSteps(templateSteps)
	}
	if len(ov.Steps) > 0 {
		return cloneSteps(ov.Steps)
	}
	steps := cloneSteps(templateSteps)

	for i, s := range steps {
		if s.Type != pipeline.StepScript {
			continue
		}
		if ov.PanelCount > 0 {
			if steps[i].Params == nil {
				steps[i].Params = map[string]any{}
			}
			steps[i].Params["panel_count"] = ov.PanelCount
		}
		if ov.SystemPrompt != "" {
			steps[i].SystemPrompt = ov.SystemPrompt
		}
		if ov.ScriptModel != "" {
			steps[i].Model = ov.ScriptModel
		}
		break
	}

	if ov.ImageModel != "" {
		for i, s := range steps {
			if s.Type == pipeline.StepImage {
				steps[i].Model = ov.ImageModel
				break
			}
		}
	}

	if ov.StyleReference != "" {
		for i, s := range steps {
			if s.Type != pipeline.StepImage {
				continue
			}
			if steps[i].Params == nil {
				steps[i].Params = map[string]any{}
			}
			steps[i].Params["style_reference"] = ov.StyleReference
			break
		}
	}

	// Format-driven image prompt cues live on image step's params; the Run
	// aggregate reads them when emitting ImageRequested.
	if ov.ImagePromptPrefix != "" || ov.ImagePromptSuffix != "" {
		for i, s := range steps {
			if s.Type != pipeline.StepImage {
				continue
			}
			if steps[i].Params == nil {
				steps[i].Params = map[string]any{}
			}
			if ov.ImagePromptPrefix != "" {
				steps[i].Params["image_prompt_prefix"] = ov.ImagePromptPrefix
			}
			if ov.ImagePromptSuffix != "" {
				steps[i].Params["image_prompt_suffix"] = ov.ImagePromptSuffix
			}
			break
		}
	}

	if ov.EnableAudio != nil {
		steps = setAudioStep(steps, *ov.EnableAudio)
	}

	if ov.Upload != nil {
		steps = setCaptionAndUploadSteps(steps, ov.Upload)
	}

	if ov.Audio != nil {
		for i, s := range steps {
			if s.Type != pipeline.StepAudio {
				continue
			}
			if steps[i].Params == nil {
				steps[i].Params = map[string]any{}
			}
			if ov.Audio.VoiceID != "" {
				steps[i].Params["voice_id"] = ov.Audio.VoiceID
			}
			if ov.Audio.Model != "" {
				steps[i].Model = ov.Audio.Model
			}
			if ov.Audio.Speed > 0 {
				steps[i].Params["speed"] = ov.Audio.Speed
			}
			break
		}
	}

	if ov.Assemble != nil {
		for i, s := range steps {
			if s.Type != pipeline.StepAssemble {
				continue
			}
			if steps[i].Params == nil {
				steps[i].Params = map[string]any{}
			}
			if ov.Assemble.FPS > 0 {
				steps[i].Params["fps"] = ov.Assemble.FPS
			}
			if ov.Assemble.Width > 0 {
				steps[i].Params["width"] = ov.Assemble.Width
			}
			if ov.Assemble.Height > 0 {
				steps[i].Params["height"] = ov.Assemble.Height
			}
			if ov.Assemble.Codec != "" {
				steps[i].Params["codec"] = ov.Assemble.Codec
			}
			if ov.Assemble.PanelDurationMs > 0 {
				steps[i].Params["panel_duration_ms"] = ov.Assemble.PanelDurationMs
			}
			if ov.Assemble.Transition != "" {
				steps[i].Params["transition"] = ov.Assemble.Transition
			}
			if ov.Assemble.AmbientObjectKey != "" {
				steps[i].Params["ambient_object_key"] = ov.Assemble.AmbientObjectKey
			}
			break
		}
	}
	if ov.Music != nil {
		for i, s := range steps {
			if s.Type != pipeline.StepMusic {
				continue
			}
			if steps[i].Params == nil {
				steps[i].Params = map[string]any{}
			}
			if ov.Music.PreferredMood != "" {
				steps[i].Params["preferred_mood"] = ov.Music.PreferredMood
			}
			if ov.Music.TrackID != "" {
				steps[i].Params["track_id"] = ov.Music.TrackID
			}
			break
		}
	}

	// Nested ov.Assemble.PanelDurationMs (per-step override) wins over the
	// top-level ov.PanelDurationMs (legacy format-derived default). Without
	// this guard, the format library overwrites the user's per-run assemble
	// override and audio/video timing diverges.
	assemblePanelOverrideSet := ov.Assemble != nil && ov.Assemble.PanelDurationMs > 0
	if ov.TargetDurationMs > 0 && !assemblePanelOverrideSet {
		panels := ov.PanelCount
		if panels <= 0 {
			panels = panelCountFromScript(steps)
		}
		if panels <= 0 {
			panels = 1
		}
		perPanel := ov.TargetDurationMs / panels
		for i, s := range steps {
			if s.Type != pipeline.StepAssemble {
				continue
			}
			if steps[i].Params == nil {
				steps[i].Params = map[string]any{}
			}
			steps[i].Params["panel_duration_ms"] = perPanel
			break
		}
	} else if ov.PanelDurationMs > 0 && !assemblePanelOverrideSet {
		for i, s := range steps {
			if s.Type != pipeline.StepAssemble {
				continue
			}
			if steps[i].Params == nil {
				steps[i].Params = map[string]any{}
			}
			steps[i].Params["panel_duration_ms"] = ov.PanelDurationMs
			break
		}
	}
	assembleTransitionOverrideSet := ov.Assemble != nil && ov.Assemble.Transition != ""
	if ov.Transition != "" && !assembleTransitionOverrideSet {
		for i, s := range steps {
			if s.Type != pipeline.StepAssemble {
				continue
			}
			if steps[i].Params == nil {
				steps[i].Params = map[string]any{}
			}
			steps[i].Params["transition"] = ov.Transition
			break
		}
	}
	return steps
}

func cloneSteps(in []pipeline.StepConfig) []pipeline.StepConfig {
	out := make([]pipeline.StepConfig, len(in))
	for i, s := range in {
		out[i] = s
		if s.Params != nil {
			cp := make(map[string]any, len(s.Params))
			maps.Copy(cp, s.Params)
			out[i].Params = cp
		}
	}
	return out
}

// setCaptionAndUploadSteps appends caption + upload step(s) when Upload.Enabled
// is true; removes any existing ones when false. Caption sits between assemble
// and upload so the upload step can attach the generated caption metadata.
func setCaptionAndUploadSteps(steps []pipeline.StepConfig, up *UploadOverride) []pipeline.StepConfig {
	if up == nil {
		return steps
	}
	enabled := up.Enabled != nil && *up.Enabled
	// Drop existing caption/upload steps first so caller decides clean state.
	cleaned := steps[:0:0]
	for _, s := range steps {
		if s.Type == pipeline.StepCaption || s.Type == pipeline.StepUpload {
			continue
		}
		cleaned = append(cleaned, s)
	}
	steps = cleaned
	if !enabled {
		return steps
	}
	captionStep := pipeline.StepConfig{
		Type:   pipeline.StepCaption,
		Model:  up.CaptionModel,
		Params: map[string]any{},
	}
	if len(up.Platforms) > 0 {
		captionStep.Params["platforms"] = up.Platforms
	}
	if up.CaptionOverride != "" {
		captionStep.Params["caption_override"] = up.CaptionOverride
	}
	uploadStep := pipeline.StepConfig{
		Type:     pipeline.StepUpload,
		Provider: up.Provider,
		Params:   map[string]any{},
	}
	if len(up.SocialAccountIDs) > 0 {
		uploadStep.Params["social_account_ids"] = up.SocialAccountIDs
	}
	if up.ScheduledAt != "" {
		uploadStep.Params["scheduled_at"] = up.ScheduledAt
	}
	return append(steps, captionStep, uploadStep)
}

func setAudioStep(steps []pipeline.StepConfig, enable bool) []pipeline.StepConfig {
	hasAudio, hasImage, hasAssemble := -1, -1, -1
	for i, s := range steps {
		switch s.Type {
		case pipeline.StepAudio:
			hasAudio = i
		case pipeline.StepImage:
			hasImage = i
		case pipeline.StepAssemble:
			hasAssemble = i
		}
	}
	if enable {
		if hasAudio >= 0 {
			return steps
		}
		insertAt := hasAssemble
		if insertAt < 0 {
			insertAt = hasImage + 1
		}
		if insertAt < 0 {
			insertAt = len(steps)
		}
		out := make([]pipeline.StepConfig, 0, len(steps)+1)
		out = append(out, steps[:insertAt]...)
		out = append(out, pipeline.StepConfig{Type: pipeline.StepAudio})
		out = append(out, steps[insertAt:]...)
		return out
	}
	if hasAudio < 0 {
		return steps
	}
	out := make([]pipeline.StepConfig, 0, len(steps)-1)
	out = append(out, steps[:hasAudio]...)
	out = append(out, steps[hasAudio+1:]...)
	return out
}

func panelCountFromScript(steps []pipeline.StepConfig) int {
	for _, s := range steps {
		if s.Type != pipeline.StepScript {
			continue
		}
		if s.Params == nil {
			return 0
		}
		switch v := s.Params["panel_count"].(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		}
	}
	return 0
}

func CreateRunOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[CreateRun, CreateRunResult](r, NewCreateRunHandler(m))
}

// RetryRun clones a terminal run (failed/cancelled/completed) into a new run
// using the same template + prompt. The new run gets a fresh id and goes
// through the normal Start path.
type RetryRun struct {
	RunID string
}

func (RetryRun) IsCommand()         {}
func (c RetryRun) GetRunID() string { return c.RunID }

type RetryRunResult struct {
	RunID string
}

type RetryRunHandler struct{ uow uow.Manager }

func NewRetryRunHandler(m uow.Manager) *RetryRunHandler { return &RetryRunHandler{uow: m} }

func (h *RetryRunHandler) Handle(ctx context.Context, cmd RetryRun) (RetryRunResult, error) {
	var out RetryRunResult
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repos := u.Repositories()
		orig, err := repos.PipelineRuns().GetByID(ctx, pipeline.RunID(cmd.RunID))
		if err != nil {
			return err
		}
		tpl, err := repos.PipelineTemplates().GetByID(ctx, orig.TemplateID())
		if err != nil {
			return err
		}
		newRun, err := pipeline.NewRun(orig.Prompt(), tpl)
		if err != nil {
			return err
		}
		if err := newRun.Start(); err != nil {
			return err
		}
		if err := repos.PipelineRuns().Save(ctx, newRun); err != nil {
			return err
		}
		if err := repos.Outbox().Add(ctx, newRun.PullEvents()...); err != nil {
			return err
		}
		out.RunID = newRun.ID().String()
		return nil
	})
	return out, err
}

func RetryRunOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[RetryRun, RetryRunResult](r, NewRetryRunHandler(m))
}
