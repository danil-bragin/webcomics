package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/example/dddcqrs/internal/app/bus"
	projcmd "github.com/example/dddcqrs/internal/app/command/projects"
	projq "github.com/example/dddcqrs/internal/app/query/projects"
)

// findSocialAccount resolves a global social account by id via the list query
// (no single-account read exists yet; the library is small).
func (s *Server) findSocialAccount(ctx context.Context, id string) (projq.SocialAccountView, error) {
	all, err := bus.Ask[[]projq.SocialAccountView](ctx, s.reg, projq.ListSocialAccountsGlobal{})
	if err != nil {
		return projq.SocialAccountView{}, err
	}
	for _, a := range all {
		if a.ID == id {
			return a, nil
		}
	}
	return projq.SocialAccountView{}, fmt.Errorf("social account %s not found", id)
}

// MountSocial wires the global /api/social/* routes + the link/unlink/default
// endpoints under /api/projects/:id/social-accounts/:aid.
//
// Sits next to MountAudioLibrary semantically: the audio library is the
// global track store, the social library is the global account store.
// Projects pull both via reference, not ownership.
func (s *Server) MountSocial(r chi.Router) {
	// --- Global library ---

	r.Get("/api/social/accounts", func(w http.ResponseWriter, req *http.Request) {
		platform := req.URL.Query().Get("platform")
		out, err := bus.Ask[[]projq.SocialAccountView](req.Context(), s.reg, projq.ListSocialAccountsGlobal{Platform: platform})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, out)
	})

	r.Post("/api/social/accounts", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Platform           string         `json:"platform"`
			Label              string         `json:"label"`
			FirefoxProfilePath string         `json:"firefox_profile_path"`
			Extra              map[string]any `json:"extra"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		cmd := projcmd.UpsertSocialAccount{
			Platform:           body.Platform,
			Label:              body.Label,
			FirefoxProfilePath: body.FirefoxProfilePath,
			Extra:              body.Extra,
		}
		res, err := bus.Dispatch[projcmd.UpsertSocialAccountResult](req.Context(), s.reg, cmd)
		if err != nil {
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": res.ID})
	})

	r.Patch("/api/social/accounts/{id}", func(w http.ResponseWriter, req *http.Request) {
		id := chi.URLParam(req, "id")
		var body struct {
			Platform           string         `json:"platform"`
			Label              string         `json:"label"`
			FirefoxProfilePath string         `json:"firefox_profile_path"`
			Extra              map[string]any `json:"extra"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		cmd := projcmd.UpsertSocialAccount{
			ID:                 id,
			Platform:           body.Platform,
			Label:              body.Label,
			FirefoxProfilePath: body.FirefoxProfilePath,
			Extra:              body.Extra,
		}
		if _, err := bus.Dispatch[projcmd.UpsertSocialAccountResult](req.Context(), s.reg, cmd); err != nil {
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	r.Delete("/api/social/accounts/{id}", func(w http.ResponseWriter, req *http.Request) {
		id := chi.URLParam(req, "id")
		if _, err := bus.Dispatch[projcmd.DeleteSocialAccountResult](req.Context(), s.reg, projcmd.DeleteSocialAccount{ID: id}); err != nil {
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// --- Inspect session: open a viewable Firefox on the account's profile so
	// the user can watch the live session over noVNC and verify auth works. ---

	r.Post("/api/social/accounts/{id}/inspect", func(w http.ResponseWriter, req *http.Request) {
		if s.fxLogin == nil {
			writeErr(w, http.StatusServiceUnavailable, "firefox viewer disabled: set FIREFOX_PROFILES_DIR env")
			return
		}
		id := chi.URLParam(req, "id")
		acct, err := s.findSocialAccount(req.Context(), id)
		if err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		if acct.FirefoxProfilePath == "" {
			writeErr(w, http.StatusUnprocessableEntity, "account has no firefox profile")
			return
		}
		sess, err := s.fxLogin.inspect(req.Context(), id, acct.FirefoxProfilePath, acct.Label)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, sess)
			return
		}
		writeJSON(w, http.StatusOK, sess)
	})

	r.Get("/api/social/accounts/{id}/inspect", func(w http.ResponseWriter, req *http.Request) {
		if s.fxLogin == nil {
			writeErr(w, http.StatusServiceUnavailable, "firefox viewer disabled")
			return
		}
		sess, ok := s.fxLogin.getInspect(chi.URLParam(req, "id"))
		if !ok {
			writeJSON(w, http.StatusOK, map[string]any{"status": "none"})
			return
		}
		writeJSON(w, http.StatusOK, sess)
	})

	r.Post("/api/social/accounts/{id}/inspect/stop", func(w http.ResponseWriter, req *http.Request) {
		if s.fxLogin == nil {
			writeErr(w, http.StatusServiceUnavailable, "firefox viewer disabled")
			return
		}
		if err := s.fxLogin.stopInspect(chi.URLParam(req, "id")); err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// --- Project ↔ account link routes ---

	r.Post("/api/projects/{project_id}/social-accounts/{account_id}/link", func(w http.ResponseWriter, req *http.Request) {
		pid := chi.URLParam(req, "project_id")
		aid := chi.URLParam(req, "account_id")
		var body struct {
			AsDefault bool `json:"as_default"`
		}
		_ = json.NewDecoder(req.Body).Decode(&body)
		if _, err := bus.Dispatch[projcmd.LinkSocialAccountResult](req.Context(), s.reg, projcmd.LinkSocialAccount{
			ProjectID: pid, SocialAccountID: aid, AsDefault: body.AsDefault,
		}); err != nil {
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	r.Delete("/api/projects/{project_id}/social-accounts/{account_id}/link", func(w http.ResponseWriter, req *http.Request) {
		pid := chi.URLParam(req, "project_id")
		aid := chi.URLParam(req, "account_id")
		if _, err := bus.Dispatch[projcmd.UnlinkSocialAccountResult](req.Context(), s.reg, projcmd.UnlinkSocialAccount{
			ProjectID: pid, SocialAccountID: aid,
		}); err != nil {
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	r.Put("/api/projects/{project_id}/social-accounts/{account_id}/default", func(w http.ResponseWriter, req *http.Request) {
		pid := chi.URLParam(req, "project_id")
		aid := chi.URLParam(req, "account_id")
		if _, err := bus.Dispatch[projcmd.SetDefaultSocialAccountResult](req.Context(), s.reg, projcmd.SetDefaultSocialAccount{
			ProjectID: pid, SocialAccountID: aid,
		}); err != nil {
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
