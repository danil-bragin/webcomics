# dddcqrs

DDD + Clean Architecture + CQRS boilerplate in Go.

- **Any entry** (HTTP, gRPC, consumer, CLI) → turns input into a **Command** or
  **Query** and dispatches via a generic, type-safe **bus**.
- **Commands** run on the **write side** inside an **explicit Unit of Work**
  (the transaction boundary is owned and visible in the command handler).
- **Queries** run on the **read side** via a **separate read pool** — point it
  at a replica in prod for master-slave; in dev it falls back to master.
- **Domain events** are written to a **transactional outbox** in the same tx,
  then published to **Redis Streams** by a relay (at-least-once).

See `CLAUDE.md` (root) and the per-package `CLAUDE.md` files for the full rules
on where everything belongs.

## Layout

```
cmd/                 process entry points (api, grpc, consumer, outbox-relay, migrate)
internal/
  domain/            pure business logic (aggregates, value objects, events, ports)
  app/
    bus/             CQRS dispatch core (generics + middleware pipeline)
    command/         WRITE handlers (own the transaction via UoW)
    query/           READ handlers + flat DTOs (read pool only)
    middleware/      recover / logging / validation
    composition.go   DI wiring + handler registration (composition root)
  infrastructure/
    persistence/
      uow/           Unit of Work (transaction boundary)
      write/         write repositories (operate on pgx.Tx)
      read/          read-models (operate on read pool)
      outbox/        transactional outbox repo + relay
    config/          env config (write/read DSN split)
    messaging/       broker adapters
  interfaces/        entry adapters (http, grpc, consumer)
  platform/          pools (write+read), redis, logger
migrations/          goose SQL (embedded), targets master
api/                 openapi + proto contracts
dev.compose.yml      Postgres (wal_level=replica) + Redis
```

## Quick start

```bash
cp .env.example .env
make up                 # postgres + redis (dev.compose.yml)
make migrate-up         # goose migrations against master
make api                # HTTP entry on :8080
# in other terminals:
make relay              # outbox → Redis Streams
make consumer           # Redis Streams → bus (commands)
go mod tidy             # resolve dependency versions
```

### Try it

```bash
# write (command) — opens a UoW transaction, writes user + outbox event
curl -X POST localhost:8080/users -d '{"email":"a@b.com","password":"secret123"}'

# read (query) — hits the read pool
curl localhost:8080/users/<id>
```

## Master-slave later

Set `READ_DATABASE_URL` to your replica's DSN. Nothing else changes: queries
already go through `ReadPool`, commands through `WritePool`. `dev.compose.yml`
already sets `wal_level=replica` so you can attach a replica locally.

## gRPC

`internal/interfaces/grpc` + `cmd/grpc` are a transport skeleton. Generate Go
from `api/proto/user.proto` (buf/protoc), implement the generated server by
delegating each RPC to `bus.Dispatch` / `bus.Ask`, then serve on `GRPC_PORT`.

## Notes

- Run `go mod tidy` first — versions in `go.mod` are pinned from memory and may
  need refreshing.
- Consumers are at-least-once; keep commands idempotent (e.g. `Activate` is a
  no-op when already active) or dedup via `processed_messages`.
- The outbox relay uses `FOR UPDATE SKIP LOCKED`, so it's safe to scale out.
