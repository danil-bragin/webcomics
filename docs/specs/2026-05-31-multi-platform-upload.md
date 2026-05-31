# Multi-platform Upload: Instagram + TikTok + Facebook

## Scope

Extend the Selenium-based upload flow to three more platforms:

- **Instagram Reels** — `instagram_selenium` (instagram.com/reels/create)
- **TikTok** — `tiktok_selenium` (tiktok.com/upload)
- **Facebook Reels/Video** — `facebook_selenium` (facebook.com/<page>/reels)

Caption step gains per-platform output so the worker doesn't have to invent
text. Schedule modal warns when video aspect doesn't match a platform's
strict requirement (9:16 for IG Reels + TT).

## Architecture overview

```
+-----------------+      +-----------------+      +------------------+
| /social page    | ---> | social_accounts | ---> | scheduled_uploads|
| (enabled tabs)  |      | platform=*      |      | per-account row  |
+-----------------+      +-----------------+      +------------------+
                                                            |
                                                            v
+-----------------+      +-----------------+      +------------------+
| scheduler tick  | ---> | XADD upload.req | ---> | worker-upload    |
| (cmd/api)       |      | provider field  |      | dispatcher       |
+-----------------+      +-----------------+      +------------------+
                                                            |
       +-----------+-----------+-----------+----------------+
       v           v           v           v
 youtube_sel  instagram_sel  tiktok_sel  facebook_sel
```

## Phases

- [ ] **Phase 1 — Platform enum + /social tabs enabled**
  - Drop "SOON" gating on IG/TT/FB tabs in `/social` page.
  - `Connect <Platform>` button per tab; Firefox-login flow accepts any
    `platform` string the UI sends.
  - Default `daily_upload_limit` per platform written by the UI on first
    connect (IG=25, TT=10, FB=25).

- [ ] **Phase 2 — Aspect-ratio resolver**
  - Backend: extend `runs view` to surface `assemble_width` + `assemble_height`
    from the assemble step's input.
  - Schedule modal client-side check: if account.platform in (IG, TT) and
    video !=9:16 (1080×1920) → show warning above Schedule button with the
    text "This video is W×H. {platform} Reels require 9:16 — upload may be
    cropped or rejected." Caller can force-confirm.

- [ ] **Phase 3 — Worker provider dispatch**
  - `workers-py/src/worker/handlers/upload.py` looks at `msg["provider"]`
    and dispatches to:
    - `providers/selenium_youtube.py` (existing)
    - `providers/selenium_instagram.py` (new)
    - `providers/selenium_tiktok.py` (new)
    - `providers/selenium_facebook.py` (new)
  - Each provider exposes the same interface:
    `upload(video_path, profile_path, params, captions) -> (external_ref, final_visibility)`
  - Each provider takes screenshot trail per step the same way YT does.

- [ ] **Phase 4 — Caption per-platform fan-out**
  - Extend caption worker to emit:
    ```json
    {"youtube": {...}, "instagram": {...}, "tiktok": {...}, "facebook": {...}}
    ```
  - Each block has `title?, description, tags[], hashtags[]` — IG/TT lean
    on hashtags; YT/FB lean on description + tags.
  - LLM prompt updated to ask for all four blocks at once.

- [ ] **Phase 5 — Polish + verify**
  - i18n strings.
  - Smoke: connect a fake IG account (label only) → schedule a run →
    inspect XADD payload routes to instagram provider.

## Per-platform notes

### Instagram

- URL flow: `instagram.com` → click "Create" → "Post" or "Reel" → file input
  → caption textarea → "Share" button. UI calls them "Видео Reels" in RU.
- Required: 9:16 vertical, ≤90s for Reels, ≤60min for video.
- Default daily limit: 25 (conservative — IG starts shadow-banning above).
- External ref: after publish, navigate to profile → grab the topmost reel
  link `https://www.instagram.com/reel/<shortcode>/`.

### TikTok

- URL flow: `tiktok.com/upload` → file input → caption textarea → privacy
  selector → "Post".
- Required: 9:16. ≤10 min.
- Default daily limit: 10.
- External ref: `https://www.tiktok.com/@<username>/video/<id>`.

### Facebook

- URL flow: `facebook.com/<page>/reels` or composer route. Handles both
  personal feed video + Reels.
- Default daily limit: 25.
- External ref: `https://www.facebook.com/reel/<id>` (Reels) or feed-post URL.

## Out of scope

- Official Graph API integrations (FB/IG) — needs business account + app
  review; selenium is the MVP.
- Instagram carousel uploads (single-asset only).
- Cross-post (one upload triggering N platforms at once) — schedule N rows.
- Per-platform schedule presets ("post to all at 10:00 daily").
