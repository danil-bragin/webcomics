package pipeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/example/dddcqrs/internal/domain/shared"
)

var (
	ErrRunPromptEmpty        = errors.New("pipeline: prompt empty")
	ErrRunNotFound           = errors.New("pipeline: run not found")
	ErrTemplateNotFound      = errors.New("pipeline: template not found")
	ErrRunNotQueued          = errors.New("pipeline: run not in queued state")
	ErrRunNotRunning         = errors.New("pipeline: run not running")
	ErrRunNotRegeneratable   = errors.New("pipeline: run not in a state that allows regeneration")
	ErrStepIndexMismatch     = errors.New("pipeline: step index mismatch")
	ErrStepTypeMismatch      = errors.New("pipeline: step type mismatch")
	ErrUnknownPanel          = errors.New("pipeline: unknown panel index")
	ErrFirstStepMustBeScript = errors.New("pipeline: first step must be type=script")
	ErrNoAssembleStep        = errors.New("pipeline: run has no assemble step")
)

type RunStatus string

const (
	RunStatusQueued         RunStatus = "queued"
	RunStatusRunning        RunStatus = "running"
	RunStatusCompleted      RunStatus = "completed"
	RunStatusFailed         RunStatus = "failed"
	RunStatusCancelled      RunStatus = "cancelled"
	RunStatusAwaitingAction RunStatus = "awaiting_action"
)

// Run is the aggregate root for an in-flight pipeline execution.
type Run struct {
	shared.AggregateRoot

	id               RunID
	templateID       TemplateID
	prompt           string
	configSnapshot   []StepConfig
	autoAssemble     bool
	// requireReviewBeforeUpload pauses the run in awaiting_action when the
	// next step is `upload`. The user resumes via ResumeFromReview after
	// reviewing/editing/approving the upload records.
	requireReviewBeforeUpload bool
	status           RunStatus
	currentStepIndex int
	totalCostUSD     float64
	maxCostUSD       float64
	errorMsg         string
	createdAt        time.Time
	startedAt        *time.Time
	finishedAt       *time.Time

	// Project linkage (optional).
	projectID      string
	characterIDs   []string
	environmentIDs []string
	plotID         string
	// Content language (captions + voice + social copy). Image prompts
	// stay English regardless. "en" | "ru" | "fr". Default "en".
	language string
	// pauseAfterStep tracks a manual regenerate target — when its step
	// finishes, advance() parks the run in awaiting_action instead of
	// auto-cascading into downstream. -1 = no pause pending.
	pauseAfterStep int
	// Resolved context — set by command handlers via SetLinkedContext.
	// Persisted IDs above; context re-resolved on regenerate.
	linked *LinkedContext

	steps []*Step

	newAssets []Asset
	newCosts  []CostEntry
}

func (r *Run) ProjectID() string             { return r.projectID }
func (r *Run) Language() string               { return normalizeLanguage(r.language) }

func normalizeLanguage(lang string) string {
	switch lang {
	case "en", "ru", "fr":
		return lang
	}
	return "en"
}
func (r *Run) SetLanguage(lang string) {
	switch lang {
	case "en", "ru", "fr":
		r.language = lang
	}
}
func (r *Run) CharacterIDs() []string        { return r.characterIDs }
func (r *Run) EnvironmentIDs() []string      { return r.environmentIDs }
func (r *Run) PlotID() string                { return r.plotID }
func (r *Run) LinkedContext() *LinkedContext { return r.linked }

func (r *Run) SetLinkedContext(c *LinkedContext) { r.linked = c }

// AttachLinkage is used by the write repo to set project + character/env/plot
// IDs on a reconstituted Run without going through NewRunWithOptions.
func AttachLinkage(run *Run, projectID string, characterIDs, environmentIDs []string, plotID string) {
	if run == nil {
		return
	}
	run.projectID = projectID
	run.characterIDs = characterIDs
	run.environmentIDs = environmentIDs
	run.plotID = plotID
}

func NewRun(prompt string, template *Template) (*Run, error) {
	return NewRunWithOptions(prompt, template, RunOptions{AutoAssemble: true})
}

type RunOptions struct {
	AutoAssemble              bool
	RequireReviewBeforeUpload bool
	ProjectID                 string
	CharacterIDs              []string
	EnvironmentIDs            []string
	PlotID                    string
	LinkedContext             *LinkedContext
	Language                  string // en|ru|fr; "" → "en"
}

func NewRunWithOptions(prompt string, template *Template, opts RunOptions) (*Run, error) {
	if prompt == "" {
		return nil, ErrRunPromptEmpty
	}
	if template == nil {
		return nil, ErrTemplateNotFound
	}
	cfg := template.Steps()
	if len(cfg) == 0 {
		return nil, ErrTemplateNoSteps
	}
	if cfg[0].Type != StepScript {
		return nil, ErrFirstStepMustBeScript
	}

	now := time.Now().UTC()
	r := &Run{
		id:               NewRunID(),
		templateID:       template.ID(),
		prompt:           prompt,
		configSnapshot:   cfg,
		autoAssemble:     opts.AutoAssemble,
		requireReviewBeforeUpload: opts.RequireReviewBeforeUpload,
		status:           RunStatusQueued,
		currentStepIndex: 0,
		maxCostUSD:       template.MaxCostUSD(),
		createdAt:        now,
		projectID:        opts.ProjectID,
		characterIDs:     opts.CharacterIDs,
		environmentIDs:   opts.EnvironmentIDs,
		plotID:           opts.PlotID,
		linked:           opts.LinkedContext,
		language:         normalizeLanguage(opts.Language),
		pauseAfterStep:   -1,
	}
	for idx, c := range cfg {
		r.steps = append(r.steps, newStep(idx, c))
	}
	return r, nil
}

// Start moves the run from queued → running and emits the first step request.
func (r *Run) Start() error {
	if r.status != RunStatusQueued {
		return ErrRunNotQueued
	}
	now := time.Now().UTC()
	r.status = RunStatusRunning
	r.startedAt = &now
	return r.requestStep(0, nil)
}

