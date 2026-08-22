// What a newly-found device should become in the store.
//
// Adding a device used to make the user answer this themselves, twice over:
// once by choosing which screen to start from (Devices adds a Socket, Sensors
// adds a Sensor) and again by picking a protocol out of an eight-entry
// dropdown. Both questions are answerable from what the device itself
// reports, so they are answered here instead — and the add-device wizard
// asks only for a name and a room.
//
// This module is deliberately markup-free: it is the rule, and it is the
// part worth testing.

import type { MatterState, SensorKind } from "./types";

// How a device is reached. This is the one thing the user genuinely has to
// tell us, because it is a fact about the object in their hand rather than
// anything we can probe for.
export type ConnectionMethod = "matter" | "tasmota" | "rf-socket" | "rf-sensor" | "mqtt";

// A sensor the wizard intends to create. `field` selects which measurement
// the backend reads — for Matter that is the cluster name, for rtl_433/MQTT
// the JSON key.
export interface PlannedSensor {
  kind: SensorKind;
  unit: string;
  field: string;
  /** Appended to the device name so two sensors off one device stay apart. */
  suffix: string;
}

// What one physical device becomes: at most one Socket, plus any number of
// Sensors. A Matter thermo-hygrometer is zero sockets and two sensors; a
// Matter plug is one socket and none; a plug that also meters is both.
export interface DevicePlan {
  /** True when the device can be switched, i.e. it is worth a Socket. */
  socket: boolean;
  sensors: PlannedSensor[];
}

export function defaultUnit(kind: SensorKind): string {
  switch (kind) {
    case "temperature": return "°C";
    case "humidity":    return "%";
    case "light":       return "lux";
    case "power":       return "W";
    case "motion":      return "";
    default:            return "";
  }
}

// The measurement name the 433/MQTT decoders emit. Matter has its own
// vocabulary (see planFromMatterState) because it reads clusters, not JSON.
export function defaultField(kind: SensorKind): string {
  switch (kind) {
    case "temperature": return "temperature_C";
    case "humidity":    return "humidity";
    case "light":       return "lux";
    case "power":       return "power_W";
    case "motion":      return "motion";
    default:            return "";
  }
}

/**
 * Decide what a freshly commissioned Matter node becomes, from the
 * capabilities the bridge reported for it.
 *
 * The rule is "render only what it answered for", the same discipline the
 * speaker panes follow: a field the bridge omitted means the node has no
 * such cluster, so we must not create a record that would never be filled.
 */
export function planFromMatterState(state: MatterState): DevicePlan {
  const sensors: PlannedSensor[] = [];
  if (state.temperature !== undefined) {
    sensors.push({ kind: "temperature", unit: "°C", field: "temperature", suffix: "temperature" });
  }
  if (state.humidity !== undefined) {
    sensors.push({ kind: "humidity", unit: "%", field: "humidity", suffix: "humidity" });
  }

  // OnOff is what makes a device switchable, and a Socket is the switchable
  // thing. A pure thermometer gets none: a read-only tile that can never be
  // tapped is a control that would be refused.
  const switchable = state.on !== undefined;

  // A node that reported nothing we understand is still commissioned onto
  // the fabric, and leaving it with no record at all would strand it — the
  // user would have no way to see it, name it, or remove it. So it falls
  // back to a Socket, which is the one record that can hold a bare node id.
  if (!switchable && sensors.length === 0) {
    return { socket: true, sensors: [] };
  }
  return { socket: switchable, sensors };
}

/**
 * Name one planned sensor, given the name the user gave the device.
 * A device producing a single sensor keeps the plain name; one producing
 * several gets the measurement appended, so "Greenhouse" becomes
 * "Greenhouse temperature" and "Greenhouse humidity".
 */
export function sensorName(deviceName: string, plan: PlannedSensor, total: number): string {
  const base = deviceName.trim();
  if (total <= 1) return base;
  return `${base} ${plan.suffix}`;
}

/** The protocol string a Socket created through this method should carry. */
export function socketProtocol(method: ConnectionMethod, transport?: string): string {
  switch (method) {
    case "matter":  return transport === "thread" ? "matter-thread" : "matter";
    case "tasmota": return "tasmota";
    case "mqtt":    return "mqtt";
    default:        return "nexa";
  }
}
