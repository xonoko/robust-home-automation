# 0002. Deployment modes on k3s

- Status: Accepted
- Date: 2026-11-08

## Context

The default deployment is a k3s cluster on simple hardware. We want one code path, not separate "simple" and "HA" products.

## Decision

Two supported modes, both on k3s:

| Mode | k3s datastore | NATS | Postgres | Survives |
|---|---|---|---|---|
| Single node | SQLite (kine) | 1 server, R1 | 1 instance | nothing (no HA, easy) |
| 3 nodes | embedded etcd, 3 servers | 3 servers, R3 | 1 primary + 2 replicas, sync replication | one node down |

- Kubernetes is the **supervisor and packager**, not the failover mechanism. Failover is driven by the application ([0006](0006-leases-and-fencing.md)) and never waits for pod rescheduling (default eviction is ~5 minutes; StatefulSet pods on an unreachable node are never rescheduled automatically).
- The hub runs as a **DaemonSet**: one pod per node, pinned, using local storage only through NATS and Postgres.
- k3s upgrades are done **without draining**: restarting k3s does not stop running containers. Only OS/kernel reboots go through the maintenance sequence in [0005](0005-continuity-edge.md).
- Local development uses Docker Desktop's built-in Kubernetes (kind provisioner, 3 nodes).

## Consequences

- With 3 nodes we tolerate one failure *or* one node under maintenance, not both at once.
- Lease and storage code has a single implementation path; tests can use the single-node mode.

## Rejected alternatives

- **Plain binary / docker-compose for single node**: a second deployment path to maintain; kept for local development only.
- **2 nodes (with or without witness)**: see [0001](0001-availability-guarantees.md).
- **k3s with an external datastore**: moves the single point of failure to that datastore.
