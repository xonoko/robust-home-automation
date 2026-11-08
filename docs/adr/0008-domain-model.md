# 0008. Domain model: Device, Channel, Entity, Area

- Status: Accepted
- Date: 2026-11-08

## Context

The domain model shapes the database schema, the bus subjects and the API; it is expensive to change later. Home Assistant models devices with typed entities; openHAB separates the physical side (Things, Channels) from the user-facing side (Items) and links them.

## Decision

We follow openHAB's separation:

- **Device**: a physical thing reached through an integration (a KNX actuator, a Modbus heat pump, a Zigbee sensor). Identified by `(integration, integration_ref)`.
- **Channel**: one capability of a device, with its protocol-specific configuration (a KNX group address, a Modbus register, a Zigbee2MQTT attribute). Belongs to exactly one device.
- **Entity**: the user-facing state and control (a light, a temperature, a blind). Linked to one or more channels, so a KNX light with separate switch, dim and status group addresses is one entity.
- **Area**: floors, rooms and zones, nested. Devices and entities can be placed in areas.

Automations, the UI and the API work on entities; integrations work on channels.

## Consequences

- Replacing a physical device means relinking channels; entities, automations and history are untouched.
- The schema lives in `internal/store/ent/schema`.

## Rejected alternatives

- **Home Assistant's device → entity model**: merges the protocol side and the user side, which makes multi-channel KNX objects awkward.
