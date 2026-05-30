// Package formats exposes the read-model + queries for the format-presets
// marketplace. The DB is the source of truth (migrated from the old hardcoded
// domain/formats/library.go in 00018).
package formats

import (
	"context"

	"github.com/example/dddcqrs/internal/app/bus"
)

type FormatView struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Description        string `json:"description"`
	Scope              string `json:"scope"`
	Icon               string `json:"icon"`
	ScriptSystemSuffix string `json:"script_system_suffix,omitempty"`
	ImagePromptPrefix  string `json:"image_prompt_prefix,omitempty"`
	ImagePromptSuffix  string `json:"image_prompt_suffix,omitempty"`
	ImageModel         string `json:"image_model,omitempty"`
	StyleReference     string `json:"style_reference,omitempty"`
	FPS                int    `json:"fps,omitempty"`
	Width              int    `json:"width,omitempty"`
	Height             int    `json:"height,omitempty"`
	Codec              string `json:"codec,omitempty"`
	PanelDurationMs    int    `json:"panel_duration_ms,omitempty"`
	Transition         string `json:"transition,omitempty"`
	IsSystem           bool   `json:"is_system"`
	CreatedAt          string `json:"created_at,omitempty"`
	UpdatedAt          string `json:"updated_at,omitempty"`
}

type ReadModel interface {
	GetFormat(ctx context.Context, id string) (FormatView, error)
	ListFormats(ctx context.Context) ([]FormatView, error)
}

type ListFormats struct{}

func (ListFormats) IsQuery() {}

type ListFormatsHandler struct{ m ReadModel }

func (h ListFormatsHandler) Handle(ctx context.Context, _ ListFormats) ([]FormatView, error) {
	return h.m.ListFormats(ctx)
}

func ListFormatsOnBus(r *bus.Registry, m ReadModel) {
	bus.RegisterQuery[ListFormats, []FormatView](r, ListFormatsHandler{m: m})
}

type GetFormat struct{ ID string }

func (GetFormat) IsQuery() {}

type GetFormatHandler struct{ m ReadModel }

func (h GetFormatHandler) Handle(ctx context.Context, q GetFormat) (FormatView, error) {
	return h.m.GetFormat(ctx, q.ID)
}

func GetFormatOnBus(r *bus.Registry, m ReadModel) {
	bus.RegisterQuery[GetFormat, FormatView](r, GetFormatHandler{m: m})
}
