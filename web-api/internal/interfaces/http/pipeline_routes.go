package http

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/example/dddcqrs/internal/app/bus"
	pipecmd "github.com/example/dddcqrs/internal/app/command/pipeline"
	pipeq "github.com/example/dddcqrs/internal/app/query/pipeline"
	"github.com/example/dddcqrs/internal/domain/pipeline"
	"github.com/example/dddcqrs/internal/interfaces/http/gen"
)

// runOverridesExtension covers fields that the OpenAPI codegen doesn't expose
// yet. Decoded from the raw body alongside the typed struct.
type runOverridesExtension struct {
	Overrides struct {
		Assemble struct {
			PanelDurationMs *int    `json:"panel_duration_ms,omitempty"`
			Transition      *string `json:"transition,omitempty"`
		} `json:"assemble"`
		Music struct {
			PreferredMood *string `json:"preferred_mood,omitempty"`
			TrackID       *string `json:"track_id,omitempty"`
		} `json:"music"`
	} `json:"overrides"`
	// Top-level fields the codegen doesn't expose yet.
	Language *string `json:"language,omitempty"`
}

// --- runs ---

func (s *Server) CreateRun(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	var body gen.CreateRunJSONRequestBody
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	var overrides *pipecmd.RunOverrides
	if body.Overrides != nil {
		ov := &pipecmd.RunOverrides{}
		if body.Overrides.PanelCount != nil {
			ov.PanelCount = *body.Overrides.PanelCount
		}
		if body.Overrides.TargetDurationMs != nil {
			ov.TargetDurationMs = *body.Overrides.TargetDurationMs
		}
		if body.Overrides.EnableAudio != nil {
			ov.EnableAudio = body.Overrides.EnableAudio
		}
		if body.Overrides.AutoAssemble != nil {
			ov.AutoAssemble = body.Overrides.AutoAssemble
		}
		if body.Overrides.Audio != nil {
			au := &pipecmd.AudioOverride{}
			if body.Overrides.Audio.VoiceId != nil {
				au.VoiceID = *body.Overrides.Audio.VoiceId
			}
			if body.Overrides.Audio.Model != nil {
				au.Model = *body.Overrides.Audio.Model
			}
			if body.Overrides.Audio.Speed != nil {
				au.Speed = *body.Overrides.Audio.Speed
			}
			ov.Audio = au
		}
		if body.Overrides.Upload != nil {
			up := &pipecmd.UploadOverride{}
			if body.Overrides.Upload.Enabled != nil {
				up.Enabled = body.Overrides.Upload.Enabled
			}
			if body.Overrides.Upload.Provider != nil {
				up.Provider = *body.Overrides.Upload.Provider
			}
			if body.Overrides.Upload.SocialAccountIds != nil {
				up.SocialAccountIDs = *body.Overrides.Upload.SocialAccountIds
			}
			if body.Overrides.Upload.ScheduledAt != nil {
				up.ScheduledAt = body.Overrides.Upload.ScheduledAt.Format("2006-01-02T15:04:05Z07:00")
			}
			if body.Overrides.Upload.CaptionModel != nil {
				up.CaptionModel = *body.Overrides.Upload.CaptionModel
			}
			if body.Overrides.Upload.CaptionOverride != nil {
				up.CaptionOverride = *body.Overrides.Upload.CaptionOverride
			}
			if body.Overrides.Upload.Platforms != nil {
				up.Platforms = *body.Overrides.Upload.Platforms
			}
			ov.Upload = up
		}
		if body.Overrides.Assemble != nil {
			as := &pipecmd.AssembleOverride{}
			if body.Overrides.Assemble.Fps != nil {
				as.FPS = *body.Overrides.Assemble.Fps
			}
			if body.Overrides.Assemble.Width != nil {
				as.Width = *body.Overrides.Assemble.Width
			}
			if body.Overrides.Assemble.Height != nil {
				as.Height = *body.Overrides.Assemble.Height
			}
			if body.Overrides.Assemble.Codec != nil {
				as.Codec = string(*body.Overrides.Assemble.Codec)
			}
			// Codegen doesn't expose panel_duration_ms + transition + music
			// yet — pull them from the raw body via the extension struct.
			var ext runOverridesExtension
			_ = json.Unmarshal(raw, &ext)
			if ext.Overrides.Assemble.PanelDurationMs != nil {
				as.PanelDurationMs = *ext.Overrides.Assemble.PanelDurationMs
			}
			if ext.Overrides.Assemble.Transition != nil {
				as.Transition = *ext.Overrides.Assemble.Transition
			}
			ov.Assemble = as
			if ext.Overrides.Music.PreferredMood != nil || ext.Overrides.Music.TrackID != nil {
				m := &pipecmd.MusicOverride{}
				if ext.Overrides.Music.PreferredMood != nil {
					m.PreferredMood = *ext.Overrides.Music.PreferredMood
				}
				if ext.Overrides.Music.TrackID != nil {
					m.TrackID = *ext.Overrides.Music.TrackID
				}
				ov.Music = m
			}
		}
		if body.Overrides.SystemPrompt != nil {
			ov.SystemPrompt = *body.Overrides.SystemPrompt
		}
		if body.Overrides.ScriptModel != nil {
			ov.ScriptModel = *body.Overrides.ScriptModel
		}
		if body.Overrides.ImageModel != nil {
			ov.ImageModel = *body.Overrides.ImageModel
		}
		if body.Overrides.StyleReference != nil {
			ov.StyleReference = string(*body.Overrides.StyleReference)
		}
		if body.Overrides.Steps != nil {
			ov.Steps = convertSteps(*body.Overrides.Steps)
		}
		overrides = ov
	}
	cmd := pipecmd.CreateRun{
		Prompt:     body.Prompt,
		TemplateID: body.TemplateId,
		Overrides:  overrides,
	}
	if body.ProjectId != nil {
		cmd.ProjectID = *body.ProjectId
	}
	if body.CharacterIds != nil {
		cmd.CharacterIDs = *body.CharacterIds
	}
	if body.EnvironmentIds != nil {
		cmd.EnvironmentIDs = *body.EnvironmentIds
	}
	if body.UsePlot != nil {
		cmd.UsePlot = *body.UsePlot
	}
	if body.FormatId != nil {
		cmd.FormatID = *body.FormatId
	}
	// Language is not in OpenAPI yet — pull from raw body extension.
	{
		var ext runOverridesExtension
		_ = json.Unmarshal(raw, &ext)
		if ext.Language != nil {
			cmd.Language = *ext.Language
		}
	}
	res, err := bus.Dispatch[pipecmd.CreateRunResult](r.Context(), s.reg, cmd)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, gen.IDResponse{Id: res.RunID})
}