// requestStep creates a fresh attempt for the step at idx and emits the
// corresponding *.requested event. paramsOverride is merged on top of the
// step's config-snapshot params (nil for the initial run).
func (r *Run) requestStep(idx int, paramsOverride map[string]any) error {
	if idx < 0 || idx >= len(r.steps) {
		return fmt.Errorf("pipeline: step index out of range")
	}
	step := r.steps[idx]
	cfg := r.mergedCfg(idx, paramsOverride)
	now := time.Now().UTC()
	r.currentStepIndex = idx

	base := shared.BaseEvent{ID: r.id.String(), Occurred: now}
	upstream := r.upstreamVersions(idx)

	switch step.stepType {
	case StepScript:
		panelHint := max(cfg.PanelCount(), 0)
		params := cloneParams(cfg.Params)
		if panelHint > 0 {
			if params == nil {
				params = map[string]any{}
			}
			if _, ok := params["panel_count"]; !ok {
				params["panel_count"] = panelHint
			}
		}
		lang := r.Language()
		inputJSON, _ := json.Marshal(map[string]any{
			"prompt":        r.prompt,
			"system_prompt": cfg.SystemPrompt,
			"model":         cfg.Model,
			"provider":      cfg.Provider,
			"language":      lang,
			"params":        params,
		})
		attempt := step.addAttempt(inputJSON, marshalParams(paramsOverride), upstream, 1, cfg.Provider, cfg.Model)
		evt := ScriptRequested{
			BaseEvent:    base,
			RunID:        r.id.String(),
			StepIndex:    idx,
			StepID:       step.id.String(),
			AttemptID:    attempt.id.String(),
			Prompt:       r.prompt,
			SystemPrompt: cfg.SystemPrompt,
			Model:        cfg.Model,
			Provider:     cfg.Provider,
			Language:     lang,
			Params:       params,
		}
		if r.linked != nil {
			evt.Characters = r.linked.Characters
			evt.Environments = r.linked.Environments
			evt.Plot = r.linked.Plot
		}
		r.Record(evt)

	case StepImage:
		panels, err := r.scriptPanels()
		if err != nil {
			return err
		}
		// Partial regen: when params contain panel_indices, only those panels
		// are emitted; the rest are seeded from the prior active attempt.
		// Works in "none" mode; for ref modes a partial regen still walks the
		// full sequence (the chain would otherwise miss references).
		partialIdx := panelIndicesSet(cfg.Params)
		var priorOutputs []map[string]any
		if len(partialIdx) > 0 {
			if prior := step.ActiveAttempt(); prior != nil {
				_ = json.Unmarshal(prior.outputs, &priorOutputs)
			}
		}

		outputs := make([]map[string]any, len(panels))
		preseeded := 0
		for i := range outputs {
			outputs[i] = map[string]any{"index": i, "object_key": ""}
			if len(partialIdx) > 0 && !partialIdx[i] && i < len(priorOutputs) {
				if k, _ := priorOutputs[i]["object_key"].(string); k != "" {
					outputs[i]["object_key"] = k
					preseeded++
				}
			}
		}
		outJSON, _ := json.Marshal(outputs)
		inputJSON, _ := json.Marshal(map[string]any{
			"panels":   panels,
			"model":    cfg.Model,
			"provider": cfg.Provider,
			"params":   cfg.Params,
		})
		attempt := step.addAttempt(inputJSON, marshalParams(paramsOverride), upstream, len(panels), cfg.Provider, cfg.Model)
		attempt.outputs = outJSON
		attempt.panelsCompleted = preseeded

		mode := styleRefMode(cfg.Params)
		overrides := panelOverrides(cfg.Params)
		// Character/environment ref images are prepended to every panel's refs.
		linkedRefs := r.linkedRefKeys()
		if mode == "none" {
			for _, p := range panels {
				if len(partialIdx) > 0 && !partialIdx[p.Index] {
					continue
				}
				model, prompt := applyPanelOverride(cfg.Model, p.Prompt, overrides[p.Index])
				prompt = applyFormatPromptCues(prompt, cfg.Params)
				prompt = composePromptWithLinks(prompt, r.linked)
				outputKey := fmt.Sprintf("runs/%s/%d/v%d/panel-%d.png", r.id, idx, step.currentVersion, p.Index)
				r.Record(ImageRequested{
					BaseEvent: base, RunID: r.id.String(),
					StepIndex: idx, StepID: step.id.String(), AttemptID: attempt.id.String(),
					PanelIndex: p.Index, Prompt: prompt, Caption: p.Caption,
					Model: model, Provider: cfg.Provider, Params: cfg.Params,
					OutputKey:     outputKey,
					RefObjectKeys: linkedRefs,
				})
			}
		} else {
			p := panels[0]
			// Panel 0 in ref modes uses the anchor model unless overridden.
			model := anchorModel(cfg)
			prompt := p.Prompt
			if ov := overrides[p.Index]; ov != nil {
				if m, ok := ov["model"].(string); ok && m != "" {
					model = m
				}
				if pa, ok := ov["prompt_append"].(string); ok && pa != "" {
					prompt = prompt + " " + pa
				}
			}
			prompt = applyFormatPromptCues(prompt, cfg.Params)
			prompt = composePromptWithLinks(prompt, r.linked)
			outputKey := fmt.Sprintf("runs/%s/%d/v%d/panel-%d.png", r.id, idx, step.currentVersion, p.Index)
			r.Record(ImageRequested{
				BaseEvent: base, RunID: r.id.String(),
				StepIndex: idx, StepID: step.id.String(), AttemptID: attempt.id.String(),
				PanelIndex: p.Index, Prompt: prompt, Caption: p.Caption,
				Model: model, Provider: cfg.Provider, Params: cfg.Params,
				OutputKey:     outputKey,
				RefObjectKeys: linkedRefs,
			})
		}

	case StepMusic:
		outputKey := fmt.Sprintf("runs/%s/%d/v%d/music.mp3", r.id, idx, step.currentVersion+1)
		inputJSON, _ := json.Marshal(map[string]any{
			"prompt":     r.prompt,
			"output_key": outputKey,
			"model":      cfg.Model,
			"provider":   cfg.Provider,
			"project_id": r.projectID,
			"params":     cfg.Params,
		})
		attempt := step.addAttempt(inputJSON, marshalParams(paramsOverride), upstream, 1, cfg.Provider, cfg.Model)
		r.Record(MusicRequested{
			BaseEvent: base,
			RunID:     r.id.String(),
			StepIndex: idx, StepID: step.id.String(), AttemptID: attempt.id.String(),
			ProjectID: r.projectID,
			Prompt:    r.prompt, Model: cfg.Model, Provider: cfg.Provider, Params: cfg.Params,
			OutputKey: outputKey,
		})

	case StepAudio:
		captions, prompt, err := r.audioInputs(cfg)
		if err != nil {
			return err
		}
		// Per-panel timing so the audio worker can stretch / pad voiceover
		// to match the assemble step's panel cadence.
		panelDur := assemblePanelDurationMs(r.configSnapshot)
		audioLang := r.Language()
		outputKey := fmt.Sprintf("runs/%s/%d/v%d/audio.mp3", r.id, idx, step.currentVersion+1)
		inputJSON, _ := json.Marshal(map[string]any{
			"captions":          captions,
			"prompt":            prompt,
			"output_key":        outputKey,
			"model":             cfg.Model,
			"provider":          cfg.Provider,
			"language":          audioLang,
			"params":            cfg.Params,
			"panel_count":       len(captions),
			"panel_duration_ms": panelDur,
		})
		attempt := step.addAttempt(inputJSON, marshalParams(paramsOverride), upstream, 1, cfg.Provider, cfg.Model)
		r.Record(AudioRequested{
			BaseEvent: base, RunID: r.id.String(),
			StepIndex: idx, StepID: step.id.String(), AttemptID: attempt.id.String(),
			Captions: captions, Prompt: prompt, Model: cfg.Model, Provider: cfg.Provider,
			Language: audioLang, Params: cfg.Params,
			OutputKey:       outputKey,
			PanelCount:      len(captions),
			PanelDurationMs: panelDur,
		})

	case StepCaption:
		panels, _ := r.scriptPanels()
		platforms := stringsFromParam(cfg.Params, "platforms")
		capLang := r.Language()
		inputJSON, _ := json.Marshal(map[string]any{
			"panels":    panels,
			"platforms": platforms,
			"model":     cfg.Model,
			"language":  capLang,
			"params":    cfg.Params,
		})
		attempt := step.addAttempt(inputJSON, marshalParams(paramsOverride), upstream, 1, cfg.Provider, cfg.Model)
		evt := CaptionRequested{
			BaseEvent: base, RunID: r.id.String(),
			StepIndex: idx, StepID: step.id.String(), AttemptID: attempt.id.String(),
			Prompt: r.prompt, Model: cfg.Model, Provider: cfg.Provider,
			Language: capLang, Params: cfg.Params, Panels: panels, Platforms: platforms,
		}
		if r.linked != nil {
			evt.Characters = r.linked.Characters
			evt.Environments = r.linked.Environments
			evt.Plot = r.linked.Plot
		}
		r.Record(evt)

	case StepUpload:
		videoKey, err := r.priorVideoKey()
		if err != nil {
			return err
		}
		captions := r.priorCaptions()
		// upload step's params carry social_account resolution + scheduled_at.
		socialAccountID, _ := cfg.Params["social_account_id"].(string)
		profilePath, _ := cfg.Params["firefox_profile_path"].(string)
		scheduledAt, _ := cfg.Params["scheduled_at"].(string)
		inputJSON, _ := json.Marshal(map[string]any{
			"video_key":            videoKey,
			"provider":             cfg.Provider,
			"params":               cfg.Params,
			"social_account_id":    socialAccountID,
			"firefox_profile_path": profilePath,
			"captions":             captions,
			"scheduled_at":         scheduledAt,
		})
		attempt := step.addAttempt(inputJSON, marshalParams(paramsOverride), upstream, 1, cfg.Provider, cfg.Model)
		r.Record(UploadRequested{
			BaseEvent: base, RunID: r.id.String(),
			StepIndex: idx, StepID: step.id.String(), AttemptID: attempt.id.String(),
			VideoKey: videoKey, Provider: cfg.Provider, Params: cfg.Params,
			SocialAccountID:    socialAccountID,
			FirefoxProfilePath: profilePath,
			Captions:           captions,
			ScheduledAt:        scheduledAt,
		})

	case StepAssemble:
		refs, err := r.assemblePanels(cfg)
		if err != nil {
			return err
		}
		audioKey := r.priorAudioKey()
		musicKey := r.priorMusicKey()
		ambientKey, _ := cfg.Params["ambient_object_key"].(string)
		sfxKeys := collectSFXKeys(refs)
		outputKey := fmt.Sprintf("runs/%s/%d/v%d/video.mp4", r.id, idx, step.currentVersion+1)
		width, height, fps := assembleDims(cfg.Params)
		inputJSON, _ := json.Marshal(map[string]any{
			"panels":      refs,
			"output_key":  outputKey,
			"audio_key":   audioKey,
			"music_key":   musicKey,
			"ambient_key": ambientKey,
			"sfx_keys":    sfxKeys,
			"width":       width,
			"height":      height,
			"fps":         fps,
			"params":      cfg.Params,
		})
		attempt := step.addAttempt(inputJSON, marshalParams(paramsOverride), upstream, 1, cfg.Provider, cfg.Model)
		r.Record(AssembleRequested{
			BaseEvent: base, RunID: r.id.String(),
			StepIndex: idx, StepID: step.id.String(), AttemptID: attempt.id.String(),
			Panels: refs, Width: width, Height: height, FPS: fps,
			OutputKey: outputKey, AudioKey: audioKey, MusicKey: musicKey,
			AmbientKey: ambientKey, SFXKeys: sfxKeys, Params: cfg.Params,
		})
	default:
		return ErrUnknownStepType
	}
	return nil
}

