package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/example/dddcqrs/internal/app/bus"
	projcmd "github.com/example/dddcqrs/internal/app/command/projects"
)

// googleOAuth holds the single OAuth app credentials used to connect channels
// for API upload. Per-channel refresh tokens live on each account.
type googleOAuth struct {
	clientID     string
	clientSecret string
	redirectURL  string
}

// WithGoogleOAuth enables the /api/youtube-oauth/* connect flow.
func (s *Server) WithGoogleOAuth(clientID, clientSecret, redirectURL string) *Server {
	if clientID != "" && clientSecret != "" {
		s.googleOAuth = &googleOAuth{clientID: clientID, clientSecret: clientSecret, redirectURL: redirectURL}
	}
	return s
}

const (
	googleAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL    = "https://oauth2.googleapis.com/token"
	youtubeChannelURL = "https://www.googleapis.com/youtube/v3/channels?part=snippet&mine=true"
	ytUploadScope     = "https://www.googleapis.com/auth/youtube.upload https://www.googleapis.com/auth/youtube.readonly"
)

// MountYouTubeOAuth registers the connect-via-API consent flow:
//
//	GET /api/youtube-oauth/start?account_id=<id>  → 302 to Google consent
//	GET /api/youtube-oauth/callback?code&state    → exchange + persist token
func (s *Server) MountYouTubeOAuth(r chi.Router) {
	r.Get("/api/youtube-oauth/start", func(w http.ResponseWriter, req *http.Request) {
		if s.googleOAuth == nil {
			writeErr(w, http.StatusServiceUnavailable, "youtube API disabled: set GOOGLE_OAUTH_CLIENT_ID/SECRET")
			return
		}
		accountID := req.URL.Query().Get("account_id")
		if accountID == "" {
			writeErr(w, http.StatusBadRequest, "account_id required")
			return
		}
		q := url.Values{}
		q.Set("client_id", s.googleOAuth.clientID)
		q.Set("redirect_uri", s.googleOAuth.redirectURL)
		q.Set("response_type", "code")
		q.Set("scope", ytUploadScope)
		q.Set("access_type", "offline")
		q.Set("prompt", "consent") // force a refresh_token every time
		q.Set("include_granted_scopes", "true")
		q.Set("state", accountID)
		http.Redirect(w, req, googleAuthURL+"?"+q.Encode(), http.StatusFound)
	})

	r.Get("/api/youtube-oauth/callback", func(w http.ResponseWriter, req *http.Request) {
		if s.googleOAuth == nil {
			writeErr(w, http.StatusServiceUnavailable, "youtube API disabled")
			return
		}
		if e := req.URL.Query().Get("error"); e != "" {
			oauthResultPage(w, false, "Google denied access: "+e)
			return
		}
		code := req.URL.Query().Get("code")
		accountID := req.URL.Query().Get("state")
		if code == "" || accountID == "" {
			oauthResultPage(w, false, "missing code/state")
			return
		}
		refreshToken, accessToken, err := s.googleOAuth.exchangeCode(req.Context(), code)
		if err != nil {
			oauthResultPage(w, false, "token exchange failed: "+err.Error())
			return
		}
		if refreshToken == "" {
			oauthResultPage(w, false, "Google returned no refresh token — remove the app's access at myaccount.google.com/permissions and retry.")
			return
		}
		title := fetchChannelTitle(req.Context(), accessToken)
		if _, err := bus.Dispatch[projcmd.SetSocialAccountOAuthResult](req.Context(), s.reg, projcmd.SetSocialAccountOAuth{
			ID: accountID, RefreshToken: refreshToken, ChannelTitle: title,
		}); err != nil {
			oauthResultPage(w, false, "could not save token: "+err.Error())
			return
		}
		oauthResultPage(w, true, "Connected "+title+" for API upload. You can close this tab.")
	})
}

// exchangeCode swaps an auth code for refresh + access tokens.
func (g *googleOAuth) exchangeCode(ctx context.Context, code string) (refresh, access string, err error) {
	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", g.clientID)
	form.Set("client_secret", g.clientSecret)
	form.Set("redirect_uri", g.redirectURL)
	form.Set("grant_type", "authorization_code")

	reqHTTP, _ := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL, strings.NewReader(form.Encode()))
	reqHTTP.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(reqHTTP)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("google token %d: %s", resp.StatusCode, string(raw))
	}
	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(raw, &tok); err != nil {
		return "", "", err
	}
	return tok.RefreshToken, tok.AccessToken, nil
}

// fetchChannelTitle returns the authorized channel's title (best-effort).
func fetchChannelTitle(ctx context.Context, accessToken string) string {
	reqHTTP, _ := http.NewRequestWithContext(ctx, http.MethodGet, youtubeChannelURL, nil)
	reqHTTP.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(reqHTTP)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var body struct {
		Items []struct {
			Snippet struct {
				Title string `json:"title"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if json.NewDecoder(resp.Body).Decode(&body) == nil && len(body.Items) > 0 {
		return body.Items[0].Snippet.Title
	}
	return ""
}

func oauthResultPage(w http.ResponseWriter, ok bool, msg string) {
	icon := "✅"
	if !ok {
		icon = "⚠️"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, fmt.Sprintf(
		`<!doctype html><meta charset="utf-8"><body style="font-family:system-ui;background:#0b0b0f;color:#eee;display:flex;align-items:center;justify-content:center;height:100vh;margin:0"><div style="text-align:center"><div style="font-size:48px">%s</div><p style="max-width:420px">%s</p></div></body>`,
		icon, msg))
}
