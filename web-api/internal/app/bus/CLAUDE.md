# CLAUDE.md — app/bus

The CQRS dispatch core. Type-safe over generics; the middleware pipeline runs on
a type-erased boundary so one pipeline covers all messages.

## Contracts
- `Command` / `Query` — marker interfaces (`isCommand()` / `isQuery()`).
- `RegisterCommand[C,R]` / `RegisterQuery[Q,R]` — wire a handler; applies the
  command or query middleware pipeline respectively.
- `Dispatch[R]` — send a command, get typed result `R`.
- `Ask[R]` — send a query, get typed result `R`.

## Rules
- Commands get the COMMAND pipeline; queries the QUERY pipeline. They are set
  separately in `composition.go` (commands may carry transactional concerns;
  queries must not).
- Keep this package free of domain/infrastructure imports. It only knows the
  marker interfaces and `reflect` for routing.
- One handler per message type. Re-registering overwrites.
