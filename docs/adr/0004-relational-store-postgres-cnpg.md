# 0004. Relational store: Postgres on CloudNativePG

- Status: Accepted
- Date: 2026-11-08

## Context

Configuration (devices, channels, entities, areas, users, automations) is relational. Projects that start on a key-value store and later need a relational database go through painful migrations.

## Decision

- **Postgres managed by CloudNativePG (CNPG)**: 1 instance in single-node mode, 1 primary + 2 replicas in 3-node mode, synchronous replication to at least one replica so an isolated primary cannot commit.
- Postgres holds configuration and history only. **The hot path never touches it**: device → VIP → NATS → automations → NATS → VIP → device works while Postgres fails over.
- Access through **ent**; schema migrations are versioned with **Atlas** and are **expand-then-contract only**, because two hub versions run side by side during a rollout.
- Migrations run in the hub pod's init container with `atlas migrate apply`, which takes a lock so concurrent pods do not race.
- API writes retry with idempotency keys, so a CNPG switchover (a few seconds) shows up as a slower request, never as an error.
- History is written by a recorder consuming a JetStream stream, so Postgres unavailability only delays it.
- Major Postgres upgrades are done blue/green through CNPG's logical-replication import, not in-place `pg_upgrade`.

## Consequences

- Two stateful systems (NATS and Postgres), each with a clear role. Only CNPG decides the Postgres primary; every application-level ownership decision lives in NATS ([0006](0006-leases-and-fencing.md)).
- Nodes need SSD storage; SD cards are too slow and wear out.

## Rejected alternatives

- **JetStream KV as the only store**: configuration is relational; indexes and referential integrity would be hand-written.
- **Event log in JetStream + SQLite projection per node**: elegant (one quorum) but more design work and an unusual model; rejected together with CQRS.
- **rqlite / dqlite**: no interactive transactions (rqlite) or cgo and a smaller ecosystem (dqlite).
