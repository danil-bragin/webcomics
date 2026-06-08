# YouTube API Upload (alongside Selenium) Implementation Plan

> **For agentic workers:** Steps use checkbox (`- [ ]`). Verification is by real runs + curl (no per-handler unit harness in this repo).

**Goal:** Upload to YouTube via the official **Data API v3** as an alternative to Selenium, so a banned/automation-flagged channel can use the legit API path. Auto-priority: try **API first**, fall back to **Selenium**; both selectable; track remaining API quota (~6 uploads/day).

**Architecture:** A social account gains optional OAuth (`youtube_api`) capability stored alongside its Selenium Firefox profile. An in-app OAuth consent flow captures a refresh token. The scheduler/run picks the provider per account + remaining API quota (API → Selenium). A new Python `youtube_api` upload provider does a simple multipart `videos.insert` over httpx (videos are ~15–25 MB, no resumable chunking needed). Quota usage is counted from upload records in a rolling 24h window.

**Tech stack:** Go (config, domain account, scheduler provider-pick, OAuth HTTP routes), Python upload worker (httpx OAuth + multipart upload), React (Connect-via-API button, method toggle, quota badge), Google Cloud (user sets up OAuth client — see guide).

---

## Phase 0 — GCP setup guide (do first; user sets up creds in parallel)

### Task 0.1: Write the setup guide
**Files:** Create `docs/youtube-api-setup.md`

- [ ] Step-by-step: create Google Cloud project → enable **YouTube Data API v3** → OAuth consent screen (External, add self as **Test user**, scope `.../auth/youtube.upload`) → create **OAuth client ID** (type *Web application*) with redirect URI `http://localhost:8080/api/youtube-oauth/callback` → copy **client_id** + **client_secret** → put into `web-api/.env` as `GOOGLE_OAUTH_CLIENT_ID` / `GOOGLE_OAUTH_CLIENT_SECRET`. Note quota: `videos.insert` = 1600 units, default 10000/day ⇒ ~6 uploads/day; request increase under *IAM & Admin → Quotas* if needed.
- [ ] Commit.

---

## Phase 1 — Config + account OAuth fields

### Task 1.1: OAuth env config
**Files:** Modify `web-api/internal/infrastructure/config/config.go`

- [ ] Add after `YouTubeAPIKey`:
```go
	// Google OAuth client for the youtube_api upload provider (videos.insert).
	GoogleOAuthClientID     string `env:"GOOGLE_OAUTH_CLIENT_ID" envDefault:""`
	GoogleOAuthClientSecret string `env:"GOOGLE_OAUTH_CLIENT_SECRET" envDefault:""`
	GoogleOAuthRedirectURL  string `env:"GOOGLE_OAUTH_REDIRECT_URL" envDefault:"http://localhost:8080/api/youtube-oauth/callback"`
```
- [ ] `go build ./...`; commit.

### Task 1.2: Store OAuth tokens on the account
**Files:** Modify `web-api/internal/domain/projects/social_account.go`

The account already has `extra map[string]any`. Use it (no migration): keys `oauth_refresh_token` (string), `oauth_connected_at` (RFC3339), `oauth_channel_title` (string). Add typed accessors so callers don't string-key the map.

- [ ] Add methods:
```go
func (a *SocialAccount) OAuthRefreshToken() string {
	if a.extra == nil { return "" }
	v, _ := a.extra["oauth_refresh_token"].(string)
	return v
}
func (a *SocialAccount) HasAPIUpload() bool { return a.OAuthRefreshToken() != "" }
func (a *SocialAccount) HasSeleniumUpload() bool { return a.firefoxProfilePath != "" }
func (a *SocialAccount) SetOAuth(refreshToken, channelTitle string) {
	if a.extra == nil { a.extra = map[string]any{} }
	a.extra["oauth_refresh_token"] = refreshToken
	a.extra["oauth_channel_title"] = channelTitle
	a.extra["oauth_connected_at"] = time.Now().UTC().Format(time.RFC3339)
	a.updatedAt = time.Now().UTC()
}
```
- [ ] `go build ./...`; commit.

### Task 1.3: Surface capability flags + quota in the read view
**Files:** Modify `web-api/internal/app/query/projects/views.go` + the read mapper `internal/infrastructure/persistence/read/*social*`

