# Resource model: proposal

- Status: Proposal, 2026-11-09, updated 2026-11-10. Choices marked **Decided** in [section 7](#7-choices-to-make) were agreed. This becomes ADR 0008 once the others are made.
- API group: `home.xonoko.com/v1alpha1`, served by the hub as an extension API server ([ADR 0011](../adr/0011-api-extension-api-server.md)): spec in Postgres, status from NATS. Live state is never stored in etcd.

## 1. What the model must support

Already decided:
- **Desired vs reported state, with deadlines.** A command is done when the device reports the new state ([0009](../adr/0009-integrations-and-command-delivery.md)). Toggles and relative steps are turned into an absolute value when the command is issued.
- **Late events.** A change recovered after a disruption is marked `late`; each trigger chooses whether to run, skip, or run only within a time window.
- **Failover profiles** per integration: VIP migration, multiple listeners, lease, disruption-aware.
- **Every connection terminates on the virtual IP.** Service mode works without host networking, with reduced features ([0005](../adr/0005-virtual-ip.md)).
- **Read-only when managed elsewhere.** A field owned by something other than the UI (ArgoCD, Helm, Flux, an import, an integration) is read-only in the UI, and the UI names the owner.
- **Desired values are never in spec.** They go through a `commands` subresource.

From the discussion:
- Swapping hardware keeps entities, automations and history.
- KNX first, configured mostly from ETS.
- Zigbee is disruption-aware.
- Cameras (streams, PTZ, detection events, recording) and network devices (switch ports, PoE, Wi-Fi networks, connected clients).
- GitOps-friendly: the config of a whole house can live in a Git repo.

## 2. State of the art

| System | Physical side | Functional side | Desired vs reported | Worth borrowing | Weaknesses |
|---|---|---|---|---|---|
| **Home Assistant** | Integration → config entry → Device (with a `via_device` pointing to its hub) | Entities in domains (`light`, `cover`, `climate`…) with a string state, attributes and a `supported_features` bitmask. Areas, floors, labels. | No: actions are fire-and-forget, and optimistic state depends on the integration | Rich domains; **automation traces**; **blueprints** (reusable automations with inputs); helpers (`input_boolean`, timers, counters); run modes (`single`, `restart`, `queued`, `parallel`) | Wiring hidden in integration code; replacing a device means new entities; `unavailable` replaces the value; Jinja templates are untyped and fail at runtime |
| **openHAB** | Binding → Bridge → Thing → Channel (state or trigger) | Items (primitive types with units), Groups with aggregation, semantic tags (Location / Equipment / Point) | Command and update are separate events; `autoupdate` guesses | **Links with profiles** (transforms between channel and item); trigger channels; separation of the two sides | Four concepts for one light; Items too primitive; semantic model added later |
| **ioBroker** | adapter.instance → device → channel → state | States with a *role* | **`ack` flag**: `ack=false` is a command, `ack=true` the confirmed state | The command / confirmation distinction is built into every state | Weak typing; the tree mixes the physical and functional sides |
| **Matter** | Node → Endpoint → Cluster (attributes, commands, events); bridged devices behind an aggregator | Device types (required and optional clusters per endpoint). Recent versions add cameras. | Attributes are reported; commands are separate | **Standard, precise capability semantics**: OnOff, LevelControl, ColorControl, WindowCovering, Thermostat, measurement clusters, Switch events | No composition across devices; nothing above the device |
| **HomeKit** | Accessory (bridges) | Service (Lightbulb) → Characteristics (On, Brightness) | Characteristics are read and written | Typed characteristics; scenes; secure video with analysis on the home hub | Closed; no physical-side model |
| **SmartThings / Hubitat** | Device → Components | Capabilities (attributes + commands) from a public catalog; location modes | Attributes reported, commands separate | **Public capability catalog**; **modes** (Home, Away, Night) as first-class objects | Cloud-centred (SmartThings); weak cross-device composition |
| **Google Home, Alexa** | — | Device types + traits / interfaces | — | Our types and capabilities must map onto them cleanly (voice and ecosystem export) | — |
| **Homey** | App → driver → device | Device class + capabilities; Flows (when / and / then) | — | Simple automation model that non-technical users understand | Proprietary; no physical layer |
| **KNX / ETS** | Group objects of devices, group addresses with DPT | ETS6 *functions* (light, blind) group the group addresses; KNX IoT adds a semantic API | Write vs status group addresses; read requests | **The ETS project is the source of truth**: import, don't retype | Group addresses don't belong to one device |
| **Loxone** | Extensions, Tree, Air | Function blocks wired graphically | Block outputs | Reliable building blocks (lighting controller, shading, intelligent room controller) | Proprietary; config only offline |
| **Project Haystack / Brick** | Equipment + points | Tags and ontology for buildings | — | Tags for Equipment, Point and Location | Building management focus, heavy |
| **Frigate, Scrypted** | Cameras, streams (go2rtc) | Detections and review items; recording modes with retention per mode | — | **Recording modes** (continuous, motion, objects) with retention per mode; detection on the hub with an accelerator | Separate products |
| **UniFi, Omada** | Devices, ports, port profiles | Clients, networks | Desired port profile vs observed | Ports and Wi-Fi networks as controllable things; clients as observed objects | Vendor-specific |

**What we take from them:**
- Matter's capability semantics, so a Matter bridge and voice assistants map cleanly.
- openHAB's links with transforms.
- ioBroker's command/confirmation distinction, generalised into desired, reported and freshness.
- Home Assistant's traces, blueprints, helpers and run modes.
- Hubitat's modes.
- ETS as the source of truth for KNX.
- Frigate's recording modes.
- UniFi's split between controllable ports and networks and observed clients.
- From outside home automation: Temporal's durable execution by deterministic replay (5.7), and the Kubernetes controller pattern for rules: desired state computed from current state, then reconciled (5.7).

## 3. Conventions shared by every resource

**Scope (decided).** Resources are **namespaced**, but the hub accepts a **single namespace for now**: `home` by default.
- **Why namespaced:** a cluster has only one APIService per group and version, so there is one hub per cluster whatever the scope. Namespaces therefore only matter if one hub ever serves several homes (an apartment building, a house with a rented-out studio). Changing scope later would be a breaking API change, so the door stays open now by lifting the single-namespace restriction later.
- **The hub's workloads run in `home-system`; the house config lives in `home`.** Uninstalling the hub never touches the config.
- **Protection against `kubectl delete namespace home`,** which would delete the whole house config: a ValidatingAdmissionPolicy refuses deleting that namespace unless it carries an explicit annotation. Postgres backups remain the last resort.
- **Convenience:** the kubeconfig the hub generates sets `home` as the default namespace, so `kubectl get entities` works without `-n`.
- **Secrets** are referenced in the same namespace.
- **References across namespaces are forbidden.** Equipment shared between units of a building will need its own design if multi-home is ever enabled.

Rejected: cluster-scoped. It's simpler, but closes multi-home for good.

**Identity and references.**
- `metadata.name` is a stable, immutable ID. The UI generates it from the display name when the object is created.
- `spec.displayName` is what people see, and can be renamed freely.
- References use names within the namespace, which keeps Git repos readable.

| Benefits | Limitations |
|---|---|
| Renaming never breaks references (Home Assistant's `entity_id` rename problem disappears); diffs in Git are readable | An object name chosen at creation stays forever, even if it no longer matches the display name |

**Spec and status.**
- `spec` is configuration only.
- `status.conditions` follow Kubernetes conventions (`Ready`, `Available`, `Bound`, `Valid`, `Degraded`), with `observedGeneration`.
- Live state appears in `status`, served from NATS; it never passes through Postgres.

**Read-only management.**
- A field owned by a server-side apply manager of type `Apply` other than `ui`, or carrying Helm, ArgoCD or Flux labels, is read-only in the UI.
- The UI writes with server-side apply as `ui`, without forcing, so the server enforces this rule itself.
- `status.managedBy` (computed) names the owner: `ArgoCD application house-config`, `KNX import ets-2026`, `integration zigbee2mqtt`.

**Live state record**, kept per capability attribute:

```yaml
level:
  desired:  {value: 60, at: …, by: automation/evening, deadline: …}   # writable attributes only
  reported: {value: 40, at: …, source: datapoint/ga-1-1-2}
  freshness: live          # live | late | stale | unavailable
```

**Freshness:**

| Value | Meaning |
|---|---|
| `live` | Confirmed recently |
| `late` | Recovered after a disruption |
| `stale` | Not reported within `staleAfter` |
| `unavailable` | Source unreachable; the last value is kept |

**Commands** are sent with `POST …/<resource>/<name>/commands`. They work on entities, groups and scenes alike:

```yaml
capability: level
action: set          # set | toggle | step | stop | or a capability-specific action (ptz.goToPreset…)
value: 60
transition: 2s       # optional, when the capability supports it
deadline: 5s         # default per capability
```
- Toggles and steps are resolved to an absolute value at issue time, so retries never flip twice.
- The response carries a command ID; its result (`Confirmed`, `Expired`, `Superseded`, `Rejected`) shows in status and history.

**No embedded expression language.**
- Transforms and conditions are structured: invert, scale, map; state equals, above or below, for a duration, time window, sun, presence, mode.
- Anything beyond that is a function in an `Automation` (5.7), written in any language.
- CEL is used only inside the API's own schema validation, where it's the Kubernetes standard, and is never exposed to users.

| Benefits | Limitations |
|---|---|
| No extra language to learn beyond the one you already write automations in; custom logic gets the same build, tests and determinism as automations; structured transforms and conditions are checked when saving and shown by the visual editor | A one-line custom transform needs an Automation to hold it; an unusual condition means writing the automation in a real language instead of the declarative one |

An expression language (a small language written as strings inside fields, like Home Assistant's Jinja templates) was rejected:
- it would be a second, fixed language, contradicting "any language";
- its errors appear only at runtime;
- Home Assistant shows how confusing it becomes once it's everywhere.

**Rule that keeps the declarative language small:** it covers what the visual editor can build. Gaps are filled by writing the automation in Go, TypeScript or another language, not by adding conditions until the YAML becomes an expression language.

## 4. The physical side

### 4.1 Integration (decided)

A configured connection to a protocol or a system. Details and rationale are in [ADR 0009](../adr/0009-integrations-and-command-delivery.md).

**An integration is a workload that the hub runs as a Kubernetes operator.** It is a container image, possibly with several containers in one pod (Zigbee2MQTT plus our adapter, for instance). It talks to the hub through a simple interface: **MQTT 5 with JSON payloads**. That interface can be used directly from any language, without our SDK.

```yaml
apiVersion: home.xonoko.com/v1alpha1
kind: Integration
metadata: {name: knx-main, namespace: home}
spec:
  image: {builtin: knx}                 # or "ghcr.io/vendor/integration-x@sha256:…"
  config:                               # validated against the schema the integration declares
    connection: {preferred: routing, fallback: tunnelling, tunnel: 2}
    gateway: {device: knx-ip-router}
  secretRef: {name: knx-keys}
  permissions:                          # required for third-party integrations, approved on install
    hostNetwork: true                   # KNX multicast; refused in service mode
  placement: {}                         # optional: nodes allowed to run it (a USB radio, for instance)
status:
  describe: {version: 1.4.0, failoverProfile: MultipleListeners, eventsDuringHandover: preserved}
  workload: {kind: DaemonSet, ready: 3}
  active: {pod: knx-main-x7k2p, node: node-2, fencingToken: 18342}
  effectiveMode: tunnelling             # e.g. routing is unavailable in service mode
  limitations: ["KNX routing needs host networking: using tunnelling"]
  discovered: [...]
  conditions: [{type: Connected, status: "True"}]
```

- **The integration describes itself** on its `describe` topic: configuration schema (JSON Schema), failover profile, functions, and how events behave during a handover. The API validates `spec.config` against that schema, and the UI generates its form from it.
- **The hub creates the workload matching the failover profile:**

  | Profile | Workload |
  |---|---|
  | Multiple listeners | DaemonSet, one pod per node |
  | Lease | Deployment: one active pod plus warm standbys |
  | VIP migration | On the virtual IP's node, migrating its connections through the SDK |
  | Disruption-aware | Deployment, restarted elsewhere |

  Upgrades are rolling and make-before-break: the new pod is ready before the active role moves.
- **The hub manages roles and fencing,** not the integration. It decides which instance is active, sends commands only to that one, and ignores results from any other.
- **Built-in integrations ship in the hub's image,** one entry point each (`hub integration knx`). Third-party ones are their own images.
- **Permissions:** third-party integrations run under the `restricted` Pod Security level with a NetworkPolicy by default. Host networking, devices, extra capabilities and ports on the virtual IP (`listen`) must be declared and approved.
- **Events during a handover:** each integration declares `preserved`, or `lost` with an expected gap (Zigbee: a few seconds). The UI and `Installation.status.guarantees` show it.
- `status.limitations` makes service-mode reductions visible for each integration.

**The interface,** under `hub/v1/integrations/<instance>/`:

| Topic | Direction | Content |
|---|---|---|
| `config` (retained) | Hub → integration | Configuration |
| `role` (retained) | Hub → integration | `standby`, `prepare` (connect to the devices now), `active` with a fencing token, `stop` (finish in-flight work, then confirm) |
| `describe` (retained) | Integration → hub | Schema, failover profile, functions, behaviour of events during handover |
| `commands` → `commands/results` | Both | A command with its ID, deadline and token → `confirmed`, `failed` or `expired` |
| `state` | Integration → hub | Values per datapoint or device capability, observation time, `late` flag |
| `events` | Integration → hub | Button presses and other events with no state |
| `discovered` | Integration → hub | Suggestions for `status.discovered` |
| `functions/<name>` | Hub → integration, reply on the MQTT 5 response topic | Calls from automations |
| `health` | Integration → hub | Health; the MQTT last will reports a disconnection |

- **MQTT 5 is required.** Response topics, user properties, session expiry and reason codes don't exist in 3.1.1.
- **Commands use QoS 1 with persistent sessions,** so a command sent while an integration restarts waits for it.
- **Payloads are JSON,** described by published JSON Schemas generated from the hub's protobuf definitions.
- **The optional SDK (Go first)** covers the interface, make-before-break helpers, recovering state after a reconnect, and connection migration with the virtual IP for integrations that want it.

### 4.2 Device

A physical device. Creating a Device is how a discovered device is adopted (see 4.4).

```yaml
kind: Device
metadata: {name: cam-garden}
spec:
  integration: dahua-cameras
  ref: "9F0A2B…"                  # identifier inside the integration (serial number, IEEE address…)
  displayName: Garden camera
  area: garden
  via: null                       # gateway device: KNX IP router, Zigbee adapter, NVR, switch
status:
  manufacturer: Dahua   model: SD49425   firmware: "2.840…"   serial: …
  network: {mac: …, ip: 192.168.1.42, uplink: {device: switch-office, port: 5}}
  offers: [videoStream, snapshot, ptz, motion, objectDetection, firmware]   # API integrations only
  conditions: [{type: Reachable, status: "True"}]
```

**Device replacement:** `POST …/devices/<old>/replace {with: <new>}`.
- Every entity binding pointing at the old device or its datapoints is moved to the new one in one transaction.
- The old device is then deleted.
- Entities, automations and history are untouched.

| Benefits | Limitations |
|---|---|
| Swapping a broken actuator or camera takes one operation | Only works when the new device offers the same capabilities; otherwise the UI lists the bindings to fix |

### 4.3 Datapoint

One value on the wire, for point-oriented protocols: KNX, Modbus, BACnet, Zigbee2MQTT attributes, MQTT topics of generic devices.

```yaml
kind: Datapoint
metadata: {name: ga-1-1-2}
spec:
  integration: knx-main
  device: kitchen-actuator        # optional: KNX group addresses don't need one
  access: read                    # read | write | readWrite | event
  knx: {groupAddress: "1/1/2", dpt: "5.001", readOnStart: true}
  staleAfter: null                # e.g. 15m for a sensor that reports periodically
status:
  conditions: [{type: Valid, status: "True"}]
```

**Choice: what the physical side looks like for API-based integrations** (ONVIF, Dahua, UniFi, Shelly, cloud APIs):

| Option | Benefits | Limitations |
|---|---|---|
| **Two binding styles** (recommended): point protocols bind entities to Datapoints; API integrations bind to a capability the Device offers | Each protocol uses its natural model; no object explosion for API devices; PTZ or a video stream are not "values" anyway | Two binding styles to document and support in the UI |
| Datapoints for everything: API integrations create one Datapoint per attribute | One generic binding mechanism; transforms work everywhere | Thousands of objects; meaningless for streams, PTZ and structured data |
| Matter-like endpoints: Device → Endpoint → capability, no Datapoint | Closest to Matter | Doesn't fit KNX, where group addresses are shared and don't belong to a device |

### 4.4 Discovery (decided)

**The hub suggests; nothing becomes config until it is created as a resource.** There is no discovery resource.

Discovered devices appear in the **status of the integration** that found them, served from NATS like any status. Only devices that have no `Device` yet and aren't ignored are listed. The UI watches the Integration, so the list updates live.

```yaml
kind: Integration
metadata: {name: zigbee2mqtt}
status:
  discovered:
  - ref: "0x00158d0001a2b3c4"
    suggested: {displayName: "Aqara door sensor", manufacturer: Aqara, model: MCCGQ11LM}
    firstSeen: …   lastSeen: …
```

- **Adopting a device is creating a `Device`** with `spec.integration` and `spec.ref`: an ordinary create, from the UI, `kubectl` or Git. Once it exists, the entry leaves `status.discovered`.
- **Suggested entities** appear in the new Device's `status.suggestedEntities`. The UI offers to create them as ordinary `Entity` resources.
- **Ignoring a device** adds its ref to `Integration.spec.discovery.ignore`. That's config, so it's versioned in Git and covered by the read-only rule like everything else.
- **Pre-provisioning** works naturally: a Device created from Git before the hardware is installed (say, with a known Zigbee address) shows `Reachable: False` until the integration sees it.

| Benefits | Limitations |
|---|---|
| No extra kind; adoption is the standard create; devices that come and go never reach Postgres or Git | Discovery data travels in every Integration watch event; fine for tens or hundreds of devices, not for thousands (no integration discovers that many) |

Rejected:
- a separate read-only `DiscoveredDevice` resource with an `adopt` subresource: an extra kind for something that's only a suggestion;
- `Device` with `adopted: false`: unadopted devices end up in Postgres and Git.

### 4.5 Import

An `Import` turns an external source of truth into resources, and owns everything it creates. The read-only rule then applies automatically: the UI shows those objects as "Imported from …", and changes are made in the source and imported again. The first source is the KNX ETS project (4.6). The same kind can later cover others: a Home Assistant export, a Loxone config, a CSV of Modbus registers.

```yaml
kind: Import
metadata: {name: ets-2026}
spec:
  type: knxProject
  integration: knx-main
  source: {upload: knxproj-2026-11-09}      # files uploaded through the API
  keyring: {upload: knxkeys-2026-11-09}     # optional: KNX IP Secure and Data Secure keys
  secretRef: {name: ets-project-password}   # project and keyring passwords
  createEntities: fromFunctions             # none | fromFunctions
status:
  lastImport: …
  created: {datapoints: 1840, entities: 212, areas: 24, devices: 96}
  diff: {added: 12, changed: 3, removed: 1}
  warnings:
  - "1/7/1 (Hallway button 1 request) has no receiver on the IP side: the KNX IP router will filter it (see 4.6)"
```

### 4.6 KNX and ETS

**ETS** (Engineering Tool Software, from the KNX Association) is the tool every KNX installation is designed and programmed with. The installer's **ETS project** (a `.knxproj` file: a zip of XML files, often password-protected) contains:
- the topology and each device's individual address;
- every device's application, parameters and associations;
- the **group addresses**, with their names and **DPT** (datapoint type);
- the building structure (floors, rooms);
- since ETS6, *functions*, which group the group addresses of one light, blind or heating circuit.

The security keys are exported separately, as a keyring (`.knxkeys`).

**The ETS project is the source of truth for the KNX installation, and the hub never reprograms devices.**

#### What importing creates

| From ETS | Becomes |
|---|---|
| Group addresses, names, DPTs | `Datapoint` resources |
| Associations (which object sends status) | Binding roles in Entities: `command` and `status` chosen automatically |
| Functions | `Entity` resources with their bindings |
| Building structure | `Area` resources |
| Devices | `Device` resources, for inventory and diagnostics |
| Keyring | Secrets for KNX IP Secure and Data Secure |

**Re-importing** after the installer changes the project shows a diff:
- renamed group addresses update;
- new ones appear;
- a deleted one leaves its entity with `Bound: False`, so broken bindings are visible.

Home Assistant's KNX integration imports ETS projects the same way (Python library `xknxproject`), which shows the format is tractable. Our parser is written in Go.

**Why import is needed in practice:**
- KNX telegrams don't carry their type: a byte on `1/1/2` could be a percentage, a scene number or a counter. Only the DPT in the project says which.
- Data Secure group addresses can't be read or written at all without the keyring.
- Typing thousands of group addresses by hand is slow and error-prone.

**Other ways to get datapoints:**
- **Manual entry,** for small installations and one-off additions.
- **Suggestions from bus monitoring.** Group addresses seen on the bus appear in `Integration.status.discovered`. The size of the value narrows the DPT (1 bit, 1 byte, 2 bytes…), but a person chooses. Data Secure traffic stays unreadable.

#### What the hub can change without ETS

| What | From the hub? | Why |
|---|---|---|
| Device configuration: parameters, associations, logic built into actuators or logic modules | **No** | Programming needs the manufacturer's product data, which is encrypted for ETS, and ETS's download protocol. Devices would also no longer match the ETS project, and the installer's next download would silently overwrite the change. Circumventing ETS's protection is excluded, for legal reasons (licence, anti-circumvention law) as well as practical ones. |
| Values exposed as group objects: setpoints, lock objects, timer durations, thresholds | **Yes** | They're ordinary datapoints |
| Scenes stored in actuators | **Yes** | KNX scene control (DPT 18.001) has a "learn" bit. The hub sets the states, then sends "learn scene n", and the actuators store them. A `Scene` with a KNX native binding is edited in the UI, taught to the devices, and then runs without the hub. |
| New logic | **Yes**, as Automations | Replaces logic modules and KNX servers, and combines KNX with other protocols |
| New group addresses for the hub's own use | **Partly** | The hub writes to any group address, but linking devices to a new one is done in ETS. The hub exports the group addresses it needs as a CSV file for ETS, with explicit names and descriptions. |

**Principle:** basic functions stay in the KNX devices, so lights, blinds and heating work when the hub is down ([0009](../adr/0009-integrations-and-command-delivery.md)). The hub adds comfort and cross-protocol logic.

**To investigate:** an ETS App (the KNX Association's official way to extend ETS) could create the hub's group addresses, placeholder device and links directly in the project, instead of the installer importing a CSV file. It requires KNX membership and their SDK; the current terms haven't been checked.

#### What the ETS project must contain for the hub

The hub isn't a programmed KNX device. Like visualisation servers (Gira HomeServer, X1), it listens and sends on group addresses through the KNX IP router. Three things must be handled in ETS:

1. **Filter tables.** KNX IP routers and line couplers only forward a group telegram to another line if a device on that line receives it *in the project*. A group address read only by the hub has no such receiver, so the router filters it and the hub never sees it. This always applies with KNX IP routing. Telegrams the hub sends to actuators aren't affected, since the actuators are receivers.
   - **Fix (recommended):** a placeholder device for the hub on the IP side, linked to every group address the hub must receive. Several manufacturers publish placeholder products for this. It also documents in the project what the hub listens to.
   - **Alternative:** the router forwards all group telegrams for the relevant main groups. Simpler, but it adds traffic between lines and hides the dependency.
   - **The import warns** when a group address the hub uses would be filtered: the project contains the router's parameters and every association.
2. **Documentation.** The CSV file the hub generates names and describes its group addresses ("Hallway button 1: request, decided by the hub"), so another installer understands the project.
3. **Data Secure, if used.** Receiving works with the keyring. For sending, each receiving device only accepts known senders, so the hub's individual address must be in the project as a sender for those group addresses. ETS6 does this by assigning secure group addresses to a tunnel of the KNX IP interface. How this combines with routing must be verified on hardware (see section 8, prototypes).

#### Button patterns

There's nothing button-specific in the model: a button is an Entity with the `button` capability, whose `event` binding is the group address it sends to. How a press is handled is a choice made in ETS plus an ordinary Automation. Three patterns are documented, two of them shipped as `AutomationTemplate`s:

| Pattern | ETS | Hub | Hub completely down | Node crash |
|---|---|---|---|---|
| **Direct** | Button linked to the actuator | An Automation can react to the press or to the actuator's status, after KNX has acted | ✅ KNX acts | ✅ KNX acts |
| **Decided by the hub** | Button sends to a request group address that no actuator listens to | An Automation decides what to do | ❌ The button does nothing | ⚠️ Handled 2–3 s late, marked `late` (the engine guarantee in 5.7) |
| **Decided by the hub, KNX default** (template) | Request group address + a logic block in a KNX device: delay of 200–300 ms, then the default command, unless a claim telegram arrives first | The Automation **first writes the claim group address**, then decides. The claim is an ordinary Entity (`type: generic`, command-only binding). The template skips the claim if the press is already too old to claim in time. | ✅ The KNX default runs after the delay | ✅ The KNX default runs after the delay |

Timing of the claim:

| Step | Typical |
|---|---|
| Press on the bus (KNX TP, 9,600 bit/s) | 20–40 ms |
| KNX IP router → hub | a few ms |
| Hub receives → claim sent | < 10 ms |
| Claim on the bus | 20–40 ms |
| **Claim round trip** | **≈ 50–90 ms**, so a 200–300 ms delay in the logic block leaves a margin under bus load |

- **Double actions** remain possible only when a claim is delayed on a congested bus. Push buttons usually send absolute values (tracked from the actuator's status) rather than toggles, so a double action is harmless when both sides send the same state.
- **Limits:**
  - each button function needs a logic block with a delay and a gate, and logic capacity per device varies by manufacturer;
  - delay resolution depends on the device;
  - existing installations must be re-engineered in ETS for the buttons that use this pattern.
- **Self-test** (second template): for one press of a quiet button, the Automation doesn't claim, then checks that the actuator's status changed through the KNX default. A broken fallback (logic block reprogrammed, group address changed) shows up as a failed run, instead of during an outage.

## 5. The functional side

### 5.1 Entity

What users, the UI, voice assistants and automations work with.
- **The type** says what the entity is (light, cover, climate, camera…). It decides the UI, voice export and mapping to Matter device types.
- **The capabilities** say what it can do.

```yaml
kind: Entity
metadata: {name: kitchen-ceiling}
spec:
  displayName: Kitchen ceiling
  type: light
  area: kitchen
  capabilities:
    onOff:
      command: {datapoint: ga-1-0-1}
      status:  {datapoint: ga-1-0-2}
    level:
      command: {datapoint: ga-1-1-1}
      status:  {datapoint: ga-1-1-2}
      step:    {datapoint: ga-1-1-3}          # KNX relative dimming (DPT 3.007), optional
    colorTemperature:
      command: {datapoint: ga-1-2-1}
      status:  {datapoint: ga-1-2-2}
      range: {minKelvin: 2700, maxKelvin: 6500}
status:
  conditions: [{type: Bound, status: "True"}, {type: Available, status: "True"}]
  capabilities:            # live, from NATS
    onOff: {reported: {value: true, at: …}, freshness: live}
    level: {desired: {value: 60, deadline: …}, reported: {value: 40, at: …}, freshness: live}
  pendingCommands: 1
  managedBy: "KNX import ets-2026"
```

**Bindings:**

| Binding | Meaning |
|---|---|
| `command` | Where writes go |
| `status` | Where the confirmed value comes from. It can be a list: the first source to report wins |
| `read` | Where to send read requests: KNX read, Modbus poll |
| Action roles (`step`, `stop`, `goToPreset`…) | Actions a capability defines |
| `{device: X}` or `{device: X, key: …}` | API integrations: the capability the device offers |
| `{hub: {}}` | A helper whose value is set by commands or by an Automation's desired-state output (see 5.2) |

Each binding can have a **transform**:
- `invert`, for blinds where KNX says 100% = closed;
- `scale`;
- `map` between enums;
- `function: <automation>/<name>`, a function exported by an Automation, in any language.

That's openHAB's profiles, typed.

**Choice: how entities are typed.**

| Option | Benefits | Limitations |
|---|---|---|
| **Type + capabilities** (recommended): the type has required and optional capabilities, as in a Matter device type or a HomeKit service | Validated shape; good UI and voice by default; clean mapping to Matter, HomeKit, Google, Alexa | A device that doesn't fit any type needs `type: generic` |
| Capabilities only (SmartThings, Homey) | Maximum flexibility | The UI and voice export have to guess what the thing is |
| One value per entity, grouped in equipment (openHAB Items) | Simplest state model | A light with color temperature is three objects; automations get verbose |

### 5.2 Helpers and derived entities

There's no separate kind. Helpers are Entities whose bindings point to the built-in `hub` integration: the desired value becomes the reported value immediately, and it's stored in NATS. That gives:
- toggles and modes (Home, Away, Night, Vacation, as Hubitat's *modes*);
- numbers and timers;
- counters.

Derived values (average temperature of a floor, "any window open", cost of energy today) are helpers set by an Automation's desired-state output (5.7). They are re-evaluated whenever an input changes, in any language, with no separate mechanism.

```yaml
kind: Entity
metadata: {name: house-mode}
spec:
  type: mode
  capabilities:
    mode: {hub: {options: [home, away, night, vacation], initial: home}}
---
kind: Entity
metadata: {name: ground-floor-temperature}
spec:
  type: sensor
  capabilities:
    measurement:
      kind: temperature
      derived: 'avg(area("ground-floor").entities({ type: "sensor" }).map(e => e.measurement.value))'
```

| Benefits | Limitations |
|---|---|
| Everything addressable is an entity: automations, the UI, history and voice treat helpers like any device | The `hub` integration is special (no failover profile, state only in NATS) |

### 5.3 Capability catalog

The catalog is defined in code and versioned with the hub. Semantics come from Matter clusters where they exist.

| Capability | Attributes | Actions | Events | Matter / HA equivalent |
|---|---|---|---|---|
| `onOff` | `on` (bool) | set, toggle | — | OnOff / light, switch |
| `level` | `level` (0–100 %) | set (transition), step, stop | — | LevelControl |
| `colorTemperature` | `kelvin` | set (transition) | — | ColorControl (CT) |
| `color` | `xy` or `hs` | set (transition) | — | ColorControl |
| `position` | `position`, `tilt` (%; 0 = closed), `moving` | set, open, close, stop | — | WindowCovering / cover |
| `thermostat` | `setpoint(s)`, `mode` (off, heat, cool, auto), `action` (idle, heating, cooling) | set | — | Thermostat / climate |
| `measurement` | `value` + `kind` (temperature, humidity, CO₂, illuminance, pressure…) + unit | — | — | Measurement clusters / sensor |
| `binaryState` | `state` + `kind` (contact, occupancy, leak, smoke, vibration) | — | — | BooleanState, OccupancySensing / binary_sensor |
| `button` | — | — | `pressed` (short, long, double, index) | Switch events / event |
| `lock` | `state` (locked, unlocked, jammed) | lock, unlock | — | DoorLock / lock |
| `mode` | `mode` (enum) | set | — | ModeSelect / select |
| `power` | `watts` | — | — | ElectricalPowerMeasurement |
| `energy` | `kWh` (monotonic; resets detected) | — | — | ElectricalEnergyMeasurement |
| `battery` | `level`, `low` | — | — | PowerSource |
| `videoStream` | `profiles` (codec, resolution) | — | — | Camera AV stream (recent Matter) / camera |
| `snapshot` | — | capture | — | camera |
| `ptz` | `pan`, `tilt`, `zoom`, `presets` | move, goToPreset, stop | — | Camera PTZ (recent Matter) |
| `motion`, `objectDetection` | — | — | `detected` (object class, zone, score) | camera events |
| `networkPort` | `link` (up, speed), `poe` (on, watts) | setPoe, powerCycle | — | — |
| `wifiNetwork` | `enabled`, `clients` | set | — | — |
| `firmware` | `installed`, `available` | install | — | update |
| `presence` | `state` (home, away, zone) | — | — | person / device_tracker |
| `custom` | typed value + unit | set | named events | escape hatch for unusual devices |

Every capability has a default command deadline and a default `staleAfter`.

| Option | Benefits | Limitations |
|---|---|---|
| **Catalog in code, Matter semantics, plus `custom`** (recommended) | Consistent semantics everywhere; Matter bridge and voice export come almost for free | New capabilities wait for a hub release; `custom` covers the gap |
| Catalog as resources (`CapabilityType`) | Integrations can add capabilities without a hub release | The UI, voice and history can't know what a new capability means; drift between installations |

### 5.4 Area and Zone

```yaml
kind: Area
metadata: {name: kitchen}
spec: {displayName: Kitchen, kind: room, parent: ground-floor}   # kind: floor | room | zone | outdoor
---
kind: Zone          # geographic, for presence
metadata: {name: office}
spec: {displayName: Office, latitude: …, longitude: …, radiusMeters: 150}
```

Areas form a tree. Devices and entities each have one area; free tagging uses labels.

### 5.5 Group

Selects entities and is commandable like an entity, with the capabilities its members share.

```yaml
kind: Group
metadata: {name: ground-floor-lights}
spec:
  type: light
  selector:
    matchLabels: {}
    types: [light]
    area: {name: ground-floor, includeChildren: true}
  aggregate: {onOff: any, level: average}
  native:                                    # optional: one command reaches every member at once
    onOff: {command: {datapoint: ga-0-0-1}}  # KNX central function
status:
  members: 14
  capabilities: {onOff: {reported: {value: true}, freshness: live}}
```

| Benefits | Limitations |
|---|---|
| Selectors keep groups correct as entities are added; the native binding uses KNX central functions or Zigbee groups, which are atomic and instant; each member's state is still confirmed | When both are possible, the hub must choose between a native command and fanning out to members (native first, then confirmation per member) |

### 5.6 Scene

```yaml
kind: Scene
metadata: {name: evening}
spec:
  displayName: Evening
  states:
  - {entity: kitchen-ceiling, onOff: true, level: 40, colorTemperature: 2700}
  - {group: ground-floor-blinds, position: 20}
  transition: 3s
  native:                                    # optional: device-side scene
    knx: {datapoint: ga-0-1-0, sceneNumber: 5}
status:
  lastActivation: {at: …, by: user/lucas, result: Confirmed}   # or Partial, with the failed entities
```

Actions:
- `POST …/scenes/evening/activate` applies the scene. Every state goes through command delivery, so the result is either confirmed or lists what failed.
- `POST …/scenes/evening/capture` fills `states` from the current reported values.

### 5.7 Automation

There is **one resource type for all logic**, `Automation`, written in **any language** and managed from the UI or from Git like any other resource.

#### Source and languages

The source can be given in two ways:
1. **In the resource** (`language` + `files`), edited in the UI or kept in Git. The hub builds it in the cluster with the official toolchain image for that language, and stores the `.wasm` result in the NATS object store (R3), keyed by digest.
2. **As a prebuilt artifact** (`artifact: {oci: …@sha256:…}`), built by your own CI. Suited to a large project in its own repository.

| Language | How it becomes WebAssembly |
|---|---|
| Go, TinyGo, Rust, C/C++, Zig, AssemblyScript | Compiled directly for WASI |
| TypeScript/JavaScript, Python | The interpreter is packaged with the code into one module |
| **Declarative** (YAML) | Interpreted directly by the hub, no build. The visual editor in the UI produces it. |

- **The read-only rule applies:** when ArgoCD manages an `Automation`, its source is read-only in the UI.
- **The UI editor:** syntax highlighting for every language. Build errors are shown when saving. Completion is available for TypeScript in the browser; for other languages it comes from your own editor, with the generated `house` package.
- **What limits "any language" is the SDK** (typed `house` package, effects, durable sleep), not the runtime. The calling layer is small (protobuf over pointer and length), so community SDKs are possible. Go and TypeScript ship first.

#### Two behaviours: event handlers and desired state

One `Automation` can contain both, in any language:

| | Event handler | Desired state |
|---|---|---|
| Runs | When something *happens*: button pressed, person detected, door opened | **Continuously**: re-evaluated whenever an input changes, like a Kubernetes controller |
| Does | A sequence over time: turn on, wait, turn off, notify | Declares what should be true now: "setpoint 7 °C if a window has been open for 2 minutes, else the schedule" |
| Missed or late event | The trigger's `lateEvents` policy: `run`, `skip`, `runIfWithin` | Can't leave the house wrong: the next evaluation corrects it |
| After a failover | Replays its history and continues | Nothing to resume: evaluated again |
| Typical | Doorbell, notifications, scenes on a button, "welcome home" | Heating, blinds, lighting by presence, ventilation, derived values |

- **Event handlers** have Home Assistant's run modes: `single`, `restart`, `queued`, `parallel`.
- **Desired-state outputs carry a priority.** Two outputs writing the same attribute with the same priority are rejected when saving. A manual command overrides an output as its `manualOverride` says: `untilInputsChange`, `{for: 2h}` or `never`.
- **Derived values** ("average temperature of the floor", "any window open") are desired-state outputs written to a helper entity (5.2).

```yaml
kind: Automation
metadata: {name: living-room}
spec:
  source:
    language: declarative
    definition:
      on:
      - triggers: [{entity: garden-camera, event: objectDetected, match: {object: person}, lateEvents: skip}]
        conditions: [{sun: belowHorizon}, {entity: house-mode, mode: {not: away}}]
        mode: restart
        steps: [{scene: entrance-lights}, {delay: 5m}, {entity: entrance-light, onOff: false}]
      desired:
      - {priority: 100, when: [{area: living-room, windowsOpen: {for: 2m}}], set: {entity: living-room-thermostat, setpoint: 7}}
      - {priority: 50,  set: {entity: living-room-thermostat, setpoint: {schedule: heating-comfort}}}
---
kind: Automation
metadata: {name: my-rules}
spec:
  source:
    language: go
    files: {main.go: "…", go.mod: "…"}
    # or: artifact: {oci: "ghcr.io/you/home-rules@sha256:…"}
  limits: {memory: 64Mi, effectsPerRun: 10000}
status:
  build: {digest: "sha256:…", toolchain: go1.27.3, conditions: [{type: Built, status: "True"}]}
  handlers:
  - {name: entrance-light, kind: event, triggers: ["garden-camera objectDetected"], mode: restart}
  - {name: heating, kind: desiredState, outputs: [living-room-thermostat.setpoint], priority: 100}
  lastRuns: [{name: my-rules-entrance-light-8f2c, result: Succeeded, at: …}]
```

The same in Go:

```go
func init() {
	automation.OnEvent("entrance-light", automation.Restart,
		house.GardenCamera.OnObjectDetected().LateEvents(automation.Skip),
		func(ctx automation.Context, ev house.ObjectDetected) error {
			if ev.Object != "person" || !ctx.Sun().BelowHorizon() {
				return nil
			}
			if err := house.EntranceLights.Activate(ctx); err != nil {
				return err
			}
			ctx.Sleep(5 * time.Minute) // durable
			return house.EntranceLight.OnOff().Set(ctx, false)
		})

	automation.Desired("heating", 100, func(s automation.State) automation.Outputs {
		setpoint := s.Schedule(house.HeatingComfort)
		if s.Area(house.LivingRoom).WindowsOpenFor(2 * time.Minute) {
			setpoint = 7
		}
		return automation.Set(house.LivingRoomThermostat.Setpoint(), setpoint)
	})
}
```

The hub learns an Automation's handlers and outputs by calling its `describe` export after the build, and shows them in status.

#### Execution engine

- **WebAssembly on wazero,** inside the hub. The declarative language is interpreted by the hub with the same history model.
- **Determinism is enforced, not left to convention:**
  - The clock and randomness imports return values recorded in the run's history. The Go runtime seeds map iteration and `select` from that randomness, so they replay identically.
  - There is no network, no filesystem and no threads.
  - A plain `time.Sleep` traps with "use `ctx.Sleep`".
- **History and replay**, as in Temporal:
  1. Every call that leaves the sandbox is recorded with its result in the run's history in NATS.
  2. After a failover, or when a waiting run was evicted from memory, the run executes again from the start. Calls already recorded return their recorded result and are not made again.
  3. The run continues live from the first new call.
  4. If the replayed code makes different calls than the history, the run fails with a non-determinism error.
- **Version pinning:** a run in flight finishes on the build it started with.
- **Memory:** runs waiting on a timer or an event are evicted and replayed when they wake. Only active runs use memory.
- **Limits per run:** memory, number of calls, CPU time per activation. `continueAsNew` restarts a long-lived run with an empty history.
- **Guarantee after a node crash:**
  - An event is acknowledged only once its run is recorded in NATS. Each run has an owner, through a lease with the 2 s TTL ([0006](../adr/0006-leases-and-fencing.md)).
  - When the owner's node crashes, the run resumes on another node once the lease expires, and replays. Events that arrive in the meantime are delivered marked `late`, and each trigger's `lateEvents` policy decides (`runIfWithin: 5s` suits a button).
  - In practice, an event during a crash is handled 2–3 s late, never lost.

#### What code can call

There are two kinds of call, distinguished by whether they leave the sandbox.

**1. Calls that leave the sandbox: recorded.** The code asks something outside to act, or for data that can change.

| Example | Implemented by |
|---|---|
| `house.EntranceLight.OnOff().Set(ctx, false)`: send a command, wait for confirmation or expiry | Hub |
| `ctx.Sleep(5 * time.Minute)`: durable timer | Hub |
| Reading an entity's current state, waiting for an event, notify, sun, schedules | Hub |
| `house.WeatherHome.Hourly(ctx, req)`: weather forecast; also electricity prices, calendar lookup, snapshot analysis | An integration that provides functions |

- **Every call and its result is recorded** in the run's history. On replay, the call isn't made again; the recorded result is returned. That's what lets a run continue after a failover without sending a command twice.
- **Integration functions:**
  - They're how plugins extend what code can call.
  - The integration declares them in its `describe` message, with JSON Schemas for requests and responses. Each is a `QUERY` (no side effects, retried freely) or an `ACTION` (side effects, carries the run's step ID as an idempotency key), with a default deadline. The hub calls them over MQTT 5 request and reply (4.1).
  - Typed bindings are generated for every SDK language from those JSON Schemas.
  - A schema change must stay compatible with existing automations; the hub checks it when the integration publishes a new `describe`.

**2. Code inside the sandbox: not recorded.** The automation's own functions and the libraries it imports (PID controller, hysteresis, sun position maths). They only compute, with no outside access, so they're deterministic: replaying them gives the same result.

#### Permissions: for code written by someone else

`spec.permissions` limits what an Automation may call. It only matters for code you didn't write: a template shared by someone else, or a prebuilt artifact from a third party. Without it, such code could unlock a door or send presence data out through a webhook integration.

- **Optional.** When it's absent, the Automation can call anything, under the identity it runs as. For your own code this is right: whoever can create an Automation is already authorized by the API's roles to do anything it could do.
- **Enforced by the sandbox when present.** Any call outside it fails. It covers commands, integration functions and notifications:
  ```yaml
  permissions:
    command: {areas: [garden], entities: [entrance-light]}
    functions: [weather-home/Forecast.Hourly]
    notify: [phones]
  ```
- **Required for `AutomationTemplate` and for third-party artifacts.** The UI shows it for approval before installing, like an app's permissions on a phone.

#### Runs, templates, developer experience

- **Runs** are a read-only resource, `AutomationRun`, with a step-by-step trace like Home Assistant's traces: trigger, conditions, every recorded call and its result, and any replays.
- **Templates (blueprints):** an `AutomationTemplate` is an Automation with declared inputs. An `Automation` with `template: {name, inputs}` instantiates it, and updating the template updates every instance.
- **A typed `house` package** for each SDK language, generated from the house's API, covers entities, scenes, schedules and integration functions. A wrong name or type fails at build time.
- **Tests run natively:** `go test` against the SDK's simulated hub with a test clock, debuggable with Delve. A recorded run can be downloaded and replayed locally.

**Choice: execution engine.**

| Option | Benefits | Limitations |
|---|---|---|
| **wazero with our own protobuf calling layer** (recommended) | Pure Go, actively maintained; we control every import, which is what enforces determinism | We write the SDK per language: Go and TypeScript first |
| Extism (plugin framework on wazero) | SDKs for many languages, manifests, host functions | **Rejected:** its Go host SDK has had no commits since May 2025, is pinned to wazero v1.9.0 (latest v1.12.0), and a bug reported in October 2026 is unanswered. Its ideas are kept: modules pinned by digest, limits, host functions as the only way out. |
| External workers, Temporal-style | Native code, any library | Determinism by convention only; users build, deploy and run containers; no sandbox |
| Temporal itself | Proven | Several services plus a database on Pis; a second failover model beside our NATS leases |

**Limitations:**
- The replay engine is real engineering and the core of the automation engine.
- **Builds in the cluster use toolchain images** (hundreds of MB each, pulled once) and take seconds to minutes on a Pi. Building outside, with a prebuilt artifact, avoids this.
- Go modules are several megabytes, and compiling one with wazero takes seconds on a Pi. The compiled code is cached per digest.
- Automations have no network access: anything that reaches the outside world is an integration, called through its functions.

### 5.8 Schedule

A weekly program with exceptions: heating setpoints, blinds, lighting.

```yaml
kind: Schedule
metadata: {name: heating-comfort}
spec:
  type: number                     # number | onOff | mode
  weekly:
    monday:    [{at: "06:30", value: 21}, {at: "22:30", value: 18}]
    # …
  exceptions: [{date: "2026-12-25", copy: sunday}]
  pushToDevice: false              # optional: KNX thermostats with their own time programs
status:
  current: {value: 21, since: …, next: {at: "22:30", value: 18}}
```

An entity capability can bind its setpoint to a schedule (`setpoint: {schedule: heating-comfort}`), and automations can use it as a trigger or a condition.

| Benefits | Limitations |
|---|---|
| Heating is the main use case and Home Assistant has no good answer for it; pushing to the device keeps the program running even if the hub is down | Device-side programs vary a lot between manufacturers |

### 5.9 Person, User and access

```yaml
kind: Person
metadata: {name: lucas}
spec:
  displayName: Lucas
  presence:
    sources:
    - {networkClient: {mac: "aa:bb:…"}}      # phone seen on an access point
    - {app: {device: lucas-phone}}           # geofence from the mobile app
    combine: any                             # any | all
    awayAfter: 10m
status:
  presence: {state: home, since: …, zone: null}
---
kind: User
metadata: {name: lucas}
spec:
  displayName: Lucas
  person: lucas
  roles: [installer]                         # installer | resident | guest
  scope: {}                                  # optional: limit to areas or entities (children, guests)
  expires: null                              # for guests
status:
  lastLogin: …
```

- **Credentials** (password, passkeys, OIDC subject) are set through `POST …/users/lucas/credentials` and never appear in spec.
- **Requests coming through Kubernetes aggregation** map Kubernetes identities to the same roles.

| Benefits | Limitations |
|---|---|
| Scopes give guests and children limited control ("garage door only on weekdays"); users in Git make an installation reproducible | The roles must be designed once for both entry points (local accounts and Kubernetes identities) |

### 5.10 Notifications

```yaml
kind: NotificationChannel
metadata: {name: phones}
spec:
  type: appPush                    # appPush | email | ntfy | webhook
  recipients: {users: [lucas]}
  secretRef: null
```

Automations use `notify: {channel: phones, message: …, priority: critical}`. A notification is a momentary command with a deadline, so its delivery is tracked like any other command.

### 5.11 History policy

```yaml
kind: HistoryPolicy
metadata: {name: default}
spec:
  selector: {}                     # all entities
  raw: 30d
  downsample: [{every: 5m, keep: 2y, aggregate: [mean, min, max]}]
  events: 1y
```

Several policies can apply; the most specific selector wins. There are built-in defaults, so nothing needs configuring.

### 5.12 Cameras: Recorder and Recording

The streams themselves go through a media service (a separate design to do). The resources describe policy and index the clips.

```yaml
kind: Recorder
metadata: {name: outdoor}
spec:
  cameras: {area: {name: outdoor, includeChildren: true}, types: [camera]}
  modes:
    continuous: {retention: 3d, profile: sub}
    objects:    {retention: 30d, profile: main, objects: [person, vehicle], preRoll: 5s, postRoll: 10s}
  detection: camera                # camera (on-device analytics) | hub (needs an accelerator)
  storage: {persistentVolumeClaim: recordings}
status:
  storage: {usedBytes: …, capacityBytes: …}
  cameras: [{entity: garden-camera, recording: true}]
---
kind: Recording                    # read-only, indexed in Postgres
metadata: {name: garden-camera-20261109-071502}
status:
  entity: garden-camera   start: …   end: …   reason: {object: person}
  thumbnail: …   media: /media/recordings/…
```

| Benefits | Limitations |
|---|---|
| Recording modes and retention per mode (as in Frigate); detection on the camera works on a Pi with no accelerator | **Recording continuity during a node failure is not designed yet**: recording on two nodes doubles storage and bandwidth, and recording on one node loses a few seconds when it crashes |

### 5.13 Network devices

- **Switch ports and Wi-Fi networks are Entities** (`networkPort`, `wifiNetwork`), proposed by the network integration.
- **Connected clients are read-only `NetworkClient` resources** served from NATS: MAC, IP, hostname, access point or port, connected since.
- **Presence and automations** reference clients by MAC.

All of this is optional and only available when the network gear can be managed. It's never a requirement.

### 5.14 Dashboard

```yaml
kind: Dashboard
metadata: {name: tablet-hallway}
spec:
  views:
  - {title: Ground floor, cards: [{area: kitchen}, {entity: garden-camera, display: live}, {group: ground-floor-lights}]}
```

- **Default views are generated** from areas and entity types, as in Apple Home, so a new installation never needs a dashboard.
- **Dashboards are optional customisations**, and Git-managed ones are read-only like everything else.

| Option | Benefits | Limitations |
|---|---|---|
| **Resources** (recommended) | Same API, GitOps, read-only rule and RBAC as everything else | The schema must be versioned like any API |
| UI-only storage | Free-form | Not reproducible; outside GitOps |

### 5.15 Energy (later)

`EnergySite` names the grid meter, solar production, battery and EV chargers (as entities with `power` and `energy`), plus tariffs: static, or dynamic from an integration. It feeds an energy view and optimisation automations. It's only listed here so the capability catalog includes `power` and `energy` from the start.

### 5.16 Installation (read-only singleton)

Shows the network mode (virtual IP or service), the result of each preflight check, and the resulting guarantees:

```yaml
kind: Installation
metadata: {name: current}
status:
  networkMode: virtualIP
  guarantees: {plannedOperationsLossless: true, …}
  checks: [{name: gracefulNodeShutdown, status: "False", impact: "a manual reboot is handled as a crash"}]
```

## 6. Overview

```
Integration ──owns──▶ Device ──via──▶ Device
     │   ▲              ▲
     │   └── Import ────┼──owns──▶ Datapoint, Entity
     └──owns──▶ Datapoint
Integration.status.discovered ──suggests──▶ Device (created by the user) ──suggests──▶ Entities

Entity.capabilities ──binds──▶ Datapoint | Device capability | hub helper | Schedule   (transform ──▶ Automation function)
Entity, Device ──area──▶ Area ──parent──▶ Area
Group ──selector──▶ Entities         Scene ──▶ Entities, Groups
Automation ──▶ Entities, Groups, Scenes, Schedules, NotificationChannels, integration functions
Automation ──template──▶ AutomationTemplate          Automation ──runs──▶ AutomationRun
Integration ──provides──▶ functions (declared in `describe`, called over MQTT 5)
Person ──presence──▶ NetworkClient | app | Entity     User ──▶ Person
Recorder ──selector──▶ camera Entities ──▶ Recording
HistoryPolicy ──selector──▶ Entities           Dashboard ──▶ Areas, Entities, Groups

Read-only, served from NATS or Postgres: NetworkClient, AutomationRun, Recording, Installation
Subresources: entities|groups/commands, scenes/activate|capture, devices/replace,
              automations/trigger, users/credentials
```

## 7. Choices to make

| # | Choice | Decision or recommendation | Main alternative |
|---|---|---|---|
| 1 | Scope | **Decided:** namespaced, single namespace `home` for now; workloads in `home-system`; namespace deletion guarded | Cluster-scoped (rejected) |
| 2 | Physical side for API integrations | Two binding styles: Datapoints for point protocols, device capabilities for API integrations | Datapoints for everything |
| 3 | Discovery | **Decided:** suggestions in `Integration.status.discovered`; adopting = creating a `Device`; ignore list in the Integration spec | `DiscoveredDevice` resource, `Device` with `adopted: false` (both rejected) |
| 4 | KNX configuration | **Decided:** `Import` from the ETS project (and keyring), owning what it creates; manual entry and bus-monitoring suggestions as complements; the hub never reprograms devices; buttons handled by documented patterns and templates, not a model concept | Manual entry only |
| 5 | Entity typing | Type + capabilities | Capabilities only |
| 6 | Capability catalog | In code, Matter semantics, plus `custom` | Catalog as resources |
| 7 | Helpers, modes, derived values | Entities bound to the `hub` integration, set by commands or by an Automation's desired-state outputs | Separate kinds |
| 8 | Languages | **Decided:** Any language compiled to WebAssembly, plus a declarative YAML language for the UI; no embedded expression language (CEL only inside API validation); Go and TypeScript SDKs first | A single scripting language |
| 9 | Logic resource | **Decided:** One `Automation` type: source in the resource (built in the cluster) or a prebuilt artifact; event handlers and desired-state outputs, with durable replay | Separate kinds for scripts, modules and rules |
| 10 | Integration model | **Decided:** workloads managed by the hub; interface MQTT 5 + JSON, usable without the SDK; roles and fencing managed by the hub; built-ins in the hub image (ADR 0009) | In-process Go, WebAssembly modules (rejected) |
| 11 | Dashboards | Resources, with generated defaults | UI-only storage |
| 12 | Camera recording across node failures | Open: needs its own design | — |
| 13 | Execution engine | **Decided:** wazero with our own protobuf calling layer | Extism (rejected), external workers, Temporal |
| 14 | Extending what code can call | **Decided:** Integrations provide functions, declared in `describe` and called as recorded calls that leave the sandbox | — |
| 15 | Permissions | **Decided:** Optional; enforced by the sandbox when present; required for templates and third-party artifacts | Always required |

## 8. Prototypes to build before the ADR

| Prototype | Question it answers |
|---|---|
| resourceVersion and watch | Can one opaque resourceVersion over the Postgres outbox feed and the NATS KV revision support Kubernetes list and watch semantics, including resuming? |
| TLS migration | Can kernel TLS state move with `TCP_REPAIR`, between processes and between nodes? |
| Deterministic Go in WebAssembly | Under recorded clock and randomness imports, does a Go `wasip1` module (goroutines, maps, `select`) replay identically? What do compiling and replaying cost on a Pi? |
| Interpreted languages | How are TypeScript and Python packaged with their interpreter into one WebAssembly module, and is it fast enough on a Pi? |
| Builds in the cluster | How long does building a Go or TypeScript Automation take on a Pi, with which toolchain images, and where are build logs shown? |
| KNX Data Secure from the hub | With an MDT KNX IP router and Data Secure actuators: what must the ETS project contain for the hub to send secure telegrams over routing and over tunnelling? |
| ETS project parser | Does a Go parser read real ETS6 projects (password-protected, with functions and keyrings) completely, and can it compute which group addresses the routers' filter tables block? |
