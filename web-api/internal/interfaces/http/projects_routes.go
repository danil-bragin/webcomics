package http

import (
	"encoding/json"
	"net/http"

	"github.com/example/dddcqrs/internal/app/bus"
	projcmd "github.com/example/dddcqrs/internal/app/command/projects"
	projq "github.com/example/dddcqrs/internal/app/query/projects"
	"github.com/example/dddcqrs/internal/domain/projects"
	"github.com/example/dddcqrs/internal/interfaces/http/gen"
)

func (s *Server) ListProjects(w http.ResponseWriter, r *http.Request) {
	res, err := bus.Ask[[]projq.ProjectView](r.Context(), s.reg, projq.ListProjects{})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) CreateProject(w http.ResponseWriter, r *http.Request) {
	var body gen.CreateProjectJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	description := ""
	if body.Description != nil {
		description = *body.Description
	}
	cmd := projcmd.CreateProject{Name: body.Name, Description: description}
	if body.Defaults != nil {
		cmd.Defaults = *body.Defaults
	}
	res, err := bus.Dispatch[projcmd.CreateProjectResult](r.Context(), s.reg, cmd)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, gen.IDResponse{Id: res.ID})
}

func (s *Server) GetProject(w http.ResponseWriter, r *http.Request, id gen.PathID) {
	res, err := bus.Ask[projq.ProjectDetailView](r.Context(), s.reg, projq.GetProjectDetail{ID: id})
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) UpdateProject(w http.ResponseWriter, r *http.Request, id gen.PathID) {
	var body gen.UpdateProjectJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	cmd := projcmd.UpdateProject{ID: id, Name: body.Name}
	if body.Description != nil {
		cmd.Description = *body.Description
	}
	if body.Defaults != nil {
		cmd.Defaults = *body.Defaults
	}
	if body.Archived != nil {
		cmd.Archived = body.Archived
	}
	if _, err := bus.Dispatch[projcmd.UpdateProjectResult](r.Context(), s.reg, cmd); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) DeleteProject(w http.ResponseWriter, r *http.Request, id gen.PathID) {
	if _, err := bus.Dispatch[projcmd.DeleteProjectResult](r.Context(), s.reg, projcmd.DeleteProject{ID: id}); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Characters ---

func (s *Server) CreateCharacter(w http.ResponseWriter, r *http.Request, id gen.PathID) {
	var body gen.CreateCharacterJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	cmd := projcmd.UpsertCharacter{ProjectID: id, Name: body.Name}
	if body.Description != nil {
		cmd.Description = *body.Description
	}
	if body.Traits != nil {
		cmd.Traits = *body.Traits
	}
	if body.RefAssetIds != nil {
		cmd.RefAssetIDs = *body.RefAssetIds
	}
	res, err := bus.Dispatch[projcmd.UpsertCharacterResult](r.Context(), s.reg, cmd)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, gen.IDResponse{Id: res.ID})
}

func (s *Server) UpdateCharacter(w http.ResponseWriter, r *http.Request, id gen.PathID) {
	var body gen.UpdateCharacterJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	cmd := projcmd.UpsertCharacter{ID: id, Name: body.Name}
	if body.Description != nil {
		cmd.Description = *body.Description
	}
	if body.Traits != nil {
		cmd.Traits = *body.Traits
	}
	if body.RefAssetIds != nil {
		cmd.RefAssetIDs = *body.RefAssetIds
	}
	if _, err := bus.Dispatch[projcmd.UpsertCharacterResult](r.Context(), s.reg, cmd); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) DeleteCharacter(w http.ResponseWriter, r *http.Request, id gen.PathID) {
	if _, err := bus.Dispatch[projcmd.DeleteCharacterResult](r.Context(), s.reg, projcmd.DeleteCharacter{ID: id}); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Environments ---

func (s *Server) CreateEnvironment(w http.ResponseWriter, r *http.Request, id gen.PathID) {
	var body gen.CreateEnvironmentJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	cmd := projcmd.UpsertEnvironment{ProjectID: id, Name: body.Name}
	if body.Description != nil {
		cmd.Description = *body.Description
	}
	if body.Traits != nil {
		cmd.Traits = *body.Traits
	}
	if body.RefAssetIds != nil {
		cmd.RefAssetIDs = *body.RefAssetIds
	}
	res, err := bus.Dispatch[projcmd.UpsertEnvironmentResult](r.Context(), s.reg, cmd)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, gen.IDResponse{Id: res.ID})
}

