# Upload Scheduler + Per-account Rate Limits

## Problem

Today uploads are fire-and-forget: either a run includes an `upload` step that
fires the moment the run finishes, or the operator publishes an ad-hoc XADD
hack. Two real issues:

1. **No window into the future.** Once an upload is queued there is no page
   that says "here is what is scheduled for `@web.comics.forever` over the
   next 24h". Operator has to read worker logs.
2. **No platform rate-limit awareness.** YouTube caps unverified channels at
   ~15 uploads / 24h (rolling). Today's flow happily kicks 20 uploads and the
   last 5 fail at YT's "Загрузка недоступна" gate after a full Selenium round
   trip. Each failed try costs ~30s of worker time + a screenshot.

## Goal

Schedule uploads from any run to any linked account at any future time, with
the system enforcing per-account rate limits at schedule time. One operator
page shows the whole pipeline.

## Scope (MVP)

- Schedule button on `/runs/:id` → modal: pick account + datetime → POST creates
  a `scheduled_upload` row.
- Rolling 24-hour rate-limit window per account. If scheduling the new upload
  would put the count above the limit inside its 24h window, **block** and
  suggest the next free slot (block + suggest, not auto-shift).
- Per-account `daily_upload_limit` + `verified` flag editable on the
  `/social` card.
- Global page `/schedule`: table of all pending scheduled uploads with cancel /
  reschedule actions.
- Times shown + entered in the browser's local TZ; backend stores UTC.
- Worker tick: every 30s, scheduler service picks rows whose
  `scheduled_at <= now() AND status = 'pending'`, marks them `in_flight`, and
  emits `pipeline.upload.requested` for each.

Out of scope for this iteration: auto-shift policy, per-account TZ, recurring
schedules, generation step inside scheduler (only EXISTING completed runs are
schedulable), in-flight cooldown between uploads (handled at worker level).

## Data model

### New migration `00020_upload_scheduler.sql`

```sql
-- 1. New columns on social_accounts for rate-limit config
ALTER TABLE social_accounts
    ADD COLUMN daily_upload_limit INT  NOT NULL DEFAULT 15,
    ADD COLUMN limit_window_hours INT  NOT NULL DEFAULT 24,
    ADD COLUMN is_verified        BOOL NOT NULL DEFAULT FALSE,
    ADD COLUMN min_gap_seconds    INT  NOT NULL DEFAULT 60;
-- daily_upload_limit + limit_window_hours together let us model
-- YT unverified (15 / 24h), YT verified (100 / 24h), IG (10 / 24h), …

-- 2. The scheduler table
CREATE TABLE scheduled_uploads (
    id                TEXT PRIMARY KEY,
    run_id            TEXT NOT NULL REFERENCES pipeline_runs(id)  ON DELETE CASCADE,
    social_account_id TEXT NOT NULL REFERENCES social_accounts(id) ON DELETE CASCADE,
    scheduled_at      TIMESTAMPTZ NOT NULL,
    status            TEXT NOT NULL DEFAULT 'pending',
        -- pending | in_flight | completed | failed | cancelled
    external_ref      TEXT,
    error             TEXT,
    metadata          JSONB NOT NULL DEFAULT '{}'::jsonb,
        -- snapshot of upload params (visibility, tags, title, description …)
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    fired_at          TIMESTAMPTZ,
    completed_at      TIMESTAMPTZ
);
CREATE INDEX idx_sched_due       ON scheduled_uploads (scheduled_at)
    WHERE status = 'pending';
CREATE INDEX idx_sched_account   ON scheduled_uploads (social_account_id, scheduled_at);
CREATE INDEX idx_sched_run       ON scheduled_uploads (run_id);
```

### Resolution math: "next free slot for this account"

```
window = limit_window_hours hours
limit  = daily_upload_limit
now    = current time

counted = count of scheduled_uploads
  WHERE social_account_id = $aid
    AND status IN ('pending', 'in_flight', 'completed')
    AND scheduled_at BETWEEN ($target - window) AND ($target + window)

if counted < limit  → slot OK
else                → find the earliest TIMESTAMP T >= now where the
                      same window query returns < limit.
                      Done by binary search across existing rows + min_gap_seconds.
```

