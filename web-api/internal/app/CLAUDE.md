# CLAUDE.md — app (application layer)

Orchestrates use cases. Imports `domain` and `app/bus`; defines PORTS it needs
(e.g. `uow.Manager`, `query.ReadModel`) but never concrete infrastructure.

## Sub-packages
- `bus/` — the CQRS core. `Command`/`Query` marker interfaces, generic
  `RegisterCommand`/`RegisterQuery`, `Dispatch`/`Ask`, and the middleware
  pipeline type. Do not put business logic here.
- `command/` — WRITE handlers. Each opens a Unit of Work and owns its
  transaction explicitly. This is where transaction boundaries live.
- `query/` — READ handlers + read DTOs + the `ReadModel` port. No transactions,
  no domain aggregates. DTOs are flat structs.
- `middleware/` — cross-cutting concerns (recover, logging, validation).
- `composition.go` — the composition root: builds the do graph, sets the
  command/query middleware pipelines, and registers every handler on the bus.

## Rules
- A command handler MUST go through `uow.Manager` for any write. Never write via
  the read model or a raw pool.
- A query handler MUST use `query.ReadModel` only. If you import the write pool
  or a domain aggregate here, you've broken CQRS.
- Commands pre-process transport concerns already done (e.g. password is already
  hashed by the entry point); the domain never sees plaintext secrets.
- Register new handlers in `composition.go` — that's the single wiring point.
