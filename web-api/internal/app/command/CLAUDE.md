# CLAUDE.md — app/command (WRITE side)

Command handlers. THIS is where transactions are controlled, explicitly.

## The pattern (follow it exactly)
```go
func (h *XHandler) Handle(ctx, cmd) (Result, error) {
    // validate/convert primitives into value objects
    return result, h.uow.WithinTx(ctx, func(ctx, u uow.UnitOfWork) error {
        repos := u.Repositories()
        // load aggregate(s) via repos.X()
        // call domain behavior (enforces invariants)
        // repos.X().Save(...)
        // repos.Outbox().Add(agg.PullEvents()...)  // events in SAME tx
        return nil // commit on nil, rollback on error
    })
}
```

## Rules
- Always use `uow.Manager`. Never touch a pgx pool directly.
- Persist domain events to the outbox inside the same UoW as the state change.
- Each command struct implements `isCommand()` and is registered via
  `XOnBus(reg, manager)` called from `composition.go`.
- No reads from the read model here. If you need data to decide, load the
  aggregate through the write repo (it's in-transaction and consistent).
- Keep handlers thin: orchestration only; invariants live in the domain.