// mergedCfg returns the run's snapshot config for step idx with paramsOverride
// merged on top of cfg.Params.
func (r *Run) mergedCfg(idx int, paramsOverride map[string]any) StepConfig {
	cfg := r.configSnapshot[idx]
	if len(paramsOverride) == 0 {
		return cfg
	}
	merged := cloneParams(cfg.Params)
	if merged == nil {
		merged = map[string]any{}
	}
	out := cfg
	for k, v := range paramsOverride {
		switch k {
		case "system_prompt":
			if s, ok := v.(string); ok {
				out.SystemPrompt = s
			}
		case "model":
			if s, ok := v.(string); ok {
				out.Model = s
			}
		case "provider":
			if s, ok := v.(string); ok {
				out.Provider = s
			}
		default:
			merged[k] = v
		}
	}
	out.Params = merged
	return out
}

// upstreamVersions snapshots the active version of every upstream step at the
// moment a new attempt is started. Lets the UI detect that a step is stale
// when an upstream gets regenerated.
func (r *Run) upstreamVersions(idx int) map[int]int {
	v := map[int]int{}
	for i := range idx {
		v[i] = r.steps[i].currentVersion
	}
	return v
}

// RegenerateStep starts a new attempt for the step at idx. Downstream steps
// are marked stale but not re-run — when the regenerated step finishes,
// advance() pauses the run in awaiting_action so the user can decide whether
// to re-run downstream (per the chosen manual-cascade policy).
// Allowed from running / awaiting_action / completed / failed runs.
func (r *Run) RegenerateStep(idx int, paramsOverride map[string]any) error {
	if idx < 0 || idx >= len(r.steps) {
		return ErrStepIndexMismatch
	}
	if r.status == RunStatusCancelled {
		return ErrRunNotRegeneratable
	}
	r.status = RunStatusRunning
	r.errorMsg = ""
	r.finishedAt = nil
	r.pauseAfterStep = idx
	if err := r.requestStep(idx, paramsOverride); err != nil {
		return err
	}
	for j := idx + 1; j < len(r.steps); j++ {
		r.steps[j].markStale()
	}
	return nil
}

