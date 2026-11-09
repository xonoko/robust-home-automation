# 0005. Virtual IP: connections that survive planned operations

- Status: Accepted
- Date: 2026-11-08

## Context

Most devices keep one long-lived connection (MQTT clients stay connected for days, Shelly opens an outbound WebSocket, EV chargers keep an OCPP WebSocket) and accept a single server address. Many cheap devices (Tasmota, Shelly, ESPHome) drop what they publish while disconnected. Draining long-lived connections therefore loses data: the only lossless option for planned operations is to **move the live connection**.

## Decision

The **virtual IP** (VIP) is the hub's own network layer, inside the hub (not a separate project). Every connection the hub holds terminates on it, and moves with it: our own MQTT server (sessions, retained messages and pending messages in JetStream), the Matter controller endpoint, device WebSockets, and outbound connections to single-client devices.

**Addressing.** One pod holds the **VIP** on a macvlan interface with a stable **virtual MAC**, so devices' ARP caches stay valid when it moves. The VIP's IPv4 address is obtained by DHCP using that virtual MAC ([0007](0007-zero-config-networking.md)).

**Same-node handover (hub rollout).** The DaemonSet rolls out with `maxSurge: 1`. The new pod connects to the old one over a Unix socket in a shared `hostPath`, receives the socket file descriptors (`SCM_RIGHTS`) and each connection's application state, and resumes. The kernel connection never closes.

**Cross-node migration (drain, reboot).** Kernel `TCP_REPAIR`, as CRIU's `libsoccr` does:
1. The target hub pod prepares (session state is already in JetStream).
2. Source: firewall drop on the VIP (devices retransmit), sockets into repair mode, dump sequence numbers, queues, TCP options and window, plus application state.
3. Transfer to the target (hundreds of connections × a few KB: well under 50 ms on gigabit).
4. Source removes the VIP and virtual-MAC interface; closing a repaired socket sends nothing.
5. Target creates the interface with the same MAC and VIP, recreates each socket in repair mode, leaves repair mode, sends a gratuitous ARP and lifts its own drop rule.

Expected pause: 100–300 ms; no reset, no reconnect.

**Ownership.** The VIP lease lives in NATS KV with a 2 s TTL, renewed every 500 ms ([0006](0006-leases-and-fencing.md)).

**Crash path.** The lease expires, a standby takes the VIP and virtual MAC and sends a gratuitous ARP. A device's next packet gets a reset, so it notices immediately and reconnects with its persistent session.

**Maintenance sequence for an OS reboot** (run by the hub's maintenance controller): migrate the VIP if the node holds it → move integration leases (make-before-break) → step down NATS leaders on the node → CNPG switchover if the primary is there → drain with PodDisruptionBudgets → reboot → wait healthy → uncordon. Kubelet graceful node shutdown runs the same preStop hooks on a manual reboot.

**Service mode: no host networking, no added capabilities.** The hub must also run where pods get neither, for example under the `restricted` Pod Security level or in a shared cluster, with reduced features. The chart selects the mode (`network.mode: virtualIP | service`). At startup the hub checks what it actually has and shows in the UI which guarantees apply.
- Devices reach the hub through a Kubernetes Service: `LoadBalancer` (k3s ServiceLB, MetalLB, kube-vip) or `NodePort`. The cluster provides that address, not DHCP.
- No connection migration. On a planned operation, an integration first stops taking work, then closes its connections cleanly (MQTT 5 sends `DISCONNECT` with reason "server shutting down"). Devices reconnect through the Service to another pod. Integrations with the VIP migration profile behave as disruption-aware ([0009](0009-integrations-and-command-delivery.md)): MQTT clients with persistent sessions and QoS ≥ 1 lose nothing, but devices that drop what they publish while disconnected (Tasmota, Shelly, ESPHome) can lose messages during the reconnect.
- No LAN multicast or link-local traffic. KNX uses tunnelling in NAT mode instead of routing. There is no mDNS: devices and joining nodes are given the Service's address. Cameras are added by address, without ONVIF WS-Discovery. Matter is not available, because it needs IPv6 link-local and mDNS on the LAN.
- Unchanged: leases and fencing, command delivery, outbound connections (Modbus, the Zigbee2MQTT adapter, RTSP, cloud APIs), callbacks (sent to the Service's address) and crash recovery.

## Consequences

- In virtual IP mode, hub pods need host networking and `CAP_NET_ADMIN`, Linux ≥ 4.8, and wired Ethernet (macvlan does not work over Wi-Fi). Service mode needs none of these.
- Every device-facing protocol implementation must export and import its per-connection state.
- At home scale all device connections sit on one node; that keeps the design simple.
- Open risk: TLS connections (MQTT on 8883, OCPP) need a TLS implementation whose state can be exported for cross-node migration.

## Rejected alternatives

- **Pass-through L4 load balancer (Cilium Maglev + DSR, LVS)**: survives the VIP moving but not the server holding the connection; it chooses backends by hash, with no API to follow a migrated connection.
- **Proxy load balancers (HAProxy, Envoy)**: terminate TCP, so their own failover breaks every connection.
- **A standalone, protocol-agnostic load-balancer project**: the connection's application state lives in the hub; keeping it inside the hub is what makes migration possible.
- **CRIU for whole processes**: only useful for third-party processes; not needed for code we own.
