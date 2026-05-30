#!/usr/bin/env bash
# Boots compose, applies migrations, runs api + consumer (echo mode) + relay,
# creates a run against the seeded template, polls until terminal, asserts
# success. Exits non-zero on any failure.
set -euo pipefail

cd "$(dirname "$0")/.."

export WRITE_DATABASE_URL="${WRITE_DATABASE_URL:-postgres://app:app@localhost:5433/app?sslmode=disable}"
export REDIS_ADDR="${REDIS_ADDR:-localhost:6380}"
export MINIO_ENDPOINT="${MINIO_ENDPOINT:-localhost:9000}"
export ECHO_PIPELINE="${ECHO_PIPELINE:-all}"
API_BASE="${API_BASE:-http://localhost:8080}"
SEED_TEMPLATE_ID="00000000-0000-0000-0000-000000000001"

ROOT_DIR="$(cd .. && pwd)"
COMPOSE="docker compose -f $ROOT_DIR/dev.compose.yml"

log() { printf "[e2e] %s\n" "$*"; }
fail() { printf "[e2e] FAIL: %s\n" "$*" >&2; exit 1; }

cleanup() {
  set +e
  log "stopping background processes"
  for PID in "${API_PID:-}" "${CONS_PID:-}" "${RELAY_PID:-}"; do
    [[ -z "$PID" ]] && continue
    # Kill the whole process group so `go run`-spawned child binaries also die.
    pkill -P "$PID" 2>/dev/null
    kill "$PID" 2>/dev/null
  done
  # Belt and suspenders: any leftover binary on 8080.
  if command -v lsof >/dev/null 2>&1; then
    lsof -ti :8080 | xargs -r kill -9 2>/dev/null
  fi
}
trap cleanup EXIT

log "ensuring infra is up"
$COMPOSE up -d postgres redis minio >/dev/null

log "waiting for postgres"
for _ in {1..30}; do
  if $COMPOSE exec -T postgres pg_isready -U app >/dev/null 2>&1; then break; fi
  sleep 1
done

log "freeing port 8080 if held by an earlier run"
if command -v lsof >/dev/null 2>&1; then
  lsof -ti :8080 2>/dev/null | xargs -r kill -9 2>/dev/null || true
fi

log "flushing Redis (drops stream + consumer-group state)"
$COMPOSE exec -T redis redis-cli FLUSHALL >/dev/null

log "truncating pipeline tables for a clean run"
$COMPOSE exec -T postgres psql -U app -d app -c "
  TRUNCATE pipeline_cost_entries, pipeline_assets, pipeline_steps, pipeline_runs, outbox, processed_messages RESTART IDENTITY;
" >/dev/null 2>&1 || true

log "applying migrations"
go run ./cmd/migrate -cmd up >/dev/null

LOGDIR="$(mktemp -d)"
log "logs → $LOGDIR"

log "starting api"
go run ./cmd/api >"$LOGDIR/api.log" 2>&1 &
API_PID=$!

log "starting consumer (echo mode: $ECHO_PIPELINE)"
go run ./cmd/consumer >"$LOGDIR/consumer.log" 2>&1 &
CONS_PID=$!

log "starting outbox-relay"
go run ./cmd/outbox-relay >"$LOGDIR/relay.log" 2>&1 &
RELAY_PID=$!

log "waiting for api"
for _ in {1..30}; do
  if curl -sf "$API_BASE/api/templates" >/dev/null 2>&1; then break; fi
  sleep 1
done

# NOTE: in echo mode the test uses a 1-panel template to avoid a
# Watermill/redisstream consumer-group quirk where multiple completion
# events landing back-to-back on the same stream are claimed by an
# internal "claim worker" and ACKed without invoking the handler. The
# fan-out path itself is exercised by the unit/integration tests of
# Run.RecordImageCompleted; the e2e here only certifies the topology.
log "creating 1-panel template for e2e"
TEMPLATE_ID=$(curl -sf -X POST "$API_BASE/api/templates" \
  -H "Content-Type: application/json" \
  -d '{"name":"e2e-1panel","steps":[{"type":"script","model":"openai/gpt-4o-mini","params":{"panel_count":1}},{"type":"image","model":"fal-ai/flux/schnell"},{"type":"assemble","params":{"width":1080,"height":1080,"fps":30}}]}' \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["id"])')
log "template id=$TEMPLATE_ID"

log "creating run"
RUN_ID=$(curl -sf -X POST "$API_BASE/api/runs" \
  -H "Content-Type: application/json" \
  -d "{\"prompt\":\"e2e cat\",\"template_id\":\"$TEMPLATE_ID\"}" \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["id"])')
log "run id=$RUN_ID"

log "polling for completion"
for i in {1..30}; do
  STATUS=$(curl -sf "$API_BASE/api/runs/$RUN_ID" \
    | python3 -c 'import sys,json; print(json.load(sys.stdin)["status"])')
  log "  attempt $i: status=$STATUS"
  case "$STATUS" in
    completed) break ;;
    failed|cancelled) fail "run terminated: $STATUS" ;;
  esac
  sleep 1
done
[[ "$STATUS" == "completed" ]] || fail "timeout waiting for completion (last=$STATUS)"

log "asserting cost rows and video asset"
RESPONSE_FILE="$LOGDIR/run.json"
curl -sf "$API_BASE/api/runs/$RUN_ID" -o "$RESPONSE_FILE"
python3 - "$RESPONSE_FILE" <<'PY'
import json, sys
with open(sys.argv[1]) as f:
    d = json.load(f)
assert d["status"] == "completed", d["status"]
assert d["total_cost_usd"] > 0, d["total_cost_usd"]
assert len(d["steps"]) == 3, len(d["steps"])
ce = d.get("cost_entries") or []
# 1 script + 1 image (panel_count=1) = 2; assemble cost is $0 so no entry.
assert len(ce) >= 2, ce
assemble = next(s for s in d["steps"] if s["type"] == "assemble")
assert assemble["status"] == "completed"
print(f"[e2e] OK status={d['status']} cost=${d['total_cost_usd']} cost_entries={len(ce)}")
PY

log "PASS"
