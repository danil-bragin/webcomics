# YouTube Selenium Upload — Production-Ready Design

Date: 2026-05-29 · Status: design+impl

## Context

Pipeline already produces video + audio, embed login flow works, single upload smoke-test landed video in YT Studio as draft titled "Generated comic" with no description, no tags, unlisted by default. **Goal**: full automation `run → upload → public` with per-project / per-run account binding, configurable metadata, metric trail.

## Decisions (locked w/ user)

| Topic | Decision |
|---|---|
| Metadata source | Chain: run override → template defaults → `caption` step LLM |
| Visibility | Configurable per project & per run + manual "publish public" button in UI |
| Account binding | Explicit: one SocialAccount per project, optional override per run |
| Workflow | Full autopilot run → upload → public (default), manual mode = override visibility=private/unlisted |
| Notifications | Skip for now. Collect upload trail + per-account counters + error screenshots. YT-side stats scrape later. |
| Metafields | Title, description, tags, hashtags, made-for-kids, age restriction, playlists, category, comments, scheduled publish, custom thumbnail |

## Architecture

### Domain additions

#### `SocialAccount` (existing → extend)
New fields:
- `status` — `active | needs_relogin | banned | disabled`
- `last_used_at` — RFC3339 (nullable)
- `cooldown_until` — RFC3339 (nullable) — set on failures
- `failure_streak` — int
- `default_visibility` — `public | unlisted | private` (account-level default)
- `default_made_for_kids` — bool
- `default_category_id` — string (YT category ID, default "22" = People & Blogs)

#### `UploadRecord` (new entity) — write+read
One row per upload attempt. Columns:
```
id, run_id, social_account_id, project_id,
status (pending|uploaded|published|failed),
external_ref (youtube video_id when known),
video_url, title, description, tags[], hashtags[],
visibility, made_for_kids, age_restriction, category_id,
playlist_names[], comments_enabled, scheduled_at,
thumbnail_asset_id, error, error_screenshot_asset_id,
attempts, started_at, finished_at, created_at
```

Stored in new `pipeline_upload_records` table.

#### `Project.defaults.upload` (extension to existing defaults jsonb)
```jsonc
{
  "social_account_id": "<uuid>",   // bound account, may be overridden per-run
  "visibility": "public",
  "made_for_kids": false,
  "age_restriction": "none",       // none | 18plus
  "category_id": "22",
  "comments_enabled": true,
  "tags": ["webcomics", "ai", "shorts"],
  "playlist_names": ["My channel"],
  "title_template": "{{plot.name}} — episode {{run.seq}}",
  "description_template": "Generated from prompt: {{run.prompt}}\\n\\n#Shorts {{tags|join(\" #\")}}",
  "thumbnail_strategy": "frame0" // frame0 | none | custom_per_run
}
```

#### `UploadOverride` (existing → extend)
Add fields matching defaults so any can be overridden per run.

### Caption step → metadata producer
Existing `caption` step generates title/description from script. Extend output to include:
- `title` (≤100 chars)
- `description` (≤5000 chars)
- `tags[]` (16 max)
- `hashtags[]` (3 max — YT shows top of title)

Format prompt: `Generate YouTube Shorts metadata as JSON: {title, description, tags, hashtags}. Keep title catchy (≤100), description engaging (≤300), 5-8 tags, 3 hashtags.`

### Effective metadata resolution (Go side, command time)
```go
// CreateRun handler computes effective upload params:
// 1. Start with caption_output (LLM-generated)
// 2. Merge project.defaults.upload (template substitution applied)
// 3. Merge template.upload_defaults
// 4. Merge run.overrides.upload (highest priority)
// 5. Persist resolved snapshot to UploadRecord
```

### Selenium handler — full metadata flow

Rewrite `selenium_youtube.py` so the upload sequence is:

1. Navigate `studio.youtube.com/channel/UC.../videos/upload`
2. Wait file picker → upload video
3. Wait until upload starts (progress > 0%)
4. **Edit metadata** (current bug — selectors race / wrong index):
   - Title: clear default ("Generated comic"), type configured title
   - Description: type configured description
   - Made for kids radio
5. **Next → "More options"** → expand:
   - Tags input
   - Category dropdown
   - Comments + age restriction
6. **Next → Video elements** (skip — endscreen/cards not in scope)
7. **Next → Checks** (wait passes)
8. **Next → Visibility**:
   - Select `public | unlisted | private` radio
   - If `scheduled_at` → "Schedule" radio + date+time pickers
9. **Publish** / **Save** / **Schedule** button
10. Wait for confirmation modal → extract `https://youtu.be/<id>` link
11. Upload custom thumbnail if asset present (post-publish endpoint)
12. Add to playlists if specified (Studio sidebar → Playlist → toggle)

