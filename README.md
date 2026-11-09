# Robust Home Automation

A home automation hub whose main feature is high availability: it runs as a 3-node k3s cluster on Raspberry Pi-class hardware and simple home network gear, and planned operations (upgrades, reboots, drains) never drop a device connection, a message or an action.

Status: bootstrapping. Nothing is usable yet.

## Design

The architecture is documented as decision records in [docs/adr](docs/adr/README.md). Start with [0001 – Availability guarantees](docs/adr/0001-availability-guarantees.md).

## Layout

| Path | Contents |
|---|---|
| `cmd/hub` | Hub binary |
| `internal/` | Hub packages (API, bus, store, virtual IP, leases, automations, integrations, maintenance) |
| `proto/` | Protocol Buffers definitions for bus messages |
| `web/` | Web UI (React + TypeScript) |
| `deploy/` | Helm chart and local development cluster scripts |
| `spikes/` | Throwaway experiments that validate risky parts of the design |
| `docs/` | Architecture decision records and notes |

## Development

Requirements: Go 1.27, Node 26, Docker Desktop. See `make help`.

## License

[AGPL-3.0](LICENSE)
