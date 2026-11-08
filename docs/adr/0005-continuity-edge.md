# 0005. Continuity Edge: device connections that survive planned operations

- Status: Accepted
- Date: 2026-11-08

## Context

Most devices keep one long-lived connection (MQTT clients stay connected for days, Shelly opens an outbound WebSocket, EV chargers keep an OCPP WebSocket) and accept a single server address. Many cheap devices (Tasmota, Shelly, ESPHome) drop what they publish while disconnected. Draining long-lived connections therefore loses data: the only lossless option for planned operations is to **move the live connection**.

## Decision

The **Continuity Edge** is the hub's own network layer, inside the hub (not a separate project). Every device-facing connection terminates there: our own MQTT server (sessions, retained messages and pending messages in JetStream), the Matter controller endpoint, device WebSockets, and outbound connections to single-client devices.

**Addressing.** One pod holds the device-facing **VIP** on a macvlan interface with a stable **virtual MAC**, so devices' ARP caches stay valid when it moves. The VIP's IPv4 address is obtained by DHCP using that virtual MAC ([0007](0007-zero-config-networking.md)).

**Same-node handover (hub rollout).** The DaemonSet rolls out with `maxSurge: 1`. The new pod connects to the old one over a Unix socket in a shared `hostPath`, receives the socket file descriptors (`SCM_RIGHTS`) and each connection's application state, and resumes. The kernel connection never closes.

**Cross-node migration (drain, reboot).** Kernel `TCP_REPAIR`, as CRIU's `libsoccr` does:
1. The target Edge prepares (session state is already in JetStream).
2. Source: firewall drop on the VIP (devices retransmit), sockets into repair mode, dump sequence numbers, queues, TCP options and window, plus application state.
3. Transfer to the target (hundreds of connections × a few KB: well under 50 ms on gigabit).
4. Source removes the VIP and virtual-MAC interface; closing a repaired socket sends nothing.
5. Target creates the interface with the same MAC and VIP, recreates each socket in repair mode, leaves repair mode, sends a gratuitous ARP and lifts its own drop rule.

Expected pause: 100–300 ms; no reset, no reconnect.

**Ownership.** The Edge lease lives in NATS KV with a 2 s TTL, renewed every 500 ms ([0006](0006-leases-and-fencing.md)).

**Crash path.** The lease expires, a standby takes the VIP and virtual MAC and sends a gratuitous ARP. A device's next packet gets a reset, so it notices immediately and reconnects with its persistent session.

**Maintenance sequence for an OS reboot** (run by the hub's maintenance controller): migrate the Edge if it holds the VIP → move integration leases (make-before-break) → step down NATS leaders on the node → CNPG switchover if the primary is there → drain with PodDisruptionBudgets → reboot → wait healthy → uncordon. Kubelet graceful node shutdown runs the same preStop hooks on a manual reboot.

## Consequences

- Hub pods need host networking and `CAP_NET_ADMIN`, Linux ≥ 4.8, and wired Ethernet (macvlan does not work over Wi-Fi).
- Every device-facing protocol implementation must export and import its per-connection state.
- At home scale all device connections sit on one node; that keeps the design simple.
- Open risk: TLS connections (MQTT on 8883, OCPP) need a TLS implementation whose state can be exported for cross-node migration.

## Rejected alternatives

- **Pass-through L4 load balancer (Cilium Maglev + DSR, LVS)**: survives the VIP moving but not the server holding the connection; it chooses backends by hash, with no API to follow a migrated connection.
- **Proxy load balancers (HAProxy, Envoy)**: terminate TCP, so their own failover breaks every connection.
- **A standalone, protocol-agnostic edge project**: the connection's application state lives in the hub; keeping it inside the hub is what makes migration possible.
- **CRIU for whole processes**: only useful for third-party processes; not needed for code we own.
