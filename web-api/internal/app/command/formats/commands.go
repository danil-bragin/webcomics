// Package formats holds write-side commands for format presets (visual style
// recipes that drive image prompt prefix/suffix + script suffix + render dims
// + image model).
package formats

import (
	"context"
	"errors"
	"strings"

	"github.com/example/dddcqrs/internal/app/bus"
	"github.com/example/dddcqrs/internal/domain/formats"
	"github.com/example/dddcqrs/internal/infrastructure/persistence/uow"
)

type SaveFormat struct {
	ID                 string // empty → generate from name slug
	Name               string
	Description        string
	Scope              string // user (created via UI) | system (curated)
	Icon               string
	ScriptSystemSuffix string
	ImagePromptPrefix  string
	ImagePromptSuffix  string
	ImageModel         string
	StyleReference     string
	FPS                int
	Width              int
	Height             int
	Codec              string
	PanelDurationMs    int
	Transition         string
}

func (SaveFormat) IsCommand() {}

type SaveFormatResult struct{ ID string }

type SaveFormatHandler struct{ uow uow.Manager }

func NewSaveFormatHandler(m uow.Manager) *SaveFormatHandler {
	return &SaveFormatHandler{uow: m}
}

func (h *SaveFormatHandler) Handle(ctx context.Context, cmd SaveFormat) (SaveFormatResult, error) {
	var out SaveFormatResult
	if strings.TrimSpace(cmd.Name) == "" {
		return out, errors.New("format: name empty")
	}
	id := cmd.ID
	if id == "" {
		id = slugify(cmd.Name)
	}
	scope := cmd.Scope
	if scope == "" {
		scope = "user"
	}
	f := &formats.Format{
		ID: id, Name: cmd.Name, Description: cmd.Description, Scope: scope,
		Icon: cmd.Icon, ScriptSystemSuffix: cmd.ScriptSystemSuffix,
		ImagePromptPrefix: cmd.ImagePromptPrefix, ImagePromptSuffix: cmd.ImagePromptSuffix,
		ImageModel: cmd.ImageModel, StyleReference: cmd.StyleReference,
		FPS: cmd.FPS, Width: cmd.Width, Height: cmd.Height, Codec: cmd.Codec,
		PanelDurationMs: cmd.PanelDurationMs, Transition: cmd.Transition,
	}
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		return u.Repositories().Formats().Save(ctx, f)
	})
	if err != nil {
		return out, err
	}
	out.ID = id
	return out, nil
}

type DeleteFormat struct{ ID string }

func (DeleteFormat) IsCommand() {}

type DeleteFormatResult struct{}

type DeleteFormatHandler struct{ uow uow.Manager }

func NewDeleteFormatHandler(m uow.Manager) *DeleteFormatHandler {
	return &DeleteFormatHandler{uow: m}
}

func (h *DeleteFormatHandler) Handle(ctx context.Context, cmd DeleteFormat) (DeleteFormatResult, error) {
	return DeleteFormatResult{}, h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		return u.Repositories().Formats().Delete(ctx, cmd.ID)
	})
}

func SaveFormatOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[SaveFormat, SaveFormatResult](r, NewSaveFormatHandler(m))
}
func DeleteFormatOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[DeleteFormat, DeleteFormatResult](r, NewDeleteFormatHandler(m))
}

func slugify(s string) string {
	out := []byte{}
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out = append(out, byte(r))
		case r == ' ' || r == '_' || r == '-':
			out = append(out, '-')
		}
	}
	if len(out) == 0 {
		return "untitled"
	}
	return string(out)
}