The query is server-side; UI just gets a yes/no + suggestion.

## Backend

### Domain (new pkg `internal/domain/scheduler/`)

- `ScheduledUpload` aggregate: id, runID, accountID, scheduledAt, status,
  metadata, ext refs.
- `ScheduledUploadStatus` enum.
- `RateLimitWindow` value object: `LimitN int, WindowHours int, MinGapSec int`.
- Pure business method: `CanSchedule(now time.Time, existing []SlotPoint,
  limit RateLimitWindow) error` returns `ErrLimitExceeded` or nil. Caller
  passes `SlotPoint{ScheduledAt}` rows inside the window.

### App commands

- `ScheduleUpload{RunID, SocialAccountID, ScheduledAt, MetadataOverride map}`
  → `ScheduleUploadResult{ID, NextFreeSlot?}`. Handler:
  1. Load social account → rate limit config.
  2. Load existing scheduled rows in [target±window].
  3. Call `CanSchedule`. On `ErrLimitExceeded` return result with
     `NextFreeSlot` set, no insert.
  4. Else insert row.
- `CancelScheduledUpload{ID}`.
- `RescheduleUpload{ID, ScheduledAt}` → re-runs limit check.

### App queries

- `ListScheduledUploads(filter{accountID?, status?, since?, until?})` →
  flat rows with `account_label, account_platform, run_prompt`.
- `GetSlotAvailability(accountID, targetAt)` → `{ok bool, next_free_at? }`
  for inline UI feedback.

### Scheduler worker

A new lightweight Go goroutine started in `cmd/api` (no separate process for
MVP):

```go
go scheduler.Run(ctx, deps)
```

Tick = 30s. Loop:

1. `SELECT id FROM scheduled_uploads WHERE status='pending' AND scheduled_at <= now() ORDER BY scheduled_at ASC FOR UPDATE SKIP LOCKED LIMIT 50`.
2. For each row: mark `in_flight`, fired_at=now.
3. Build the `pipeline.upload.requested` Redis payload from metadata + run
   video key + account profile path.
4. XADD to the stream.
5. Existing upload consumer eventually publishes `upload.completed/failed`
   → a new `RawCompletionConsumer.handleScheduledUploadCompleted` updates
   the row status + external_ref.

### HTTP

```
GET    /api/schedule                  (filters: account_id, status, since, until)
GET    /api/schedule/availability?account_id=X&at=ISO
POST   /api/schedule                  body: { run_id, social_account_id, scheduled_at, metadata }
PATCH  /api/schedule/{id}             body: { scheduled_at? }
DELETE /api/schedule/{id}             cancel
```

Per-account limit edits go through the existing
`PATCH /api/social/accounts/{id}` (Phase 2 of the social-accounts refactor)
extended to accept `daily_upload_limit`, `limit_window_hours`, `is_verified`,
`min_gap_seconds`.

## UI

### `/runs/:id` — Schedule button

Add next to existing actions in the header:

```
[ Regenerate step ] [ Delete run ] [ Schedule upload ]
                                   └─ opens modal
```

Modal:

```
┌─ Schedule upload ────────────────────────────────────────┐
│ Account       [ @web.comics.forever (YT) ▾ ]            │
│ Date          [ 2026-06-01 ]    Time [ 10:30 ]          │
│ Visibility    [ public ▾ ]                              │
│ Tags          [meme,shorts,manga…]                      │
│                                                          │
│ ⚠ This account already has 14 uploads in the 24h around │
│   that time. Next free slot: 2026-06-01 14:30           │
│   [Use suggested]                                       │
│                                                          │
│                              [ Cancel ]  [ Schedule ]   │
└──────────────────────────────────────────────────────────┘
```

Live availability check fires on date/time change (debounced 400ms).

### `/schedule` page (nav item: Планировщик)

Default view: list grouped by account, sorted by scheduled_at ASC.