- [ ] Add to `SocialAccountView`: `HasAPI bool json:"has_api"`, `HasSelenium bool json:"has_selenium"`, `OAuthChannelTitle string json:"oauth_channel_title,omitempty"`, `APIUploadsUsed int json:"api_uploads_used"`, `APIUploadsLimit int json:"api_uploads_limit"`. Populate `has_api` = refresh token present (from extra), `has_selenium` = profile path non-empty. `api_uploads_*` filled by the quota query (Task 4.2) — default limit 6.
- [ ] `go build ./...`; commit.

---

## Phase 2 — OAuth consent flow (Go routes)

### Task 2.1: OAuth start + callback routes
**Files:** Create `web-api/internal/interfaces/http/youtube_oauth_routes.go`; mount in `server.go`

Holds client id/secret/redirect (inject via `WithGoogleOAuth(id, secret, redirect string)` on `*Server`, wired in `cmd/api/main.go`).

- [ ] `GET /api/youtube-oauth/start?account_id=<id>` → 302 to Google consent:
  `https://accounts.google.com/o/oauth2/v2/auth?client_id=..&redirect_uri=..&response_type=code&scope=https://www.googleapis.com/auth/youtube.upload%20https://www.googleapis.com/auth/youtube.readonly&access_type=offline&prompt=consent&state=<account_id>`
- [ ] `GET /api/youtube-oauth/callback?code=..&state=<account_id>`:
  1. POST `https://oauth2.googleapis.com/token` (code, client_id, client_secret, redirect_uri, grant_type=authorization_code) → `{access_token, refresh_token}`.
  2. GET `https://www.googleapis.com/youtube/v3/channels?part=snippet&mine=true` with `Authorization: Bearer <access_token>` → channel title.
  3. Dispatch a command to persist `SetOAuth(refresh_token, channelTitle)` on the account (see Task 2.2).
  4. Return a tiny HTML page: "✅ Connected <title>. You can close this tab."
- [ ] Return 503 with a clear message when client id/secret unset.
- [ ] `go build ./...`; commit.

### Task 2.2: Command to save OAuth on account
**Files:** Modify `web-api/internal/app/command/projects/*social*`

- [ ] Add `SetSocialAccountOAuth{ID, RefreshToken, ChannelTitle}` command + handler: load account via uow, `acct.SetOAuth(...)`, save. Register on bus. Used by the callback.
- [ ] `go build ./...`; commit.

---

## Phase 3 — Python youtube_api upload provider

### Task 3.1: API upload provider (httpx, simple multipart)
**Files:** Create `workers-py/src/worker/providers/youtube_api.py`

- [ ] `def upload(store, video_key, refresh_token, client_id, client_secret, meta) -> dict`:
  1. Refresh access token: POST `https://oauth2.googleapis.com/token` form `{client_id, client_secret, refresh_token, grant_type:"refresh_token"}` → `access_token`.
  2. Download video bytes from MinIO (`store.client.get_object(bucket, video_key).read()`).
  3. Build metadata JSON:
```python
body = {
  "snippet": {
    "title": meta["title"][:100],
    "description": meta.get("description",""),
    "tags": list(meta.get("tags") or []),
    "categoryId": str(meta.get("category_id") or "22"),
  },
  "status": {
    "privacyStatus": meta.get("visibility","unlisted"),  # public|unlisted|private
    "selfDeclaredMadeForKids": bool(meta.get("made_for_kids", False)),
  },
}
```
  4. Multipart-related POST to
     `https://www.googleapis.com/upload/youtube/v3/videos?part=snippet,status&uploadType=multipart&notifySubscribers=false`
     with `Authorization: Bearer <token>`, two parts: `application/json` (body) + `video/*` (bytes). Use httpx multipart or a hand-built `multipart/related` body.
  5. On 200: `vid = resp["id"]`; return `{"video_url": f"https://youtu.be/{vid}", "video_id": vid, "final_visibility": body["status"]["privacyStatus"]}`.
  6. On error: raise `UploadError(f"youtube api {status}: {text[:300]}")` (reuse the selenium common error type or a local one).
- [ ] Quick local check (with a real refresh token once Phase 0 done): import + run against a tiny mp4.

### Task 3.2: Dispatch youtube_api in the upload handler
**Files:** Modify `workers-py/src/worker/handlers/upload.py`

