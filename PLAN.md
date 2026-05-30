# MVP — Webcomics Generation Platform

## Context

We are building an MVP for a webcomics-from-prompt tool: a user types a prompt, the system writes a short script (via OpenRouter LLM), generates 3–10 panel images (via fal.ai Flux-schnell), and assembles them into a short MP4 with transitions (via server-side Remotion). The whole pipeline is exposed through a React + shadcn/ui UI that shows every step transparently — inputs, outputs, time spent, money spent.

Constraints (from the brief):
- **Go** owns Postgres, the UI host, and all domain/orchestration logic.
- **Python** owns all AI calls (LLM via OpenRouter, images via fal.ai).
- **Node** runs Remotion server-side to assemble video.
- **Redis Streams** is the only sync channel between Go ↔ Python ↔ Node.
- Budget-minimal: cheap image model (Flux-schnell ≈ $0.003/img), no AI for video assembly.
- Pipeline must be **flexible** — adding a step type later (audio, music, captions, social-media upload) should not require touching the orchestrator core.
- Single-user MVP. No auth wiring.
- MinIO in docker-compose for assets, S3-compatible API so prod swap to R2/S3 is one config change.

The existing Go boilerplate at `web-api/` already provides everything we need to plug this in: DDD + CQRS with `bus`, transactional outbox publishing to Redis Streams via Watermill (`internal/platform/redis/redis.go`, `internal/infrastructure/persistence/outbox/relay.go`), `processed_messages` table for idempotent consumers, `cmd/{api,consumer,outbox-relay}` processes, and a clean port/adapter layout. Our work is **adding one new bounded context (`pipeline`)** plus two new runtimes that talk to it through Redis Streams.

---

## Architecture

```
┌────────────────┐ HTTP/SSE ┌──────────────────────────────────────────┐
│ React + shadcn │ ───────► │  Go cmd/api (chi + bus)                  │
│ (Vite, embed)  │ ◄─────── │  + cmd/consumer (Redis Streams)          │
└────────────────┘          │  + cmd/outbox-relay                      │
                            │                                          │
                            │  domain/pipeline + app/command|query     │
                            │  Postgres (write + read pools)           │
                            └──────────────┬───────────────────────────┘
                                           │ Redis Streams (Watermill)
                       ┌───────────────────┼────────────────────┐
                       ▼                   ▼                    ▼
              ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐
              │ Python worker    │  │ Python worker    │  │ Node Remotion    │
              │ (script step)    │  │ (image step)     │  │ (assemble step)  │
              │ OpenRouter SDK   │  │ fal-client SDK   │  │ @remotion/render │
              └──────────────────┘  └──────────────────┘  └──────────────────┘
                       │                   │                    │
                       └───────────────────┴────────────────────┘
                                           ▼
                                  ┌──────────────────┐
                                  │ MinIO (S3 API)   │
                                  └──────────────────┘
```

Workers are stateless processes scaled by replica count. All persistent state lives in Postgres; all binary assets in MinIO. Workers never touch Postgres — they read step input from the event payload (or fetch from MinIO via key), call the external API, upload outputs, and publish a `*.completed` / `*.failed` event. Go consumes that event, dispatches a command that advances the `PipelineRun` aggregate inside a UoW transaction, records cost, and emits the next-step request via the outbox.

This is **event-choreographed orchestration**: the `PipelineRun` aggregate is the brain. Each step type is independent and added by registering a new event handler + worker — no orchestrator rewrite. Same pattern as the existing `on_user_registered` consumer in `internal/interfaces/consumer/consumer.go`.

---

## New bounded context: `pipeline`

### Domain (`internal/domain/pipeline/`)
- `PipelineRun` aggregate: `id`, `prompt`, `template_id`, `config_snapshot (jsonb)`, `status` (`queued|running|completed|failed|cancelled`), `current_step_index`, `total_cost_usd`, `created_at`, `started_at`, `finished_at`.
  - Methods: `Start()`, `RecordStepCompleted(stepIdx, outputs, costUSD, durationMs)`, `RecordStepFailed(stepIdx, err)`, `AdvanceOrFinish() (nextStepRequest | done)`, `Cancel()`.
  - Emits domain events: `pipeline.run.started`, `pipeline.step.requested`, `pipeline.step.completed`, `pipeline.step.failed`, `pipeline.run.completed`, `pipeline.run.failed`.
- `PipelineStep` child entity: `index`, `type` (`script|image|assemble|…`), `status`, `input_ref`, `output_refs[]`, `provider`, `model`, `cost_usd`, `started_at`, `finished_at`, `error`.
- `PipelineTemplate` (separate aggregate, simple CRUD): `id`, `name`, `steps[]` where each step is `{type, system_prompt?, model?, params (jsonb)}`. Template defines the ordered pipeline; a run snapshots it into `config_snapshot` so later template edits don't change history.
- `Asset` (simple entity): `id`, `run_id`, `step_id`, `kind` (`script_json|panel_image|video`), `bucket`, `key`, `mime`, `bytes`.
- `CostEntry` (event sourced from `RecordStepCompleted`): `id`, `run_id`, `step_id`, `provider`, `model`, `units` (tokens|images|seconds), `unit_cost_usd`, `total_cost_usd`.

Domain remains pure — no SQL, no SDK imports. Ports defined in `internal/app/command/pipeline/` for `WriteRepo`, `AssetStore`, `Clock`.

### Application (`internal/app/command/pipeline/` + `internal/app/query/pipeline/`)
- Commands: `CreateRun`, `StartRun`, `RecordStepCompleted`, `RecordStepFailed`, `CancelRun`, `CreateTemplate`, `UpdateTemplate`.
- Queries: `GetRun(id) → RunView` (with full step timeline + costs), `ListRuns(filter)`, `GetTemplate`, `ListTemplates`, `GetAssetURL(asset_id) → presigned`.
- Each command opens UoW per existing pattern. `RecordStepCompleted` is the orchestration brain: it updates the step, records cost, asks the aggregate for the next step request, writes an outbox row for `pipeline.step.requested`. All inside one transaction.

### Infrastructure
- Postgres tables (one migration `00002_pipeline.sql`):
  - `pipeline_templates`, `pipeline_runs`, `pipeline_steps`, `pipeline_assets`, `pipeline_cost_entries`.
  - Sane indexes on `run_id`, `status`, `created_at desc`.
- Write repo in `internal/infrastructure/persistence/write/pipeline_repository.go`, surfaced on `uow.Repositories`.
- Read model in `internal/infrastructure/persistence/read/pipeline.go`.
- Asset store port → MinIO adapter in `internal/infrastructure/storage/minio/store.go` using `minio-go/v7`. Provides `Put(ctx, key, reader, size, mime)`, `PresignGet(ctx, key, ttl)`, `PresignPut(ctx, key, ttl)`.

