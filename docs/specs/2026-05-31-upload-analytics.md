# Upload Analytics: per-upload metrics collection + history

## Problem

Today an upload completes, we store `external_ref` (the YT/IG/TT/FB URL) and
that's it. The operator can't tell from the dashboard whether a video got
100 views or 100k, how comments are trending, when likes plateaued. To
decide which prompts/formats actually work the system needs to *observe*
each upload over time.

## Goal

Background ticker visits every active upload at a configurable interval
(default 6h), pulls platform metrics (views, likes, comments, shares,
duration), stores a snapshot, updates last-known counters on the upload
row. UI shows the trend per-upload + an aggregate dashboard.

## Scope (MVP)

- New `upload_metrics_snapshots` table — append-only time-series rows.
- `pipeline_upload_records` gains `last_known_views/likes/comments/...`
  + `last_fetched_at`.
- One ticker goroutine in `cmd/api` (same model as the scheduler tick) at
  configurable interval (default 6h via `UPLOAD_METRICS_INTERVAL_HOURS`).
- YT: official Data API v3 `videos.list` with API key — public counts
  only (views, likes, comments, durationSec, publishedAt).
- IG / TT / FB: Selenium fetcher reusing the existing Firefox profile
  per account. Open post URL → parse counters from DOM → close.
- UI: line chart per upload on RunDetail's "Scheduled & uploaded"
  section + a sortable "top performers" panel on /dashboard.

Out of scope: YouTube Analytics OAuth (watch-time / CTR / audience),
official IG/FB Graph API, push-based real-time updates, comment
sentiment, alerting on viral spikes.

## Data model

### Migration `00021_upload_metrics.sql`

```sql
-- Add last-known fields to upload row (denormalised for fast list display).
ALTER TABLE pipeline_upload_records
    ADD COLUMN last_known_views    BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN last_known_likes    BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN last_known_comments BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN last_known_shares   BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN duration_seconds    INT NOT NULL DEFAULT 0,
    ADD COLUMN published_at        TIMESTAMPTZ,
    ADD COLUMN last_fetched_at     TIMESTAMPTZ,
    ADD COLUMN fetch_error         TEXT NOT NULL DEFAULT '',
    ADD COLUMN fetch_attempt_count INT NOT NULL DEFAULT 0;

-- Append-only snapshots.
CREATE TABLE upload_metrics_snapshots (
    id                TEXT PRIMARY KEY,
    upload_record_id  TEXT NOT NULL REFERENCES pipeline_upload_records(id) ON DELETE CASCADE,
    fetched_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    views             BIGINT NOT NULL DEFAULT 0,
    likes             BIGINT NOT NULL DEFAULT 0,
    comments          BIGINT NOT NULL DEFAULT 0,
    shares            BIGINT NOT NULL DEFAULT 0,
    raw_json          JSONB NOT NULL DEFAULT '{}'::jsonb -- platform-specific extras
);
CREATE INDEX idx_metrics_upload_time
    ON upload_metrics_snapshots (upload_record_id, fetched_at DESC);
```

### Bridge scheduled_uploads → UploadRecord

The current XADD-style scheduled uploads bypass UploadRecord creation.
Phase 1 also wires the scheduler tick to INSERT a `pipeline_upload_records`
row when the schedule fires. That row carries:

- `run_id` (= scheduled.run_id)
- `social_account_id` (= scheduled.social_account_id)
- `external_ref` set on completion (handleUpload already does this)

Once the row exists, the metrics fetcher has a target to poll.

## Backend

### `internal/domain/uploadmetrics/` (new pkg)

- `Snapshot` value object: views, likes, comments, shares, duration, fetchedAt
- `Fetcher` port:
  ```go
  type Fetcher interface {
      Platform() string
      Fetch(ctx context.Context, externalRef string, profilePath string) (Snapshot, error)
  }
  ```
- Pure helpers: `Delta(prev, next Snapshot) Delta` for growth tracking.

### App layer

- Command `RecordMetricsSnapshot{UploadRecordID, Snapshot}` inserts a snapshot
  row + denormalises last-known into pipeline_upload_records inside one UoW.
- Query `ListUploadSnapshots(uploadID)` → time-series.
- Query `TopPerformers(platform?, since?)` → top-N by views in window.

### Infrastructure fetchers

- `infrastructure/uploadmetrics/youtube_api.go` — videos.list via API key.
  Extracts videoId from URL (`youtu.be/<id>` or `youtube.com/watch?v=<id>`).
