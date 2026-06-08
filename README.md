# Webcomics

A web-comics / short-video generation platform. Prompt → LLM script →
Flux-schnell images → audio + music → Remotion MP4 → scheduled upload to
YouTube / Instagram / TikTok / Facebook.

## Features

- **Generation pipeline** — script (OpenRouter) → panel images (fal.ai Flux
  schnell) → narration audio + background music → Remotion MP4 assembly. The
  step pipeline is data-driven, so new step types plug in without touching the
  orchestrator core.
- **Social accounts** — a global library of accounts shared across projects.
- **Upload** — multi-platform publishing. YouTube via the **official Data API**
  (OAuth, per-channel refresh tokens) with a **Selenium fallback** when the
  daily API quota is exhausted; Instagram / TikTok / Facebook via Selenium.
- **Scheduler** — per-account rate-limited upload scheduling with a recurring
  tick.
- **Metadata** — viral title/description generation via OpenRouter.
- **Analytics** — per-upload metrics tracked over time.

## Architecture

```
React + shadcn (web-ui)
        │ HTTP
        ▼
Go API (web-api, DDD + CQRS, oapi-codegen spec-first)
        │ Postgres (write/read pools, transactional outbox)
        │ Redis Streams (Watermill)
        ▼
Python workers (workers-py)         Node Remotion (renderer-node)
   • script  (OpenRouter)              • assemble (Remotion → MP4)
   • image   (fal.ai Flux schnell)
   • audio   (narration / TTS)
   • music   (background track)
   • upload  (YouTube API / Selenium, IG / TikTok / FB)
        ▼                                ▼
        └──────────── MinIO (S3-compat) ────────────┘
```

Bounded contexts and the full design live in `PLAN.md`. Per-feature specs
live in `docs/specs/`; the YouTube API setup guide is `docs/youtube-api-setup.md`.

## Required env

Copy the example files and fill in your own keys — the real `.env` files are
gitignored and must never be committed:

```
cp .env.example .env                  # provider keys for the workers
cp web-api/.env.example web-api/.env  # API config: DB, MinIO, keys, YouTube OAuth
```

`.env.example` documents every variable. Provider keys (OpenRouter, fal.ai,
ElevenLabs, Pixabay) are required for the matching pipeline step; leave a key
blank to disable that step. YouTube API upload additionally needs
`GOOGLE_OAUTH_CLIENT_ID/SECRET` — see `docs/youtube-api-setup.md`.

Ports are remapped to avoid conflicts with existing local services: postgres
`5433`, redis `6380`, minio `9000` (S3) and `9001` (console). The host-facing
URLs in `web-api/.env.example` already use these ports. Update `dev.compose.yml`
if you want different ones.

## Quick start (local, full stack)

```
docker compose -f dev.compose.yml up -d            # postgres, redis, minio, workers, renderer
cd web-api && make migrate-up                       # schema + seed template
cd web-api && make api &                            # Go HTTP server (:8080)
cd web-api && make consumer &                       # Go bus consumer
cd web-api && make relay &                          # outbox → Redis Streams
cd web-api && make ui-dev                           # React dev server (:5173)
```

Open <http://localhost:5173>. Pick the seeded template, type a prompt,
watch the run timeline.

## Without real API keys (echo loop)

Set `ECHO_PIPELINE=script,image,assemble` on the consumer process and
*don't* start the Python/Node workers. The consumer will short-circuit
the streams with synthetic completions so you can exercise the full Go
pipeline without spending real money. Useful for local development of
the UI and the orchestrator.

## Repo layout

```
web-api/          Go DDD + CQRS backend
workers-py/       Python workers (script, image, audio, music, upload)
renderer-node/    Node Remotion video assembler
web-ui/           React + shadcn frontend
docs/             design specs, runbook, YouTube API setup
ops/              dev ops scripts, Grafana dashboard, music library, firefox profiles
api/              cross-runtime artifacts (proto, openapi)  → lives in web-api/api
dev.compose.yml   local infra (postgres, redis, minio, workers, renderer)
PLAN.md           full design + execution log
```

## Make targets (in `web-api/`)

| Target          | What it does                                         |
|-----------------|------------------------------------------------------|
| `up`            | start postgres+redis+minio+workers+renderer          |
| `down`          | stop and remove containers                           |
| `migrate-up`    | apply schema + seed template                         |
| `migrate-down`  | roll back the latest migration                       |
| `api`           | run the HTTP server                                  |
| `consumer`      | run the bus consumer (Redis Streams)                 |
| `relay`         | run the outbox relay                                 |
| `ui-dev`        | run Vite dev server (proxy to `localhost:8080`)      |
| `ui-build`      | build the React app to `web-ui/dist/`                |
| `gen`           | re-run `oapi-codegen` from `api/openapi/openapi.yaml`|
| `install-tools` | install `oapi-codegen` to `$GOPATH/bin`              |

## Spec-first

`api/openapi/openapi.yaml` is the source of truth for the HTTP API.
The Go server interface is regenerated by `make gen`. When the spec
changes, you must re-run that target before the Go side compiles.