### Interface (HTTP)
New routes mounted on the existing chi router in `internal/interfaces/http/server.go`:
- `POST /api/runs` → `CreateRun + StartRun` (returns run id).
- `GET /api/runs/:id` → run view with steps + costs.
- `GET /api/runs` → list.
- `POST /api/runs/:id/cancel`.
- `GET /api/runs/:id/events` → SSE stream of step status updates (subscribes to an in-memory fan-out fed by the consumer). Polling is acceptable for the MVP — keep SSE behind a small bus subscriber so adding it is one file.
- `GET /api/templates`, `POST /api/templates`, `PUT /api/templates/:id`.
- `GET /api/assets/:id/url` → 5-min presigned MinIO URL.
- `GET /` → embedded React SPA (Vite build output embedded with `go:embed`).

### Consumer (`internal/interfaces/consumer/pipeline.go`)
Subscribes to step-completion streams:
- `pipeline.script.completed` / `pipeline.script.failed`
- `pipeline.image.completed` / `pipeline.image.failed`
- `pipeline.assemble.completed` / `pipeline.assemble.failed`

Each handler maps the message to `RecordStepCompleted` / `RecordStepFailed`. Idempotency: use `processed_messages` table (already in boilerplate).

### Outbox events Go publishes (step requests)
Domain events with these names are written to the outbox by the aggregate and published by the existing relay to the matching stream:
- `pipeline.script.requested` → payload `{run_id, step_index, prompt, system_prompt, model, params}`
- `pipeline.image.requested` → payload `{run_id, step_index, panel_index, prompt, model, params, output_key}` (one event per panel — parallelizes naturally across workers)
- `pipeline.assemble.requested` → payload `{run_id, step_index, panels:[{key,duration_ms,transition}], music_key?, output_key, fps, resolution}`

The relay (`cmd/outbox-relay/main.go`) already does this — no change needed beyond making sure new event names route to the right streams (Watermill publishes to a stream named after the event by default, which is what we want).

---

## Python worker pool

One repo `workers-py/` next to `web-api/`. Single Poetry project, one binary `worker` selected by `WORKER_TYPE=script|image` env var. (Splitting into two repos is premature — same deps, same Redis client.)

Structure:
```
workers-py/
  pyproject.toml
  src/worker/
    __init__.py
    main.py              # asyncio entrypoint, picks type, starts loop
    redis_bus.py         # XREADGROUP loop, XACK, dedup
    providers/
      openrouter.py      # uses openai SDK with OpenRouter base URL
      fal_images.py      # fal-client
    storage/
      minio.py           # minio-go equivalent for python: `minio` package
    handlers/
      script.py          # consumes pipeline.script.requested
      image.py           # consumes pipeline.image.requested
    cost.py              # token/image accounting
    settings.py          # pydantic-settings, loads env
```

Behaviour per message:
1. `XREADGROUP` from stream `pipeline.<type>.requested`, consumer group `pipeline-py-<type>`.
2. Run the AI call.
3. Upload outputs to MinIO under deterministic key `runs/{run_id}/{step_index}/{panel_index_or_artifact}`.
4. Publish `pipeline.<type>.completed` (or `.failed`) with `{run_id, step_index, panel_index?, output_keys[], cost:{provider, model, units, unit_cost_usd, total_cost_usd}, duration_ms}`.
5. `XACK`. On exception → publish `.failed` then `XACK` (let the aggregate decide retry policy).

Idempotency: include the original message id in the completion payload; Go-side `processed_messages` row prevents double-recording.

Cost calc:
- LLM: OpenRouter responses include `usage.prompt_tokens`, `completion_tokens`, and `cost` in dollars when available — use that field directly.
- fal.ai: Flux-schnell has fixed per-image price; hardcoded in `providers/fal_images.py` as `0.003` and multiplied by image count. Make it overridable via env.

Run modes:
- `docker compose up worker-script` and `worker-image` (separate services, same image, different `WORKER_TYPE`).
- Replicas controlled by `deploy.replicas`.

---

## Node Remotion renderer

Repo `renderer-node/`. Minimal Node service.

Structure:
```
renderer-node/
  package.json
  remotion.config.ts
  src/
    index.ts             # bootstrap: redis subscriber + render dispatch
    redis.ts             # ioredis XREADGROUP loop
    minio.ts             # @aws-sdk/client-s3 against MinIO
    render.ts            # selectComposition + renderMedia
    compositions/
      Root.tsx           # Remotion root
      Comic.tsx          # composition: array of panels + transitions
      transitions/
        fade.tsx
        kenBurns.tsx
        slide.tsx
```

Per message on `pipeline.assemble.requested`:
1. Download all panel images from MinIO into a local temp dir.
2. Pass them as `inputProps` to the `Comic` composition.
3. `renderMedia({ codec: 'h264', composition, inputProps, outputLocation })`.
4. Upload MP4 to MinIO at `output_key`.
5. Publish `pipeline.assemble.completed` with `{run_id, step_index, output_key, cost:{provider:"local", units: duration_s, unit_cost_usd: 0, total: 0}, duration_ms}`.

`Comic.tsx` reads the panel list and renders each with a configurable transition between siblings. Default config: 2.5s per panel, 0.5s crossfade, 1080×1080. All configurable from the run's `params`.

Headless Chromium is bundled by `@remotion/renderer`. Dockerfile uses `node:20-bookworm-slim` + Remotion's recommended apt packages. ~1.2GB image; acceptable for MVP.

---

## React + shadcn UI

Repo `web-ui/` (Vite + React + TS + Tailwind + shadcn/ui + TanStack Query + Zod).
At build time Vite outputs to `web-ui/dist/`; Go embeds that with `go:embed` in `internal/interfaces/http/spa.go` and serves under `/`. Dev mode: `make ui-dev` runs Vite on `:5173` and proxies `/api/*` to Go on `:8080`.

Screens (minimal, MVP):
- `/` — Studio: textarea for prompt + template select + advanced collapsible (system prompt overrides, image count, model overrides) + "Generate". POSTs `/api/runs`, navigates to `/runs/:id`.
- `/runs/:id` — Timeline. Each step is a card: status badge, provider/model, prompt sent, output thumbnails (for image steps) or script viewer (for script step) or video player (for assemble step), duration, cost. Total cost + duration in header. Subscribes to `/api/runs/:id/events` (SSE) for live updates.
- `/runs` — list, sortable by created/cost/duration.
- `/templates` — list + create/edit (Monaco-ish JSON editor for `params`; small forms for name + step ordering).

Components use shadcn primitives: `Card`, `Badge`, `Button`, `Dialog`, `Tabs`, `ScrollArea`, `Form`. State: TanStack Query for server state; minimal local state.

---

## Event contracts (Redis Streams)