func (s *Server) UpdateEnvironment(w http.ResponseWriter, r *http.Request, id gen.PathID) {
	var body gen.UpdateEnvironmentJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	cmd := projcmd.UpsertEnvironment{ID: id, Name: body.Name}
	if body.Description != nil {
		cmd.Description = *body.Description
	}
	if body.Traits != nil {
		cmd.Traits = *body.Traits
	}
	if body.RefAssetIds != nil {
		cmd.RefAssetIDs = *body.RefAssetIds
	}
	if _, err := bus.Dispatch[projcmd.UpsertEnvironmentResult](r.Context(), s.reg, cmd); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) DeleteEnvironment(w http.ResponseWriter, r *http.Request, id gen.PathID) {
	if _, err := bus.Dispatch[projcmd.DeleteEnvironmentResult](r.Context(), s.reg, projcmd.DeleteEnvironment{ID: id}); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Plot ---

func (s *Server) UpsertPlot(w http.ResponseWriter, r *http.Request, id gen.PathID) {
	var body gen.UpsertPlotJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	cmd := projcmd.UpsertPlot{ProjectID: id}
	if body.Name != nil {
		cmd.Name = *body.Name
	}
	if body.Premise != nil {
		cmd.Premise = *body.Premise
	}
	if body.Beats != nil {
		for _, b := range *body.Beats {
			cmd.Beats = append(cmd.Beats, convertBeat(b))
		}
	}
	res, err := bus.Dispatch[projcmd.UpsertPlotResult](r.Context(), s.reg, cmd)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, gen.IDResponse{Id: res.ID})
}

func convertBeat(b gen.PlotBeatView) projects.PlotBeat {
	return projects.PlotBeat{Name: b.Name, Description: b.Description, Order: b.Order}
}

// --- Social accounts ---

func (s *Server) CreateSocialAccount(w http.ResponseWriter, r *http.Request, id gen.PathID) {
	var body gen.CreateSocialAccountJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	cmd := projcmd.UpsertSocialAccount{ProjectID: id, Platform: body.Platform}
	if body.Label != nil {
		cmd.Label = *body.Label
	}
	if body.FirefoxProfilePath != nil {
		cmd.FirefoxProfilePath = *body.FirefoxProfilePath
	}
	if body.Extra != nil {
		cmd.Extra = *body.Extra
	}
	res, err := bus.Dispatch[projcmd.UpsertSocialAccountResult](r.Context(), s.reg, cmd)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, gen.IDResponse{Id: res.ID})
}

func (s *Server) UpdateSocialAccount(w http.ResponseWriter, r *http.Request, id gen.PathID) {
	var body gen.UpdateSocialAccountJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	cmd := projcmd.UpsertSocialAccount{ID: id, Platform: body.Platform}
	if body.Label != nil {
		cmd.Label = *body.Label
	}
	if body.FirefoxProfilePath != nil {
		cmd.FirefoxProfilePath = *body.FirefoxProfilePath
	}
	if body.Extra != nil {
		cmd.Extra = *body.Extra
	}
	if _, err := bus.Dispatch[projcmd.UpsertSocialAccountResult](r.Context(), s.reg, cmd); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) DeleteSocialAccount(w http.ResponseWriter, r *http.Request, id gen.PathID) {
	if _, err := bus.Dispatch[projcmd.DeleteSocialAccountResult](r.Context(), s.reg, projcmd.DeleteSocialAccount{ID: id}); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