Selector constants — one file, top-of-module, with `_v1_` prefix → YT DOM change = patch.

**Anti-bot harden**:
- realistic UA already set
- `navigator.webdriver` overridden via `set_preference("dom.webdriver.enabled", False)`
- random viewport on each run (`width = random.choice([1280, 1366, 1440])`)
- small jitter sleeps (0.2-0.6s)

**Failure capture**:
- On any exception: screenshot → MinIO `runs/{run_id}/upload/error.png` → asset_id → UploadRecord.error_screenshot_asset_id
- Body of error: `f"{type(e).__name__}: {str(e)[:500]}"`

### Account scheduling & locking

Since account is explicitly bound per project, scheduling = trivial:
- Project has `defaults.upload.social_account_id`
- Run override can pick a different one but must be from same project
- Worker takes **Redis SETNX advisory lock** `lock:upload:{social_account_id}` with TTL 10 min before opening Firefox — prevents concurrent profile lock collision
- After upload: release lock; update SocialAccount.last_used_at + reset failure_streak

### "Publish now" / "Re-upload" buttons in UI
- `POST /api/runs/:id/upload` → enqueues new upload step bound to this run
- `POST /api/upload-records/:id/publish` → if status=uploaded(unlisted), drive selenium "change visibility to public"
- `POST /api/upload-records/:id/retry` → re-upload

### UI additions

