# 0007. Zero-config networking on consumer hardware

- Status: Accepted
- Date: 2026-11-08

## Context

The hub runs on simple home hardware: a consumer router (often the ISP's box) and unmanaged switches. Users have no network knowledge: we cannot ask for DHCP reservations, static IPs or "an address outside the DHCP pool". Node addresses given by DHCP can change.

## Decision

**Requirements:** Pi-class nodes on wired Ethernet, plugged into the router or any switch. SSD recommended. UPS optional (correctness does not depend on it). No managed switch, PoE, IGMP snooping or VLAN is ever required.

**Internal addresses (node to node).** At first boot each node adds a second, self-assigned address in `169.254.0.0/16`, checked with an ARP probe (RFC 5227), persisted and defended. Routers never hand out that range, so it cannot collide with their pool. k3s/etcd, NATS, Postgres replication and connection migration use it; the node's DHCP address is only for internet access and may change freely. Fallback if Kubernetes rejects a link-local node IP: self-generated IPv6 ULA addresses.

**VIP.** The hub pod holding the VIP runs a DHCP client on its virtual-MAC interface, so the router allocates the VIP from its own pool. The lease follows the virtual MAC and client ID when the VIP moves; its state is replicated in NATS KV; the holder renews it. The request carries the hostname `home-hub`.

**Devices.** The hub writes its address into every device it can configure (ESPHome, Tasmota, Shelly, Zigbee2MQTT), prefers names (`home-hub.local` over mDNS) where devices support them, and detects VIP changes (router replaced or reset) to reconfigure devices automatically, listing in the UI the ones it could not.

**Joining nodes.** New nodes discover the cluster via mDNS (`_home-hub._tcp`) and join with a pairing code or QR code.

**Multicast (KNX IP routing, mDNS).** Works on unmanaged switches. Some routers filter multicast without an IGMP querier; the hub detects missing KNX routing traffic and falls back to tunnelling.

## Consequences

- The setup wizard never asks for an IP address.
- Must validate: k3s with a link-local `--node-ip`; DHCP lease behaviour of common ISP routers when the VIP moves.

## Rejected alternatives

- **DHCP reservations / static IPs**: require network knowledge.
- **Picking a VIP "outside the pool"**: the pool is unknown and differs per router.
