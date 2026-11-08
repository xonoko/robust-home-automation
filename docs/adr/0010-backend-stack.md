# 0010. Backend stack and tooling

- Status: Accepted
- Date: 2026-11-08

## Context

The backend is written in Go and must stay easy to read and debug for years.

## Decision

| Concern | Choice |
|---|---|
| Wiring | Plain constructors in `main`/`internal/app` + `errgroup`; `suture` supervisors for integration instances started and stopped at runtime |
| CLI and bootstrap config | cobra + koanf (defaults → file → `HUB_*` environment → flags). Runtime configuration lives in the database, not in config files |
| HTTP API | Huma v2 on the standard library mux; OpenAPI 3.1 generated from code; TypeScript client generated from it |
| Bus payloads | Protocol Buffers, managed with `buf`; JSON at the HTTP API |
| Database | ent for access, Atlas for versioned migrations |
| Logs | `log/slog`, JSON in clusters |
| Metrics and traces | OpenTelemetry SDK; Prometheus `/metrics`; OTLP traces when an endpoint is configured; trace context in NATS headers |
| Web UI | React + TypeScript (Vite) in `web/`, embedded into the hub binary |
| CI | GitHub Actions on self-hosted runners; lint and test; amd64 images for now |
| License | AGPL-3.0 |

Code generators (`buf`, `protoc-gen-go`, `ent`) are pinned as Go tool dependencies in `go.mod`, so `go generate` needs no global installs.

## Consequences

- The dependency graph is explicit and visible in one place.
- Generated code (ent, protobuf, OpenAPI client) is committed and checked for drift in CI.

## Rejected alternatives

- **uber/fx**: reflection-based wiring and startup-time errors, and it does not solve the hard lifecycle problem (integrations added and removed at runtime).
- **viper**: heavier, lowercases keys.
- **sqlc + goose**: ent + Atlas preferred.
