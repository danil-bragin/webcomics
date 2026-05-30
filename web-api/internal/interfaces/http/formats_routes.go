package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/example/dddcqrs/internal/app/bus"
	formatcmd "github.com/example/dddcqrs/internal/app/command/formats"
	formatq "github.com/example/dddcqrs/internal/app/query/formats"
)

// MountFormats registers the format-preset marketplace API. The DB is the
// source of truth as of migration 00018; system rows are seeded and marked
// read-only (DELETE blocked at the repo level).
func (s *Server) MountFormats(r chi.Router) {
	r.Get("/api/formats/{id}", s.GetFormat)
	r.Post("/api/formats", s.CreateFormat)
	r.Put("/api/formats/{id}", s.UpdateFormat)
	r.Delete("/api/formats/{id}", s.DeleteFormat)
	r.Post("/api/formats/preview", s.ComposeFormatPreview)
}

// ListFormats now hits the DB-backed read model. Legacy /api/formats path
// kept so existing UI clients work unchanged.
func (s *Server) ListFormats(w http.ResponseWriter, r *http.Request) {
	res, err := bus.Ask[[]formatq.FormatView](r.Context(), s.reg, formatq.ListFormats{})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) GetFormat(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	res, err := bus.Ask[formatq.FormatView](r.Context(), s.reg, formatq.GetFormat{ID: id})
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type formatBody struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Description        string `json:"description"`
	Scope              string `json:"scope"`
	Icon               string `json:"icon"`
	ScriptSystemSuffix string `json:"script_system_suffix"`
	ImagePromptPrefix  string `json:"image_prompt_prefix"`
	ImagePromptSuffix  string `json:"image_prompt_suffix"`
	ImageModel         string `json:"image_model"`
	StyleReference     string `json:"style_reference"`
	FPS                int    `json:"fps"`
	Width              int    `json:"width"`
	Height             int    `json:"height"`
	Codec              string `json:"codec"`
	PanelDurationMs    int    `json:"panel_duration_ms"`
	Transition         string `json:"transition"`
}

func (b formatBody) toCmd() formatcmd.SaveFormat {
	return formatcmd.SaveFormat{
		ID: b.ID, Name: b.Name, Description: b.Description, Scope: b.Scope, Icon: b.Icon,
		ScriptSystemSuffix: b.ScriptSystemSuffix,
		ImagePromptPrefix:  b.ImagePromptPrefix, ImagePromptSuffix: b.ImagePromptSuffix,
		ImageModel: b.ImageModel, StyleReference: b.StyleReference,
		FPS: b.FPS, Width: b.Width, Height: b.Height, Codec: b.Codec,
		PanelDurationMs: b.PanelDurationMs, Transition: b.Transition,
	}
}

func (s *Server) CreateFormat(w http.ResponseWriter, r *http.Request) {
	var body formatBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	cmd := body.toCmd()
	if cmd.Scope == "" {
		cmd.Scope = "user"
	}
	res, err := bus.Dispatch[formatcmd.SaveFormatResult](r.Context(), s.reg, cmd)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": res.ID})
}

func (s *Server) UpdateFormat(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body formatBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	cmd := body.toCmd()
	cmd.ID = id
	if _, err := bus.Dispatch[formatcmd.SaveFormatResult](r.Context(), s.reg, cmd); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) DeleteFormat(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := bus.Dispatch[formatcmd.DeleteFormatResult](r.Context(), s.reg, formatcmd.DeleteFormat{ID: id}); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ComposeFormatPreview shows the exact string that goes to the image model
// after format prefix + user prompt + format suffix are stitched. No image
// generation — purely textual for the editor.
func (s *Server) ComposeFormatPreview(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Prefix      string `json:"prefix"`
		Suffix      string `json:"suffix"`
		PanelPrompt string `json:"panel_prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	composed := strings.TrimSpace(body.Prefix + body.PanelPrompt + body.Suffix)
	writeJSON(w, http.StatusOK, map[string]string{"composed": composed})
}
