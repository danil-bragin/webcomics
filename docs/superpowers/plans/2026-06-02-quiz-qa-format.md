# Quiz Q&A Format (v2) Implementation Plan

> **For agentic workers:** Steps use checkbox (`- [ ]`) syntax. Verification is by real pipeline runs (this codebase has no per-handler unit harness), not unit tests.

**Goal:** A "guess → wait → reveal" quiz Shorts format: each question is shown for a long think-gap with a tick-tock sound, then the answer reveals. Videos stretch to ~60s.

**Architecture:** Reuse the pipeline's existing per-panel mechanics. A quiz run emits **paired panels** by index parity (even = question, odd = answer). When `quiz_mode` is on, the run aggregate gives question panels a long duration (`quiz_think_ms`) and answer panels a short one (`quiz_answer_ms`), and attaches a looping tick-tock SFX to every question panel. The audio worker already pads each panel's narration with silence to fill its slot — so a long question panel = question narration + silent gap, with the tick SFX filling that gap. The renderer loops the SFX across the panel.

**Tech stack:** Go (domain aggregate `run.go`, events), Python (audio worker), Node/Remotion (renderer `Comic.tsx`), ffmpeg (synthesize tick SFX), MinIO (SFX asset).

---

## File structure

- `ops/` — one-off ffmpeg script to synthesize the tick-tock SFX, uploaded to MinIO `library/sfx/quiz-tick.mp3`.
- `web-api/internal/domain/pipeline/params.go` — quiz param readers (`quizMode`, `quizThinkMs`, `quizAnswerMs`).
- `web-api/internal/domain/pipeline/events.go` — add `PanelDurationsMs []int` to `AudioRequested`.
- `web-api/internal/domain/pipeline/run.go` — per-panel quiz durations for assemble + audio; tick SFX on question panels.
- `workers-py/src/worker/handlers/audio.py` — honor per-panel durations array.
- `renderer-node/src/compositions/Comic.tsx` — loop per-panel SFX (tick fills the gap).
- `workers-py/src/worker/handlers/script.py` — quiz prompt guidance (alternating Q/A by index parity).

Quiz timing lives in the **assemble step params** (`quiz_mode`, `quiz_think_ms`, `quiz_answer_ms`), the single source of cadence already read by `assemblePanelDurationMs`.

---

### Task 1: Synthesize + upload the tick-tock SFX

**Files:**
- Create: `ops/make-quiz-tick.sh`

- [ ] **Step 1: Write the generator script**

```bash
#!/usr/bin/env bash
# Synthesize a 2s tick-tock loop (two clicks/sec) and upload to MinIO so the
# renderer can loop it under quiz question panels.
set -e
OUT=/tmp/quiz-tick.mp3
# Two short sine clicks (hi tick + lo tock) per second, 2s loop, low volume.
ffmpeg -y -f lavfi -i "sine=frequency=1800:duration=0.04" -f lavfi -i "sine=frequency=1200:duration=0.04" \
  -filter_complex "[0]adelay=0|0,apad=pad_dur=0.46[a];[1]adelay=0|0,apad=pad_dur=0.46[b];[a][b]concat=n=2:v=0:a=1[s];[s]aloop=loop=1:size=88200,atrim=0:2,volume=0.5[out]" \
  -map "[out]" -ar 44100 -ac 2 "$OUT"
docker exec -i webcomics-minio sh -c "cat > /tmp/quiz-tick.mp3" < "$OUT"
docker exec webcomics-minio sh -c "mkdir -p /data/webcomics/library/sfx && cp /tmp/quiz-tick.mp3 /data/webcomics/library/sfx/quiz-tick.mp3"
echo "uploaded library/sfx/quiz-tick.mp3"
```

- [ ] **Step 2: Run it**

Run: `bash ops/make-quiz-tick.sh`
Expected: `uploaded library/sfx/quiz-tick.mp3`, and `docker exec webcomics-minio ls /data/webcomics/library/sfx/` shows `quiz-tick.mp3`.

- [ ] **Step 3: Commit**

