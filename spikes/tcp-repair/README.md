# Spike: live TCP connection migration with TCP_REPAIR

Validates the cross-node migration of the virtual IP ([ADR 0005](../../docs/adr/0005-virtual-ip.md)): a device keeps one TCP connection open while the connection moves between two nodes, with no reconnect, no loss and no reordering.

## Setup

Runs entirely in Docker (works on Docker Desktop for Mac): three containers on a plain bridge network, which behaves like an unmanaged switch.

| Container | Role |
|---|---|
| `node-a`, `node-b` | Own the VIP `172.30.0.100` in turn, on a macvlan interface `vip0` with the virtual MAC `02:00:00:00:01:64`. Echo every 8-byte frame they receive. |
| `device` | One connection to the VIP. Sends a numbered frame every 20 ms and checks every echo arrives in order. Never reconnects: any error fails the test. |

Migration (`POST /migrate` on the current owner):
1. Firewall drop on the VIP (network lock), stop accepting.
2. Freeze connection goroutines, put sockets in repair mode, dump sequence numbers, send/receive queues, MSS, window scale, SACK, timestamps and window, plus the application's partial frame buffer.
3. Remove the VIP interface (repaired sockets close silently later).
4. Send the state to the target, which drops VIP traffic, creates the interface with the same MAC and VIP, starts its listener, rebuilds each socket in repair mode, leaves repair mode, lifts the drop and sends a gratuitous ARP.

## Run

```sh
./run.sh            # 3 migrations
ROUNDS=10 ./run.sh  # more
```

Exit code 0 means no loss, no reordering, no disconnect.

## Results (2026-11-08, Docker Desktop on Apple Silicon)

| | |
|---|---|
| Migrations | 3 (a → b → a → b) |
| Frames sent / echoed | 638 / 638 |
| Lost, out of order, reconnects | 0, 0, 0 |
| Longest pause seen by the device | 43 ms (others ~20 ms) |
| Dump on source | < 1 ms |
| Socket restore on target | ~3.5 ms |

The source reports ~1 s in total only because `arping -c 2` waits a second before its second announcement; traffic resumes after the first one.

## Findings

- **Works on a stock Docker Desktop kernel** with `CAP_NET_ADMIN` only; no CRIU needed.
- **Order matters on the target:** create the listener *before* restoring established sockets. A socket bound in repair mode ignores port conflicts, but a listener cannot bind next to an established socket.
- **ARP:** `arp_ignore=1` and `arp_announce=2` are required, plus a policy route for traffic sourced from the VIP, otherwise a node's own interface answers ARP for the VIP with its own MAC and devices follow the wrong MAC after a move.

## Not covered yet

- Real devices (lwIP on ESP32/ESP8266 has different retransmission behaviour than Linux) and real hardware (Raspberry Pi network card, consumer switch, Wi-Fi access point between device and switch).
- Two physical machines (here both nodes share the Docker Desktop VM's kernel).
- Many connections at once, and connections with data in flight in both directions under load.
- TLS: the application-level TLS state must move with the socket.
- Obtaining the VIP by DHCP with the virtual MAC ([ADR 0007](../../docs/adr/0007-zero-config-networking.md)).
