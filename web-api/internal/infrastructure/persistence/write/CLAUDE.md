# CLAUDE.md — persistence/write

Write-side repositories. Implement domain repository ports over a `pgx.Tx`.

## Rules
- Constructor takes `pgx.Tx` (the UoW transaction), never a pool.
- Deal in DOMAIN aggregates. Map aggregate → row on Save; row → aggregate via
  `domain.Reconstitute` on load (no events emitted on load).
- Hand-written SQL (no ORM). `ON CONFLICT` upsert is fine for Save when the
  aggregate may be new or existing.
- Return domain errors (e.g. `user.ErrNotFound`) for not-found, not pgx errors.
