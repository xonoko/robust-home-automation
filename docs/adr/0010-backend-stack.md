# 0010. Backend stack and tooling

- Status: Accepted
- Date: 2026-11-08

## Context

The backend is written in Go and must stay easy to read and debug for years.

## Decision

| Concern | Choice |
|---|---|
| Wiring | Plain constructors in `main`/`internal/app` + `errgroup`; `suture` supervisors for the hub's internal services. Integrations are separate workloads supervised by Kubernetes ([0009](0009-integrations-and-command-delivery.md)) |
| CLI and bootstrap config | cobra + koanf (defaults → file → `HUB_*` environment → flags). Runtime configuration lives in the database, not in config files |
| HTTP API | Kubernetes-style resources served by the hub's extension API server, built on `k8s.io/apiserver` ([0011](0011-api-extension-api-server.md)); its OpenAPI document generates the TypeScript client. The few endpoints outside resources (login, health) use the standard library |
| Payloads | Protocol Buffers on NATS, managed with `buf`; JSON on the API and on the integration interface (MQTT 5), with JSON Schemas generated from the protobuf definitions |
| Database | ent for access, Atlas for versioned migrations: resource specs (behind the API server's storage layer), history, accounts |
| Logs | `log/slog`, JSON in clusters |
| Metrics and traces | OpenTelemetry SDK; Prometheus `/metrics`; OTLP traces when an endpoint is configured; trace context in NATS headers |
| Web UI | React + TypeScript (Vite) in `web/`, embedded into the hub binary |
| CI | GitHub Actions on self-hosted runners; lint and test; amd64 images for now |
| License | AGPL-3.0 |

Code generators that must match a runtime library (`ent`, `protoc-gen-go`) are Go tool dependencies in `go.mod`. Other build tools (`buf`, `golangci-lint`) are tool dependencies of a separate module, `tools/go.mod`, so their dependencies never hold back the hub's. Nothing needs a global install.

The web client is generated with orval, which does not depend on the TypeScript compiler API and so works with TypeScript 7.

Versions: every language, library, tool, base image and chart starts on its latest stable release (Go 1.27, TypeScript 7, Node 26 LTS, Postgres 18). No component is pinned to an older version to work around a tool; the tool is replaced or isolated instead.

## Consequences

- The dependency graph is explicit and visible in one place.
- Generated code (ent, protobuf, OpenAPI client) is committed and checked for drift in CI.
- The bootstrap code still uses Huma (`internal/api`) and an ent schema for Device, Channel and Entity. Both are replaced when the API server and the resource model are implemented.

## Rejected alternatives

- **Huma v2:** dropped when the API became an extension API server ([0011](0011-api-extension-api-server.md)). `k8s.io/apiserver` already provides routing, validation and OpenAPI, plus list, watch and server-side apply, which Huma doesn't.
- **uber/fx**: reflection-based wiring and startup-time errors, and it does not solve the hard lifecycle problem (integrations added and removed at runtime).
- **viper**: heavier, lowercases keys.
- **sqlc + goose**: ent + Atlas preferred.
