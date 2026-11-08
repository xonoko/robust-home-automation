# 0001. Availability guarantees

- Status: Accepted
- Date: 2026-11-08

## Context

High availability is the main feature of this hub. It runs in people's homes, on Raspberry Pi-class nodes and consumer network gear, operated by people with no network or ops knowledge. Every other decision is evaluated against the guarantees below.

## Decision

**Planned operations lose nothing.** Upgrading or restarting the hub, k3s, NATS, Postgres or the OS, and draining or rebooting a node, must not drop a device connection, a message or an action. A pause of a few hundred milliseconds is acceptable.

**A node crash is as invisible as possible.** Device connections held by the crashed node may drop. Devices reconnect within about 2–3 seconds, nothing that was acknowledged is lost, and automations, timers and pending commands continue.

**Acknowledged means durable.** Anything we acknowledge to a device or a user is replicated on a quorum and fsynced before the acknowledgement.

**Limits we state openly:**
- Devices that drop messages while disconnected can lose data during a crash (never during planned operations).
- Single-coordinator radios (Zigbee) have a short disruption when the node running Zigbee2MQTT fails or is drained. See [0009](0009-integrations-and-command-delivery.md).
- The home's LAN switch or router is a single point of failure outside our control.

## Consequences

- No active/passive component may sit on the hot path if its switchover creates a gap.
- Every component needs a graceful drain and N/N-1 version compatibility for rolling upgrades.
- Database migrations are expand-then-contract only.
- A loss checker (simulated devices + gap/duplicate detection) is part of the test suite from day one; "zero loss, zero reconnects" during planned operations is a pass/fail criterion.

## Rejected alternatives

- **Full crash transparency** (replicated userspace TCP stack with output commit, VMware FT / F5 connection-mirroring style): feasible but judged too much for now. Planned operations are where homes actually see disruption.
- **2-node mode**: two voters cannot form a safe quorum; any design either stops on one failure or risks split brain. Dropped; supported modes are 1 and 3 nodes ([0002](0002-deployment-modes-k3s.md)).