// RequestAssemble is the manual-mode entry point: when auto_assemble=false
// and the pipeline has paused in awaiting_action, the user calls this to
// trigger the assemble step (optionally with params overrides like fps,
// transition, panel_duration_ms — or the full timeline JSON later).
func (r *Run) RequestAssemble(paramsOverride map[string]any) error {
	idx := r.findStepIndex(StepAssemble)
	if idx < 0 {
		return ErrNoAssembleStep
	}
	if r.status == RunStatusCancelled {
		return ErrRunNotRegeneratable
	}
	r.status = RunStatusRunning
	r.errorMsg = ""
	r.finishedAt = nil
	return r.requestStep(idx, paramsOverride)
}

// ResumeFromReview resumes a run that was paused at the upload step. Emits the
// next upload step's request to the outbox. Called by the Approve handler when
// all upload records are no longer in pending_review.
func (r *Run) ResumeFromReview(paramsOverride map[string]any) error {
	idx := r.findStepIndex(StepUpload)
	if idx < 0 {
		return errors.New("pipeline: no upload step in pipeline")
	}
	if r.status != RunStatusAwaitingAction {
		return errors.New("pipeline: run not awaiting review")
	}
	r.status = RunStatusRunning
	r.errorMsg = ""
	r.finishedAt = nil
	return r.requestStep(idx, paramsOverride)
}

// RequireReviewBeforeUpload returns the configured review gate flag.
func (r *Run) RequireReviewBeforeUpload() bool { return r.requireReviewBeforeUpload }

// SetRequireReviewBeforeUpload toggles the review gate (used by command handlers
// when building the run from project defaults).
func (r *Run) SetRequireReviewBeforeUpload(v bool) { r.requireReviewBeforeUpload = v }

func (r *Run) findStepIndex(t StepType) int {
	for i, s := range r.steps {
		if s.stepType == t {
			return i
		}
	}
	return -1
}

// RecordScriptCompleted closes the active attempt of step idx.
func (r *Run) RecordScriptCompleted(stepIdx int, scriptKey string, panels []PanelDef, cost CostInfo, _ int) error {
	step, attempt, err := r.openAttempt(stepIdx, StepScript)
	if err != nil {
		return err
	}
	if attempt.status == AttemptCompleted {
		return nil
	}
	if len(panels) == 0 {
		return fmt.Errorf("pipeline: script step produced no panels")
	}
	now := time.Now().UTC()
	outJSON, _ := json.Marshal(panels)
	attempt.outputs = outJSON
	attempt.status = AttemptCompleted
	attempt.finishedAt = &now
	attempt.panelsCompleted = 1
	attempt.costUSD += cost.TotalCostUSD
	if cost.Provider != "" {
		attempt.provider = cost.Provider
	}
	if cost.Model != "" {
		attempt.model = cost.Model
	}
	r.totalCostUSD += cost.TotalCostUSD
	r.appendCost(step.id, attempt.id, cost, now)
	if scriptKey != "" {
		r.newAssets = append(r.newAssets, NewAsset(
			r.id, step.id, attempt.id, AssetScriptJSON, "", scriptKey, "application/json", 0,
		))
	}
	return r.advance(stepIdx)
}

