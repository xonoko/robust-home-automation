# 0011. API: Kubernetes-style resources served by the hub

- Status: Accepted
- Date: 2026-11-10

## Context

Configuration (integrations, devices, datapoints, entities, areas, automations…) must be editable from the UI, and also from Git (ArgoCD, Flux) and `kubectl` like any other Kubernetes resource. Resources managed by anything other than the UI must be read-only in the UI, and the UI must say what manages them. The UI needs live updates. Live state (values, freshness, pending commands) changes far too often for etcd.

Three options were compared: our own REST API, CRDs stored in etcd, and an extension API server run by the hub. The resource model itself is described in [the resource model proposal](../design/resource-model.md).

## Decision

**The hub serves its resources as an extension API server,** registered with the Kubernetes API server through an `APIService`, in the group `home.xonoko.com/v1alpha1`.

**Storage:**
- **Spec in Postgres** ([0004](0004-relational-store-postgres-cnpg.md)), relational, with references checked in the write's transaction.
- **Status from NATS** ([0003](0003-message-bus-nats-jetstream.md)): conditions and live state. **Live state is never written to etcd.**
- **Desired values are never in spec,** so GitOps never resets a light. Commands go through a `commands` subresource (like `pods/eviction`), are stored in NATS and shown in status.

**Scope:**
- Resources are **namespaced**, with a single namespace (`home`) accepted for now. A cluster has only one APIService per group and version, hence one hub. Namespaces only matter if one hub ever serves several homes, and changing scope later would be a breaking change.
- The hub's workloads run in `home-system`.
- A ValidatingAdmissionPolicy refuses deleting the `home` namespace unless it carries an explicit annotation.

**Serving:**
- `hub-api` is the hub binary running only its API server, as its own Deployment: 3 replicas, anti-affinity, a PodDisruptionBudget, `maxSurge: 1, maxUnavailable: 0`.
- **Every client connection terminates on the virtual IP** ([0005](0005-virtual-ip.md)): the UI, phones, and `kubectl` through aggregation. The APIService points to a Service with no selector whose endpoint is the virtual IP. The virtual IP holder forwards requests to a ready `hub-api` pod, reopens watches from the last resourceVersion it passed on, and retries writes using their idempotency keys. A `hub-api` restart is therefore invisible to clients.
- Built on `k8s.io/apiserver`, which provides list, watch, server-side apply, managedFields and OpenAPI.

**resourceVersion and watch:**
- Every config change in Postgres is published to a JetStream stream through a transactional outbox.
- resourceVersion is an opaque token carrying that stream's sequence and the NATS KV revision.
- Watch merges both feeds, and resuming from a token works for both.

**Admission** runs in-process, inside the Postgres transaction: schema, references, capability catalog. Kubernetes admission webhooks don't apply to extension API servers.

**Authorization:**
- Requests through aggregation carry Kubernetes identities, checked with SubjectAccessReview.
- Direct requests (UI, phones) use local accounts.
- Both map to the same roles: installer, resident, guest.

**Read-only when managed elsewhere:**
- A field owned by a server-side apply manager of type `Apply` other than `ui`, or carrying Helm, ArgoCD or Flux labels, is read-only in the UI.
- The UI writes with server-side apply as `ui`, without forcing, so the server enforces this rule.
- `status.managedBy` names the owner.

**Remediation never depends on our API:**
- The hub's chart and ArgoCD Application contain no `home.xonoko.com` resources.
- The hub registers its own APIService, and only once it is ready.
- `hub-api` never crashes on stored data, and has a safe mode (storage and API only).
- Its readiness doesn't depend on Postgres or NATS. Reads are served from cache; writes return a retryable error while Postgres is unavailable.
- Last resorts: `kubectl delete apiservice v1alpha1.home.xonoko.com` restores complete discovery, and `psql` into the CNPG primary fixes stored data.
- A "hub API fully down" test checks that `kubectl rollout undo`, `helm upgrade`, `helm rollback` and an ArgoCD sync of the hub still work while the APIService is unavailable.

## Consequences

- One API for the UI, `kubectl`, k9s, ArgoCD and Flux. The UI uses list and watch, so live updates come built in. Its TypeScript client is generated from the OpenAPI document the API server publishes.
- **Huma is dropped** ([0010](0010-backend-stack.md)): `k8s.io/apiserver` already provides routing, validation and OpenAPI, plus list and watch.
- We depend on `k8s.io/apiserver` internals (its storage interface changes between releases) and follow every Kubernetes release.
- While no `hub-api` pod is ready, namespace deletion blocks cluster-wide. Some tools may also refuse to run because API discovery is incomplete (older ArgoCD versions did). The remediation rules above handle this.
- Installing the hub needs cluster-admin (APIService, auth delegation).
- Prototypes needed before implementation: the resourceVersion token and resuming watches across the two feeds; migrating TLS connections (all API clients use HTTPS) with kernel TLS and `TCP_REPAIR`.

## Rejected alternatives

- **Our own REST API (Huma):** no `kubectl`, no GitOps tooling, no field ownership. Live updates and the read-only rule would have to be invented.
- **CRDs stored in etcd:** live state can't go in status (too many etcd writes), so it would need a second resource served elsewhere. No transactions across objects, conversion webhooks for schema changes, and config split between etcd and Postgres. `kubectl` would also read etcd directly, so its connections would never go through the virtual IP.
- **CRDs plus a read-only extension group for live state:** two resources per entity for the same thing, and the same discovery risk on a smaller scale.