```
┌─ Планировщик ────────────────────────────────────────────┐
│ Account: [ all ▾ ]  Status: [ pending ▾ ]  Date: […]   │
├──────────────────────────────────────────────────────────┤
│ @web.comics.forever · YT · 13/15 in next 24h ████████░░ │
│                                                          │
│ 06-01 10:30  cat reads emails       [✎] [✗]  pending    │
│ 06-01 14:30  dog barks at mailman   [✎] [✗]  pending    │
│ 06-02 09:00  ninja sloth …          [✎] [✗]  pending    │
│                                                          │
│ @another.channel · YT · 0/100 in next 24h ──────────    │
│ (empty)                                                  │
└──────────────────────────────────────────────────────────┘
```

Per-row: ✎ opens edit-time modal; ✗ cancels. Toasts on success/error.

### `/social` account card — limit config

Inline editable fields under each account card:

```
@web.comics.forever                              [active]
/profiles/.../profile
last used: 2h · projects: 0 · uploads: 12

Daily limit: [ 15 ]   Window (h): [ 24 ]   Verified: [☐]   Min gap (s): [ 60 ]
                                                         [ Save ]
```

Defaults: 15/24/false/60 (YT unverified). User flips Verified → suggests
auto-changing limit to 100 (one click).

## Implementation phases

- [ ] **Phase 1 — schema + domain**
  - migration 00020 (4 new account cols + scheduled_uploads table)
  - `internal/domain/scheduler/` aggregate + pure `CanSchedule`
  - WriteRepo: Save / Get / List / Cancel / ListInWindow
  - **Summary:** _pending_

- [ ] **Phase 2 — commands, queries, HTTP**
  - `ScheduleUpload` command (limit check + insert)
  - `CancelScheduledUpload`, `RescheduleUpload`
  - `ListScheduledUploads`, `GetSlotAvailability`
  - HTTP routes mounted under `/api/schedule/*`
  - Extend `PATCH /api/social/accounts/{id}` for limit config
  - **Summary:** _pending_

- [ ] **Phase 3 — scheduler tick goroutine**
  - new `internal/platform/scheduler/runner.go` started from `cmd/api`
  - 30s ticker → SELECT FOR UPDATE SKIP LOCKED → XADD
  - completion handler updates scheduled_uploads row
  - **Summary:** _pending_

- [ ] **Phase 4 — UI: schedule modal + /schedule page**
  - `<ScheduleUploadModal>` reusable
  - Schedule button on `RunDetail`
  - `/schedule` page with grouped list + cancel/reschedule
  - Live availability check
  - **Summary:** _pending_

- [ ] **Phase 5 — UI: per-account limit editor on /social**
  - Inline form on `AccountCard`
  - Verified toggle suggests limit auto-bump
  - Toast on save
  - **Summary:** _pending_

- [ ] **Phase 6 — polish**
  - i18n strings (ru/en)
  - `/schedule` nav item
  - Empty states
  - Integration test: schedule 16 uploads on unverified account → 15 succeed, 16th gets next-slot suggestion
  - **Summary:** _pending_

## Open questions deferred to Phase 7 (future)

- TZ per account (when the operator manages YT channels in different markets).
- Recurring schedules ("every Mon/Wed/Fri at 10:00").
- Generation-on-schedule: schedule a run that hasn't been generated yet; system
  generates closer to the upload time.
- Soft-block notifications when an account is close to the limit (12/15).
- Migration to a dedicated `cmd/scheduler` process when N accounts > 50.

## Verification per phase

- **Phase 1**: `make migrate-up` green. `go test ./internal/domain/scheduler/...` covers `CanSchedule` edge cases (boundary at window edge, gap rule).
- **Phase 2**: curl scenarios — `POST /api/schedule` for 16 rows on a 15-limit account; 16th returns 422 with `next_free_at`.
- **Phase 3**: schedule a row 60s ahead, observe XADD on `pipeline.upload.requested` at the right time; row flips pending → in_flight → completed.
- **Phase 4–5**: browser walk — schedule from RunDetail, see in /schedule list, cancel, observe row gone.
- **Phase 6**: existing runs schedulable end-to-end without errors.
