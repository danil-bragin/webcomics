# Subtitles + Background Music + Shorts Hardening

Date: 2026-05-29 (continuation)

## Goals

1. **Subtitles** — two flavors:
   - **Burned-in** in the video frame so mobile viewers reading w/o sound still consume the joke.
   - **YouTube native CC** (.srt) uploaded as caption track so search + accessibility benefit.
2. **Background music** — every short has audible character even when no voiceover.
   - Bundled CC0 library in MinIO with mood/genre/tempo tags.
   - LLM picks track based on script tone.
   - Mixed under voiceover with sidechain-style ducking (-12 dB when voice plays).
3. **YT Shorts hardening** — guarantee every upload qualifies as Shorts:
   - Vertical 9:16 (already done — Phase O).
   - Duration cap 60s (the discovery sweet spot, not the 180s upper limit).
   - `#Shorts` hashtag forced into title or description.

## Decisions (locked w/ user)

| Topic | Decision |
|---|---|
| Subtitle text source | `panel.caption` from existing script LLM (no extra LLM call) |
| Burn style | Per-project / per-run select: bottom-karaoke / impact-meme / word-pop |
| Music library | Bundled CC0 set in MinIO; LLM picks by `tags` JSON |
| Shorts mode | vertical 9:16 + ≤60s + `#Shorts` enforced in caption LLM output |

## Architecture

### Subtitles — burned-in

Implemented in **Remotion `Comic.tsx`**, NOT as a separate worker. Each panel already has `caption` + `duration_ms` + index. Add a subtitle layer:

```jsx
<SubtitleLayer panels={panels} style={subtitleStyle} />
```

Three preset components:
- `BottomKaraoke` — fixed bottom-center, white text + black outline + drop shadow, bold sans-serif (Inter or Anton), 48-64 pt, fade in/out per panel boundary.
- `ImpactMeme` — Impact font, top OR bottom, all-caps, classic meme look. White + black stroke.
- `WordPop` — split caption into words, each appears with bounce/scale animation timed to TTS word boundary (if voiceover present) or evenly spaced.

Style selected via `assemble` step `params.subtitles = {enabled: bool, style: "bottom_karaoke" | "impact_meme" | "word_pop", position: "bottom" | "top"}`. Set from `project.defaults.subtitles` or run override.

#### Safe area for vertical (9:16)
YT Shorts overlays its own UI on bottom 30% and top 15%. Subtitle layer renders at vertical center 60-80% by default. Configurable.

### Subtitles — YT native CC

After `assemble` finishes, build an `.srt` file from `(panel.index, caption, duration_ms)`. Push to MinIO as `runs/<id>/captions/youtube.srt`. Upload worker switches to:
1. Upload video file (existing flow).
2. After successful upload → navigate to video's edit page → Subtitles tab → upload .srt.

Selenium addition only — no domain change. Toggle via `subtitles.upload_cc` flag.

### Background music library

#### Storage layout
MinIO bucket `webcomics` already exists. New prefix:
```
library/music/
  energetic-pop-01.mp3
  energetic-pop-02.mp3
  chill-lofi-01.mp3
  comedic-bouncy-01.mp3
  cinematic-build-01.mp3
  ...
  manifest.json
```

`manifest.json` schema:
```json
[
  {
    "id": "energetic-pop-01",
    "title": "Sunset Drive",
    "object_key": "library/music/energetic-pop-01.mp3",
    "duration_s": 35.0,
    "bpm": 124,
    "mood": ["upbeat", "happy", "summer"],
    "genre": ["pop", "edm"],
    "tempo": "fast",
    "loop_friendly": true,
    "license": "CC0",
    "source": "pixabay"
  }
]
```

Initial seeding: ~20 tracks across moods. One-shot Make target `make seed-music` downloads from a curated Pixabay/Freesound list (CC0 only) into MinIO.

#### LLM picker
After script step, before assemble, new step `music_pick` (or inline inside existing `music` step). Worker:
1. Reads script's overall tone (premise + panel captions).
2. Calls LLM with manifest's mood/genre/tempo + the script.
3. Returns chosen `object_key` + reasoning.
4. Persists choice as step output.

Prompt:
```
You are a music supervisor for a 30-60 second comic short.
Script: {script_text}
Available tracks (id, mood, genre, tempo):
{track_list}
Pick the ONE track id that best matches the comedic / emotional tone.
Respond JSON: {"track_id": "...", "reasoning": "..."}
```