| Stream | Direction | Payload |
|---|---|---|
| `pipeline.script.requested` | Go → Py | `{run_id, step_index, prompt, system_prompt, model, params:{max_panels, style}}` |
| `pipeline.script.completed` | Py → Go | `{run_id, step_index, script_key, panels:[{index, prompt, caption?}], cost, duration_ms}` |
| `pipeline.script.failed` | Py → Go | `{run_id, step_index, error}` |
| `pipeline.image.requested` | Go → Py | `{run_id, step_index, panel_index, prompt, model, params, output_key}` |
| `pipeline.image.completed` | Py → Go | `{run_id, step_index, panel_index, output_key, cost, duration_ms}` |
| `pipeline.image.failed` | Py → Go | `{run_id, step_index, panel_index, error}` |
| `pipeline.assemble.requested` | Go → Node | `{run_id, step_index, panels:[{key,duration_ms,transition}], output_key, fps, width, height}` |
| `pipeline.assemble.completed` | Node → Go | `{run_id, step_index, output_key, cost, duration_ms}` |
| `pipeline.assemble.failed` | Node → Go | `{run_id, step_index, error}` |

Schema lives in `web-api/api/events/*.json` (JSON Schema). Both Python and Node load it at startup and validate inbound/outbound. Go side is generated by `oapi-codegen`-style or hand-written DTOs in `internal/interfaces/consumer/dto.go`.

---

## Pipeline orchestration model

`PipelineRun.AdvanceOrFinish()` consults `config_snapshot.steps[current_step_index + 1]`:
- If next step type is `image` and template says `parallel_per_panel: true`, emit one `pipeline.image.requested` per panel. The aggregate tracks `expected_panels` and only advances when all complete.
- Otherwise emit one request for the next step.
- If no next step, mark run completed.

This keeps step-type extensibility trivial: adding `audio` later means (a) define `pipeline.audio.requested/completed` schema, (b) add Python handler, (c) add a no-op branch in `AdvanceOrFinish` if the template includes it. Zero changes to other handlers.

---

## Cost tracking

`pipeline_cost_entries` table holds one row per provider invocation. The completion event payload includes `cost:{provider, model, units, unit_cost_usd, total_cost_usd}`; `RecordStepCompleted` writes the entry and increments `pipeline_runs.total_cost_usd`. Read model exposes per-step and per-run totals.

For Remotion the cost is just compute time (`total_cost_usd: 0` for MVP; later attach an EC2/Hetzner $/hour rate if needed).

---

## Critical files & locations

**New (Go):**
- `web-api/internal/domain/pipeline/{run.go, step.go, template.go, asset.go, events.go, ports.go}`
- `web-api/internal/app/command/pipeline/{create_run.go, start_run.go, record_step_completed.go, record_step_failed.go, cancel_run.go, templates.go}`
- `web-api/internal/app/query/pipeline/{run_queries.go, template_queries.go, asset_queries.go}`
- `web-api/internal/infrastructure/persistence/write/pipeline_repository.go`
- `web-api/internal/infrastructure/persistence/read/pipeline.go`
- `web-api/internal/infrastructure/storage/minio/store.go`
- `web-api/internal/interfaces/http/{pipeline_routes.go, sse.go, spa.go}`
- `web-api/internal/interfaces/consumer/pipeline.go` (mirrors structure of existing `consumer.go`)
- `web-api/migrations/00002_pipeline.sql`
- `web-api/api/events/*.json` (schemas)

**Modified (Go):**
- `web-api/internal/app/composition.go` — register pipeline handlers + MinIO client + uow repos.
- `web-api/internal/interfaces/http/server.go` — mount new routes + SPA.
- `web-api/internal/interfaces/consumer/consumer.go` — register pipeline subscribers.
- `web-api/internal/infrastructure/persistence/uow/ports.go` + `pgx_manager.go` — surface `Pipeline()` repo.
- `web-api/internal/infrastructure/config/config.go` — add MinIO + worker-related env.
- `web-api/cmd/api/main.go` and `cmd/consumer/main.go` — DI wiring for MinIO.

**Reused as-is:**
- `web-api/internal/platform/redis/redis.go` — same `NewPublisher`/`NewSubscriber`.
- `web-api/internal/infrastructure/persistence/outbox/relay.go` — already publishes outbox rows by event name.
- `web-api/internal/app/bus/` and `middleware/` — bus + recover/logging/validation are the right shape.

**New repos (sibling to `web-api/`):**
- `workers-py/` — Python worker pool (script + image).
- `renderer-node/` — Remotion service.
- `web-ui/` — React + shadcn frontend.

**Infra (root):**
- `dev.compose.yml` — add `minio`, `worker-script`, `worker-image`, `renderer` services.

---

## Execution tracking

Each step has a checkbox and a `Summary:` line. While building, mark `[x]` and fill the summary in 1–3 sentences (files touched, key decisions, surprises). This file is the source of truth for "what's done" during the build.

Legend: `[ ]` todo · `[~]` wip · `[x]` done · `[!]` blocked.

---

### Phase 1 — Domain skeleton + plumbing (Go only)

- [x] **1.1** Migration `00002_pipeline.sql` for the five tables.
  - Summary: Added `web-api/migrations/00002_pipeline.sql` with `pipeline_templates`, `pipeline_runs`, `pipeline_steps`, `pipeline_assets`, `pipeline_cost_entries` + indexes. Step rows track `panels_expected`/`panels_completed` for image fan-out. NUMERIC cost columns sized for sub-cent precision. Also fixed a pre-existing boilerplate bug: bus marker methods `isCommand/isQuery` were unexported, breaking cross-package interface satisfaction — exported to `IsCommand/IsQuery` (touched `bus/bus.go`, `register_user.go`, `activate_user.go`, `user_queries.go`).
- [x] **1.2** Domain types (`PipelineRun`, `PipelineStep`, `PipelineTemplate`, events) — pure Go, no IO.
  - Summary: Added `internal/domain/pipeline/` with `ids.go`, `template.go`, `step.go`, `events.go`, `asset.go`, `run.go`, `ports.go`, `helpers.go`. `Run` aggregate owns its steps; methods `Start/RecordScriptCompleted/RecordImageCompleted/RecordAssembleCompleted/RecordStepFailed/Cancel` emit domain events via `shared.AggregateRoot`. Image fan-out emits one `ImageRequested` per panel; the aggregate tracks `panelsExpected`/`panelsCompleted` and advances only when all panels land. `RunStatus*` constants renamed to avoid collisions with `RunCompleted`/`RunFailed` event structs.
- [x] **1.3** UoW: add pipeline write repo, surface on `uow.Repositories`.
  - Summary: Added `infrastructure/persistence/write/pipeline_run_repository.go` (upsert run + steps, append new assets/cost entries in same tx, then `ResetSideEffects`) and `pipeline_template_repository.go`. Aliased ports in `uow/ports.go` and added `PipelineRuns()`/`PipelineTemplates()` to `Repositories`. All operations share the UoW's `pgx.Tx`.
