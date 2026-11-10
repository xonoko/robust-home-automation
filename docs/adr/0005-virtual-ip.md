# 0005. Virtual IP: connections that survive planned operations

- Status: Accepted
- Date: 2026-11-08

## Context

Most devices keep one long-lived connection (MQTT clients stay connected for days, Shelly opens an outbound WebSocket, EV chargers keep an OCPP WebSocket) and accept a single server address. Many cheap devices (Tasmota, Shelly, ESPHome) drop what they publish while disconnected. Draining long-lived connections therefore loses data: the only lossless option for planned operations is to **move the live connection**.

## Decision

The **virtual IP** (VIP) is the hub's own network layer, inside the hub (not a separate project). **Every connection terminates on it:** devices, integrations, the UI, phone apps, and `kubectl` through API aggregation ([0011](0011-api-extension-api-server.md)).

**Who migrates connections:**
- **Always, the hub core:**
  - the API and UI (the virtual IP holder forwards to `hub-api` and resumes watches);
  - the MQTT server: the integration interface ([0009](0009-integrations-and-command-delivery.md)) and devices that speak MQTT, with sessions, retained messages and pending messages in JetStream.
- **Optionally, an integration that chooses it.** An integration that accepts connections from devices (a WebSocket endpoint, OCPP) or keeps one to a single-client device can migrate them through the SDK. It declares host networking, `NET_ADMIN` and its ports on the virtual IP. Integrations that don't need it use active-active, graceful handover or reconnect with replay instead.

The maintenance controller orchestrates every process holding sockets on the virtual IP together, in one migration.

**Addressing.** One pod holds the **VIP** on a macvlan interface with a stable **virtual MAC**, so devices' ARP caches stay valid when it moves. The VIP's IPv4 address is obtained by DHCP using that virtual MAC ([0007](0007-zero-config-networking.md)).

**Same-node handover (hub rollout).** The DaemonSet rolls out with `maxSurge: 1`. The new pod connects to the old one over a Unix socket in a shared `hostPath`, receives the socket file descriptors (`SCM_RIGHTS`) and each connection's application state, and resumes. The kernel connection never closes.

**Cross-node migration (drain, reboot).** Kernel `TCP_REPAIR`, as CRIU's `libsoccr` does, for every process holding sockets on the virtual IP:
1. Each target process prepares (session state is already in JetStream).
2. Source: packets for the VIP are **held** in `NFQUEUE` instead of reaching the sockets. Each process puts its sockets into repair mode and dumps sequence numbers, queues, TCP options and window, plus its application state.
3. Transfer to the targets (hundreds of connections × a few KB: well under 50 ms on gigabit).
4. Source removes the VIP and virtual-MAC interface; closing a repaired socket sends nothing.
5. Targets create the interface with the same MAC and VIP, and recreate each socket in repair mode, then leave repair mode.
6. The held packets are sent to the target node and re-injected (through a TUN interface). The target sends a gratuitous ARP and accepts traffic again.

**Why packets must not reach the old sockets during the freeze.** A segment arriving after the snapshot would be acknowledged by the old socket. The device would discard it from its send buffer, while the new socket, restored from the snapshot, never saw it: the data would be lost. Holding the packets prevents the acknowledgement. It also prevents the old kernel from answering with a reset once its sockets are gone. Re-injecting the held packets then makes the pause equal to the migration time, instead of waiting for the device's retransmission timer (200 ms to 1 s on embedded stacks), and it also keeps UDP datagrams sent to the VIP during the freeze, which a drop rule would lose. Frames already in flight to the old switch port when the virtual MAC moves are still lost: TCP retransmits them, and for UDP it's a window of a few milliseconds.

Expected pause: the migration time itself, tens of milliseconds; no reset, no reconnect. The tcp-repair prototype measured at most 43 ms between two Linux machines (with a drop rule). It must be extended to holding packets and to an embedded TCP stack (ESP32).

**Ownership.** The VIP lease lives in NATS KV with a 2 s TTL, renewed every 500 ms ([0006](0006-leases-and-fencing.md)).