func (s *Server) ListRuns(w http.ResponseWriter, r *http.Request, params gen.ListRunsParams) {
	limit, offset := 50, 0
	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}
	var statuses []string
	if params.Status != nil && *params.Status != "" {
		for _, t := range strings.Split(*params.Status, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				statuses = append(statuses, t)
			}
		}
	}
	search := ""
	if params.Q != nil {
		search = strings.TrimSpace(*params.Q)
	}
	projectID := ""
	if params.ProjectId != nil {
		projectID = strings.TrimSpace(*params.ProjectId)
	}
	res, err := bus.Ask[[]pipeq.RunSummary](r.Context(), s.reg, pipeq.ListRuns{Limit: limit, Offset: offset, Statuses: statuses, Search: search, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) GetRun(w http.ResponseWriter, r *http.Request, id gen.PathID) {
	res, err := bus.Ask[pipeq.RunView](r.Context(), s.reg, pipeq.GetRun{RunID: id})
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// StreamRunEvents — server-poll-backed SSE. Pushes a fresh RunView whenever the
// run's status / step state / asset list has changed. Terminates when the run
// reaches a terminal state. Simpler than a cross-process pub/sub for MVP; if
// the load profile demands it, swap in a Watermill consumer subscription that
// fans out push-style.
func (s *Server) StreamRunEvents(w http.ResponseWriter, r *http.Request, id gen.PathID) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	ctx := r.Context()
	var lastEtag string
	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		view, err := bus.Ask[pipeq.RunView](ctx, s.reg, pipeq.GetRun{RunID: id})
		if err != nil {
			_, _ = w.Write([]byte("event: error\ndata: " + err.Error() + "\n\n"))
			flusher.Flush()
			return
		}
		etag := runEtag(view)
		if etag != lastEtag {
			payload, _ := json.Marshal(view)
			_, _ = w.Write([]byte("event: state\ndata: "))
			_, _ = w.Write(payload)
			_, _ = w.Write([]byte("\n\n"))
			flusher.Flush()
			lastEtag = etag
		}
		switch view.Status {
		case "completed", "failed", "cancelled":
			return
		}
	}
}

// runEtag is a cheap state hash: status + per-step (status, panels_completed).
// If those don't change, the view hasn't meaningfully changed for the client.
func runEtag(v pipeq.RunView) string {
	var b strings.Builder
	b.WriteString(v.Status)
	b.WriteByte('|')
	for _, s := range v.Steps {
		b.WriteString(s.Status)
		b.WriteByte(':')
		fmtInt(&b, s.PanelsCompleted)
		b.WriteByte(',')
	}
	return b.String()
}

func fmtInt(b *strings.Builder, n int) {
	if n == 0 {
		b.WriteByte('0')
		return
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	b.Write(buf[i:])
}

func (s *Server) RetryRun(w http.ResponseWriter, r *http.Request, id gen.PathID) {
	res, err := bus.Dispatch[pipecmd.RetryRunResult](r.Context(), s.reg, pipecmd.RetryRun{RunID: id})
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, gen.IDResponse{Id: res.RunID})
}

