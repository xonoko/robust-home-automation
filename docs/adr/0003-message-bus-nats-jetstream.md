# 0003. Message bus: NATS JetStream

- Status: Accepted
- Date: 2026-11-08

## Context

Device events, commands, automation triggers and internal coordination all flow through a bus. It must be clustered (no switchover gap), durable before acknowledgement, push-based with sub-millisecond fan-out, and light enough for a Raspberry Pi.

## Decision

- **NATS server with JetStream**, deployed as its own StatefulSet (not embedded in the hub), so hub releases never restart the broker.
- Streams and KV buckets that carry state use **R3** replication.
- **`sync_interval: always`**. The JetStream default flushes to disk every 2 minutes (`defaultSyncInterval` in `server/filestore.go`); with all nodes in one house, a power cut would lose up to 2 minutes of acknowledged data.
- The hub uses the **native NATS protocol** with all server URLs. Devices do not connect to NATS; they connect to the Continuity Edge ([0005](0005-continuity-edge.md)).
- Consumers are durable pull consumers, acked after processing; handlers are idempotent; publishes carry `Nats-Msg-Id` for deduplication.
- Trace context travels in NATS message headers.
- Subjects follow `hub.v1.<kind>.<id>.<verb>`, e.g. `hub.v1.entity.<id>.state`, `hub.v1.device.<id>.cmd`, `hub.v1.integration.<id>.health`.
- Payloads are Protocol Buffers ([0010](0010-backend-stack.md)).

## Consequences

- NATS upgrades: step stream and meta leaders down from a server, put it in lame duck mode, restart, wait until JetStream reports it caught up, then the next one.
- JetStream KV also provides leases ([0006](0006-leases-and-fencing.md)), automation run state and durable timers (scheduled messages, NATS ≥ 2.12).

## Rejected alternatives

- **Embedded MQTT broker (mochi-mqtt) as the bus**: no clustering; a switchover creates a gap. mochi's main branch has had no commits since March 2025.
- **NATS built-in MQTT gateway for devices**: correct durability (PUBACK is sent only after the JetStream store acknowledges, `server/mqtt.go`), but MQTT 3.1.1 only and its connections cannot be migrated. Usable as a stepping stone only.
- **Postgres as the queue (River, LISTEN/NOTIFY)**: polling or change-feed latency, write amplification per subscriber, and failover of tens of seconds on the hot path.
- **Multi-master Postgres**: asynchronous replication acknowledges before replicating, which loses data on a crash.
- **Distributed SQL (YugabyteDB, CockroachDB)**: meets the durability rule but too heavy for Pis and a poor message bus.
- **Watermill / CQRS**: an abstraction layer that hides the JetStream features we rely on; CQRS is overkill for CRUD configuration.