// RecordImageCompleted updates one panel's slot in the active attempt's outputs.
func (r *Run) RecordImageCompleted(stepIdx, panelIdx int, objectKey string, cost CostInfo, _ int) error {
	step, attempt, err := r.openAttempt(stepIdx, StepImage)
	if err != nil {
		return err
	}
	if attempt.status == AttemptCompleted {
		return nil
	}
	if panelIdx < 0 || panelIdx >= attempt.panelsExpected {
		return ErrUnknownPanel
	}
	now := time.Now().UTC()

	var arr []map[string]any
	if len(attempt.outputs) > 0 {
		_ = json.Unmarshal(attempt.outputs, &arr)
	}
	for len(arr) <= panelIdx {
		arr = append(arr, map[string]any{"index": len(arr), "object_key": ""})
	}
	if existing, _ := arr[panelIdx]["object_key"].(string); existing != "" && existing == objectKey {
		return nil
	}
	arr[panelIdx] = map[string]any{"index": panelIdx, "object_key": objectKey}
	attempt.outputs, _ = json.Marshal(arr)
	attempt.panelsCompleted++
	attempt.costUSD += cost.TotalCostUSD
	if cost.Provider != "" {
		attempt.provider = cost.Provider
	}
	if cost.Model != "" {
		attempt.model = cost.Model
	}
	r.totalCostUSD += cost.TotalCostUSD
	r.appendCost(step.id, attempt.id, cost, now)
	if objectKey != "" {
		r.newAssets = append(r.newAssets, NewAsset(
			r.id, step.id, attempt.id, AssetPanelImage, "", objectKey, "image/png", 0,
		))
	}
	if attempt.panelsCompleted >= attempt.panelsExpected {
		attempt.status = AttemptCompleted
		attempt.finishedAt = &now
		return r.advance(stepIdx)
	}
	// Sequential ref modes: emit the next panel's ImageRequested with refs.
	cfg := r.configSnapshot[stepIdx]
	mode := styleRefMode(cfg.Params)
	if mode == "anchor" || mode == "previous" {
		nextIdx := attempt.panelsCompleted
		if nextIdx >= attempt.panelsExpected {
			return nil
		}
		var inputPanels struct {
			Panels []PanelDef `json:"panels"`
		}
		_ = json.Unmarshal(attempt.input, &inputPanels)
		if nextIdx >= len(inputPanels.Panels) {
			return nil
		}
		nextP := inputPanels.Panels[nextIdx]

		var refs []string
		switch mode {
		case "anchor":
			if k, _ := arr[0]["object_key"].(string); k != "" {
				refs = []string{k}
			}
		case "previous":
			for i := range nextIdx {
				if i < len(arr) {
					if k, _ := arr[i]["object_key"].(string); k != "" {
						refs = append(refs, k)
					}
				}
			}
		}

		outputKey := fmt.Sprintf("runs/%s/%d/v%d/panel-%d.png", r.id, stepIdx, step.currentVersion, nextP.Index)
		overrides := panelOverrides(cfg.Params)
		model, prompt := applyPanelOverride(cfg.Model, nextP.Prompt, overrides[nextP.Index])
		prompt = applyFormatPromptCues(prompt, cfg.Params)
		prompt = composePromptWithLinks(prompt, r.linked)
		// Character/environment refs always come first; style anchor refs after.
		mergedRefs := append([]string{}, r.linkedRefKeys()...)
		mergedRefs = append(mergedRefs, refs...)
		r.Record(ImageRequested{
			BaseEvent:     shared.BaseEvent{ID: r.id.String(), Occurred: now},
			RunID:         r.id.String(),
			StepIndex:     stepIdx,
			StepID:        step.id.String(),
			AttemptID:     attempt.id.String(),
			PanelIndex:    nextP.Index,
			Prompt:        prompt,
			Caption:       nextP.Caption,
			Model:         model,
			Provider:      cfg.Provider,
			Params:        cfg.Params,
			OutputKey:     outputKey,
			RefObjectKeys: mergedRefs,
		})
	}
	return nil
}

// RecordAssembleCompleted finishes the assemble step's active attempt.
func (r *Run) RecordAssembleCompleted(stepIdx int, objectKey string, cost CostInfo, _ int) error {
	step, attempt, err := r.openAttempt(stepIdx, StepAssemble)
	if err != nil {
		return err
	}
	if attempt.status == AttemptCompleted {
		return nil
	}
	r.closeAttempt(step, attempt, objectKey, AssetVideo, "video/mp4", cost)
	return r.advance(stepIdx)
}

// RecordAudioCompleted finishes the audio step's active attempt.
func (r *Run) RecordAudioCompleted(stepIdx int, objectKey string, cost CostInfo, _ int) error {
	step, attempt, err := r.openAttempt(stepIdx, StepAudio)
	if err != nil {
		return err
	}
	if attempt.status == AttemptCompleted {
		return nil
	}
	r.closeAttempt(step, attempt, objectKey, AssetAudio, "audio/mpeg", cost)
	return r.advance(stepIdx)
}

// RecordMusicCompleted finishes the music step's active attempt.
func (r *Run) RecordMusicCompleted(stepIdx int, objectKey string, cost CostInfo, _ int) error {
	step, attempt, err := r.openAttempt(stepIdx, StepMusic)
	if err != nil {
		return err
	}
	if attempt.status == AttemptCompleted {
		return nil
	}
	r.closeAttempt(step, attempt, objectKey, AssetMusic, "audio/mpeg", cost)
	return r.advance(stepIdx)
}

// RecordUploadCompleted finishes the upload step's active attempt.
func (r *Run) RecordUploadCompleted(stepIdx int, externalRef string, cost CostInfo, _ int) error {
	step, attempt, err := r.openAttempt(stepIdx, StepUpload)
	if err != nil {
		return err
	}
	if attempt.status == AttemptCompleted {
		return nil
	}
	now := time.Now().UTC()
	outJSON, _ := json.Marshal([]map[string]any{{"external_ref": externalRef}})
	attempt.outputs = outJSON
	attempt.status = AttemptCompleted
	attempt.finishedAt = &now
	attempt.panelsCompleted = 1
	attempt.costUSD += cost.TotalCostUSD
	if cost.Provider != "" {
		attempt.provider = cost.Provider
	}
	r.totalCostUSD += cost.TotalCostUSD
	r.appendCost(step.id, attempt.id, cost, now)
	return r.advance(stepIdx)
}

// RecordStepFailed terminates the run.
func (r *Run) RecordStepFailed(stepIdx int, errMsg string) error {
	if stepIdx < 0 || stepIdx >= len(r.steps) {
		return ErrStepIndexMismatch
	}
	if r.status != RunStatusRunning {
		return ErrRunNotRunning
	}
	step := r.steps[stepIdx]
	attempt := step.ActiveAttempt()
	if attempt == nil {
		return fmt.Errorf("pipeline: failure on step %d with no active attempt", stepIdx)
	}
	now := time.Now().UTC()
	attempt.status = AttemptFailed
	attempt.errorMsg = errMsg
	attempt.finishedAt = &now
	r.status = RunStatusFailed
	r.errorMsg = errMsg
	r.finishedAt = &now
	r.Record(RunFailed{
		BaseEvent: shared.BaseEvent{ID: r.id.String(), Occurred: now},
		RunID:     r.id.String(),
		Error:     errMsg,
	})
	return nil
}

