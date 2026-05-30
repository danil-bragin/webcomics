package pipeline

import (
	"encoding/json"

	"github.com/example/dddcqrs/internal/domain/shared"
)

// PanelDef is one panel definition emitted by the script step + consumed by
// image-request events. Kept here (not in transport) because it crosses the
// outbox payload boundary.
type PanelDef struct {
	Index   int    `json:"index"`
	Prompt  string `json:"prompt"`
	Caption string `json:"caption,omitempty"`
}

// AssemblePanelRef tells the renderer which key to read and how long to show it.
// Transition is the legacy string form ("crossfade"|"slide"|"none"). TransitionIn,
// Effects and Caption come from the timeline editor and override the legacy field.
type AssemblePanelRef struct {
	Index        int            `json:"index"`
	ObjectKey    string         `json:"object_key"`
	DurationMs   int            `json:"duration_ms"`
	Transition   string         `json:"transition,omitempty"`
	TransitionIn map[string]any `json:"transition_in,omitempty"`
	Effects      []any          `json:"effects,omitempty"`
	Caption      map[string]any `json:"caption,omitempty"`
}

// --- step request events (Go → workers/renderer) ---

// ScriptRequested asks the LLM worker to generate the panel script.
type ScriptRequested struct {
	shared.BaseEvent
	RunID        string         `json:"run_id"`
	StepIndex    int            `json:"step_index"`
	StepID       string         `json:"step_id"`
	AttemptID    string         `json:"attempt_id"`
	Prompt       string         `json:"prompt"`
	SystemPrompt string         `json:"system_prompt,omitempty"`
	Model        string         `json:"model,omitempty"`
	Provider     string         `json:"provider,omitempty"`
	Language     string         `json:"language,omitempty"` // en|ru|fr
	Params       map[string]any `json:"params,omitempty"`
	// Project-linked data. Empty when run has no project / no linkage.
	Characters   []CharacterContext   `json:"characters,omitempty"`
	Environments []EnvironmentContext `json:"environments,omitempty"`
	Plot         *PlotContext         `json:"plot,omitempty"`
}

func (ScriptRequested) EventName() string { return "pipeline.script.requested" }

// ImageRequested asks the image worker to generate ONE panel.
// The aggregate emits one event per panel — natural fan-out across workers
// when style_reference="none", or sequentially when "anchor"/"previous".
type ImageRequested struct {
	shared.BaseEvent
	RunID      string         `json:"run_id"`
	StepIndex  int            `json:"step_index"`
	StepID     string         `json:"step_id"`
	AttemptID  string         `json:"attempt_id"`
	PanelIndex int            `json:"panel_index"`
	Prompt     string         `json:"prompt"`
	Caption    string         `json:"caption,omitempty"`
	Model      string         `json:"model,omitempty"`
	Provider   string         `json:"provider,omitempty"`
	Params     map[string]any `json:"params,omitempty"`
	OutputKey  string         `json:"output_key"`
	// RefObjectKeys are MinIO keys of prior panels used as style references.
	// Empty for the first panel or for "none" mode.
	RefObjectKeys []string `json:"ref_object_keys,omitempty"`
}

func (ImageRequested) EventName() string { return "pipeline.image.requested" }

// AssembleRequested asks the renderer to combine panel images into one MP4.
type AssembleRequested struct {
	shared.BaseEvent
	RunID      string             `json:"run_id"`
	StepIndex  int                `json:"step_index"`
	StepID     string             `json:"step_id"`
	AttemptID  string             `json:"attempt_id"`
	Panels     []AssemblePanelRef `json:"panels"`
	Width      int                `json:"width"`
	Height     int                `json:"height"`
	FPS        int                `json:"fps"`
	OutputKey  string             `json:"output_key"`
	AudioKey   string             `json:"audio_key,omitempty"`
	MusicKey   string             `json:"music_key,omitempty"`
	AmbientKey string             `json:"ambient_key,omitempty"`
	// SFXKeys maps panel_index → SFX object_key. Mixed at the start of each
	// panel slot (volume 0.7 by default).
	SFXKeys map[int]string `json:"sfx_keys,omitempty"`
	Params  map[string]any `json:"params,omitempty"`
}

// AudioRequested asks an audio worker to produce a soundtrack (TTS over the
// captions, generated music, etc). Output is one audio file referenced from
// the next assemble step's AudioKey.
type AudioRequested struct {
	shared.BaseEvent
	RunID           string         `json:"run_id"`
	StepIndex       int            `json:"step_index"`
	StepID          string         `json:"step_id"`
	AttemptID       string         `json:"attempt_id"`
	Captions        []string       `json:"captions,omitempty"`
	Prompt          string         `json:"prompt,omitempty"`
	Model           string         `json:"model,omitempty"`
	Provider        string         `json:"provider,omitempty"`
	Language        string         `json:"language,omitempty"`
	Params          map[string]any `json:"params,omitempty"`
	OutputKey       string         `json:"output_key"`
	PanelCount      int            `json:"panel_count,omitempty"`
	PanelDurationMs int            `json:"panel_duration_ms,omitempty"`
}

