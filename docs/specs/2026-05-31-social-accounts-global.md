# Social Accounts: Global Library

## Problem

Today `social_accounts.project_id` is mandatory: every YouTube channel/Telegram bot/etc. lives inside one project. So:

- Same channel = re-auth + re-create per project (Firefox login flow each time)
- No global "social media" picture; user has to drill into a project to see what's connected
- Cooldown / failure-streak per (account, project) instead of per real account
- Mental model mismatch with the audio library, which is global and *referenced* from projects

## Goal

Lift social accounts out of project scope into a global library, mirror the audio-library UX, and let projects (and individual runs) reference them.

## Scope

- New `/social` page: list/create/auth/delete accounts globally
- Project Social tab refactor: link/unlink global accounts, mark one default
- Upload pipeline resolves which account to use: explicit run override → project default → fail
- YouTube only in MVP; UI scaffolded for IG/TikTok/Telegram (disabled tabs)
- Existing `social_accounts` rows: 0 (wiped). No migration data; just schema change.

Out of scope: OAuth2 (still Firefox-profile login), multi-user, public API tokens.

---

## Data model

### Schema changes (single migration `00019_social_global.sql`)

```sql
-- 1. Truncate (already empty, defensive)
DELETE FROM social_accounts;

-- 2. Make global by allowing NULL project_id; backfill not needed.
ALTER TABLE social_accounts ALTER COLUMN project_id DROP NOT NULL;
-- (column was already nullable per inspection — confirm before drop)

-- 3. New many-to-many link table
CREATE TABLE project_social_account_links (
  project_id        TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  social_account_id TEXT NOT NULL REFERENCES social_accounts(id) ON DELETE CASCADE,
  is_default        BOOLEAN NOT NULL DEFAULT FALSE,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (project_id, social_account_id)
);
CREATE INDEX project_social_account_links_account ON project_social_account_links(social_account_id);

-- 4. Partial unique: at most one default per (project_id, platform)
CREATE UNIQUE INDEX project_social_account_links_default
  ON project_social_account_links (project_id,
    (SELECT platform FROM social_accounts WHERE id = social_account_id))
  WHERE is_default;
-- NOTE: subselect in index not allowed in PG; enforce via trigger or app code instead.
-- Pick app-side enforcement (simpler): on set-default, transactionally clear other defaults
-- for same (project_id, platform).
```

### Aggregate boundaries

- `SocialAccount` aggregate (new pkg `internal/domain/socialaccounts/`): identity-only, no project knowledge. Owns auth state, cooldown, defaults.
- `Project` aggregate: holds a list of linked `SocialAccountID` + optional `defaultSocialAccountIDByPlatform map[string]SocialAccountID`. Link/unlink/setDefault are project mutations.
- `Run` (already exists): upload step input carries optional `social_account_id`.

### Account resolution order at upload time

1. Step params: `step.params.social_account_id` (explicit per-step)
2. Run overrides: `overrides.upload.social_account_id`
3. Project default for upload's platform: `project.defaultSocialAccountIDByPlatform[platform]`
4. Otherwise fail: `upload_step requires social_account_id`

This keeps the existing "first account" magic gone — explicit defaults make behavior predictable.

---

## App layer

### New commands (under `internal/app/command/socialaccounts/`)

- `CreateSocialAccount{platform, label, firefoxProfilePath}` → emits domain event
- `RenameSocialAccount{id, label}`
- `DeleteSocialAccount{id}` — cascades unlink from all projects
- `RecordAccountUsed{id, ok|fail}` — bumps `last_used_at` / `failure_streak`; sets cooldown if too many failures

### Project commands (extend existing `internal/app/command/projects/`)

- `LinkSocialAccount{projectID, socialAccountID, asDefault bool}`
- `UnlinkSocialAccount{projectID, socialAccountID}`
- `SetDefaultSocialAccount{projectID, platform, socialAccountID}`

### Queries (under `internal/app/query/socialaccounts/`)

- `ListSocialAccounts(filter{platform?, status?})` → flat list
- `GetSocialAccount(id)`
- `ListProjectLinkedAccounts(projectID)` → list of `{account, is_default}`

### Pipeline reuse

Existing upload command already takes a `social_account_id`. Just feed it from new resolver chain. No new command needed there.

---

## HTTP routes

New, under `/api/social/...` (mirrors `/api/audio/...`):

- `GET    /api/social/accounts?platform=youtube&status=active` → list
- `POST   /api/social/accounts` → create stub + return id (auth happens via Firefox flow next)
- `GET    /api/social/accounts/:id` → detail
- `PATCH  /api/social/accounts/:id` → rename / archive
- `DELETE /api/social/accounts/:id` → cascade unlink

Auth flow (existing `firefox_login.go`):
- Today: `POST /api/projects/:id/social-accounts/firefox/start` returns container URL
- New: `POST /api/social/accounts/:id/firefox/start` (no project) — same handler, just doesn't link a project
- Keep the project route as a convenience that does both

