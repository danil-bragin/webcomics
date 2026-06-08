package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/example/dddcqrs/internal/app/bus"
	projcmd "github.com/example/dddcqrs/internal/app/command/projects"
	schcmd "github.com/example/dddcqrs/internal/app/command/scheduler"
	schq "github.com/example/dddcqrs/internal/app/query/scheduler"
)

// MountScheduler wires /api/schedule/* + extends /api/social/accounts/{id}
// rate-limit fields. Sits next to /api/social/* in the route table.
func (s *Server) MountScheduler(r chi.Router) {
	// List scheduled rows. Filters: account_id, status, since, until, limit.
	r.Get("/api/schedule", func(w http.ResponseWriter, req *http.Request) {
		f := schq.ListFilter{
			AccountID: req.URL.Query().Get("account_id"),
			Status:    req.URL.Query().Get("status"),
		}
		if v := req.URL.Query().Get("since"); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				f.Since = &t
			}
		}
		if v := req.URL.Query().Get("until"); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				f.Until = &t
			}
		}
		if v := req.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				f.Limit = n
			}
		}
		out, err := bus.Ask[[]schq.View](req.Context(), s.reg, schq.ListScheduled{Filter: f})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, out)
	})

	// Availability check used by the schedule modal.
	r.Get("/api/schedule/availability", func(w http.ResponseWriter, req *http.Request) {
		aid := req.URL.Query().Get("account_id")
		at := req.URL.Query().Get("at")
		if aid == "" || at == "" {
			writeErr(w, http.StatusBadRequest, "account_id and at required")
			return
		}
		ts, err := time.Parse(time.RFC3339, at)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "at must be RFC3339")
			return
		}
		out, err := bus.Ask[schq.AccountWindowStats](req.Context(), s.reg, schq.GetSlotAvailability{AccountID: aid, TargetAt: ts})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, out)
	})

	// Create scheduled upload.
	r.Post("/api/schedule", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			RunID           string         `json:"run_id"`
			SocialAccountID string         `json:"social_account_id"`
			ScheduledAt     string         `json:"scheduled_at"` // RFC3339
			Metadata        map[string]any `json:"metadata"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		at, err := time.Parse(time.RFC3339, body.ScheduledAt)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "scheduled_at must be RFC3339")
			return
		}
		res, err := bus.Dispatch[schcmd.ScheduleUploadResult](req.Context(), s.reg, schcmd.ScheduleUpload{
			RunID: body.RunID, SocialAccountID: body.SocialAccountID, ScheduledAt: at, Metadata: body.Metadata,
		})
		if err != nil {
			if next, ok := schcmd.IsBlockedByLimit(err); ok {
				writeJSON(w, http.StatusConflict, map[string]any{
					"error":          err.Error(),
					"next_free_slot": next.Format(time.RFC3339),
				})
				return
			}
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"id": res.ID})
	})

	// Reschedule.
	r.Patch("/api/schedule/{id}", func(w http.ResponseWriter, req *http.Request) {
		id := chi.URLParam(req, "id")
		var body struct {
			ScheduledAt string `json:"scheduled_at"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		at, err := time.Parse(time.RFC3339, body.ScheduledAt)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "scheduled_at must be RFC3339")
			return
		}
		res, err := bus.Dispatch[schcmd.RescheduleUploadResult](req.Context(), s.reg, schcmd.RescheduleUpload{ID: id, ScheduledAt: at})
		if err != nil {
			if next, ok := schcmd.IsBlockedByLimit(err); ok {
				writeJSON(w, http.StatusConflict, map[string]any{
					"error":          err.Error(),
					"next_free_slot": next.Format(time.RFC3339),
				})
				return
			}
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	// Cancel.
	r.Delete("/api/schedule/{id}", func(w http.ResponseWriter, req *http.Request) {
		id := chi.URLParam(req, "id")
		if _, err := bus.Dispatch[schcmd.CancelScheduledUploadResult](req.Context(), s.reg, schcmd.CancelScheduledUpload{ID: id}); err != nil {
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// Extend rate-limit config on social account PATCH route.
	// Existing PATCH /api/social/accounts/{id} accepts platform/label/etc; we
	// add a sibling endpoint for limits to keep the existing one focused.
	r.Patch("/api/social/accounts/{id}/limits", func(w http.ResponseWriter, req *http.Request) {
		id := chi.URLParam(req, "id")
		var body struct {
			DailyUploadLimit *int  `json:"daily_upload_limit"`
			LimitWindowHours *int  `json:"limit_window_hours"`
			IsVerified       *bool `json:"is_verified"`
			MinGapSeconds    *int  `json:"min_gap_seconds"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		cmd := projcmd.SetSocialAccountLimits{ID: id}
		if body.DailyUploadLimit != nil {
			cmd.DailyUploadLimit = *body.DailyUploadLimit
		} else {
			cmd.DailyUploadLimit = -1
		}
		if body.LimitWindowHours != nil {
			cmd.LimitWindowHours = *body.LimitWindowHours
		}
		if body.MinGapSeconds != nil {
			cmd.MinGapSeconds = *body.MinGapSeconds
		} else {
			cmd.MinGapSeconds = -1
		}
		if body.IsVerified != nil {
			cmd.IsVerified = *body.IsVerified
		}
		if _, err := bus.Dispatch[projcmd.SetSocialAccountLimitsResult](req.Context(), s.reg, cmd); err != nil {
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