// Cancel transitions to cancelled (only allowed while running/queued).
func (r *Run) Cancel() error {
	if r.status != RunStatusRunning && r.status != RunStatusQueued && r.status != RunStatusAwaitingAction {
		return errors.New("pipeline: only running/queued/awaiting_action runs can be cancelled")
	}
	now := time.Now().UTC()
	r.status = RunStatusCancelled
	r.finishedAt = &now
	r.Record(RunCancelled{
		BaseEvent: shared.BaseEvent{ID: r.id.String(), Occurred: now},
		RunID:     r.id.String(),
	})
	return nil
}

func (r *Run) advance(fromIdx int) error {
	if r.maxCostUSD > 0 && r.totalCostUSD > r.maxCostUSD {
		now := time.Now().UTC()
		r.status = RunStatusFailed
		r.finishedAt = &now
		r.errorMsg = fmt.Sprintf("cost cap exceeded: spent $%.4f > cap $%.4f", r.totalCostUSD, r.maxCostUSD)
		r.Record(RunFailed{
			BaseEvent: shared.BaseEvent{ID: r.id.String(), Occurred: now},
			RunID:     r.id.String(),
			Error:     r.errorMsg,
		})
		return nil
	}
	// Manual cascade gate: when the step that just finished was a regenerate
	// (current_version > 1 — its first attempt would've been v1), park the
	// run in awaiting_action instead of auto-cascading. The user re-runs
	// downstream explicitly. Downstream stays flagged stale.
	if fromIdx >= 0 && fromIdx < len(r.steps) && r.steps[fromIdx].currentVersion > 1 {
		anyDownstreamStale := false
		for j := fromIdx + 1; j < len(r.steps); j++ {
			if r.steps[j].isStale {
				anyDownstreamStale = true
				break
			}
		}
		if anyDownstreamStale {
			now := time.Now().UTC()
			r.status = RunStatusAwaitingAction
			r.finishedAt = &now
			return nil
		}
	}
	next := fromIdx + 1
	if next >= len(r.steps) {
		now := time.Now().UTC()
		r.status = RunStatusCompleted
		r.finishedAt = &now
		r.Record(RunCompleted{
			BaseEvent:    shared.BaseEvent{ID: r.id.String(), Occurred: now},
			RunID:        r.id.String(),
			TotalCostUSD: r.totalCostUSD,
		})
		return nil
	}
	// Auto-assemble gate: when disabled, pause at the assemble step instead
	// of firing its request. The user calls RequestAssemble to resume.
	if !r.autoAssemble && r.steps[next].stepType == StepAssemble {
		now := time.Now().UTC()
		r.status = RunStatusAwaitingAction
		r.finishedAt = &now
		return nil
	}
	// Review gate before upload: when project review_mode = review, pause so
	// the user can edit metadata + approve. ResumeFromReview re-enters here.
	if r.requireReviewBeforeUpload && r.steps[next].stepType == StepUpload {
		now := time.Now().UTC()
		r.status = RunStatusAwaitingAction
		r.finishedAt = &now
		return nil
	}
	return r.requestStep(next, nil)
}

// openAttempt looks up the active attempt of step idx + validates the type.
// Caller is responsible for the run status check; many flows want to allow
// completions to land even on completed/cancelled runs (idempotent re-deliveries).
func (r *Run) openAttempt(stepIdx int, expected StepType) (*Step, *StepAttempt, error) {
	if stepIdx < 0 || stepIdx >= len(r.steps) {
		return nil, nil, ErrStepIndexMismatch
	}
	step := r.steps[stepIdx]
	if step.stepType != expected {
		return nil, nil, ErrStepTypeMismatch
	}
	if r.status != RunStatusRunning {
		return nil, nil, ErrRunNotRunning
	}
	a := step.ActiveAttempt()
	if a == nil {
		return nil, nil, fmt.Errorf("pipeline: step %d has no active attempt", stepIdx)
	}
	return step, a, nil
}

// closeAttempt marks an attempt completed with a single-output payload and
// records the asset + cost. Shared by assemble / audio / music.
func (r *Run) closeAttempt(step *Step, attempt *StepAttempt, objectKey string, kind AssetKind, mime string, cost CostInfo) {
	now := time.Now().UTC()
	outJSON, _ := json.Marshal([]map[string]any{{"object_key": objectKey}})
	attempt.outputs = outJSON
	attempt.status = AttemptCompleted
	attempt.finishedAt = &now
	attempt.panelsCompleted = 1
	attempt.costUSD += cost.TotalCostUSD
	if cost.Provider != "" {
		attempt.provider = cost.Provider
	}
	if cost.Model != "" {
		attempt.model = cost.Model
	}
	r.totalCostUSD += cost.TotalCostUSD
	r.appendCost(step.id, attempt.id, cost, now)
	if objectKey != "" {
		r.newAssets = append(r.newAssets, NewAsset(
			r.id, step.id, attempt.id, kind, "", objectKey, mime, 0,
		))
	}
}

// SubtitlesConfig is parsed from assemble step params; controls per-panel
// caption rendering in the renderer.
type SubtitlesConfig struct {
	Enabled  bool
	Style    string // bottom_karaoke | impact_meme | word_pop
	Position string // bottom | top | center
}

func subtitlesConfig(params map[string]any) SubtitlesConfig {
	out := SubtitlesConfig{Style: "bottom_karaoke", Position: "bottom"}
	if params == nil {
		return out
	}
	s, ok := params["subtitles"].(map[string]any)
	if !ok {
		return out
	}
	if v, ok := s["enabled"].(bool); ok {
		out.Enabled = v
	}
	if v, ok := s["style"].(string); ok && v != "" {
		out.Style = v
	}
	if v, ok := s["position"].(string); ok && v != "" {
		out.Position = v
	}
	return out
}