Project-scoped link routes (new):
- `GET    /api/projects/:id/social-accounts` (already exists — extend to return `is_default`)
- `POST   /api/projects/:id/social-accounts/:aid` body `{as_default: bool}` → link
- `DELETE /api/projects/:id/social-accounts/:aid` → unlink
- `PUT    /api/projects/:id/social-accounts/:aid/default` → mark default for that account's platform

CreateRun/RegenerateUploadStep:
- `overrides.upload.social_account_id?: string` — passed through to upload step params.

---

## UI

### `/social` (new page)

Layout mirrors `/library/audio`:

```
┌──────────────────────────────────────────────────────────────┐
│ Social Accounts                                              │
│ Global library — link to projects on demand                  │
├──────────────────────────────────────────────────────────────┤
│ [ YouTube ] [ Telegram ▾coming ] [ Instagram ▾coming ] ...   │ ← platform tabs
├──────────────────────────────────────────────────────────────┤
│ [ + Connect YouTube ]                                        │
│                                                              │
│ ┌──────────────────────────┐  ┌──────────────────────────┐   │
│ │ ▶ MemeMachine (YT)       │  │ ▶ DailyShorts (YT)       │   │
│ │   active · used 2h ago   │  │   cooldown until 12:00   │   │
│ │   12 uploads · 3 projects│  │   8 uploads  · 1 project │   │
│ │   [edit] [archive] [del] │  │   [edit]      [del]      │   │
│ └──────────────────────────┘  └──────────────────────────┘   │
└──────────────────────────────────────────────────────────────┘
```

Add flow: click "+ Connect YouTube" → modal with label input → POST stub → opens Firefox container in new tab for login → polls until profile saved.

### Project detail — "Social & Upload" tab refactor

Today: list of project-owned accounts + add button.
After: list of LINKED global accounts + "Link account" button.

```
Linked YouTube channels
┌────────────────────────────────────────────────────────┐
│ ⭐ MemeMachine (default) · used today · 12 uploads     │
│    [set as not default] [unlink]                       │
├────────────────────────────────────────────────────────┤
│   DailyShorts · used 3d ago · 8 uploads                │
│    [set as default] [unlink]                           │
└────────────────────────────────────────────────────────┘
[ + Link account ]    ← opens picker modal of global accounts
                       (with inline "create new" → /social page)
```

### Studio / RunDetail upload picker

Existing field gains a dropdown: "Upload to: ⭐ MemeMachine (project default) | DailyShorts | …" — only shows accounts linked to the project (or any global if no project).

---

## Implementation order

Five small steps, each shippable independently.

### Phase 1: schema + domain (no UI change)
- migration `00019_social_global.sql`
- new `socialaccounts/` domain pkg, write repo
- adapt existing project domain to use link table (project.SocialAccountIDs() → reads link table)
- Phase exit: existing UI still works (project Social tab keeps acting as before, just routing through link table)

### Phase 2: global CRUD endpoints + /social page
- list/create/delete/rename routes
- React page with platform tabs + cards + add modal
- existing Firefox flow reachable without project
- Phase exit: user can create + auth YT account from /social, sees it in list

### Phase 3: project link UI
- `POST/DELETE /api/projects/:id/social-accounts/:aid`
- project Social tab shows "Link account" picker
- mark/clear default
- Phase exit: user links MemeMachine to two projects

### Phase 4: upload resolution chain
- account_id resolver in upload command handler
- studio/run detail dropdown
- pipeline upload step uses resolver, fails clearly if none found
- Phase exit: user submits a run with explicit account override; upload uses it

### Phase 5: polish
- shared cooldown across projects (compute on global account, not per-link)
- audit log: who linked/unlinked when
- empty states + toasts on link/unlink

---

## Open trade-offs called out

- **Default per platform vs per-project**: chose per (project, platform) so one project can have YT default + IG default simultaneously. Slightly more code but matches mental model.
- **Cooldown shared across projects**: yes — if YT throttles us, that account is in cooldown everywhere, not per project.
- **Firefox profile path**: still account-local (one profile per account). Multiple projects sharing an account share the profile. Lock contention: serialize uploads per account (existing per-account queue assumed).
- **Migration**: pure delete + new schema. If we ever ship to multi-env we'll need backfill — flagged in commit message.

---

## Verification

- Phase 1: `go test ./internal/domain/...` + `make migrate-up` green, existing /projects still loads
- Phase 2: create 2 YT accounts from /social, both visible
- Phase 3: link both to one project, mark one default; unlink, verify cascade
- Phase 4: run with explicit social_account_id uploads to the picked account; run without falls back to project default; run without default fails with clear error
- Phase 5: kill upload mid-flight to trigger failure_streak, verify cooldown applies to both linked projects