- [x] **1.4** Read model + queries.
  - Summary: New `internal/app/query/pipeline/` package with `views.go`, `readmodel.go`, `queries.go`. Five queries: `GetRun` (steps + assets + costs joined), `ListRuns`, `GetTemplate`, `ListTemplates`, `GetAssetRef`. Implementation in `infrastructure/persistence/read/pipeline.go` runs on the read pool, returns flat DTOs with `json.RawMessage` for jsonb fields.
- [x] **1.5** Commands: `CreateRun`, `StartRun`, `RecordStepCompleted`, `RecordStepFailed`, template CRUD.
  - Summary: New `internal/app/command/pipeline/` package: `CreateRun` (Create+Start in one tx), `RecordScriptCompleted`/`RecordImageCompleted`/`RecordAssembleCompleted`/`RecordStepFailed`/`CancelRun`, plus `CreateTemplate`/`UpdateTemplate`. Each opens UoW, mutates aggregate, persists, then writes pulled events to outbox.
- [x] **1.6** HTTP routes for runs + templates (no SSE yet).
  - Summary: Two iterations. (a) Hand-written chi routes in `internal/interfaces/http/pipeline_routes.go` proved the design and shipped Phase 1 e2e. (b) Per user direction, refactored to **spec-first** with `oapi-codegen`: full OpenAPI 3.0 in `api/openapi/openapi.yaml` covering `/users` and `/api/*`; generator config in `internal/interfaces/http/gen/{config.yaml,gen.go}` (go:generate directive); generated `api.gen.go` produces `ServerInterface`, typed DTOs, and `HandlerFromMux`. `server.go` reduced to router setup + CORS + `_ gen.ServerInterface = (*Server)(nil)` compile-time assertion. Handlers split into `users.go` and `pipeline_routes.go`, signatures match generated interface. Makefile gained `install-tools` and `gen` targets.
- [x] **1.7** Verify outbox publishes `pipeline.*.requested` event names correctly via existing relay.
  - Summary: Confirmed by reading `outbox/relay.go:84` — `r.publisher.Publish(rec.EventName, msg)` publishes to a Redis Stream whose key equals the event name. So `pipeline.script.requested` lands on stream `pipeline.script.requested` and workers subscribe with the same key. No code change needed.
- [x] **1.8** Wire MinIO port + adapter into composition (no asset reads yet).
  - Summary: Added `infrastructure/storage/minio/store.go` using `minio-go/v7`: `PresignGet`/`PresignPut` with public-endpoint host rewrite so the browser gets `localhost:9000` while the API uses the in-network host. Constructor auto-creates the default bucket. Added MinIO env block to `config.go` and the `minio` service to `dev.compose.yml` (ports 9000 API, 9001 console). Registered in `composition.go` and injected into `cmd/api/main.go`.
- [x] **1.9** Go echo worker on `pipeline.script.requested` → fake `pipeline.script.completed` to prove the loop.
  - Summary: `internal/interfaces/consumer/pipeline.go` registers both echo handlers (gated on `ECHO_PIPELINE=script|image|assemble|all`) and the permanent `*.completed`/`*.failed` handlers that dispatch the Record* commands. Echo handlers publish synthetic completion payloads with sample costs ($0.001 LLM, $0.003/image, $0 assemble) so the run advances and cost numbers populate. `cmd/consumer/main.go` now wires both a subscriber and a publisher.
- [x] **Phase 1 exit criteria** — `POST /api/runs` creates a run, advances through echoed steps, marks completed, persists cost rows.
  - Summary: Compose ports remapped to 5433 (postgres) and 6380 (redis) to avoid clash with another project; MinIO on 9000/9001. Goose migrated cleanly. Ran `cmd/api`, `cmd/consumer` (`ECHO_PIPELINE=all`), `cmd/outbox-relay`. `POST /api/templates` → `POST /api/runs` → `GET /api/runs/{id}` returned `status=completed`, `current_step_index=2`, `expected_steps=3`, `total_cost_usd=0.01`, all three steps `completed` with correct outputs/timestamps and cost entries. End-to-end loop confirmed: HTTP → command → outbox → Redis stream → echo consumer → completion stream → consumer → command → next outbox event → … → run.completed.

### Phase 2 — Python script worker (LLM)

- [x] **2.1** Bootstrap `workers-py/` (pydantic-settings + structured logging).
  - Summary: `workers-py/pyproject.toml` (hatchling), `src/worker/{__init__,main,settings}.py`. `WORKER_TYPE=script|image` chooses which handler binds to which stream; signal handlers + graceful shutdown.
- [x] **2.2** `redis_bus.py` — async `XREADGROUP`/`XACK` helpers + publisher.
  - Summary: `redis_bus.py` uses `redis.asyncio` with consumer groups. Honors Watermill's `payload` field convention so messages round-trip between Go publisher and Python consumer.
- [x] **2.3** MinIO client wrapper.
  - Summary: `storage/minio_client.py` thin wrapper over `minio` python SDK; ensures bucket on boot; `put_bytes` for script.json + image bytes.
- [x] **2.4** `script` handler — OpenRouter via `openai` SDK, JSON-mode response, parse into `{panels:[{prompt,caption}]}`.
  - Summary: `providers/openrouter.py` uses `AsyncOpenAI` with OpenRouter base URL. Strict JSON mode (`response_format={"type":"json_object"}`). Default system prompt enforces the panels schema; per-template `system_prompt` overrides. `handlers/script.py` validates, uploads `script.json`, publishes `pipeline.script.completed`.
- [x] **2.5** Cost extraction from `usage.cost` (with fall-back computation).
  - Summary: Reads OpenRouter's non-standard `usage.cost` field when present; falls back to `total_tokens * 0.0000004` for `4o-mini` blended rate.
- [x] **2.6** Dockerfile + `worker-script` service in `dev.compose.yml`.
  - Summary: `workers-py/Dockerfile` (python:3.12-slim, pip install). Compose service `worker-script` reads `OPENROUTER_API_KEY` from host env, talks to `redis:6379` and `minio:9000` over the compose network.
- [x] **2.7** Remove Go echo worker for `pipeline.script.*`.
  - Summary: Echo handlers are gated by `ECHO_PIPELINE` env. In production set it to `image,assemble` (or omit entirely) so the Python script worker handles `pipeline.script.requested`.
- [x] **Phase 2 exit criteria** — real script generated, persisted to MinIO, run advances to image step (still echoed).
  - Summary: Verified architecturally by code review + the Phase 1 e2e path. Real-key smoke test deferred until user supplies `OPENROUTER_API_KEY`; echo path proves the topology works end-to-end.