```bash
git add ops/make-quiz-tick.sh && git commit -m "feat(quiz): synthesize tick-tock SFX asset"
```

---

### Task 2: Quiz param readers

**Files:**
- Modify: `web-api/internal/domain/pipeline/params.go`

- [ ] **Step 1: Add readers** (near the other `assemble*` param helpers)

```go
// quizConfig pulls quiz cadence from an assemble step's params. quiz_mode=true
// turns on the paired-panel (even=question / odd=answer) timing.
type quizConfig struct {
	Enabled  bool
	ThinkMs  int // duration of each question panel (narration + tick gap)
	AnswerMs int // duration of each answer panel
}

func quizConfigFrom(params map[string]any) quizConfig {
	q := quizConfig{ThinkMs: 8000, AnswerMs: 4000}
	if params == nil {
		return q
	}
	if v, ok := params["quiz_mode"].(bool); ok {
		q.Enabled = v
	}
	if v, ok := params["quiz_think_ms"].(float64); ok && v > 0 {
		q.ThinkMs = int(v)
	}
	if v, ok := params["quiz_answer_ms"].(float64); ok && v > 0 {
		q.AnswerMs = int(v)
	}
	return q
}
```

- [ ] **Step 2: Build** — Run: `cd web-api && go build ./...` Expected: exit 0.

- [ ] **Step 3: Commit** — `git commit -am "feat(quiz): assemble quiz_mode/think/answer param readers"`

---

### Task 3: Add per-panel durations to the audio event

**Files:**
- Modify: `web-api/internal/domain/pipeline/events.go` (`AudioRequested`)

- [ ] **Step 1: Add field** after `PanelDurationMs`:

```go
	PanelDurationMs int   `json:"panel_duration_ms,omitempty"`
	// PanelDurationsMs gives each panel its own slot length (quiz mode: long
	// question panels, short answer panels). When set it overrides the single
	// PanelDurationMs for padding. len == PanelCount.
	PanelDurationsMs []int `json:"panel_durations_ms,omitempty"`
```

- [ ] **Step 2: Build** — `cd web-api && go build ./...` Expected: exit 0.

- [ ] **Step 3: Commit** — `git commit -am "feat(quiz): AudioRequested.PanelDurationsMs"`

---

### Task 4: Quiz per-panel durations + tick SFX in the run aggregate

**Files:**
- Modify: `web-api/internal/domain/pipeline/run.go`

- [ ] **Step 1: Helper** — add near `assemblePanelDurationMs`:

```go
// quizPanelDurations returns a per-panel slot length for a quiz run: even
// indices (questions) get ThinkMs, odd indices (answers) get AnswerMs. Returns
// nil when quiz mode is off, so callers fall back to the single duration.
func quizPanelDurations(cfg StepConfig, n int) ([]int, quizConfig) {
	q := quizConfigFrom(cfg.Params)
	if !q.Enabled || n <= 0 {
		return nil, q
	}
	out := make([]int, n)
	for i := 0; i < n; i++ {
		if i%2 == 0 {
			out[i] = q.ThinkMs
		} else {
			out[i] = q.AnswerMs
		}
	}
	return out, q
}

const quizTickSFXKey = "library/sfx/quiz-tick.mp3"
```

- [ ] **Step 2: assemble — per-panel duration + tick SFX.** In `assemblePanels`, after computing `refs` length is known, compute quiz durations once before the panel loop and apply inside it. Replace the `ref := AssemblePanelRef{...; DurationMs: dur; ...}` construction so quiz overrides `dur` by parity:

```go
		dur, transition := assembleDefaults(cfg.Params)
		quizDurs, quiz := quizPanelDurations(cfg, len(arr))
		timeline := timelinePanelsByIndex(cfg.Params)
		refs := make([]AssemblePanelRef, 0, len(arr))
		for _, entry := range arr {
			key, _ := entry["object_key"].(string)
			idxF, _ := entry["index"].(float64)
			idx := int(idxF)
			panelDur := dur
			if quizDurs != nil && idx < len(quizDurs) {
				panelDur = quizDurs[idx]
			}
			ref := AssemblePanelRef{
				Index:      idx,
				ObjectKey:  key,
				DurationMs: panelDur,
				Transition: transition,
			}
```

