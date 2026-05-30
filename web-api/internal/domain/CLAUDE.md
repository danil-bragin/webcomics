# CLAUDE.md — domain

Pure business logic. The innermost layer. **No imports from app, infrastructure,
interfaces, or any framework/driver.** Only the standard library and tiny pure
helpers (e.g. uuid) are allowed.

## What goes here
- Aggregates (e.g. `user/user.go`) — entities with identity + invariants.
- Value objects (e.g. `Email`, `ID`) — self-validating, immutable.
- Domain events (e.g. `UserRegistered`) — past-tense facts.
- Repository PORTS (interfaces) the domain/app needs — e.g. `user.WriteRepository`.
- Domain errors (`ErrEmailInvalid`, `ErrNotFound`).

## What must NOT go here
- SQL, pgx, HTTP, gRPC, Watermill, slog, do — anything infrastructural.
- Mapping to/from DB rows or JSON (that's infrastructure's job).
- Transaction handling (that's the UoW, in infrastructure/app).

## Rules
- State changes go through behavior (methods), never public setters.
- Factories (`Register`) record domain events via `shared.AggregateRoot.Record`.
- `Reconstitute` rebuilds an aggregate from storage WITHOUT emitting events.
- Keep one package per aggregate / bounded-context concept.
