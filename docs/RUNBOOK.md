# Operations Runbook

Practical playbooks for keeping the Webcomics pipeline healthy.

## Quick reference

| Symptom | First look at | Common fix |
|---|---|---|
| Run stuck on `running` | `pipeline_steps_total` (failed?) + worker logs | restart stuck worker (it's idempotent) |
| Spike in `*.failed` events | provider's status page | flip cancel switch, retry once provider recovers |
| Cost climbing without runs | `pipeline_step_cost_usd_total` rate vs `pipeline_runs_total` rate | check for runaway loop in a template |
| `dispatch failed` in api logs | matching command type + run_id | usually a transient DB error → bus retries |
| Worker container restarting | `docker compose logs worker-*` | OOM or upstream API auth failure |

## Common operations

### Tail logs for a single run

Every runtime emits JSON with `run_id`. Pick the run id from `/runs/:id` and
grep:

```bash
docker compose -f dev.compose.yml logs --tail=200 -f \
  worker-script worker-image renderer api consumer \
  | grep '"run_id":"<run-id>"'
```

### Cancel an in-flight run

From the UI (RunDetail → `cancel` button) or via API:

```bash
curl -X POST http://localhost:8080/api/runs/<id>/cancel \
  -H "X-API-Key: $API_KEY"
```

The cancel event hits every worker; in-flight calls finish but no new work
is started for that run.

### Retry a failed run

```bash
curl -X POST http://localhost:8080/api/runs/<id>/retry \
  -H "X-API-Key: $API_KEY"
```

Returns the new run id. Original is preserved for audit.

### Scale a worker

```bash
docker compose -f dev.compose.yml up -d \
  --scale worker-image=6
```

Image step parallelism caps at `panels_expected` per run; over-scaling
above that doesn't speed up a single run but absorbs concurrent runs.

### Drain Redis Streams (operator escape hatch)

If a poison message is wedging a worker (handler stuck retrying):

```bash
docker exec dddcqrs-redis redis-cli XINFO STREAM pipeline.image.requested
docker exec dddcqrs-redis redis-cli XGROUP DELCONSUMER \
  pipeline.image.requested pipeline-py-image <consumer-uuid>
```

Restart the worker — it picks up a fresh consumer name.

### Recover after a worker crash

Redis Streams + consumer groups make this safe by design:

- in-flight messages stay in PEL (pending entry list) until ACKed
- a freshly-started worker auto-claims them after `MaxIdleTime`
- Go aggregate rejects duplicate `Record*Completed` calls (idempotent)

Just restart the worker — no manual replay required.

### Rotate `OPENROUTER_API_KEY` / `FAL_KEY` / `TELEGRAM_BOT_TOKEN`

1. Add the new key to your secret store.
2. Rolling-restart the corresponding worker(s):

   ```bash
   docker compose -f dev.compose.yml restart worker-script
   docker compose -f dev.compose.yml restart worker-image
   docker compose -f dev.compose.yml restart worker-upload
   ```

3. Retire the old key.

### Migrate the database

```bash
WRITE_DATABASE_URL=... go run ./cmd/migrate -cmd up    # apply
WRITE_DATABASE_URL=... go run ./cmd/migrate -cmd down  # roll back one
WRITE_DATABASE_URL=... go run ./cmd/migrate -cmd status
```

Migrations are embedded in the binary via `go:embed`; no separate file
deploy required.

## Pipeline health checks

| Check | Command | Healthy |
|---|---|---|
| API responding | `curl :8080/api/templates` | 200 |
| Prometheus scraping | `curl :8080/metrics \| head -3` | `# HELP …` |
| Outbox lag | `SELECT count(*) FROM outbox WHERE published_at IS NULL` | < 100 |
| Workers attached | `docker exec dddcqrs-redis redis-cli XINFO GROUPS pipeline.script.requested` | ≥ 1 consumer |
| MinIO ready | `curl :9000/minio/health/live` | 200 |

## Stuck-run triage

1. `GET /api/runs/<id>` — what step is current?
2. Inspect that step's outbox row: was the *.requested event published?
   ```sql
   SELECT event_name, published_at FROM outbox WHERE aggregate_id=$1 ORDER BY occurred_at;
   ```
3. Check Redis Streams for delivery:
   ```bash
   docker exec dddcqrs-redis redis-cli XINFO GROUPS pipeline.<type>.requested
   ```
4. Tail the worker logs for that `run_id`.
5. If the worker crashed mid-flight: restart it; pending messages auto-redeliver.
6. If the worker returned a fatal error: it published `*.failed`; the run
   should be in `failed` status. If not, re-dispatch:
   ```bash
   curl -X POST http://localhost:8080/api/runs/<id>/retry
   ```

## Cost guardrails

- Cap `panel_count` in the script-step `params`. Each panel ≈ one image
  generation (≈ $0.003 with Flux schnell).
- Alert when `rate(pipeline_step_cost_usd_total[1h]) > $X` per provider
  (see `ops/grafana/webcomics-dashboard.json`, panel "Cost by provider").
- The Dashboard UI surfaces total cost + provider breakdown at a glance —
  share it with whoever owns the budget.
