# CLAUDE.md — persistence/outbox

Transactional outbox: atomic event emission + async publish.

## Files
- `repository.go` — `Add(events...)` inserts events into `outbox` within the
  UoW transaction. Called by command handlers via `repos.Outbox()`.
- `relay.go` — `Relay` polls unpublished rows (`FOR UPDATE SKIP LOCKED`),
  publishes to Redis Streams, marks them published, in its own tx.

## Rules
- `Add` MUST run in the same tx as the aggregate change — never publish to the
  broker directly from a command handler.
- The relay is at-least-once: a crash after publish, before mark, re-publishes.
  Consumers MUST be idempotent.
- Run the relay as its own process (`cmd/outbox-relay`). `SKIP LOCKED` makes it
  safe to run multiple instances.
