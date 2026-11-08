# 0009. Integrations, failover profiles and command delivery

- Status: Accepted
- Date: 2026-11-08

## Context

Most target installations are KNX with an IP gateway (typically an MDT SCN-IP100.03: KNX IP router, 4 tunnels, KNX IP Secure). Other protocols have very different failover properties, and some (Zigbee) cannot be made lossless at a reasonable cost.

## Decision

**Every integration declares a failover profile**, which the maintenance controller uses to move it:

| Profile | Meaning | Examples |
|---|---|---|
| Edge migration | The device keeps a long-lived connection to us; it moves with the Edge | MQTT clients, Shelly outbound WebSocket, OCPP |
| Multiple listeners | Several nodes receive the same traffic; duplicates are removed | KNX routing or one tunnel per node, ONVIF events, EnOcean with two gateways, BLE proxies |
| Lease | One owner; clean handover on planned operations | Modbus (often single-client), SIP (planned moves wait for calls to end), outgoing KNX commands |
| Disruption-aware | A short gap is accepted and handled by the command model | Zigbee2MQTT, cloud APIs |

**KNX.** KNX devices work without the hub. Prefer KNX IP routing (multicast, no tunnel limit), with KNX IP Secure; fall back to tunnels from two nodes. Request a specific tunnel (MDT routers can shift tunnels, which breaks Data Secure reception). A second KNX IP interface on the line removes the gateway as a single point of failure.

**Zigbee.** An Ethernet adapter driven by one Zigbee2MQTT instance (lease-gated, warm standbys, data synced to JetStream). A node failure or drain causes a short disruption; we do not try to make Zigbee failure-proof.

**Command delivery** (all integrations):
- The hub tracks a **desired state**, not a stream of commands. State commands are coalesced to the latest value per attribute and reconciled against the reported state when the integration comes back.
- **Momentary** commands (chime, scene) have a short TTL. **Toggles** are resolved to an absolute state when issued, so retries never flip twice.
- A command is done when the device **reports** the new state; until then the hub retries with backoff until the command's deadline. Every command has a deadline.
- Before a planned move, the maintenance controller announces it; the hub stops sending to that integration, waits for in-flight commands, and only then stops it.

**Missed events.** After a reconnect, the hub reads the current state, compares it with the last known state and emits recovered changes marked `late`. Automation triggers choose `run`, `skip` or `run_if_within(<duration>)` for late events.

**Callbacks.** Any protocol where a device calls us back (OCPP, UPnP events, webhooks, Shelly WebSocket) is given the VIP, never a node address.

## Consequences

- Integration code implements a small contract: failover profile, state export/import where relevant, desired-state reconciliation.
- The UI shows integration availability and pending commands.

## Rejected alternatives

- **Making Zigbee lossless** (radio relay and warm standby on TI coordinators, spare coordinator with the same identity, a Go Zigbee host stack): too much engineering for a protocol with a single coordinator by design.
