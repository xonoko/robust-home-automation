# 0009. Integrations, failover profiles and command delivery

- Status: Accepted
- Date: 2026-11-08

## Context

Most target installations are KNX with an IP gateway (typically an MDT SCN-IP100.03: KNX IP router, 4 tunnels, KNX IP Secure). Other protocols have very different failover properties, and some (Zigbee) cannot be made lossless at a reasonable cost.

## Decision

**Integration model: one model, workloads managed by the hub.**
- **An integration is a container image,** possibly with several containers in one pod (Zigbee2MQTT plus our adapter). The hub runs it as a Kubernetes operator from an `Integration` resource ([resource model](../design/resource-model.md#41-integration-decided)).
- **Any language, any library.** Heavy third-party programs (Zigbee2MQTT, Z-Wave JS, a Matter controller) run inside the integration's pod.
- **Built-in integrations ship in the hub's image,** one entry point each (`hub integration knx`). Third-party ones are their own images.
- **Isolation:** third-party integrations run under the `restricted` Pod Security level with a NetworkPolicy by default. Host networking, devices, extra capabilities and ports on the virtual IP must be declared in `permissions` and approved.

**The interface: MQTT 5 with JSON payloads**, on the hub's MQTT server, under `hub/v1/integrations/<instance>/`. It's simple enough to use without our SDK, from any language with an MQTT 5 client.

| Topic | Direction | Content |
|---|---|---|
| `config` (retained) | Hub → integration | Configuration |
| `role` (retained) | Hub → integration | `standby`, `prepare` (connect to the devices now), `active` with a fencing token, `stop` (finish in-flight work, then confirm) |
| `describe` (retained) | Integration → hub | Configuration schema (JSON Schema), failover profile, functions, behaviour of events during handover |
| `commands` → `commands/results` | Both | A command with its ID, deadline and token → `confirmed`, `failed` or `expired` |
| `state` | Integration → hub | Values per datapoint or device capability, observation time, `late` flag |
| `events` | Integration → hub | Button presses and other events with no state |
| `discovered` | Integration → hub | Device suggestions |
| `functions/<name>` | Hub → integration, reply on the MQTT 5 response topic | Calls from automations |
| `health` | Integration → hub | Health; the MQTT last will reports a disconnection |

- **The hub manages roles and fencing:** it holds the leases ([0006](0006-leases-and-fencing.md)), decides which instance is active, sends commands only to it, and ignores results from any other instance.
- **Commands use QoS 1 with persistent sessions** stored in JetStream, so a command sent while an integration restarts waits for it.
- **JSON Schemas** for the payloads are published, generated from the hub's protobuf definitions; NATS keeps protobuf internally.
- **MQTT 5 is required** (response topics, user properties, session expiry, reason codes).
- **The SDK is optional** (Go first). It covers the interface, make-before-break helpers, recovering state after a reconnect, and connection migration with the virtual IP ([0005](0005-virtual-ip.md)) for integrations that want it.

**Every integration declares a failover profile**, which decides its workload and how the maintenance controller moves it:

| Profile | Meaning | Workload | Examples |
|---|---|---|---|
| VIP migration | The device keeps a long-lived connection to the integration, which migrates it through the SDK | On the virtual IP's node | Shelly outbound WebSocket, OCPP, single-client Modbus devices |
| Multiple listeners | Several nodes receive the same traffic; duplicates are removed | DaemonSet | KNX routing or one tunnel per node, ONVIF events, EnOcean with two gateways, BLE proxies |
| Lease | One active instance; make-before-break on planned operations when the device accepts several clients, clean handover otherwise | Deployment with warm standbys | ESPHome, Hue, Modbus, SIP (planned moves wait for calls to end), outgoing KNX commands |
| Disruption-aware | A short gap is accepted and handled by the command model | Deployment | Zigbee2MQTT, Z-Wave JS, cloud APIs |

MQTT devices (Tasmota, Shelly over MQTT, ESPHome over MQTT) connect to the hub's MQTT server, which the hub core always migrates. The integrations handling them consume their topics, granted by ACL, so they need no migration of their own.

**Events during a handover.** Re-reading state can't recover a button press, which is an event. Each integration declares `eventsDuringHandover: preserved`, or `lost` with an expected gap, and the UI and `Installation.status.guarantees` show it.

**KNX.** KNX devices work without the hub. Prefer KNX IP routing (multicast, no tunnel limit), with KNX IP Secure; fall back to tunnels from two nodes. Request a specific tunnel (MDT routers can shift tunnels, which breaks Data Secure reception). A second KNX IP interface on the line removes the gateway as a single point of failure.

**KNX and ETS** (details in [the resource model proposal](../design/resource-model.md#46-knx-and-ets)):
- **The ETS project is the source of truth, and the hub never reprograms devices.** Datapoints, entities, areas and devices are imported from the `.knxproj` file, and the keyring is imported for Secure. A re-import shows a diff.
- **What the hub changes directly:** values exposed as group objects, and scenes taught to actuators through the scene "learn" bit. Logic lives in Automations.
- **What the ETS project must contain for the hub:**
  - a placeholder device for the hub, linked to every group address the hub must receive, so the KNX IP router's filter tables forward them (the import warns when one would be filtered);
  - descriptions of the hub's group addresses, from the CSV file the hub generates;
  - for Data Secure, the hub's address as a sender.
- **Buttons:** three documented patterns:
  - **direct link;**
  - **decided by the hub;**
  - **decided by the hub with a KNX default:** a logic block in a KNX device runs the default after 200–300 ms unless the hub's Automation writes a claim group address first.

  Only the third keeps a button working when the whole hub is down. It ships as an Automation template, together with a self-test template.

**Zigbee.** An Ethernet adapter driven by one Zigbee2MQTT, running in the zigbee2mqtt integration's pod next to our adapter container (lease-gated, warm standbys, data synced to JetStream). A node failure or drain causes a disruption of a few seconds, and button presses in that window can be lost (`eventsDuringHandover: lost`). We do not try to make Zigbee failure-proof: essential devices should use more robust protocols (KNX, EnOcean with two gateways, ESPHome).

**Command delivery** (all integrations):
- The hub tracks a **desired state**, not a stream of commands. State commands are coalesced to the latest value per attribute and reconciled against the reported state when the integration comes back.
- **Momentary** commands (chime, scene) have a short TTL. **Toggles** are resolved to an absolute state when issued, so retries never flip twice.
- A command is done when the device **reports** the new state; until then the hub retries with backoff until the command's deadline. Every command has a deadline.
- Before a planned move, the maintenance controller announces it; the hub stops sending to that integration, waits for in-flight commands, and only then stops it.

**Missed events.** After a reconnect, the hub reads the current state, compares it with the last known state and emits recovered changes marked `late`. Automation triggers choose `run`, `skip` or `run_if_within(<duration>)` for late events.

**Callbacks.** Any protocol where a device calls us back (OCPP, UPnP events, webhooks, Shelly WebSocket) is given the VIP, never a node address. The integration receiving them declares its ports in `permissions.listen`; the hub refuses conflicts.

## Consequences

- An integration implements a small contract over MQTT 5: obey `role`, apply `config`, execute commands and report results, publish state, events and discovery, describe itself. Leases, fencing, retries and command reconciliation are the hub's job.
- Each integration runs in its own pod: memory per integration (Go ~20 MB, Python ~50–100 MB, Node ~100 MB), about 1–2 GB for a typical home. Fine on 8 GB nodes, tight on 4 GB.
- Third-party integrations need multi-architecture images, and homes without internet need images loaded in advance (k3s registry mirrors).
- The UI shows integration availability, pending commands and how events behave during handover.

## Rejected alternatives

- **In-process Go integrations:** an unrecovered panic, a deadlock or a CPU hog in one integration affects the whole hub, including the virtual IP holder. Integrations could only be written in Go.
- **WebAssembly integrations run by the hub:** their main advantage was generic connection migration through memory snapshots, which most integrations don't need. Integrations needing native libraries, threads or hardware access would still have needed separate programs: two mechanisms instead of one.
- **Interface over NATS:** it would expose our internal bus, and durable delivery needs JetStream consumers and acknowledgements, which are heavier for third parties.
- **Interface over HTTP:** commands, configuration and roles go from the hub to the integration, so it would need streaming (SSE, WebSocket) or a server in every integration, plus our own acknowledgements and retries.
- **Interface over gRPC:** needs code generation from `.proto` files, which is less simple without an SDK.
- **MQTT 3.1.1:** no response topics, user properties, session expiry or reason codes.
- **Making Zigbee lossless**, re-examined on 2026-11-09 and 2026-11-10 and rejected again: essential devices should use more robust protocols.
  - **A relay with a TI coordinator:** herdsman attaches to a running TI coordinator without resetting it, but frames during startup and the Zigbee2MQTT state files remain open questions.
  - **A herdsman change for Silicon Labs coordinators:** they are reset at every start, and no option skips it. The change would be large and its acceptance upstream uncertain; the maintainer considers a cold standby sufficient.
  - **A Go Zigbee stack:** a core of ≈70,000 lines of TypeScript to port, while device support is 265,000 lines for 4,798 models.
  - **CRIU and KubeVirt:** see [0005](0005-virtual-ip.md).
  - **Earlier ideas:** a radio relay, a spare coordinator with the same identity.