func (s *Server) CancelRun(w http.ResponseWriter, r *http.Request, id gen.PathID) {
	if _, err := bus.Dispatch[pipecmd.CancelRunResult](r.Context(), s.reg, pipecmd.CancelRun{RunID: id}); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) RegenerateStep(w http.ResponseWriter, r *http.Request, id gen.PathID, idx int) {
	var body gen.RegenerateStepJSONRequestBody
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
	}
	var params map[string]any
	if body.Params != nil {
		params = *body.Params
	}
	res, err := bus.Dispatch[pipecmd.RegenerateStepResult](r.Context(), s.reg, pipecmd.RegenerateStep{
		RunID:          id,
		StepIndex:      idx,
		ParamsOverride: params,
	})
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	out := gen.RegenerateStepResponse{
		RunId:      res.RunID,
		StepIndex:  res.StepIndex,
		NewVersion: res.NewVersion,
	}
	if res.NewAttemptID != "" {
		out.NewAttemptId = &res.NewAttemptID
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) RequestAssemble(w http.ResponseWriter, r *http.Request, id gen.PathID) {
	var body gen.RequestAssembleJSONRequestBody
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
	}
	var params map[string]any
	if body.Params != nil {
		params = *body.Params
	}
	res, err := bus.Dispatch[pipecmd.RequestAssembleResult](r.Context(), s.reg, pipecmd.RequestAssemble{
		RunID:          id,
		ParamsOverride: params,
	})
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, gen.RequestAssembleResponse{RunId: res.RunID, StepIndex: res.StepIndex})
}

// --- cleanup ---

func (s *Server) CleanupRuns(w http.ResponseWriter, r *http.Request) {
	var body gen.CleanupRunsJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	var statuses []string
	if body.Statuses != nil {
		statuses = *body.Statuses
	}
	res, err := bus.Dispatch[pipecmd.CleanupRunsResult](r.Context(), s.reg, pipecmd.CleanupRuns{
		OlderThanDays: body.OlderThanDays,
		Statuses:      statuses,
	})
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, gen.CleanupRunsResponse{Deleted: res.Deleted})
}

// --- templates ---

func (s *Server) ListTemplates(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	var body gen.CreateTemplateJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	var maxCost float64
	if body.MaxCostUsd != nil {
		maxCost = *body.MaxCostUsd
	}
	res, err := bus.Dispatch[pipecmd.CreateTemplateResult](r.Context(), s.reg, pipecmd.CreateTemplate{
		Name:       body.Name,
		Steps:      convertSteps(body.Steps),
		MaxCostUSD: maxCost,
	})
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, gen.IDResponse{Id: res.TemplateID})
}

func (s *Server) GetTemplate(w http.ResponseWriter, r *http.Request, id gen.PathID) {
	res, err := bus.Ask[pipeq.TemplateView](r.Context(), s.reg, pipeq.GetTemplate{TemplateID: id})
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) UpdateTemplate(w http.ResponseWriter, r *http.Request, id gen.PathID) {
	var body gen.UpdateTemplateJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	upd := pipecmd.UpdateTemplate{
		TemplateID: id,
		Name:       body.Name,
		Steps:      convertSteps(body.Steps),
	}
	if body.MaxCostUsd != nil {
		upd.MaxCostUSD = *body.MaxCostUsd
		upd.UpdateMaxCost = true
	}
	if _, err := bus.Dispatch[pipecmd.UpdateTemplateResult](r.Context(), s.reg, upd); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- balances ---

func (s *Server) GetBalances(w http.ResponseWriter, r *http.Request) {
	if s.balances == nil {
		writeErr(w, http.StatusServiceUnavailable, "balances disabled")
		return
	}
	writeJSON(w, http.StatusOK, s.balances.Snapshot(r.Context()))
}

// --- stats ---

func (s *Server) GetStats(w http.ResponseWriter, r *http.Request) {
	v, err := bus.Ask[pipeq.StatsView](r.Context(), s.reg, pipeq.GetStats{})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// --- assets ---

func (s *Server) GetAssetUrl(w http.ResponseWriter, r *http.Request, id gen.PathID) {
	ref, err := bus.Ask[pipeq.AssetRef](r.Context(), s.reg, pipeq.GetAssetRef{AssetID: id})
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	if s.store == nil {
		writeErr(w, http.StatusServiceUnavailable, "asset store not configured")
		return
	}
	url, err := s.store.PresignGet(r.Context(), ref.Bucket, ref.ObjectKey, 300)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, gen.PresignedAssetURL{Url: url, Mime: ref.Mime})
}

// convertSteps maps gen.StepConfig (codegen DTO) → domain.StepConfig.
func convertSteps(in []gen.StepConfig) []pipeline.StepConfig {
	out := make([]pipeline.StepConfig, 0, len(in))
	for _, s := range in {
		var sys, model, provider string
		if s.SystemPrompt != nil {
			sys = *s.SystemPrompt
		}
		if s.Model != nil {
			model = *s.Model
		}
		if s.Provider != nil {
			provider = *s.Provider
		}
		var params map[string]any
		if s.Params != nil {
			params = *s.Params
		}
		out = append(out, pipeline.StepConfig{
			Type:         pipeline.StepType(string(s.Type)),
			SystemPrompt: sys,
			Model:        model,
			Provider:     provider,
			Params:       params,
		})
	}
	return out
}