func (r *Run) scriptPanels() ([]PanelDef, error) {
	for i := r.currentStepIndex - 1; i >= 0; i-- {
		s := r.steps[i]
		if s.stepType != StepScript {
			continue
		}
		a := s.ActiveAttempt()
		if a == nil || a.status != AttemptCompleted {
			continue
		}
		var panels []PanelDef
		if err := json.Unmarshal(a.outputs, &panels); err != nil {
			return nil, fmt.Errorf("pipeline: parse script outputs: %w", err)
		}
		for i := range panels {
			if panels[i].Index == 0 && i != 0 {
				panels[i].Index = i
			}
		}
		return panels, nil
	}
	return nil, errors.New("pipeline: no completed script step before image step")
}

func (r *Run) assemblePanels(cfg StepConfig) ([]AssemblePanelRef, error) {
	// Pull script panels for caption text fallback (when timeline editor hasn't
	// supplied per-panel captions and subtitles are enabled in cfg.Params).
	scriptPanels, _ := r.scriptPanels()
	captionTextByIndex := map[int]string{}
	for _, p := range scriptPanels {
		captionTextByIndex[p.Index] = p.Caption
	}
	subtitlesCfg := subtitlesConfig(cfg.Params)
	for i := r.currentStepIndex - 1; i >= 0; i-- {
		s := r.steps[i]
		if s.stepType != StepImage {
			continue
		}
		a := s.ActiveAttempt()
		if a == nil || a.status != AttemptCompleted {
			continue
		}
		var arr []map[string]any
		if err := json.Unmarshal(a.outputs, &arr); err != nil {
			return nil, fmt.Errorf("pipeline: parse image outputs: %w", err)
		}
		dur, transition := assembleDefaults(cfg.Params)
		// timeline.panels is an optional rich override keyed by panel index.
		// Built by the timeline editor; pipes through to the renderer per panel.
		timeline := timelinePanelsByIndex(cfg.Params)
		refs := make([]AssemblePanelRef, 0, len(arr))
		for _, entry := range arr {
			key, _ := entry["object_key"].(string)
			idxF, _ := entry["index"].(float64)
			idx := int(idxF)
			ref := AssemblePanelRef{
				Index:      idx,
				ObjectKey:  key,
				DurationMs: dur,
				Transition: transition,
			}
			if tl, ok := timeline[idx]; ok {
				if v, ok := tl["duration_ms"].(float64); ok && v > 0 {
					ref.DurationMs = int(v)
				}
				if ti, ok := tl["transition_in"].(map[string]any); ok {
					ref.TransitionIn = ti
				}
				if eff, ok := tl["effects"].([]any); ok {
					ref.Effects = eff
				}
				if cap, ok := tl["caption"].(map[string]any); ok {
					ref.Caption = cap
				}
			}
			// Subtitle fallback: use script's panel caption if the timeline
			// editor didn't supply one and the run has subtitles enabled.
			if ref.Caption == nil && subtitlesCfg.Enabled {
				if text, ok := captionTextByIndex[idx]; ok && text != "" {
					ref.Caption = map[string]any{
						"text":         text,
						"style_preset": subtitlesCfg.Style,
						"position":     subtitlesCfg.Position,
					}
				}
			}
			refs = append(refs, ref)
		}
		// timeline.order optionally reorders panels.
		if order := timelineOrder(cfg.Params); len(order) > 0 {
			refs = reorderRefs(refs, order)
		}
		return refs, nil
	}
	return nil, errors.New("pipeline: no completed image step before assemble step")
}

