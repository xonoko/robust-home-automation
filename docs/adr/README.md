# Architecture Decision Records

Each record explains one decision: the context, what we chose, the consequences and the options we rejected.
Records are immutable once accepted; a change of mind is a new record that supersedes the old one.

| # | Title | Status |
|---|---|---|
| [0001](0001-availability-guarantees.md) | Availability guarantees | Accepted |
| [0002](0002-deployment-modes-k3s.md) | Deployment modes on k3s | Accepted |
| [0003](0003-message-bus-nats-jetstream.md) | Message bus: NATS JetStream | Accepted |
| [0004](0004-relational-store-postgres-cnpg.md) | Relational store: Postgres on CloudNativePG | Accepted |
| [0005](0005-virtual-ip.md) | Virtual IP: connections that survive planned operations | Accepted |
| [0006](0006-leases-and-fencing.md) | Leases and fencing | Accepted |
| [0007](0007-zero-config-networking.md) | Zero-config networking on consumer hardware | Accepted |
| [0008](0008-domain-model.md) | Domain model: Device, Channel, Entity, Area | Accepted |
| [0009](0009-integrations-and-command-delivery.md) | Integrations, failover profiles and command delivery | Accepted |
| [0010](0010-backend-stack.md) | Backend stack and tooling | Accepted |

Template for new records: [template.md](template.md).
