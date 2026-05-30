# Smart Publish Pipeline — Metadata, Multi-Format, Review Gate, Observability

Date: 2026-05-29 (continuation of yt-upload spec)

## Goals

1. **Quality LLM metadata**: title/description natural, audience-appropriate, NO mention of "AI" / "v1" / version markers. Each publication = unique-feeling content.
2. **AI audience determination**: LLM decides `made_for_kids` based on script tone / panels. Conservative default. User override always wins.
3. **Pre-publish flexibility**: per-project toggle `auto | review`. In review mode, status = `pending_review` → UI shows metadata + video preview → user can manually edit OR retry LLM → approve → publish.
4. **Multi-format**: one master video → ffmpeg re-encode per platform. YT Shorts 9:16, IG/TikTok 9:16, X 16:9 or 1:1. Per-platform metadata variants (LLM tailors per platform).
5. **Full observability**: every step status, duration, cost, error, screenshot visible in UI; SSE live stream; structured logs per record.

## Decisions (locked w/ user)

| Topic | Decision |
|---|---|
| Multi-format | Master render → ffmpeg re-encode per platform |
| Pre-publish gate | Per-project toggle (`auto` or `review`), run override allowed |
| Made-for-kids | LLM-determined from script/tone, conservative default, manual override always shown in review |
| Platforms (v1) | YouTube Shorts, Instagram Reels / TikTok, X. YT regular added when needed. |

## Architecture additions

### LLM caption prompt — overhaul

Critical changes from current:

1. Forbid words: "AI", "AI-generated", "auto-generated", "test", "version", "v1", "v2", "experimental", "draft".
2. Write as if from a real human creator. Confident, natural tone.
3. Per-platform metadata variants in SAME LLM call (saves tokens):
   - `youtube_shorts`: title ≤80, description ≤500, 5-8 tags, hashtags must include `#Shorts` + 2 relevant niche tags.
   - `youtube_long`: title ≤80, description 200-500 words, 10-15 tags, no platform-specific hashtags required.
   - `instagram_reels`: caption ≤2200 chars, 10-20 hashtags inline.
   - `tiktok`: caption ≤150 (concise), 4-6 hashtags.
   - `twitter`: tweet ≤270, ≤3 hashtags.
4. Audience scoring:
   - `audience.made_for_kids: bool`
   - `audience.confidence: 0-1`
   - `audience.reasoning: string` (one-sentence justification, surfaced in UI)
5. Content quality fields:
   - `hook`: opening line that grabs attention (≤80)
   - `cta`: optional call-to-action (≤80)
6. Output schema (strict JSON):
   ```json
   {
     "audience": {"made_for_kids": false, "confidence": 0.9, "reasoning": "..."},
     "platforms": {
       "youtube_shorts": {"title":"...","description":"...","tags":["..."],"hashtags":["Shorts","..."]},
       "instagram_reels": {"caption":"...","hashtags":["..."]},
       "tiktok": {"caption":"...","hashtags":["..."]},
       "twitter": {"tweet":"...","hashtags":["..."]}
     }
   }
   ```

### Multi-format ffmpeg encoder

New step type: `encode` (after `assemble`). Input = master MP4. Per-platform recipes:

| Platform | Aspect | Width × Height | FPS | Codec | Notes |
|---|---|---|---|---|---|
| youtube_shorts | 9:16 | 1080×1920 | 30 | h264 high | crop master center, scale, pad if needed |
| youtube_long | 16:9 | 1920×1080 | 30 | h264 | as-is or upscale |
| instagram_reels | 9:16 | 1080×1920 | 30 | h264 baseline | same as YT Shorts but ≤90s |
| tiktok | 9:16 | 1080×1920 | 30 | h264 | same family |
| twitter | 16:9 | 1280×720 | 30 | h264 | ≤140s, ≤512MB |

If master = 1:1 (current default), Shorts/Reels: scale 1080 wide → pad top/bottom to 1920 (letterbox). Long: scale → pad sides.

Implementation: lightweight `worker-encode` (Python) using `ffmpeg-python` or shell `ffmpeg`. New step `pipeline.encode.requested` per platform → MinIO asset per output.

### UploadRecord lifecycle update

New statuses:
```
pending → metadata_ready → pending_review (if review gate) → approved → uploading → uploaded → published
                                       ↓                                ↓
                                    rejected                          failed
```

Plus: edit-able. UI sends `PATCH /api/upload-records/:id/metadata {title, description, ...}` → updates row, marks as `metadata_overridden`. Auto-resume worker on `approved` status.

### Per-project review gate config

Project.defaults.upload.review_mode: `auto | review` (default `review` for safety).

### Pipeline graph (review mode)