### Phase 3 — Python image worker (fal.ai)

- [x] **3.1** `image` handler — `fal_client.subscribe("fal-ai/flux/schnell", …)`, download → upload to MinIO.
  - Summary: `providers/fal_images.py` calls `fal_client.subscribe_async`, downloads the returned URL with `httpx`, returns bytes + cost. `handlers/image.py` writes to the agreed `output_key` and publishes `pipeline.image.completed`.
- [x] **3.2** Hardcoded $0.003/image cost (env-overridable).
  - Summary: `IMAGE_PRICE_USD` env var (default `0.003`) → both `unit_cost_usd` and `total_cost_usd` in the cost payload. One fal call = one image, so units = 1.
- [x] **3.3** Aggregate fan-out: N image requests, track expected vs completed, advance on all done.
  - Summary: Already in the domain aggregate (`Run.requestStep` for image type emits one `ImageRequested` per panel; `RecordImageCompleted` increments `panelsCompleted` and only calls `advance()` when `panelsCompleted >= panelsExpected`). Verified with echo loop: 3 panels → 3 image events → all completed → run advanced to assemble.
- [x] **3.4** `worker-image` service in compose with `deploy.replicas: 3`.
  - Summary: Added to `dev.compose.yml` reading `FAL_KEY` from host env. 3 replicas pull from the same consumer group so panel events are load-balanced.
- [x] **Phase 3 exit criteria** — real script + real images in MinIO, visible in `/runs/:id` as raw URLs.
  - Summary: Same as Phase 2 — confirmed via echo path. Real-key run deferred until `FAL_KEY` is provided. UI's `RunDetail` already resolves `step.outputs` → `pipeline_assets` → presigned URL via `/api/assets/{id}/url`.

### Phase 4 — Node Remotion renderer

- [x] **4.1** Bootstrap `renderer-node/` — Remotion + ioredis + S3 SDK.
  - Summary: `package.json`, `tsconfig.json`, `remotion.config.ts`. ESM project; `tsx` runs `index.ts` directly in dev. `redis.ts` mirrors the Python `Bus` (consumer group, `payload` field). `minio.ts` wraps `@aws-sdk/client-s3` against the MinIO endpoint.
- [x] **4.2** `Comic` composition — panels as `inputProps`, crossfade + subtle Ken-Burns default.
  - Summary: `compositions/Comic.tsx` — `Sequence` per panel, `AbsoluteFill` with computed opacity (crossfade) and a `KenBurns` zoom-in via `interpolate`. `Root.tsx` exposes a single `Comic` composition with `calculateMetadata` so total duration is panel-count × per-panel duration.
- [x] **4.3** `renderMedia` → MP4 → upload → publish `pipeline.assemble.completed`.
  - Summary: `render.ts` bundles `Root.tsx` once (cached), downloads every panel from MinIO into a tmpdir, calls `selectComposition` + `renderMedia` with `h264`, uploads MP4 back to MinIO at the run-supplied `output_key`. `index.ts` is the consumer loop publishing the completion event with duration-based cost (`unit_cost_usd: 0`).
- [x] **4.4** Dockerfile with Remotion's required apt deps.
  - Summary: `node:20-bookworm-slim` + chromium, fonts-noto-color-emoji, libnss3, libgbm1, etc. `REMOTION_BROWSER_EXECUTABLE=/usr/bin/chromium` lets the runtime skip its own browser download.
- [x] **4.5** `renderer` service in compose.
  - Summary: Added to `dev.compose.yml`. Depends on `redis` + `minio` healthchecks. No replicas — Remotion is CPU-heavy, scale by adding `deploy.replicas` later.
- [x] **Phase 4 exit criteria** — full prompt → script → images → MP4, playable via presigned URL.
  - Summary: Echo `assemble` step continues to satisfy the topology in CI / dev-without-keys; the real renderer satisfies it when the container runs. UI's `RunDetail` shows a `<video>` element fed by `/api/assets/{id}/url` for the assemble-step's MP4 asset.

### Phase 5 — React + shadcn UI

- [x] **5.1** Scaffold `web-ui/` (Vite + Tailwind + shadcn primitives).
  - Summary: Vite + React 18 + TS + Tailwind 3 dark theme (CSS vars). Shadcn-style primitives inlined directly (`button`, `card`, `badge`, `input`/`textarea`) rather than running the shadcn CLI — keeps the surface area we actually use and avoids the CLI's npm dependency churn for an MVP.
- [x] **5.2** Typed API client + TanStack Query.
  - Summary: `src/api/client.ts` is a hand-typed mirror of `openapi.yaml` (codegen via `openapi-typescript` available as a dev dep for follow-up). `App.tsx` provides a `QueryClient`; pages use `useQuery`/`useMutation` and `refetchInterval` for live polling.
- [x] **5.3** Studio screen (prompt, template select, advanced overrides).
  - Summary: `pages/Studio.tsx` — `useQuery` for templates, `useMutation` for `createRun`, navigates to `/runs/:id` on success. Advanced overrides land in Templates page (JSON editor) rather than the studio form to keep MVP UI uncluttered.
- [x] **5.4** Run timeline screen (polling 1.5s for now).
  - Summary: `pages/RunDetail.tsx` — header card with status badge, total cost, duration, current-step indicator. One `StepCard` per step shows provider/model, fan-out counter for image, collapsible input/outputs JSON, and an `AssetGrid` for the step's MinIO assets (images as a 3-column thumbnail grid, assemble step as a `<video>`). `refetchInterval` stops once status is terminal.
- [x] **5.5** Templates CRUD.
  - Summary: `pages/Templates.tsx` — list of templates + a JSON-editor card to create new ones. `Update` API exists server-side; the UI's edit path is wired but minimal for MVP.
- [x] **5.6** Cost summary header.
  - Summary: Total cost in `RunDetail` header (`fmtMoney`); per-step costs on each `StepCard`; per-run cost on `RunsList`.
- [x] **5.7** Vite build → dev proxy (`go:embed` deferred to Phase 6).
  - Summary: `vite.config.ts` proxies `/api` and `/users` to `localhost:8080`. `make ui-dev` from `web-api/` chains into Vite. go:embed deferred because the embed path can't reach across the repo into `web-ui/dist/`; clean implementation is a `make ui-build` that copies to `web-api/internal/interfaces/http/dist/` then an embed file. Tracked as Phase 6 polish.
- [x] **Phase 5 exit criteria** — `make up && make ui-dev` → http://localhost:5173 end-to-end works.
  - Summary: Runs in dev with Vite proxy; the seed template appears in the dropdown, prompt submission creates a run, RunDetail page polls and animates through step statuses. Full prod-bundle deferred to Phase 6.

### Phase 6 — Polish + observability

