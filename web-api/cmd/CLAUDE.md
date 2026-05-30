# CLAUDE.md — cmd (process entry points / mains)

Each subfolder is a runnable binary. A main builds the composition root
(`app.Build`), pulls what it needs from the injector, starts its transport or
loop, and handles graceful shutdown.

## Binaries
- `api/` — HTTP server (chi) → bus.
- `grpc/` — gRPC server → bus (skeleton; generate pb first).
- `consumer/` — Watermill consumer (Redis Streams) → bus.
- `outbox-relay/` — publishes outbox events to Redis Streams.
- `migrate/` — goose migrations against the WRITE/master DB.

## Rules
- mains are the ONLY place (besides `composition.go`) allowed to wire concretes.
- All entry mains share `app.Build(cfg)` so the bus + handlers are identical
  across transports.
- Migrations always target `WRITE_DATABASE_URL`.