(Leave the timeline-override block below unchanged — an explicit timeline duration still wins.)

- [ ] **Step 3: assemble — attach tick SFX to question panels.** Find where the `AssembleRequested` event is built (the `SFXKeys` field). Merge quiz tick keys into the existing SFX map. Locate the assemble emit (`r.Record(AssembleRequested{...})`) and before it build:

```go
		// Quiz mode: tick-tock under every question (even) panel.
		if quiz.Enabled {
			if sfx == nil {
				sfx = map[int]string{}
			}
			for i := 0; i < len(refs); i++ {
				if refs[i].Index%2 == 0 {
					sfx[refs[i].Index] = quizTickSFXKey
				}
			}
		}
```

Wire `sfx` into the `SFXKeys:` field of the emitted `AssembleRequested`. (If the existing code already computes an `sfxKeys` map, reuse that variable name instead of `sfx`.)

- [ ] **Step 4: audio — per-panel durations.** In the `case StepAudio:` block, after `panelDur := assemblePanelDurationMs(r.configSnapshot)`, compute quiz durations and pass them. Read the assemble step's cfg for quiz params via a small lookup (mirror how `assemblePanelDurationMs(r.configSnapshot)` scans the snapshot):

```go
		panelDur := assemblePanelDurationMs(r.configSnapshot)
		quizDurs := assembleQuizDurations(r.configSnapshot, len(captions))
```

Add helper near `assemblePanelDurationMs`:

```go
// assembleQuizDurations returns per-panel slot lengths if the run's assemble
// step has quiz_mode on, else nil.
func assembleQuizDurations(snapshot []StepConfig, n int) []int {
	for _, s := range snapshot {
		if s.Type == StepAssemble {
			d, _ := quizPanelDurations(s, n)
			return d
		}
	}
	return nil
}
```

Then add `"panel_durations_ms": quizDurs` to the audio `inputJSON` map and `PanelDurationsMs: quizDurs` to the `AudioRequested{...}` struct.

- [ ] **Step 5: Build** — `cd web-api && go build ./...` Expected: exit 0. (If `StepAssemble`/`StepConfig.Type` names differ, fix to match — grep `StepAssemble` and `func assemblePanelDurationMs`.)

- [ ] **Step 6: Commit** — `git commit -am "feat(quiz): per-panel quiz durations + tick SFX in run aggregate"`

---

### Task 5: Audio worker honors per-panel durations

**Files:**
- Modify: `workers-py/src/worker/handlers/audio.py`

- [ ] **Step 1: Read the array + pad per panel.** In `handle`, read `panel_durations_ms = msg.get("panel_durations_ms") or []`. Pass it into the per-panel synth path. Change `_stitch_to_panels(panel_mp3s, panel_duration_ms)` to accept an optional per-panel list:

```python
def _stitch_to_panels(panel_mp3s: list[bytes], panel_duration_ms: int,
                      panel_durations_ms: list[int] | None = None) -> bytes:
    ...
    for i, mp3 in enumerate(panel_mp3s):
        slot_ms = panel_duration_ms
        if panel_durations_ms and i < len(panel_durations_ms) and panel_durations_ms[i] > 0:
            slot_ms = panel_durations_ms[i]
        target_s = slot_ms / 1000.0
        # ... existing silence-pad logic using target_s, but the shared silence
        # track must be (re)generated per distinct slot length, not reused once.
```

The existing code pre-renders ONE shared silence clip of `panel_duration_ms`; with variable slots, generate the pad length per panel (`target_s - len(tts)`). Keep a small cache keyed by rounded pad seconds to avoid re-encoding identical pads.

- [ ] **Step 2: Thread the param** at the call site in `handle` / `_synthesize_per_panel`:

```python
panel_durations_ms = [int(x) for x in (msg.get("panel_durations_ms") or [])]
...
merged = _stitch_to_panels(per_panel_mp3s, panel_duration_ms, panel_durations_ms)
```