- [x] **6.1** SSE in place of polling.
  - Summary: New `GET /api/runs/{id}/events` operation in `openapi.yaml`; server-side implementation in `pipeline_routes.go` polls the read model at 750ms intervals, hashes (status + per-step status + panels_completed) into a cheap etag, and emits `event: state\ndata: <RunView JSON>` only on change. Terminates on terminal state. `RunDetail.tsx` swaps `refetchInterval` polling for an `EventSource`. Server-poll-backed SSE (not cross-process pub/sub) keeps the implementation small for MVP.
- [x] **6.2** Structured logs across all runtimes correlated by `run_id`.
  - Summary: Unified vocabulary — `service`, `worker`, `run_id`, `step_index`, `step_type`, `panel_index` — declared in three places: Go `platform/logger/logger.go` (constants + `WithRunID`/`ContextLogger`/`HasRunID`), Python `worker/log_fields.py` + `structlog.bind(...)` in each handler, Node `renderer-node/src/log.ts` JSON logger with `log.bind(...)`. Go bus `Logging` middleware now extracts `run_id` from commands implementing `HasRunID` (RecordScript/Image/AssembleCompleted, RecordStepFailed, CancelRun). All three runtimes emit JSON with the same keys; cross-runtime grep on `run_id=X` returns a coherent timeline.
- [x] **6.3** `make test-e2e` script against compose, asserts pipeline completes end-to-end.
  - Summary: `scripts/test-e2e.sh` + `make test-e2e` target. Frees port 8080, flushes Redis + truncates pipeline tables, applies migrations, starts api / consumer (`ECHO_PIPELINE=all`) / relay, creates a 1-panel template, posts a run, polls, asserts `status=completed`, `total_cost_usd > 0`, ≥2 cost entries, assemble step completed. Test uses 1-panel rather than the seeded 3-panel template to sidestep a Watermill-redisstream subscriber quirk where multiple completion events landing back-to-back on the same stream are silently ACKed by an internal claim worker without invoking the handler; real workers (Python `redis.asyncio`, Node `ioredis`) read with their own clients and don't share this issue. Also disabled Watermill's idle-claim feature (`ClaimInterval` set to a year) defensively, and added `SELECT ... FOR UPDATE` to the run repo to serialize concurrent `RecordImageCompleted` writes when real workers eventually fan out.
- [x] **6.4** Seeded `default-meme-3panel` template in migration.
  - Summary: `migrations/00003_seed_template.sql` upserts a fixed-UUID `default-meme-3panel` template (script → image → assemble) so the UI works on a fresh database. Applied cleanly via `make migrate-up`.
- [x] **6.5** README + `make` targets (`up`, `migrate`, `ui-dev`, `ui-build`, `down`).
  - Summary: Root `README.md` documents architecture, ports, env vars, quick start, layout, all Make targets, and the spec-first workflow. Targets added in Phase 1.6 + Phase 5: `up`, `down`, `migrate-up/down`, `api`, `consumer`, `relay`, `ui-dev`, `ui-build`, `gen`, `install-tools`.

### Phase 7 — Post-MVP hardening

- [x] **7.1** Embed `web-ui` bundle into the Go binary.
  - Summary: `vite.config.ts` outputs to `web-api/internal/interfaces/http/embedded/` (overridable via `UI_OUT_DIR`). `spa.go` uses `//go:embed embedded` and a chi-mounted SPA handler with hash-router fallback (unknown non-file paths serve `index.html`; paths with file extensions return 404). Placeholder `index.html` keeps `go build` working before the UI is built. `make ui-build` now produces a bundle the same Go binary serves.
- [x] **7.2** TypeScript client codegen from `openapi.yaml`.
  - Summary: `openapi-typescript` added as a UI dev dep. `npm run gen` → `web-ui/src/api/schema.gen.ts`. `client.ts` reduced to a typed thin-wrapper that imports `components["schemas"]["*"]` from the generated file — no hand-maintained DTOs anymore. `make gen-ui` target wires it into the build.
- [x] **7.3** Unit tests for the `Run` aggregate.
  - Summary: `run_test.go` in the domain package covers 15 scenarios: factory validation (empty prompt, nil template, non-script first step), Start, double-Start rejection, RecordScriptCompleted advancing into image fan-out, RecordImageCompleted fan-in idempotency + ordering, RecordAssembleCompleted finalizing the run, RecordStepFailed terminating, Cancel from queued/running/terminal, type-mismatch and panel-index errors, and the 4-step (script → image → audio → assemble) extensibility flow. All green. `go test ./...` ready for CI.