### Audio mixing

Existing pipeline produces a voice asset from ElevenLabs. New `assemble` flow:
1. Master video has panel images + voiceover (if any).
2. Music track loops/trims to video length.
3. Mix:
   - Voice at 0 dB (reference).
   - Music at -8 dB constant when **no voice**.
   - Music at -20 dB when voice is detected (manual ducking — we know the voice file's audible windows).
4. Output single composite audio track muxed into MP4.

Implementation lives in renderer-node (Remotion can mix multiple `<Audio>` elements with `volume` and `playbackRate`). Pass `musicSrc` + `voiceSrc` + a `voiceWindows: [{start_ms, end_ms}]` array.

If voice asset isn't available → music fills 100% at -6 dB (always audible).

### Shorts hardening

Already enforced (Phase O): vertical 1080×1920.
Add:
1. **Hard duration cap**: assemble step rejects > 60_000 ms. Auto-trim if 60_000–62_000 (small grace).
2. **#Shorts enforcement**: caption LLM prompt already requests it; on backfill, hashtag presence guaranteed before persist.
3. **Upload provider stays `youtube_selenium`** — vertical + duration triggers YT's auto-classification.
4. Optional later: `youtube_shorts_selenium` that navigates to `studio.youtube.com/shorts/upload` (verify URL exists in YT Studio).

## Plan / progress

Legend: `[ ]` todo · `[~]` wip · `[x]` done · `[!]` blocked.

### Phase R — Subtitles
- [x] R1: Domain `subtitles` field parsed from assemble step params.
- [x] R2: Renderer `BottomKaraoke`, `ImpactMeme`, `WordPop` presets.
- [x] R3: Renderer dispatches by `caption.style_preset`.
- [x] R6: Project default + UI dropdown for subtitle style/position.
- [x] R7: Verified on real Shorts upload — `https://youtu.be/-42gqkNSo3E`.
- [ ] R4 (deferred): `.srt` generation upload as YT CC track.
- [ ] R5 (deferred): Selenium navigates to video edit page → uploads CC.

### Phase S — Music library + picker (production)
- [x] S1: `ops/music-library/sources.json` curated direct CDN URL list (Kevin MacLeod incompetech, CC BY 3.0).
- [x] S1a: `scripts/refresh_music_library.py` downloads tracks → MinIO + writes manifest. 10 tracks live.
- [x] S2: Reuse existing `music` step.
- [x] S3: `worker-music` handler picks via LLM prompt over manifest mood/genre/tempo.
- [x] S4: Music step output stored as `pipeline_assets.kind = 'music'` (auto-flow via existing aggregate).
- [x] S5: Renderer mixes music + voice via existing `volume={0.3}` constants.
- [x] S7: Verified on real Shorts upload — `https://youtu.be/W8w9oTxzUG4`, AAC audio confirmed via ffprobe.
- [x] Make target `refresh-music` for periodic library updates.
- [ ] S6 (deferred): UI project toggle + manual override.
- [ ] S6a (deferred): `AppendMusicAttribution` domain method exists but not auto-wired into upload (CC BY requires credit). User can edit description before publish.
- [ ] S8 (deferred): Real Pixabay/Jamendo API integration for auto-trending refresh (Pixabay has no music API; switch to Jamendo if scale demanded).

### Phase T — Shorts hardening
- [x] T2 + T3: LLM caption prompt requires `#Shorts`; Approve handler enforces it.
- [x] T4: UploadRecord.Approve guarantees `#Shorts` in description + hashtags before transitioning to `approved`.
- [x] T5: Verified on real Shorts upload — `https://youtu.be/VVA2GjpNjz8` has `#Shorts` and vertical 9:16.
- [ ] T1 (deferred): assemble rejects >60 s + auto-trim 60–62 s.

## Verification

- Run autopilot end-to-end with subtitles=on + music=auto.
- Open the resulting YT Shorts video on a phone:
  - Subtitles visible mid-screen, readable on small display.
  - Background music audible, voiceover (if present) clear.
  - Video appears in channel's Shorts shelf (not "Videos" shelf).
- Open the video in Studio: CC track exists, subtitles tab populated.

## Open questions for later

- Music auto-pick per platform? (TikTok prefers different vibes than YT.)
- Subtitle translation: generate captions in EN + RU + ES via LLM?
- Sound effects layer (impact whooshes per panel cut) — separate spec.