func (AudioRequested) EventName() string { return "pipeline.audio.requested" }

// MusicRequested asks a music worker to produce a background music track.
// Distinct from audio (TTS). Renderer can use both audio_key and music_key
// to mix voice + background at different volumes.
type MusicRequested struct {
	shared.BaseEvent
	RunID     string         `json:"run_id"`
	StepIndex int            `json:"step_index"`
	StepID    string         `json:"step_id"`
	AttemptID string         `json:"attempt_id"`
	ProjectID string         `json:"project_id,omitempty"`
	Prompt    string         `json:"prompt,omitempty"`
	Model     string         `json:"model,omitempty"`
	Provider  string         `json:"provider,omitempty"`
	Params    map[string]any `json:"params,omitempty"`
	OutputKey string         `json:"output_key"`
}

func (MusicRequested) EventName() string { return "pipeline.music.requested" }

type MusicCompletedPayload struct {
	RunID      string   `json:"run_id"`
	StepIndex  int      `json:"step_index"`
	ObjectKey  string   `json:"object_key"`
	Bucket     string   `json:"bucket,omitempty"`
	Cost       CostInfo `json:"cost"`
	DurationMs int      `json:"duration_ms"`
}

func (AssembleRequested) EventName() string { return "pipeline.assemble.requested" }

// UploadRequested asks an upload worker to push the prior assemble step's
// video to a social destination (telegram, youtube, …). Selected by
// Provider (e.g. "telegram") + Params (e.g. {chat_id, caption}).
type UploadRequested struct {
	shared.BaseEvent
	RunID              string         `json:"run_id"`
	StepIndex          int            `json:"step_index"`
	StepID             string         `json:"step_id"`
	AttemptID          string         `json:"attempt_id"`
	VideoKey           string         `json:"video_key"`
	Provider           string         `json:"provider"`
	Params             map[string]any `json:"params,omitempty"`
	// Selenium-provider fields (resolved from SocialAccount).
	SocialAccountID    string         `json:"social_account_id,omitempty"`
	FirefoxProfilePath string         `json:"firefox_profile_path,omitempty"`
	// Composed by the caption step. Maps platform → {caption, title, hashtags}.
	Captions  map[string]any `json:"captions,omitempty"`
	ScheduledAt string       `json:"scheduled_at,omitempty"` // RFC3339, optional
}

func (UploadRequested) EventName() string { return "pipeline.upload.requested" }

// CaptionRequested asks an LLM worker to compose a social-post caption for one
// platform from the script + plot + project niche. The output lands on the
// upload step as input metadata.
type CaptionRequested struct {
	shared.BaseEvent
	RunID        string               `json:"run_id"`
	StepIndex    int                  `json:"step_index"`
	StepID       string               `json:"step_id"`
	AttemptID    string               `json:"attempt_id"`
	Prompt       string               `json:"prompt"`
	Model        string               `json:"model,omitempty"`
	Provider     string               `json:"provider,omitempty"`
	Language     string               `json:"language,omitempty"`
	Params       map[string]any       `json:"params,omitempty"`
	Panels       []PanelDef           `json:"panels,omitempty"`
	Characters   []CharacterContext   `json:"characters,omitempty"`
	Environments []EnvironmentContext `json:"environments,omitempty"`
	Plot         *PlotContext         `json:"plot,omitempty"`
	Platforms    []string             `json:"platforms,omitempty"` // e.g. ["youtube", "twitter"]
}

func (CaptionRequested) EventName() string { return "pipeline.caption.requested" }

// CaptionCompletedPayload — Python worker → Go consumer.
type CaptionCompletedPayload struct {
	RunID      string         `json:"run_id"`
	StepIndex  int            `json:"step_index"`
	Captions   map[string]any `json:"captions"` // platform → {caption, title, hashtags}
	Metadata   *CaptionMetadata `json:"metadata,omitempty"`
	Cost       CostInfo       `json:"cost"`
	DurationMs int            `json:"duration_ms"`
}

// CaptionMetadata is the structured LLM output the upload pipeline consumes.
type CaptionMetadata struct {
	Audience  CaptionAudience            `json:"audience"`
	Hook      string                     `json:"hook"`
	Platforms map[string]PlatformCaption `json:"platforms"`
}

type CaptionAudience struct {
	MadeForKids bool    `json:"made_for_kids"`
	Confidence  float64 `json:"confidence"`
	Reasoning   string  `json:"reasoning"`
}