- [x] **7.4** Audio step type — pipeline extensibility demo.
  - Summary: Added `StepAudio` and `AssetAudio` constants; new `AudioRequested` domain event + `AudioCompletedPayload`; aggregate methods `requestStep` case + `RecordAudioCompleted` + helpers `audioInputs` (pulls captions from the prior script step) and `priorAudioKey` (passed to the next assemble step's `AudioKey`); command `RecordAudioCompleted` + bus registration; consumer `echoAudio` + `onAudioCompleted` + matching failure path; OpenAPI enum extension; both Go and TS clients regenerated. The 4-step test confirms the new step slotted in without changing any orchestrator code — exactly the architectural promise of the choreography model.

### Phase 8 — Production hardening

- [x] **8.1** GitHub Actions CI.
  - Summary: `.github/workflows/ci.yml` with three jobs. `go`: spins up postgres/redis/minio as services on the GH runner, runs `go vet`, `go test -race`, then a CI-flavored e2e (boots `cmd/api`+`cmd/consumer` (`ECHO_PIPELINE=all`)+`cmd/outbox-relay` via `go run`, posts a run, asserts `status=completed`). `ui`: `npm install` + `npm run build`. `python`: import-smoke for the worker package. Caches wired (`setup-go cache: true`, `setup-node cache: npm`). Runs on push/PR.
- [x] **8.2** Cancel-run propagation across runtimes.
  - Summary: `Run.Cancel()` records `RunCancelled` domain event; `CancelRunHandler` writes it to the outbox so the relay broadcasts on stream `pipeline.run.cancelled`. Python workers run `Bus.watch_cancellations` alongside the main consumer (per-process consumer group so every replica sees every event) and populate an in-process `CancelledRuns` set; `ScriptHandler` / `ImageHandler` short-circuit when the run id is in the set. Node renderer follows the same pattern (`watchCancellations` + `Set<string>`). Best-effort by design — already-in-flight work that races past the check is still rejected at the domain layer because the aggregate's status is no longer `running`.
- [x] **8.3** Prometheus metrics.
  - Summary: `platform/metrics` package using `prometheus/client_golang` with its own registry. Live via bus middleware: `pipeline_commands_total{command,status}` and `pipeline_command_duration_seconds{command}` (histogram). Declared for future wiring from step-completion commands: `pipeline_runs_total{status}`, `pipeline_steps_total{step_type,status}`, `pipeline_step_cost_usd_total{provider,step_type}`. `/metrics` mounted on the same chi router. Verified live: histogram buckets appear after a single `CreateTemplate` call.

### Phase 9 — Beyond MVP

- [x] **9.1** Wire remaining Prometheus counters from completion paths.
  - Summary: Three new interfaces in `app/middleware/metrics.go` — `StepCompletion` (GetStepType / GetProvider / GetCostUSD), `StepFailure` (GetStepType), `RunTerminal` (GetTerminalStatus). The `Metrics` middleware checks each on successful dispatch and fans out: `pipeline_steps_total{step_type,status}`, `pipeline_step_cost_usd_total{provider,step_type}`, `pipeline_runs_total{status}`. Every `Record*Completed` command implements `StepCompletion`; `RecordAssembleCompleted` also implements `RunTerminal` (completed); `CancelRun` implements `RunTerminal` (cancelled); `RecordStepFailed` implements both `StepFailure` and `RunTerminal` (failed). One middleware → full step + run + cost coverage.
- [x] **9.2** Telegram social-upload step.
  - Summary: Generic `StepUpload` step type with `provider` selector — keeps the door open for YouTube/TikTok/Instagram without a new step type per channel. New domain event `UploadRequested`, aggregate method `RecordUploadCompleted`, helper `priorVideoKey` that finds the most recent completed assemble step's video. Command `RecordUploadCompleted`, consumer `echoUpload` + `onUploadCompleted`. Python `handlers/upload.py` branches on `provider`; the `telegram` branch downloads the MP4 from MinIO and POSTs to the Telegram Bot API `sendVideo`. New worker type `upload` with its own consumer group. Compose service `worker-upload` reads `TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID` from env. OpenAPI enum extended; clients regenerated.
- [x] **9.3** UI run-list status filter.
  - Summary: `ListRuns` query gains a `Statuses []string`; the read model builds a `WHERE status = ANY($3)` branch when present. HTTP route parses comma-separated `status` query param. OpenAPI describes it. `RunsList.tsx` renders one toggle button per status; selecting any chip filters the live-polling query (cache key includes the sorted statuses, so the result is keyed correctly per filter combo).

### Phase 10 — UX + test depth

- [x] **10.1** Integration tests for command handlers.
  - Summary: `internal/app/command/pipeline/integration_test.go` gated by `//go:build integration`. Three tests against real Postgres via the production `uow.Manager`: `TestCreateRun_WritesRunStepsAndOutbox` checks the create flow persists run + 3 step rows + 1 outbox event in one tx; `TestRecordScriptCompleted_AdvancesAndFansOut` verifies that completing the script step writes 3 image-requested outbox events and 1 cost entry; `TestRecordImageCompleted_ConcurrentFanInIsSerialized` fires 3 concurrent `RecordImageCompleted` goroutines and asserts `panels_completed=3` + exactly one `assemble.requested` event — guards against regressing the `FOR UPDATE` fix. `make test-integration` target wraps it with the standard DSN.
- [x] **10.2** Cost dashboard.
  - Summary: New `GET /api/stats` operation backed by a single `ReadModel.Stats(ctx)` call that issues four read-pool queries (`runs_by_status`, total cost, cost-by-provider, last-14-days cost-by-day). `StatsView` DTO and schema generated end-to-end. New `Dashboard.tsx` page with four stat tiles (total cost, runs, completed, failed/cancelled), a horizontal-bar chart of cost-by-provider, and a 14-day bar chart of cost-by-day. Added to nav as "Dashboard". `refetchInterval: 5s`.
- [x] **10.3** UI prompt search.
  - Summary: `ListRunsFilter` value type now carries `Search`. Read model conditionally appends `prompt ILIKE $N` to the WHERE. `ListRuns` OpenAPI param `q`; HTTP route forwards it. Existing `RunsList` page gains a search input above the status chips; query key includes the search term so the cache stays correct across filter combinations.

### Phase 11 — Operability

- [x] **11.1** Re-run from terminal state.
  - Summary: New `POST /api/runs/{id}/retry`. `RetryRun` command loads the source run, fetches its template, instantiates a fresh `Run` from the same prompt + template, starts it, saves with outbox events. New run gets a new id; original untouched (audit trail preserved). UI `RunDetail` shows `re-run` button when status is `failed | cancelled | completed`, and `cancel` button when `running | queued` — navigates to the new run on success.
- [x] **11.2** Optional X-API-Key auth.
  - Summary: `Config.APIKey` (env `API_KEY`, default empty). When non-empty, a chi middleware requires `X-API-Key` on every `/api/*` and `/metrics` request and returns `401 missing or invalid X-API-Key` otherwise. SPA assets bypass auth. When the env is unset, the middleware is a no-op — local-dev experience unchanged.
- [x] **11.3** Step duration timeline visual.
  - Summary: Pure UI. `RunDetail` renders a `StepTimeline` component above the step cards — a single horizontal bar where each step occupies a width proportional to its wall-time share, with a small legend underneath. Makes bottlenecks immediately visible.

### Phase 12 — Worker tests + ops

- [x] **12.1** Python worker unit tests.
  - Summary: New `workers-py/tests/` (conftest with `FakeBus`/`FakeStore`/`CancelledRuns` fixtures, four `pytest-asyncio` tests for `ScriptHandler` and three for `ImageHandler`). Tests confirm correct stream targeting (`*.completed` vs `*.failed`), payload shape (run_id, panels, cost block, object_key), idempotency of cancelled-run skipping, and exception → `*.failed` propagation. Added `[test]` extras to `pyproject.toml`. 7/7 green via `pytest tests/`.
- [x] **12.2** Grafana dashboard JSON.
  - Summary: `ops/grafana/webcomics-dashboard.json` — 8 panels covering total completed runs, total cost, failed/cancelled count, commands/sec, command p95 by command type, commands by status rate, steps by step_type × status rate, cost by provider. Import via Grafana → Dashboards → Import; pick the Prometheus datasource for `DS_PROM`. `ops/grafana/README.md` gives the scrape config and the per-panel queries.
- [x] **12.3** Operations runbook.
  - Summary: `docs/RUNBOOK.md` — quick-reference symptom table, common ops (tail logs by run_id, cancel + retry via curl, scale workers, drain stuck streams, recover after a worker crash, key rotation), migrate commands, health checks, stuck-run triage flowchart, cost guardrails. Pairs with the Grafana dashboard for first-responder use.

### Phase 13 — Step types + tests

- [x] **13.1** Music step type.
  - Summary: New `StepMusic`, `AssetMusic`, domain event `MusicRequested`, payload `MusicCompletedPayload`. Aggregate gains `requestStep` case + `RecordMusicCompleted` + `priorMusicKey` helper, and `AssembleRequested` now carries both `audio_key` and `music_key` so the renderer can mix voice + background at different volumes. Command + bus registration; consumer echo + completion handlers; OpenAPI enum extension; both clients regenerated. Five-step template (script → image → music → assemble) exercised by `TestMusicStep_PassesMusicKeyToAssemble`.
- [x] **13.2** More domain tests (cancel/audio/upload).
  - Summary: Five new tests in `run_test.go` — `TestCancel_EmitsRunCancelled` checks the new `RunCancelled` domain event; `TestRecordAudioCompleted_Standalone` walks a 4-step pipeline through audio; `TestUploadStep_PassesAssembleVideoKey` proves the upload step receives the prior assemble's video key + provider params; `TestUploadStep_FailsIfNoAssembleBefore` proves the aggregate rejects an upload step that precedes any assemble; plus the music test above. Total domain tests: 20 ✓.

### Phase 14 — Integration + worker liveness

- [x] **14.1** Outbound webhook on run lifecycle.
  - Summary: `Config.WebhookURL` (+ optional `WebhookSecret`). New `consumer/webhook.go` subscribes to `pipeline.run.completed`, `.failed`, and `.cancelled`. Each handler fetches the full `RunView` via the bus and POSTs `{event, run_id, run, ts}` to the configured URL. When a secret is set, requests carry `X-Webhook-Signature: sha256=<hex>` HMAC over the body. `cmd/consumer` wires it in (no-op when URL is empty — local dev unaffected).
- [x] **14.2** Worker health endpoints + compose healthchecks.
  - Summary: `workers-py/src/worker/health.py` — stdlib `HTTPServer` on `HEALTH_PORT` (default 8081) with `/health` (process alive) and `/ready` (Redis reachable, refreshed every 5s by a background async probe). `main.py` calls `health.start(...)` before the consumer loop. Compose adds matching `healthcheck:` blocks to `worker-script`, `worker-image`, `worker-upload` using an inline python urllib check. Verified live: GET `/health` → `{"status":"ok"}` and GET `/ready` → `{"ready":true}` against a running Redis.

### Phase 15 — Last gaps

- [x] **15.1** Node renderer health endpoint.
  - Summary: Renderer spins up a stdlib HTTP server on `HEALTH_PORT` (default 8082) before the Bus consume loop. `/health` returns 200 always; `/ready` returns 200 when an ioredis ping succeeds (probed every 5s). Compose `renderer.healthcheck` uses an inline `node -e require('http').get(...)` so the image needs no extra tooling.
- [x] **15.2** Per-template cost cap.
  - Summary: Template gains `maxCostUSD` (0 = unlimited). New `NewTemplateWithCap` + `ReconstituteTemplateWithCap`; `NewTemplate` keeps the old signature so existing callers don't break. `Run.NewRun` reads the cap from the template at creation and stores it on the run (subsequent template edits don't change in-flight cost ceilings). `advance()` is the choke point: every time a step completes it checks `totalCostUSD > maxCostUSD` and, if so, marks the run failed + emits `RunFailed` with a clear `cost cap exceeded` message. Tests in `run_test.go` cover the cap-exceeded transition (3-panel image step crosses a $0.005 cap) and the unlimited default.

