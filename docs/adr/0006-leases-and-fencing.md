# 0006. Leases and fencing

- Status: Accepted
- Date: 2026-11-08

## Context

Some work must have exactly one owner at a time: the VIP, each integration instance that sends commands, the config writer for some operations. Owners must change quickly on a crash and without a gap on planned operations, and a stale owner must never act.

## Decision

- **One owner per decision.** CNPG decides the Postgres primary. Every application-level ownership decision lives in **NATS JetStream KV** (R3). We do not use Kubernetes Leases for application ownership, so k3s upgrades (which restart API servers) cannot cause false failovers.
- **Per-instance leases, spread across nodes**: every node is active, each lease has one holder.
- **Server-side TTL**: the TTL is evaluated by NATS, never by local clocks. The holder stops acting after its renew deadline (shorter than the TTL), measured on its monotonic clock since the last successful renewal, so it always stops before anyone else can take over.
- **Fencing token = KV revision.** Every command carries it; our integrations and handlers reject a lower token; Postgres writes by an owner check it in the `WHERE` clause.
- **Planned handover**: the owner stops taking work, finishes what is in flight, and releases explicitly; the next owner is watching the key and acquires it immediately. Work arriving meanwhile waits in JetStream.
- Defaults: VIP lease TTL 2 s, renewed every 500 ms.

## Consequences

- Clocks only matter for display; boot with a wrong clock (no RTC on most Pis) does not break leases.
- Every component that acts on the world must accept and check a fencing token.

## Rejected alternatives

- **Kubernetes Lease API**: the client-go leader election documentation states it does not guarantee a single active leader; failover is bounded by the API server's health; k3s upgrades restart it.
- **A single global leader**: leaves the other nodes idle and makes every failover a full failover.
