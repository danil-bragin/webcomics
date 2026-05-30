# CLAUDE.md — Architecture Guide (root)

DDD + Clean Architecture + CQRS boilerplate in Go. This file tells you (and
Claude) where everything belongs and the rules that must not be broken. Each
package also has its own CLAUDE.md with local rules.

## The one-paragraph mental model

Every **entry point** (HTTP, gRPC, message consumer, CLI — anything) does one
thing: translate its input into a **Command** or a **Query** and dispatch it
through the **bus**. Commands mutate state on the **write side** inside an
**explicit Unit of Work (transaction)** owned by the command handler. Queries
read flat DTOs on the **read side**, which uses a **separate read pool** (a
replica in production). The domain layer is pure. Infrastructure implements
ports. The composition root wires it all together.

## Dependency rule (Clean Architecture)

Dependencies point INWARD only:

```
interfaces ─┐
            ├─► app (command/query/bus) ─► domain
infrastructure ─────────────────────────► domain
            (implements ports defined by app/domain)
```

- `domain` imports NOTHING from other internal layers. No SQL, no transport.
- `app` imports `domain` and `app/bus`. It defines PORTS (interfaces) it needs.
- `infrastructure` imports `app` + `domain` to IMPLEMENT those ports.
- `interfaces` imports `app` (the bus + cmd/query types) only.
- `cmd/*` and `internal/app/composition.go` are the only places allowed to wire
  concrete infrastructure to ports.

If you find yourself importing `infrastructure` from `domain` or `app`, STOP —
you've inverted a dependency. Define a port instead.

## CQRS rules (non-negotiable)

1. **Write side = commands.** Mutations go through a command handler that opens
   a `uow.UnitOfWork`. The transaction boundary is EXPLICIT and visible in the
   handler (see `internal/app/command/`). Never mutate outside a UoW.
2. **Read side = queries.** Query handlers use `query.ReadModel`, which is
   backed by the **read pool** (`postgres.ReadPool`). Queries NEVER open a
   transaction, NEVER touch domain aggregates, and NEVER use the write pool.
   This is what makes master-slave replication a config change, not a rewrite.
3. **One message, one handler.** Each command/query type maps to exactly one
   handler, registered on the bus.
4. **Entries are dumb.** Entry points contain no business logic — only
   message construction + `bus.Dispatch` / `bus.Ask`.

## Transactions & events

- The command handler calls `uow.Manager.WithinTx` (or `Begin`/`Commit`).
- Inside the tx it gets repositories via `uow.Repositories()`. All share one
  `pgx.Tx`.
- Domain events recorded by the aggregate are written to the **outbox** table in
  the SAME transaction (`repos.Outbox().Add(...)`). Atomic with the state change.
- `cmd/outbox-relay` polls the outbox and publishes to Redis Streams. Delivery
  is at-least-once → consumers must be idempotent (dedup via `processed_messages`
  or naturally-idempotent commands).

## Where things live

| Concern | Location |
|---------|----------|
| Pure business rules, aggregates, value objects, domain events | `internal/domain/` |
| Command handlers (write, transactional) | `internal/app/command/` |
| Query handlers (read, no tx) | `internal/app/query/` |
| CQRS bus + dispatch | `internal/app/bus/` |
| Cross-cutting bus middleware | `internal/app/middleware/` |
| Composition root (DI wiring + handler registration) | `internal/app/composition.go` |
| UoW contract | `internal/infrastructure/persistence/uow/uow.go` |
| UoW + write repos + outbox (pgx impl) | `internal/infrastructure/persistence/{uow,write,outbox}/` |
| Read-models (sqlc/pgx on read pool) | `internal/infrastructure/persistence/read/` |
| Brokers, external services | `internal/infrastructure/messaging/`, `platform/` |
| Entry points | `internal/interfaces/{http,grpc,consumer}/` |
| Process mains | `cmd/*` |
| DB schema | `migrations/` |

## Adding a feature (checklist)

1. Model it in `internal/domain/<aggregate>/` (aggregate + events + WriteRepo port).
2. Write the command in `internal/app/command/` (open UoW, mutate, add events).
3. Write read DTOs + query in `internal/app/query/` and back it in
   `internal/infrastructure/persistence/read/`.
4. Implement the write repo in `internal/infrastructure/persistence/write/` and
   surface it on `uow.Repositories`.
5. Register handlers in `internal/app/composition.go`.
6. Add the entry mapping in `internal/interfaces/<transport>/`.
7. Add a migration in `migrations/`.

## Run it

```bash
cp .env.example .env
make up && make migrate-up
make api        # or: make consumer / make relay / make grpc
```