// collectSFXKeys walks the assemble panel refs and pulls per-panel SFX
// object_keys out of each panel's transition_in spec. UI sets
// transition_in.sfx_object_key (resolved from the audio library picker).
func collectSFXKeys(refs []AssemblePanelRef) map[int]string {
	out := map[int]string{}
	for _, ref := range refs {
		if ref.TransitionIn == nil {
			continue
		}
		if k, ok := ref.TransitionIn["sfx_object_key"].(string); ok && k != "" {
			out[ref.Index] = k
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (r *Run) audioInputs(_ StepConfig) ([]string, string, error) {
	for i := r.currentStepIndex - 1; i >= 0; i-- {
		s := r.steps[i]
		if s.stepType != StepScript {
			continue
		}
		a := s.ActiveAttempt()
		if a == nil || a.status != AttemptCompleted {
			continue
		}
		var panels []PanelDef
		if err := json.Unmarshal(a.outputs, &panels); err == nil && len(panels) > 0 {
			caps := make([]string, 0, len(panels))
			for _, p := range panels {
				if p.Caption != "" {
					caps = append(caps, p.Caption)
				}
			}
			return caps, r.prompt, nil
		}
	}
	return nil, r.prompt, nil
}

// priorCaptions walks back through completed caption steps and returns the
// map captured by the most recent one. Empty when no caption step ran.
func (r *Run) priorCaptions() map[string]any {
	for i := r.currentStepIndex - 1; i >= 0; i-- {
		s := r.steps[i]
		if s.stepType != StepCaption {
			continue
		}
		a := s.ActiveAttempt()
		if a == nil || a.status != AttemptCompleted {
			continue
		}
		var arr []map[string]any
		if err := json.Unmarshal(a.outputs, &arr); err == nil && len(arr) > 0 {
			if m, ok := arr[0]["captions"].(map[string]any); ok {
				return m
			}
		}
	}
	return map[string]any{}
}

// RecordCaptionCompleted closes the caption step with the per-platform map.
func (r *Run) RecordCaptionCompleted(stepIdx int, captions map[string]any, cost CostInfo, _ int) error {
	step, attempt, err := r.openAttempt(stepIdx, StepCaption)
	if err != nil {
		return err
	}
	if attempt.status == AttemptCompleted {
		return nil
	}
	now := time.Now().UTC()
	outJSON, _ := json.Marshal([]map[string]any{{"captions": captions}})
	attempt.outputs = outJSON
	attempt.status = AttemptCompleted
	attempt.finishedAt = &now
	attempt.panelsCompleted = 1
	attempt.costUSD += cost.TotalCostUSD
	if cost.Provider != "" {
		attempt.provider = cost.Provider
	}
	if cost.Model != "" {
		attempt.model = cost.Model
	}
	r.totalCostUSD += cost.TotalCostUSD
	r.appendCost(step.id, attempt.id, cost, now)
	return r.advance(stepIdx)
}

func (r *Run) priorVideoKey() (string, error) {
	for i := r.currentStepIndex - 1; i >= 0; i-- {
		s := r.steps[i]
		if s.stepType != StepAssemble {
			continue
		}
		a := s.ActiveAttempt()
		if a == nil || a.status != AttemptCompleted {
			continue
		}
		var arr []map[string]any
		if err := json.Unmarshal(a.outputs, &arr); err != nil {
			return "", err
		}
		if len(arr) > 0 {
			if k, _ := arr[0]["object_key"].(string); k != "" {
				return k, nil
			}
		}
	}
	return "", errors.New("pipeline: no completed assemble step before upload step")
}

func (r *Run) priorMusicKey() string {
	for i := r.currentStepIndex - 1; i >= 0; i-- {
		s := r.steps[i]
		if s.stepType != StepMusic {
			continue
		}
		a := s.ActiveAttempt()
		if a == nil || a.status != AttemptCompleted {
			continue
		}
		var arr []map[string]any
		if err := json.Unmarshal(a.outputs, &arr); err == nil && len(arr) > 0 {
			if k, _ := arr[0]["object_key"].(string); k != "" {
				return k
			}
		}
	}
	return ""
}

func (r *Run) priorAudioKey() string {
	for i := r.currentStepIndex - 1; i >= 0; i-- {
		s := r.steps[i]
		if s.stepType != StepAudio {
			continue
		}
		a := s.ActiveAttempt()
		if a == nil || a.status != AttemptCompleted {
			continue
		}
		var arr []map[string]any
		if err := json.Unmarshal(a.outputs, &arr); err == nil && len(arr) > 0 {
			if k, _ := arr[0]["object_key"].(string); k != "" {
				return k
			}
		}
	}
	return ""
}

func (r *Run) appendCost(stepID StepID, attemptID AttemptID, cost CostInfo, occurred time.Time) {
	if cost.TotalCostUSD == 0 && cost.Units == 0 {
		return
	}
	r.newCosts = append(r.newCosts, CostEntry{
		ID:           uuidString(),
		RunID:        r.id,
		StepID:       stepID,
		AttemptID:    attemptID,
		Provider:     cost.Provider,
		Model:        cost.Model,
		Units:        cost.Units,
		UnitLabel:    cost.UnitLabel,
		UnitCostUSD:  cost.UnitCostUSD,
		TotalCostUSD: cost.TotalCostUSD,
		OccurredAt:   occurred,
	})
}

// --- accessors ---

func (r *Run) ID() RunID                    { return r.id }
func (r *Run) TemplateID() TemplateID       { return r.templateID }
func (r *Run) Prompt() string               { return r.prompt }
func (r *Run) Status() RunStatus            { return r.status }
func (r *Run) CurrentStepIndex() int        { return r.currentStepIndex }
func (r *Run) TotalCostUSD() float64        { return r.totalCostUSD }
func (r *Run) MaxCostUSD() float64          { return r.maxCostUSD }
func (r *Run) AutoAssemble() bool           { return r.autoAssemble }
func (r *Run) Error() string                { return r.errorMsg }
func (r *Run) CreatedAt() time.Time         { return r.createdAt }
func (r *Run) StartedAt() *time.Time        { return r.startedAt }
func (r *Run) FinishedAt() *time.Time       { return r.finishedAt }
func (r *Run) Steps() []*Step               { return r.steps }
func (r *Run) ExpectedSteps() int           { return len(r.steps) }
func (r *Run) ConfigSnapshot() []StepConfig { return r.configSnapshot }
func (r *Run) NewAssets() []Asset           { return r.newAssets }
func (r *Run) NewCosts() []CostEntry        { return r.newCosts }

func (r *Run) ResetSideEffects() {
	r.newAssets = nil
	r.newCosts = nil
}

// FillEmptyBucketsOnNewAssets stamps the given bucket on any newly-recorded
// asset whose Bucket is empty. Used by the command layer to capture the
// worker's actual bucket — the aggregate itself doesn't know it.
func (r *Run) FillEmptyBucketsOnNewAssets(bucket string) {
	if bucket == "" {
		return
	}
	for i := range r.newAssets {
		if r.newAssets[i].Bucket == "" {
			r.newAssets[i].Bucket = bucket
		}
	}
}

// ReconstituteRun rebuilds a Run from storage without emitting events.
func ReconstituteRun(
	id RunID, templateID TemplateID, prompt string,
	config []StepConfig, status RunStatus,
	currentStepIndex int, totalCostUSD float64, errMsg string,
	createdAt time.Time, startedAt, finishedAt *time.Time,
	steps []*Step,
) *Run {
	return ReconstituteRunFull(id, templateID, prompt, config, true, status,
		currentStepIndex, totalCostUSD, 0, errMsg,
		createdAt, startedAt, finishedAt, steps)
}

func ReconstituteRunWithCap(
	id RunID, templateID TemplateID, prompt string,
	config []StepConfig, status RunStatus,
	currentStepIndex int, totalCostUSD, maxCostUSD float64, errMsg string,
	createdAt time.Time, startedAt, finishedAt *time.Time,
	steps []*Step,
) *Run {
	return ReconstituteRunFull(id, templateID, prompt, config, true, status,
		currentStepIndex, totalCostUSD, maxCostUSD, errMsg,
		createdAt, startedAt, finishedAt, steps)
}

func ReconstituteRunFull(
	id RunID, templateID TemplateID, prompt string,
	config []StepConfig, autoAssemble bool, status RunStatus,
	currentStepIndex int, totalCostUSD, maxCostUSD float64, errMsg string,
	createdAt time.Time, startedAt, finishedAt *time.Time,
	steps []*Step,
) *Run {
	return &Run{
		id: id, templateID: templateID, prompt: prompt,
		configSnapshot: config, autoAssemble: autoAssemble, status: status,
		currentStepIndex: currentStepIndex, totalCostUSD: totalCostUSD,
		maxCostUSD: maxCostUSD,
		errorMsg:   errMsg, createdAt: createdAt,
		startedAt: startedAt, finishedAt: finishedAt,
		steps: steps,
	}
}