#### Project detail → "Upload defaults" card
- Account dropdown (project's SocialAccounts)
- Visibility default radio
- Made-for-kids checkbox
- Age restriction select
- Category select (YT categories list, hardcoded for v1)
- Comments enabled checkbox
- Tags input (chip list)
- Playlists input (chip list)
- Title template input (with help: `{{plot.name}} {{run.seq}}`)
- Description template textarea
- Thumbnail strategy select
- Save button → updates `project.defaults.upload`

#### Run create form (Studio) → "Upload override" section
- Same fields as Project upload defaults but blank = inherit
- + `social_account_id` override
- + "scheduled_at" datetime input
- + "Skip upload" toggle

#### Run detail → "Upload" tab
- Per upload record card:
  - Status badge (pending/uploaded/published/failed)
  - Account label
  - YouTube video link (when known)
  - Title / description / tags / visibility / scheduled time
  - Error screenshot inline (if failed)
  - Buttons: `Publish public`, `Retry`, `Open on YouTube`

#### Project detail → "Upload history" card
- Last N uploads across all runs of project
- Per row: thumbnail, title, video link, status, account, date
- Counter card per account: total/success/failure, last used

### Project-level "Upload" button (user request: "кнопка в самом web comics опубликовать сам такой action")
On project page header: "Upload last completed run to YouTube" button → enqueues upload step on latest finished run.

## Plan / progress

Legend: `[ ]` todo · `[~]` wip · `[x]` done · `[!]` blocked.

### Phase A — selenium fixes + metadata flow (blocks everything)
- [x] A1: Fix title/description not landing — debug current upload, find right selectors. Title shows "Generated comic" so first textbox isn't title field after file upload.
- [x] A2: Add description field, tags, made-for-kids selectors
- [x] A3: Add visibility radio selectors (public/unlisted/private)
- [x] A4: Add scheduled_at support (date+time picker walk)
- [x] A5: Capture screenshot on failure → MinIO
- [x] A6: Return `video_url` (real youtu.be link) + `video_id`

### Phase B — UploadRecord domain + persistence
- [x] B1: Migration `00003_upload_records.sql`
- [x] B2: Domain `pipeline_upload_record.go` aggregate
- [x] B3: Write repo + read model
- [x] B4: Commands `CreateUploadRecord`, `RecordUploadCompleted`, `RecordUploadFailed`, `UpdateVisibility`
- [x] B5: Queries `GetUploadRecord`, `ListByProject`, `ListByRun`, `ListByAccount`
- [x] B6: Wire into RecordStepCompleted handler — upload step completion writes UploadRecord row
- [x] B7: HTTP routes `/api/upload-records/:id`, `/api/projects/:id/upload-records`, `/api/runs/:id/upload-records`

### Phase C — SocialAccount status fields
- [x] C1: Migration add columns (status, last_used_at, cooldown_until, failure_streak, default_visibility, default_made_for_kids, default_category_id)
- [x] C2: Domain field additions + Update methods
- [x] C3: Read view extends
- [x] C4: Worker writes status updates on upload success/failure (via consumer)
- [x] C5: Redis SETNX advisory lock in upload handler

### Phase D — Project upload defaults + override resolution
- [x] D1: Extend `Project.defaults.upload` jsonb schema (docs only, validate at command time)
- [x] D2: Effective-config computation in `CreateRun` command (chain merge)
- [x] D3: Template substitution helper (`{{plot.name}}`, `{{run.seq}}`, `{{run.prompt}}`)
- [x] D4: Pass full resolved metadata to upload step payload

### Phase E — Caption step → structured metadata
- [x] E1: Update `caption` handler prompt: JSON output {title, description, tags, hashtags}
- [x] E2: Parse JSON, fallback to plain text
- [x] E3: Persist parsed metadata as caption step output
- [x] E4: Downstream consumer reads it for upload step

### Phase F — UI: project upload defaults card
- [x] F1: Form fields (account, visibility, kids, age, category, comments, tags chips, playlists chips, title template, desc template, thumbnail strategy)
- [x] F2: Save mutation hooked into existing `updateProject` defaults
- [x] F3: Categories list (hardcoded subset)

### Phase G — UI: run create override + run detail upload tab
- [ ] G1: Studio override form — `social_account_id` select, scheduled_at, visibility override, skip-upload toggle (deferred — backend types ready, Studio not yet plumbed)
- [x] G2: Run detail "Upload" tab with UploadRecord cards
- [x] G3: Buttons: Publish public, Retry, Open on YouTube
- [x] G4: Inline error screenshot viewer

### Phase H — UI: project upload history + counters
- [x] H1: "Upload history" card on ProjectDetail
- [x] H2: Per-SocialAccount stats badge (total/success/failure)
- [ ] H3: "Upload last completed run" button on project header (deferred — needs separate command)

### Phase I — Headless harden + flakiness
- [ ] I1: Randomized viewport per run
- [ ] I2: Mild typing jitter
- [ ] I3: Click → screenshot → publish_state assertion (fail fast if YT shows challenge)
- [ ] I4: Challenge detection — if URL goes to `accounts.google.com/signin/challenge` → mark account `needs_relogin`, fail UploadRecord with specific error

### Phase J — Thumbnail + playlist + post-publish actions
- [ ] J1: Thumbnail upload selenium flow
- [ ] J2: Playlist add via Studio sidebar
- [ ] J3: Visibility change action (publish public from unlisted) — separate command + handler

### Phase K — YT-side metrics scrape (future, deferred)
- [ ] K1: Periodic task per channel: open Studio analytics, scrape views/likes per video_id
- [ ] K2: New table `upload_metric_samples`
- [ ] K3: Dashboard chart

## File touch list

### web-api
- `internal/domain/pipeline/upload_record.go` (new)
- `internal/domain/pipeline/upload_record_test.go` (new)
- `internal/domain/projects/social_account.go` (extend)
- `internal/app/command/pipeline/upload_record_commands.go` (new)
- `internal/app/query/pipeline/upload_record_queries.go` (new)
- `internal/infrastructure/persistence/write/upload_record_repository.go` (new)
- `internal/infrastructure/persistence/read/upload_records.go` (new)
- `internal/interfaces/http/upload_records_routes.go` (new)
- `internal/interfaces/consumer/pipeline.go` (extend — handle upload.completed/failed → create/update UploadRecord)
- `internal/app/command/pipeline/create_run.go` (effective upload config merge)
- `migrations/00003_upload_records.sql` (new)
- `migrations/00004_social_account_status.sql` (new)

### workers-py
- `src/worker/providers/selenium_youtube.py` (rewrite metadata flow)
- `src/worker/handlers/upload.py` (Redis lock, screenshot capture, status events)
- `src/worker/handlers/caption.py` (JSON output)

### web-ui
- `src/pages/ProjectDetail.tsx` — Upload defaults card + Upload history card + project header upload button
- `src/pages/StudioOverrides.tsx` (or wherever Studio lives) — upload override fieldset
- `src/pages/RunDetail.tsx` — Upload tab
- `src/lib/yt-categories.ts` — hardcoded category list

## Verification

- Phase A: `redis-cli XADD` direct upload msg → YT Studio shows correct title/description/tags/visibility, video_url returned with actual `youtu.be/<id>`
- Phase B+C+D+E: full run via existing UI → UploadRecord persisted, account stats incremented, project defaults applied + overridden
- Phase F+G+H: UI walk-through — set project defaults, create run with override, see record in tab + history
- Phase I: kill Firefox mid-flow → screenshot saved, account marked needs_relogin
- Production check: 5 consecutive uploads on same account, varying visibility/scheduling

## Open questions for later

- Multiple accounts per project? Current decision = one. If you ever need rotation: scheduler from old plan kicks in.
- Twitter/X selenium parity? Twitter selectors very different — new provider file.
- OAuth2 fallback for non-headless reliability? Not now; revisit if selenium ban rate too high.
