# CLAUDE.md — app/query (READ side)

Query handlers + read DTOs. Built for master-slave: everything here runs on the
READ pool (a replica in prod).

## Rules
- Define flat DTOs (e.g. `UserView`) — NOT domain aggregates.
- Depend only on the `ReadModel` port; its implementation lives in
  `infrastructure/persistence/read` and uses `postgres.ReadPool`.
- NEVER open a transaction. NEVER import the write pool, UoW, or domain
  aggregates.
- Reads may be eventually-consistent (replica lag). Design queries to tolerate
  that; if a flow needs read-after-write consistency, do that read inside the
  command on the write side instead.
- Register via `XOnBus(reg, readModel)` from `composition.go`.