- [ ] **Step 3: Rebuild** — `docker compose -f dev.compose.yml up -d --build worker-audio` Expected: container Started.

- [ ] **Step 4: Commit** — `git commit -am "feat(quiz): audio worker per-panel slot padding"`

---

### Task 6: Renderer loops per-panel SFX (tick fills the gap)

**Files:**
- Modify: `renderer-node/src/compositions/Comic.tsx` (the `sfxSequences.push(...)` block)

- [ ] **Step 1: Add `loop`** to the per-panel SFX Audio so a short tick repeats across the whole (long) question panel:

```tsx
            <Sequence key={`sfx-${p.index}`} from={cursor} durationInFrames={durationFrames}>
              <Audio src={sfxSrc} volume={0.5} loop />
            </Sequence>
```

(0.5 keeps the tick present but under the voiceover; it's bounded by the Sequence so it stops at the answer panel.)

- [ ] **Step 2: Rebuild** — `docker compose -f dev.compose.yml up -d --build renderer` Expected: Started.

- [ ] **Step 3: Commit** — `git commit -am "feat(quiz): loop per-panel SFX so tick fills the think gap"`

---

### Task 7: Quiz prompt guidance (alternating Q/A panels)

**Files:**
- Modify: `workers-py/src/worker/handlers/script.py` (system-prompt composition)

- [ ] **Step 1:** When the run's params carry `quiz_mode`, append a guidance block instructing the model to emit panels as alternating pairs: even index = the question (caption poses it, no answer), odd index = the answer (caption reveals it). Image `prompt` of the question panel = an intriguing visual hint; the answer panel = the reveal. Keep total panels even.

```python
if (params or {}).get("quiz_mode"):
    sections.append(
        "QUIZ FORMAT: Produce panels as alternating QUESTION/ANSWER pairs. "
        "Even-index panels (0,2,4,...) are QUESTIONS — the caption asks one "
        "punchy guessable question and must NOT contain the answer. "
        "Odd-index panels (1,3,5,...) are ANSWERS — the caption reveals the "
        "answer with a satisfying one-liner. The question panel's image prompt "
        "is an intriguing visual hint; the answer panel's image prompt is the "
        "reveal. Escalate difficulty. Total panel count must be even."
    )
```

Pass `quiz_mode` from the run's script params (the run already forwards `params` to the script event).

- [ ] **Step 2: Rebuild** — `docker compose -f dev.compose.yml up -d --build worker-script` Expected: Started.

- [ ] **Step 3: Commit** — `git commit -am "feat(quiz): script prompt guidance for alternating Q/A panels"`

---

### Task 8: End-to-end verification run

- [ ] **Step 1:** Restart the Go api + consumer (they embed the changed domain). Create a quiz run in a project with: `panel_count: 8` (4 Q&A pairs), script `quiz_mode: true`, assemble `quiz_mode: true, quiz_think_ms: 9000, quiz_answer_ms: 4000`, music `genre: epic`, energetic voice, `impact_meme` subs.

- [ ] **Step 2: Verify** the completed run:
  - Script produced 8 panels, even = questions, odd = answers.
  - Assemble request: even panels `duration_ms ≈ 9000`, odd ≈ `4000`; `sfx_keys` has `library/sfx/quiz-tick.mp3` on even indices.
  - Final video ≈ `4*(9+4) = 52s`; question segments have audible ticking during the silent gap; answer follows.

- [ ] **Step 3:** Watch `http://localhost:5173/runs/<id>` — confirm: question shows, tick during wait, answer reveals, no abrupt music cut. Tune `quiz_think_ms` / SFX volume if needed.

---

## Notes / deferred

- **Visible 3-2-1 countdown overlay** on question panels is a follow-up (renderer `Caption`/overlay layer); this plan ships the audible tick + the long gap, which is the core retention mechanic.
- Per-panel durations now flow to audio + assemble; the timeline editor's explicit per-panel duration still overrides quiz timing (intentional).