**Crash path.** The lease expires, a standby takes the VIP and virtual MAC and sends a gratuitous ARP. A device's next packet gets a reset, so it notices immediately and reconnects with its persistent session.

**Maintenance sequence for an OS reboot** (run by the hub's maintenance controller): migrate the VIP if the node holds it → move the active role of integrations (make-before-break) → step down NATS leaders on the node → CNPG switchover if the primary is there → drain with PodDisruptionBudgets → reboot → wait healthy → uncordon. Kubelet graceful node shutdown runs the same preStop hooks on a manual reboot.

**Service mode: no host networking, no added capabilities.** The hub must also run where pods get neither, for example under the `restricted` Pod Security level or in a shared cluster, with reduced features. The chart selects the mode (`network.mode: virtualIP | service`). At startup the hub checks what it actually has and shows in the UI which guarantees apply.
- Devices reach the hub through a Kubernetes Service: `LoadBalancer` (k3s ServiceLB, MetalLB, kube-vip) or `NodePort`. The cluster provides that address, not DHCP.
- No connection migration. On a planned operation, an integration first stops taking work, then closes its connections cleanly (MQTT 5 sends `DISCONNECT` with reason "server shutting down"). Devices reconnect through the Service to another pod. Integrations with the VIP migration profile behave as disruption-aware ([0009](0009-integrations-and-command-delivery.md)): MQTT clients with persistent sessions and QoS ≥ 1 lose nothing, but devices that drop what they publish while disconnected (Tasmota, Shelly, ESPHome) can lose messages during the reconnect.
- No LAN multicast or link-local traffic. KNX uses tunnelling in NAT mode instead of routing. There is no mDNS: devices and joining nodes are given the Service's address. Cameras are added by address, without ONVIF WS-Discovery. Matter is not available, because it needs IPv6 link-local and mDNS on the LAN.
- Unchanged: leases and fencing, command delivery, outbound connections (Modbus, the Zigbee2MQTT adapter, RTSP, cloud APIs), callbacks (sent to the Service's address) and crash recovery.

## Consequences

- In virtual IP mode, hub pods need host networking and `CAP_NET_ADMIN`, Linux ≥ 4.8, and wired Ethernet (macvlan does not work over Wi-Fi). Service mode needs none of these.
- The hub core's entry points (API and UI, MQTT server) export and import their per-connection state. Integrations do so only if they choose migration, with the SDK's help.
- At home scale all device connections sit on one node; that keeps the design simple.
- Open risk: TLS connections (HTTPS for every API client, MQTT on 8883, OCPP) need their state exported for migration. Candidate: kernel TLS, where the encryption state lives in the socket. Same-node handover then carries it with the socket, and `getsockopt(SOL_TLS)` should return keys and record sequence numbers for cross-node migration. To prove with a prototype, combined with `TCP_REPAIR`.

## Rejected alternatives

- **Pass-through L4 load balancer (Cilium Maglev + DSR, LVS)**: survives the VIP moving but not the server holding the connection; it chooses backends by hash, with no API to follow a migrated connection.
- **Proxy load balancers (HAProxy, Envoy)**: terminate TCP, so their own failover breaks every connection.
- **A standalone, protocol-agnostic load-balancer project**: the connection's application state lives in the hub; keeping it inside the hub is what makes migration possible.
- **Dropping packets during the freeze** (the first design): no loss for TCP thanks to retransmissions, but the pause waits for the device's retransmission timer, and UDP datagrams to the VIP are lost.
- **CRIU for whole processes:** not needed for code we own. Reconsidered on 2026-11-10 for third-party companions (Zigbee2MQTT) and rejected again. Kubernetes can checkpoint containers (beta) but not restore them natively. containerd is deprecating restore through container creation after security vulnerabilities, and pod-level restore (KEP-5823) isn't available yet.
- **KubeVirt live migration** (third-party programs in virtual machines): KubeVirt isn't officially supported on arm64, and its live migration there is only verified with masquerade networking, which changes the address devices see. Keeping connections needs bridge binding on the LAN and ReadWriteMany storage. It would be credible only on x86 nodes.