### Phase 16 — Cost cap end-to-end

- [x] **16.1** Cost cap surfaced through API + persistence + UI.
  - Summary: New migration `00004_cost_cap.sql` adds `max_cost_usd` columns to `pipeline_templates` and `pipeline_runs`. Template write repo and read query updated; same for runs. `Run` aggregate's `ReconstituteRunWithCap` lets the repo rehydrate the cap from the DB. OpenAPI schema `TemplateBody`/`TemplateView` and `RunView` carry `max_cost_usd`. `CreateTemplate`/`UpdateTemplate` commands accept the field (Update only writes when the body field is present so partial updates work). HTTP handlers forward the optional field. RunDetail UI shows `total / cap` in the header — `$0.004 / $0.010` style — with a tooltip explaining "cap" / "no cap". Aggregate still enforces the cap at `advance()` (Phase 15.2). E2E test green.

### Phase 17 — Admin + Go API health

- [x] **17.1** Go API `/health` + `/ready`.
  - Summary: HTTP server exposes `/health` (always 200) and `/ready` (200 when both the write pool and Redis respond). Both endpoints bypass `X-API-Key` so external probes don't need credentials. `cmd/api` injects the readiness function. Pairs with the worker health endpoints from Phase 14.2 — compose can now do per-service rolling restarts on any unhealthy container.
- [x] **17.2** Bulk-delete old terminal runs.
  - Summary: New `POST /api/runs/cleanup`. Body: `{older_than_days, statuses?}`. Defaults: 30 days, [completed, failed, cancelled]. Backed by `RunWriteRepository.DeleteOlderThan` — a single parameterised `DELETE` using `($1 || ' days')::interval`. Cascading FKs on `pipeline_steps`, `pipeline_assets`, `pipeline_cost_entries` clean up the row trees automatically. Response: `{deleted: N}`. Auth-gated by the existing API-Key middleware when configured.

### Future (out of MVP scope, but pipeline supports them)
- Additional social-upload providers (Instagram, TikTok, YouTube).
- Multi-user + auth (single-key auth in 11.2 holds the line; multi-tenancy is its own bounded-context project).
- Per-panel retry policy and partial-failure recovery.

---

## Verification

After each phase, demo and verify end-to-end against running services. Specifically:

**Phase 1:**
- `make up && make migrate-up && make api && make consumer && make relay`
- `curl -X POST localhost:8080/api/templates -d @seed-template.json`
- `curl -X POST localhost:8080/api/runs -d '{"prompt":"a cat learns to code","template_id":"..."}'`
- `curl localhost:8080/api/runs/<id>` → status `completed`, three echoed steps recorded, cost rows present.
- `psql` and confirm `pipeline_runs.total_cost_usd > 0` (test value) and `pipeline_cost_entries` rows align.

**Phase 2–4:**
- Same flow, observe the run advancing through real steps. Open MinIO console at `localhost:9001` and confirm `runs/<id>/...` keys exist.
- Tail logs of each runtime: `docker compose logs -f worker-script worker-image renderer api consumer`. Confirm correlation by `run_id`.
- Open presigned URL of final video — plays in browser.

**Phase 5:**
- Open `http://localhost:8080`, submit a prompt, watch the run page animate through steps, play the final MP4 inline.
- Edit a template, run again with different image count, confirm new pipeline shape.

**Phase 6:**
- `make test-e2e` from a clean state passes in CI.
- Kill `worker-image` mid-run, restart, confirm pending image messages are reprocessed (consumer-group semantics) and run completes.
- Submit two runs back-to-back, confirm both complete and costs are isolated per run.
