# CLAUDE.md — infrastructure

Implements the ports defined by `domain` and `app`. This is the ONLY layer that
knows about pgx, Redis, Watermill, sqlc, etc. It depends inward on `app` and
`domain`; nothing inward depends on it.

## Sub-packages
- `persistence/uow/` — the Unit of Work: contract (`uow.go`), pgx manager
  (`pgx_manager.go`), and the port aliases (`ports.go`). Binds write repos +
  outbox to a single `pgx.Tx`.
- `persistence/write/` — write repositories. Operate on `pgx.Tx`, map DOMAIN
  aggregates ↔ rows by hand, write to master.
- `persistence/read/` — read-models. Operate on the READ pool, return flat DTOs.
  No domain, no tx.
- `persistence/outbox/` — transactional outbox repository (writes events in the
  command's tx) + the relay (publishes unpublished rows to Redis Streams).
- `messaging/` — broker adapters beyond the relay (optional).
- `config/` — env config (write/read DSNs split here).

## Rules
- Write repos take a `pgx.Tx` (from the UoW), NOT a pool. Read-models take the
  read POOL, never a tx.
- Mapping/translation between domain and storage happens here, never in domain.
- A new aggregate's write repo must be added to `uow.repositories` so it shares
  the transaction.