- [ ] In `handle()`, before the `provider.endswith("_selenium")` branch, add:
```python
if provider == "youtube_api":
    meta = _resolve_metadata({"youtube": (captions.get("youtube") or {})}, params)
    from worker.providers.youtube_api import upload as yt_api_upload
    loop = asyncio.get_running_loop()
    result = await loop.run_in_executor(None, lambda: yt_api_upload(
        store=self.store, video_key=video_key,
        refresh_token=params.get("oauth_refresh_token",""),
        client_id=os.getenv("GOOGLE_OAUTH_CLIENT_ID",""),
        client_secret=os.getenv("GOOGLE_OAUTH_CLIENT_SECRET",""),
        meta=meta,
    ))
    external_ref = result["video_url"]; video_id = result["video_id"]; final_visibility = result["final_visibility"]
```
  then skip the selenium/telegram branches (use `elif`). The worker container needs `GOOGLE_OAUTH_CLIENT_ID/SECRET` env (add to `dev.compose.yml` worker-upload).
- [ ] Rebuild `worker-upload`; commit.

---

## Phase 4 — Provider selection (API-priority) + quota

### Task 4.1: Provider-pick in the scheduler runner
**Files:** Modify `web-api/internal/platform/scheduler/runner.go` (`tick` + `buildUploadPayload`)

When firing a due upload for a `youtube_*` account, choose the effective provider:
- forced method (schedule metadata `upload_method` = "api"|"selenium") wins;
- else AUTO: if account has API refresh token AND API quota remaining > 0 → `youtube_api`; else if has Firefox profile → `youtube_selenium`; else fail with a clear reason.

- [ ] Pass the account (has refresh token, profile) + computed `apiRemaining` into `buildUploadPayload`. Set `payload["provider"]` accordingly, and for API set `params["oauth_refresh_token"] = acct.OAuthRefreshToken()`, `params["platform"]="youtube_api"`.
- [ ] `go build ./...`; commit.

### Task 4.2: API quota counter
**Files:** Modify `web-api/internal/infrastructure/persistence/read/*` (a small read query) + use in 4.1 and Task 1.3

- [ ] Add `CountAPIUploadsInWindow(ctx, accountID string, since time.Time) (int, error)` reading `pipeline_upload_records` where `provider='youtube_api'` (or platform_target) AND `created_at >= since` for the account. `apiRemaining = max(0, 6 - used)` (6 = floor(10000/1600); make the 6 a const `apiDailyUploadCap`).
- [ ] Wire into the scheduler tick (compute `since = now-24h`) and into `SocialAccountView` (`api_uploads_used`, `api_uploads_limit`).
- [ ] `go build ./...`; commit.

---

## Phase 5 — UI

### Task 5.1: Connect-via-API button + method on the account card
**Files:** Modify `web-ui/src/pages/SocialAccounts.tsx` + `src/api/client.ts`

- [ ] On a YouTube account card: a **"Connect via API"** button → opens `/api/youtube-oauth/start?account_id=<id>` in a new tab; after connect, show a green **"API ✓ <channel>"** badge + `api_uploads_used/limit` ("API quota: 2/6 today"). Keep the existing Selenium "Open session".
- [ ] Show which methods the account supports (API / Selenium chips).
- [ ] `npx tsc --noEmit`; commit.

### Task 5.2: Upload-method choice in the schedule modal
**Files:** Modify `web-ui/src/components/ScheduleUploadModal.tsx`

- [ ] A small select **"Method: Auto (API→Selenium) / Force API / Force Selenium"**. Send `metadata.params.upload_method`. Default "auto". Disable "Force API" when the account has no API token; disable "Force Selenium" when no profile.
- [ ] `npx tsc --noEmit`; commit.

---

## Phase 6 — End-to-end

### Task 6.1: Verify API upload
- [ ] After the user completes Phase 0 (creds in `.env`) and clicks Connect-via-API on the account: confirm `has_api=true`, channel title shown.
- [ ] Schedule an upload with `upload_method=api` (or auto) for a completed run. Verify: provider `youtube_api`, a `youtu.be/<id>` ref, video appears on the channel with correct title/description/visibility, `api_uploads_used` increments.
- [ ] Exhaust/spoof quota (set cap low) → confirm auto falls back to `youtube_selenium`.

---

## Notes / decisions
- **Single OAuth app, per-channel refresh token.** `client_id/secret` are global env; each account stores its own `refresh_token` in `extra`. Tokens are secrets — never log them; never return them in any view.
- **Simple (non-resumable) multipart upload** is fine for our ≤25 MB videos; switch to resumable only if videos grow >~100 MB.
- **Quota** is approximated by counting API uploads × 1600 vs 10000/day. Real quota also spent by other API calls (channels.list ≈ 1 unit) — negligible.
- **Scope `youtube.upload`** is *sensitive*; an unverified app works for the channel owner added as a **Test user** (consent shows an "unverified" warning — expected for personal use).
- Selenium path stays intact as the fallback.
