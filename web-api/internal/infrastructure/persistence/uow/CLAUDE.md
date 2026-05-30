# CLAUDE.md — persistence/uow (transaction boundary)

The Unit of Work is the heart of write-side transaction control.

## Files
- `uow.go` — `UnitOfWork`, `Manager`, `Repositories` contracts.
- `ports.go` — repository ports surfaced on the UoW (`UserWriteRepository`
  aliases the domain port; `OutboxRepository`). Aliases avoid import cycles.
- `pgx_manager.go` — concrete pgx implementation. `Begin` opens a tx and binds
  all repositories to it; `WithinTx` runs a func and commits/rolls back.

## Rules
- All repositories returned by one `UnitOfWork` MUST share the same `pgx.Tx`.
- `Manager` is constructed with the WRITE pool only.
- Commit is idempotent (no-op after commit); Rollback after commit is a no-op.
- To add a repository to the transaction: add a method to `Repositories`, a
  field to `repositories` in `pgx_manager.go`, and construct it in `newPgxUoW`.