- `infrastructure/uploadmetrics/selenium_instagram.go` — open URL with
  headless Firefox profile, parse meta tags / page state for counters.
- `infrastructure/uploadmetrics/selenium_tiktok.go` — same pattern.
- `infrastructure/uploadmetrics/selenium_facebook.go` — same pattern.

Selenium fetchers reuse `providers/_selenium_common.firefox_driver` so
profile cloning + bootstrap is shared with upload flow. They live in
workers-py (Python) because that's where Firefox lives.

### Cross-process design

YT fetcher (HTTP only) lives in Go (`cmd/api` tick).
IG/TT/FB scrapers live in `workers-py/src/worker/handlers/metrics.py`.
Go ticker fans out: YT runs in-process; non-YT sends an XADD payload
`pipeline.metrics.requested {upload_record_id, external_ref, platform, profile_path}`.
The Python worker (new `WORKER_TYPE=metrics`) consumes that stream,
runs the selenium scrape, publishes `pipeline.metrics.completed
{upload_record_id, snapshot}`. Go consumer dispatches RecordMetricsSnapshot.

This split keeps the heavy Firefox dependency out of Go (no docker bloat
of the API image) and reuses the existing Python deployment surface.

### Ticker (cmd/api goroutine)

```go
ticker := time.NewTicker(intervalFromEnv())
for {
    <-ticker.C
    rows := repo.ListUploadsDueForMetrics(now, limit=100)
    for _, r := range rows {
        if r.Platform == "youtube_selenium" {
            // in-process YT fetch
            snap, err := ytFetcher.Fetch(ctx, r.ExternalRef, "")
            ...
        } else {
            // XADD pipeline.metrics.requested
        }
    }
}
```

`ListUploadsDueForMetrics`: `WHERE last_fetched_at IS NULL OR
last_fetched_at < now() - interval` AND `external_ref <> ''` AND
`status = 'uploaded'`.

`intervalFromEnv()` reads `UPLOAD_METRICS_INTERVAL_HOURS` (default 6).

### HTTP

- `GET /api/upload-records/{id}/metrics?since=ISO&until=ISO` → snapshot
  time series (newest 200 by default).
- `GET /api/analytics/top?platform=X&limit=20` → top performers.

## UI

### RunDetail — extend "Scheduled & uploaded" row

Each uploaded row gets a small inline mini-chart (sparkline) of views
over the last N snapshots, plus current likes/comments. Click the row
to open a fuller chart in a modal.

```
06-01 10:30  ▒▒▒▒▒▒▒▒▒▒ 12345 views · 234 likes · 18 comments  [chart]
```

### New /analytics page (skip if scope creep)

Aggregated dashboard: top videos by views in a configurable window
(24h / 7d / 30d), grouped by platform. Each row links back to its
upload + run.

For MVP land the inline sparkline on RunDetail; promote to a full
/analytics page only if scope allows.

## Phases

- [ ] **Phase 1 — schema + scheduler→UploadRecord bridge**
  - migration 00021
  - scheduler runner creates `pipeline_upload_records` row when scheduled
    row goes `in_flight`
  - existing upload completion handler keeps writing external_ref

- [ ] **Phase 2 — YT API-key fetcher in Go**
  - `infrastructure/uploadmetrics/youtube_api.go`
  - `RecordMetricsSnapshot` command + repo
  - test against a real YT video (one of our 12 uploaded)

- [ ] **Phase 3 — metrics tick goroutine**
  - `cmd/api` starts ticker (interval from env, default 6h)
  - Picks due rows, dispatches YT in-process, XADD others

- [ ] **Phase 4 — Python metrics worker**
  - `workers-py/src/worker/handlers/metrics.py`
  - selenium scrapers per platform (reuse `_selenium_common`)
  - WORKER_TYPE=metrics added to compose

- [ ] **Phase 5 — UI inline sparkline**
  - Mini chart in RunDetail Scheduled & uploaded list
  - Last-known counters surfaced
  - Click-through chart modal

- [ ] **Phase 6 — i18n + verify**
  - ru/en strings
  - End-to-end: pick a real YT URL, force tick, verify snapshot row + UI

## Verification

- Phase 1: migrate-up + curl smoke against `/api/schedule` still works
- Phase 2: unit test against fixture videos.list JSON; integration test
  against one real video id
- Phase 3: tick once manually, observe pipeline_upload_records.last_fetched_at
- Phase 4: schedule fake upload completion, verify XADD payload + worker
  log + snapshot row
- Phase 5: open /runs/<id> with uploaded row → mini chart renders
