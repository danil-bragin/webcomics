# CLAUDE.md — persistence/read

Read-models implementing `query.ReadModel`. The master-slave seam lives here.

## Rules
- Constructor takes the READ pool (`postgres.ReadPool`). NEVER the write pool or
  a tx.
- Return flat DTOs from the `query` package. No domain objects.
- Plain `SELECT`s; optimize/denormalize freely for reads. You may add
  materialized views or projection tables independent of the write schema.
- Generate with sqlc against `queries/` if you prefer codegen; the hand-written
  pool queries here are the fallback pattern.
- Tolerate replica lag — reads can be slightly stale by design.