type PlatformCaption struct {
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Caption     string   `json:"caption,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Hashtags    []string `json:"hashtags,omitempty"`
}

type UploadCompletedPayload struct {
	RunID            string            `json:"run_id"`
	StepIndex        int               `json:"step_index"`
	ExternalRef      string            `json:"external_ref"`
	ExternalID       string            `json:"video_id,omitempty"`
	SocialAccountID  string            `json:"social_account_id,omitempty"`
	FinalVisibility  string            `json:"final_visibility,omitempty"`
	ScreenshotTrail  []ScreenshotEntry `json:"screenshot_trail,omitempty"`
	Cost             CostInfo          `json:"cost"`
	DurationMs       int               `json:"duration_ms"`
}

// UploadFailedPayload extends StepFailedPayload with upload-specific diagnostics.
type UploadFailedPayload struct {
	RunID                  string            `json:"run_id"`
	StepIndex              int               `json:"step_index"`
	SocialAccountID        string            `json:"social_account_id,omitempty"`
	Error                  string            `json:"error"`
	ErrorScreenshotAssetID string            `json:"error_screenshot_asset_id,omitempty"`
	ScreenshotTrail        []ScreenshotEntry `json:"screenshot_trail,omitempty"`
	FallbackVideoURL       string            `json:"fallback_video_url,omitempty"`
	FallbackVideoID        string            `json:"fallback_video_id,omitempty"`
}

// --- run lifecycle events (informational) ---

type RunCompleted struct {
	shared.BaseEvent
	RunID        string  `json:"run_id"`
	TotalCostUSD float64 `json:"total_cost_usd"`
}

func (RunCompleted) EventName() string { return "pipeline.run.completed" }

type RunFailed struct {
	shared.BaseEvent
	RunID string `json:"run_id"`
	Error string `json:"error"`
}

func (RunFailed) EventName() string { return "pipeline.run.failed" }

// RunCancelled is broadcast so workers can drop any in-flight task whose
// run_id matches. Workers maintain a cancelled-set and skip subsequent
// messages for those runs. Side effect is best-effort; the aggregate itself
// rejects further `Record*Completed` calls because status != running.
type RunCancelled struct {
	shared.BaseEvent
	RunID string `json:"run_id"`
}

func (RunCancelled) EventName() string { return "pipeline.run.cancelled" }

// Helper for command handlers parsing inbound completion payloads.

type ScriptCompletedPayload struct {
	RunID      string     `json:"run_id"`
	StepIndex  int        `json:"step_index"`
	ScriptKey  string     `json:"script_key"`
	Bucket     string     `json:"bucket,omitempty"`
	Panels     []PanelDef `json:"panels"`
	Cost       CostInfo   `json:"cost"`
	DurationMs int        `json:"duration_ms"`
}

type ImageCompletedPayload struct {
	RunID      string   `json:"run_id"`
	StepIndex  int      `json:"step_index"`
	PanelIndex int      `json:"panel_index"`
	ObjectKey  string   `json:"object_key"`
	Bucket     string   `json:"bucket,omitempty"`
	Cost       CostInfo `json:"cost"`
	DurationMs int      `json:"duration_ms"`
}

type AudioCompletedPayload struct {
	RunID      string   `json:"run_id"`
	StepIndex  int      `json:"step_index"`
	ObjectKey  string   `json:"object_key"`
	Bucket     string   `json:"bucket,omitempty"`
	Cost       CostInfo `json:"cost"`
	DurationMs int      `json:"duration_ms"`
}

type AssembleCompletedPayload struct {
	RunID      string   `json:"run_id"`
	StepIndex  int      `json:"step_index"`
	ObjectKey  string   `json:"object_key"`
	Bucket     string   `json:"bucket,omitempty"`
	Cost       CostInfo `json:"cost"`
	DurationMs int      `json:"duration_ms"`
}

type StepFailedPayload struct {
	RunID     string `json:"run_id"`
	StepIndex int    `json:"step_index"`
	Error     string `json:"error"`
}

type CostInfo struct {
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	Units        float64 `json:"units"`
	UnitLabel    string  `json:"unit_label"`
	UnitCostUSD  float64 `json:"unit_cost_usd"`
	TotalCostUSD float64 `json:"total_cost_usd"`
}

// Ensure compile-time these are valid JSON-marshallable.
var (
	_ shared.DomainEvent = ScriptRequested{}
	_ shared.DomainEvent = ImageRequested{}
	_ shared.DomainEvent = AudioRequested{}
	_ shared.DomainEvent = AssembleRequested{}
	_ shared.DomainEvent = UploadRequested{}
	_ shared.DomainEvent = MusicRequested{}
	_ shared.DomainEvent = RunCompleted{}
	_ shared.DomainEvent = RunFailed{}
)

// internal helper — guard against accidental json import unused warning.
var _ = json.Marshal
