# CLAUDE.md — interfaces (entry points)

Transport adapters. Each one ONLY translates its input into a command/query and
dispatches via the bus. No business logic, no DB access.

## Sub-packages
- `http/` — chi server; maps requests → `bus.Dispatch` / `bus.Ask`.
- `grpc/` — gRPC service; same pattern (generate pb from `api/proto`).
- `consumer/` — Watermill consumer; maps broker events → commands.

## Rules
- Allowed imports: `app/bus`, `app/command`, `app/query`. NOT infrastructure,
  NOT domain internals.
- Transport-specific prep (parse body, hash password, read headers, auth) is OK
  here; decisions about state are NOT — those belong in command handlers.
- Writes → `bus.Dispatch[Result]`. Reads → `bus.Ask[Result]`.
- Adding a transport = add a package here + a `cmd/<x>` main; the bus and
  handlers are reused unchanged. That's the whole point.
