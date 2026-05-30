package pipeline

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type StepType string

const (
	StepScript   StepType = "script"
	StepImage    StepType = "image"
	StepAudio    StepType = "audio"
	StepMusic    StepType = "music"
	StepAssemble StepType = "assemble"
	StepCaption  StepType = "caption"
	StepUpload   StepType = "upload"
)

var (
	ErrTemplateNameEmpty = errors.New("pipeline: template name empty")
	ErrTemplateNoSteps   = errors.New("pipeline: template has no steps")
	ErrUnknownStepType   = errors.New("pipeline: unknown step type")
)

// StepConfig is a step definition inside a template. Stored as one element of
// the `steps` jsonb array. Params is a free-form map — kept stringly to keep
// the domain free of provider-specific types.
type StepConfig struct {
	Type         StepType       `json:"type"`
	SystemPrompt string         `json:"system_prompt,omitempty"`
	Model        string         `json:"model,omitempty"`
	Provider     string         `json:"provider,omitempty"`
	Params       map[string]any `json:"params,omitempty"`
}

func (c StepConfig) validate() error {
	switch c.Type {
	case StepScript, StepImage, StepAudio, StepMusic, StepAssemble, StepCaption, StepUpload:
		return nil
	default:
		return ErrUnknownStepType
	}
}

// PanelCount returns the configured panel count for image steps, or 0 if not
// set. Read from Params["panel_count"] when the step is being instantiated.
// For script steps this is the *hint* sent to the LLM.
func (c StepConfig) PanelCount() int {
	if c.Params == nil {
		return 0
	}
	switch v := c.Params["panel_count"].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

// Template (UI-facing name: "Preset") is the aggregate root for pipeline
// presets. Simple CRUD, no event sourcing.
//
// Discoverability fields (description / category / icon / samplePrompts /
// formatID / defaults) drive the Presets marketplace UI so users can pick a
// starting point without reading raw step JSON.
type Template struct {
	id             TemplateID
	name           string
	description    string
	category       string // meme | shorts | story | demo | custom
	icon           string // single emoji
	samplePrompts  []string
	formatID       string
	defaults       map[string]any
	steps          []StepConfig
	maxCostUSD     float64
	isTest         bool
	createdAt      time.Time
	updatedAt      time.Time
}

// NewTemplate creates a template. maxCostUSD = 0 disables the cap.
func NewTemplate(name string, steps []StepConfig) (*Template, error) {
	return NewTemplateWithCap(name, steps, 0)
}

func NewTemplateWithCap(name string, steps []StepConfig, maxCostUSD float64) (*Template, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrTemplateNameEmpty
	}
	if len(steps) == 0 {
		return nil, ErrTemplateNoSteps
	}
	for _, s := range steps {
		if err := s.validate(); err != nil {
			return nil, err
		}
	}
	if maxCostUSD < 0 {
		maxCostUSD = 0
	}
	now := time.Now().UTC()
	return &Template{
		id:         NewTemplateID(),
		name:       name,
		steps:      steps,
		maxCostUSD: maxCostUSD,
		createdAt:  now,
		updatedAt:  now,
	}, nil
}

func (t *Template) UpdateName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrTemplateNameEmpty
	}
	t.name = name
	t.updatedAt = time.Now().UTC()
	return nil
}

func (t *Template) UpdateSteps(steps []StepConfig) error {
	if len(steps) == 0 {
		return ErrTemplateNoSteps
	}
	for _, s := range steps {
		if err := s.validate(); err != nil {
			return err
		}
	}
	t.steps = steps
	t.updatedAt = time.Now().UTC()
	return nil
}

func (t *Template) ID() TemplateID       { return t.id }
func (t *Template) Name() string         { return t.name }
func (t *Template) Description() string  { return t.description }
func (t *Template) Category() string {
	if t.category == "" {
		return "custom"
	}
	return t.category
}
func (t *Template) Icon() string             { return t.icon }
func (t *Template) SamplePrompts() []string  { return append([]string{}, t.samplePrompts...) }
func (t *Template) FormatID() string         { return t.formatID }
func (t *Template) Defaults() map[string]any { return t.defaults }
func (t *Template) IsTest() bool             { return t.isTest }
func (t *Template) Steps() []StepConfig      { return t.steps }
func (t *Template) MaxCostUSD() float64      { return t.maxCostUSD }
func (t *Template) CreatedAt() time.Time     { return t.createdAt }
func (t *Template) UpdatedAt() time.Time     { return t.updatedAt }

// Setters for the new metadata fields. Idempotent — empty/nil clears.
func (t *Template) SetDescription(d string) {
	t.description = strings.TrimSpace(d)
	t.updatedAt = time.Now().UTC()
}
func (t *Template) SetCategory(c string) {
	switch c {
	case "meme", "shorts", "story", "demo", "custom":
		t.category = c
		t.updatedAt = time.Now().UTC()
	}
}
func (t *Template) SetIcon(emoji string) {
	t.icon = strings.TrimSpace(emoji)
	t.updatedAt = time.Now().UTC()
}
func (t *Template) SetSamplePrompts(p []string) {
	t.samplePrompts = append([]string{}, p...)
	t.updatedAt = time.Now().UTC()
}
func (t *Template) SetFormatID(id string) {
	t.formatID = strings.TrimSpace(id)
	t.updatedAt = time.Now().UTC()
}
func (t *Template) SetDefaults(d map[string]any) {
	if d == nil {
		d = map[string]any{}
	}
	t.defaults = d
	t.updatedAt = time.Now().UTC()
}
func (t *Template) SetIsTest(v bool) {
	t.isTest = v
	t.updatedAt = time.Now().UTC()
}

func (t *Template) SetMaxCostUSD(c float64) {
	if c < 0 {
		c = 0
	}
	t.maxCostUSD = c
	t.updatedAt = time.Now().UTC()
}

func ReconstituteTemplate(id TemplateID, name string, steps []StepConfig, createdAt, updatedAt time.Time) *Template {
	return &Template{id: id, name: name, steps: steps, createdAt: createdAt, updatedAt: updatedAt, defaults: map[string]any{}}
}

func ReconstituteTemplateWithCap(id TemplateID, name string, steps []StepConfig, maxCostUSD float64, createdAt, updatedAt time.Time) *Template {
	return &Template{id: id, name: name, steps: steps, maxCostUSD: maxCostUSD, createdAt: createdAt, updatedAt: updatedAt, defaults: map[string]any{}}
}

// ReconstituteTemplateFull is the canonical loader used by the read/write
// repos now that templates carry rich metadata.
func ReconstituteTemplateFull(
	id TemplateID, name, description, category, icon string,
	steps []StepConfig, samplePrompts []string, formatID string,
	defaults map[string]any, maxCostUSD float64, isTest bool,
	createdAt, updatedAt time.Time,
) *Template {
	if defaults == nil {
		defaults = map[string]any{}
	}
	if samplePrompts == nil {
		samplePrompts = []string{}
	}
	return &Template{
		id: id, name: name, description: description, category: category,
		icon: icon, samplePrompts: samplePrompts, formatID: formatID,
		defaults: defaults, steps: steps, maxCostUSD: maxCostUSD, isTest: isTest,
		createdAt: createdAt, updatedAt: updatedAt,
	}
}

// StepsJSON marshals the steps for storage. Helper for the repo.
func (t *Template) StepsJSON() ([]byte, error) { return json.Marshal(t.steps) }

func UnmarshalSteps(raw []byte) ([]StepConfig, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var s []StepConfig
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	return s, nil
}
