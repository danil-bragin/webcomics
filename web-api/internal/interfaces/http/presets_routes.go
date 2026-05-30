package http

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/example/dddcqrs/internal/app/bus"
	pipecmd "github.com/example/dddcqrs/internal/app/command/pipeline"
	pipeq "github.com/example/dddcqrs/internal/app/query/pipeline"
	"github.com/example/dddcqrs/internal/domain/pipeline"
)

// MountPresets adds the marketplace-friendly /api/presets/* routes that
// support the rich metadata (category / icon / sample_prompts / defaults /
// format_id) the new Studio UI relies on. The legacy /api/templates routes
// remain for back-compat.
func (s *Server) MountPresets(r chi.Router) {
	r.Get("/api/presets", s.ListPresets)
	r.Post("/api/presets", s.CreatePreset)
	r.Get("/api/presets/{id}", s.GetPreset)
	r.Put("/api/presets/{id}", s.UpdatePreset)
	r.Delete("/api/presets/{id}", s.DeletePreset)
}

func (s *Server) ListPresets(w http.ResponseWriter, r *http.Request) {
	f := pipeq.TemplateFilter{
		Category:    r.URL.Query().Get("category"),
		IncludeTest: r.URL.Query().Get("include_test") == "true",
	}
	res, err := bus.Ask[[]pipeq.TemplateView](r.Context(), s.reg, pipeq.ListTemplates{Filter: f})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) GetPreset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	res, err := bus.Ask[pipeq.TemplateView](r.Context(), s.reg, pipeq.GetTemplate{TemplateID: id})
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type presetBody struct {
	Name          string                `json:"name"`
	Description   string                `json:"description"`
	Category      string                `json:"category"`
	Icon          string                `json:"icon"`
	Steps         []pipeline.StepConfig `json:"steps"`
	SamplePrompts []string              `json:"sample_prompts"`
	FormatID      string                `json:"format_id"`
	Defaults      map[string]any        `json:"defaults"`
	MaxCostUSD    float64               `json:"max_cost_usd"`
}

func (s *Server) CreatePreset(w http.ResponseWriter, r *http.Request) {
	var body presetBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	res, err := bus.Dispatch[pipecmd.CreateTemplateResult](r.Context(), s.reg, pipecmd.CreateTemplate{
		Name:          body.Name,
		Description:   body.Description,
		Category:      body.Category,
		Icon:          body.Icon,
		Steps:         body.Steps,
		SamplePrompts: body.SamplePrompts,
		FormatID:      body.FormatID,
		Defaults:      body.Defaults,
		MaxCostUSD:    body.MaxCostUSD,
	})
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": res.TemplateID})
}

func (s *Server) UpdatePreset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body presetBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	desc := body.Description
	icon := body.Icon
	fmt := body.FormatID
	samples := body.SamplePrompts
	if _, err := bus.Dispatch[pipecmd.UpdateTemplateResult](r.Context(), s.reg, pipecmd.UpdateTemplate{
		TemplateID:     id,
		Name:           body.Name,
		Description:    &desc,
		Category:       body.Category,
		Icon:           &icon,
		Steps:          body.Steps,
		SamplePrompts:  &samples,
		FormatID:       &fmt,
		Defaults:       body.Defaults,
		UpdateDefaults: body.Defaults != nil,
		MaxCostUSD:     body.MaxCostUSD,
		UpdateMaxCost:  true,
	}); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) DeletePreset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := bus.Dispatch[pipecmd.DeleteTemplateResult](r.Context(), s.reg, pipecmd.DeleteTemplate{TemplateID: id}); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
