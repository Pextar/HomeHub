import { describe, it, expect } from "vitest";
import {
  planFromMatterState,
  sensorName,
  socketProtocol,
  defaultUnit,
  defaultField,
} from "./device-setup";
import type { MatterState } from "./types";

const base: MatterState = { id: "42", reachable: true };

describe("planFromMatterState", () => {
  it("makes a thermo-hygrometer two sensors and no socket", () => {
    const plan = planFromMatterState({ ...base, temperature: 21.5, humidity: 48 });
    expect(plan.socket).toBe(false);
    expect(plan.sensors.map(s => s.kind)).toEqual(["temperature", "humidity"]);
    expect(plan.sensors.map(s => s.field)).toEqual(["temperature", "humidity"]);
    expect(plan.sensors.map(s => s.unit)).toEqual(["°C", "%"]);
  });

  it("makes a plain plug one socket and no sensors", () => {
    const plan = planFromMatterState({ ...base, on: false });
    expect(plan.socket).toBe(true);
    expect(plan.sensors).toEqual([]);
  });

  it("makes a switchable device that also measures into both", () => {
    const plan = planFromMatterState({ ...base, on: true, temperature: 19 });
    expect(plan.socket).toBe(true);
    expect(plan.sensors.map(s => s.kind)).toEqual(["temperature"]);
  });

  it("reads a temperature of zero as present, not missing", () => {
    // The bug this guards: `if (state.temperature)` would drop a freezer at 0°C.
    const plan = planFromMatterState({ ...base, temperature: 0 });
    expect(plan.sensors.map(s => s.kind)).toEqual(["temperature"]);
  });

  it("reads an off switch as present, not missing", () => {
    const plan = planFromMatterState({ ...base, on: false });
    expect(plan.socket).toBe(true);
  });

  it("falls back to a socket for a node that reported nothing we understand", () => {
    // Still commissioned onto the fabric — leaving it with no record would
    // give the user no way to see, name or remove it.
    const plan = planFromMatterState(base);
    expect(plan.socket).toBe(true);
    expect(plan.sensors).toEqual([]);
  });

  it("does not create a sensor for a cluster the bridge omitted", () => {
    const plan = planFromMatterState({ ...base, on: true, humidity: 55 });
    expect(plan.sensors.map(s => s.kind)).toEqual(["humidity"]);
  });
});

describe("sensorName", () => {
  it("keeps the plain name when the device yields one sensor", () => {
    expect(sensorName("Greenhouse", { kind: "temperature", unit: "°C", field: "temperature", suffix: "temperature" }, 1))
      .toBe("Greenhouse");
  });

  it("appends the measurement when a device yields several", () => {
    const plan = planFromMatterState({ ...base, temperature: 21, humidity: 40 });
    const names = plan.sensors.map(s => sensorName("Greenhouse", s, plan.sensors.length));
    expect(names).toEqual(["Greenhouse temperature", "Greenhouse humidity"]);
  });

  it("trims the name it was given", () => {
    expect(sensorName("  Shed  ", { kind: "humidity", unit: "%", field: "humidity", suffix: "humidity" }, 1))
      .toBe("Shed");
  });
});

describe("socketProtocol", () => {
  it("distinguishes Matter over Thread from Matter over Wi-Fi", () => {
    expect(socketProtocol("matter", "thread")).toBe("matter-thread");
    expect(socketProtocol("matter", "wifi")).toBe("matter");
    expect(socketProtocol("matter")).toBe("matter");
  });

  it("maps the other methods to their stored protocol", () => {
    expect(socketProtocol("tasmota")).toBe("tasmota");
    expect(socketProtocol("mqtt")).toBe("mqtt");
    expect(socketProtocol("rf-socket")).toBe("nexa");
  });
});

describe("defaults per sensor kind", () => {
  it("pairs each kind with its unit", () => {
    expect(defaultUnit("temperature")).toBe("°C");
    expect(defaultUnit("humidity")).toBe("%");
    expect(defaultUnit("light")).toBe("lux");
    expect(defaultUnit("power")).toBe("W");
    expect(defaultUnit("motion")).toBe("");
    expect(defaultUnit("custom")).toBe("");
  });

  it("pairs each kind with the decoder field it usually arrives under", () => {
    expect(defaultField("temperature")).toBe("temperature_C");
    expect(defaultField("humidity")).toBe("humidity");
    expect(defaultField("power")).toBe("power_W");
    expect(defaultField("custom")).toBe("");
  });
});