```
script → image → assemble → encode(per platform) → caption(LLM) → review_gate
                                                                       ↓
                                                                 [user approves]
                                                                       ↓
                                                                 upload per platform
```

`review_gate` step type just pauses run with status `awaiting_action`. User clicks approve in UI → command emits upload requests.

### Observability — what UI must show

- **Run page**: existing timeline + per-step duration + cost + error + sub-status (e.g. encoding 2/4 platforms). SSE already live.
- **Upload tab** in run page:
  - Per UploadRecord card showing: platform badge, status badge, preview thumbnail, AI-generated title/description (editable), tags, audience verdict + reasoning, scheduled_at, cost, attempts, error screenshot.
  - Buttons: edit, retry LLM, approve, retry upload, open on platform.
  - Status chip animations.
- **Project dashboard**:
  - List of recent uploads with status + platform + account.
  - Per-account stats.
  - Per-platform stats (totals, success rate).
- **Live stream**: existing SSE `/api/runs/:id/events` should fan out UploadRecord changes too.

## Plan / progress

Legend: `[ ]` todo · `[~]` wip · `[x]` done · `[!]` blocked.

### Phase L — LLM prompt overhaul (immediate win, no migration)
- [x] L1: Rewrite caption system prompt — natural tone, no AI mentions, hook/cta, audience verdict, per-platform variants.
- [x] L2: Caption response schema: `{audience, platforms{...}}`.
- [x] L3: Worker resolves per-platform metadata for the right upload record.
- [x] L4: Backfill: when caption.completed fires, update pending UploadRecord rows on the run with the LLM metadata.

### Phase M — UploadRecord lifecycle + manual edit
- [x] M1: Migration `00011_upload_lifecycle.sql`: add `metadata_overridden bool`, expanded status set.
- [x] M2: Domain methods: `ApplyLLMMetadata`, `OverrideMetadata`, `Approve`, `Reject`, `MarkUploading`.
- [x] M3: Commands: `EditUploadMetadata`, `ApproveUploadRecord`, `RejectUploadRecord`, `BackfillUploadMetadata`.
- [x] M4: HTTP: `PATCH /api/upload-records/:id/metadata`, `POST /:id/approve`, `POST /:id/reject`.
- [ ] M5 (deferred): `RetryCaptionForRun` + `POST /runs/:id/retry-caption`.

### Phase N — Review gate step + worker plumbing
- [x] N1: Migration `00012_run_review_gate.sql`: `require_review_before_upload` on `pipeline_runs`.
- [x] N2: Run.advance() pauses to `awaiting_action` before upload when flag set.
- [x] N3: `ResumeFromReview` command + Approve handler resumes run automatically once all upload records are out of pending_review.

### Phase O — Multi-format render (single primary format, full encoder deferred)
- [x] O1: `project.defaults.upload.primary_format` → drives assemble width × height (`applyPrimaryFormat`).
- [x] O2: UI dropdown in UploadDefaultsCard.
- [x] O3: Verified `shorts` → 1080×1920 native render (ffprobe-confirmed).
- [ ] O4 (deferred): Real per-platform encode step using ffmpeg (re-encodes one master into N variants).

### Phase P — Multi-platform upload providers
- [ ] P1: One UploadRecord per (run × account × platform).
- [ ] P2: `selenium_youtube_shorts` (current), `selenium_youtube_long` (DOM same, different params).
- [ ] P3: `selenium_instagram` provider (stub w/ same Firefox profile reuse pattern).
- [ ] P4: `selenium_tiktok` (stub).
- [ ] P5: `selenium_twitter` (stub).

### Phase Q — Observability UI (across-the-board)
- [ ] Q1: SSE fan-out includes upload record updates.
- [ ] Q2: Upload tab redesign — per platform card with full editable metadata + audience verdict + buttons.
- [ ] Q3: Status colours: success green, in_progress amber pulse, awaiting_action blue pulse, failed red.
- [ ] Q4: Per-step live log tail (read last 30 lines from worker log via a new HTTP shim).
- [ ] Q5: Project dashboard updated with per-platform aggregation.

## Verification

- Run autopilot run in `review` mode → status pauses at review_gate.
- UI shows pending_review card with LLM-generated title/description (natural, no AI mention), tags, audience verdict.
- User edits title, clicks approve.
- Pipeline resumes → uploads to YT, IG, TikTok, X with the platform-specific encoded video.
- All 4 UploadRecords land in `published` (or whichever final status the platform reports).
- Project dashboard shows per-platform counters.

## Open questions for later

- Should retry-caption recompute audience verdict, or keep manual override?
- For multi-account YouTube setup later: can the same encoded master be reused, or re-encode per channel for fingerprint avoidance?
- Thumbnail generation pipeline — separate spec.
