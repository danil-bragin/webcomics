# CLAUDE.md — platform

Low-level technical building blocks shared across infrastructure: connection
pools, clients, logger. No business or use-case logic.

## Sub-packages
- `postgres/` — TWO pools: `WritePool` (master) and `ReadPool` (replica). In dev
  `ReadPool` falls back to the write DSN. This split is the foundation of the
  CQRS read/write separation.
- `redis/` — Redis client + Watermill publisher/subscriber factories.
- `logger/` — slog setup.

## Rules
- Pools/clients are managed by `do` (they expose `Shutdown`/`HealthCheck`).
- Never pass the write pool where a read pool is expected, or vice versa.
- Keep this layer dependency-light; it's imported by infrastructure wiring.
