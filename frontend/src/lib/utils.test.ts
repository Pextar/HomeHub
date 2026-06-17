import { describe, it, expect } from "vitest";
import {
  plural,
  formatDays,
  protocolKind,
  isSmartProtocol,
  formatCountdown,
  formatAgo,
  sortedSockets,
  groupSocketsByRoom,
  automationsUsingSocket,
  automationsUsingSensor,
  automationsUsingTarget,
} from "./utils";
import type { Socket, Automation, AutomationRule } from "./types";

// ── Fixtures ────────────────────────────────────────────────────────────────

function socket(over: Partial<Socket> = {}): Socket {
  return {
    id: "s1",
    name: "Lamp",
    code: "",
    protocol: "nexa",
    state: false,
    room: "",
    ...over,
  };
}

function automation(rules: AutomationRule[]): Automation {
  return { id: "a1", name: "Auto", enabled: true, rules };
}

// ── plural ───────────────────────────────────────────────────────────────────

describe("plural", () => {
  it("uses the singular form for exactly one", () => {
    expect(plural(1, "automation")).toBe("1 automation");
  });
  it("appends 's' for zero and many", () => {
    expect(plural(0, "automation")).toBe("0 automations");
    expect(plural(3, "socket")).toBe("3 sockets");
  });
});

// ── formatDays ────────────────────────────────────────────────────────────────

describe("formatDays", () => {
  it("treats empty, full, and undefined as every day", () => {
    expect(formatDays(undefined)).toBe("Every day");
    expect(formatDays([])).toBe("Every day");
    expect(formatDays([0, 1, 2, 3, 4, 5, 6])).toBe("Every day");
  });
  it("recognises weekdays and weekends regardless of order", () => {
    expect(formatDays([5, 1, 2, 3, 4])).toBe("Weekdays");
    expect(formatDays([6, 0])).toBe("Weekends");
  });
  it("lists named days sorted otherwise", () => {
    expect(formatDays([3, 1])).toBe("Mon, Wed");
  });
});

// ── protocolKind / isSmartProtocol ────────────────────────────────────────────

describe("protocolKind", () => {
  it("maps each transport family", () => {
    expect(protocolKind("tasmota")).toBe("wifi");
    expect(protocolKind("wifi")).toBe("wifi");
    expect(protocolKind("matter")).toBe("matter");
    expect(protocolKind("matter-thread")).toBe("matter");
    expect(protocolKind("mqtt")).toBe("mqtt");
  });
  it("falls back to rf for unknown protocols", () => {
    expect(protocolKind("nexa")).toBe("rf");
    expect(protocolKind("anything-else")).toBe("rf");
  });
});

describe("isSmartProtocol", () => {
  it("is true only for bridged smart lights", () => {
    expect(isSmartProtocol("tasmota")).toBe(true);
    expect(isSmartProtocol("matter")).toBe(true);
    expect(isSmartProtocol("matter-thread")).toBe(true);
    expect(isSmartProtocol("nexa")).toBe(false);
    expect(isSmartProtocol("mqtt")).toBe(false);
  });
});

// ── formatCountdown ───────────────────────────────────────────────────────────

describe("formatCountdown", () => {
  it("returns 'now' for past or present targets", () => {
    expect(formatCountdown(new Date(Date.now() - 1000))).toBe("now");
  });
  it("formats seconds, minutes, and hours", () => {
    expect(formatCountdown(new Date(Date.now() + 28_000))).toBe("28s");
    expect(formatCountdown(new Date(Date.now() + 5 * 60_000 + 12_000))).toBe("5m 12s");
    expect(formatCountdown(new Date(Date.now() + 60 * 60_000 + 23 * 60_000))).toBe("1h 23m");
  });
  it("accepts an ISO string", () => {
    expect(formatCountdown(new Date(Date.now() + 30_000).toISOString())).toBe("30s");
  });
});

// ── formatAgo ──────────────────────────────────────────────────────────────────

describe("formatAgo", () => {
  it("returns empty string for falsy input", () => {
    expect(formatAgo(undefined)).toBe("");
  });
  it("clamps future and sub-minute timestamps to 'just now'", () => {
    expect(formatAgo(new Date(Date.now() + 5000).toISOString())).toBe("just now");
    expect(formatAgo(new Date(Date.now() - 5000).toISOString())).toBe("just now");
  });
  it("formats minutes, hours, and days", () => {
    expect(formatAgo(new Date(Date.now() - 5 * 60_000).toISOString())).toBe("5m ago");
    expect(formatAgo(new Date(Date.now() - 3 * 60 * 60_000).toISOString())).toBe("3h ago");
    expect(formatAgo(new Date(Date.now() - 2 * 24 * 60 * 60_000).toISOString())).toBe("2d ago");
  });
});

// ── sortedSockets ─────────────────────────────────────────────────────────────

describe("sortedSockets", () => {
  it("sorts by room then name without mutating the input", () => {
    const input = [
      socket({ id: "a", name: "Zeta", room: "Kitchen" }),
      socket({ id: "b", name: "Alpha", room: "Kitchen" }),
      socket({ id: "c", name: "Beta", room: "Bedroom" }),
    ];
    const out = sortedSockets(input);
    expect(out.map((s) => s.id)).toEqual(["c", "b", "a"]);
    // original order preserved
    expect(input.map((s) => s.id)).toEqual(["a", "b", "c"]);
  });
});

// ── groupSocketsByRoom ────────────────────────────────────────────────────────

describe("groupSocketsByRoom", () => {
  it("buckets by room and folds blank rooms into 'Unassigned'", () => {
    const map = groupSocketsByRoom([
      socket({ id: "a", room: "Kitchen" }),
      socket({ id: "b", room: "" }),
      socket({ id: "c", room: "Kitchen" }),
    ]);
    expect(map.get("Kitchen")?.map((s) => s.id)).toEqual(["a", "c"]);
    expect(map.get("Unassigned")?.map((s) => s.id)).toEqual(["b"]);
  });
});

// ── automation reference counting ─────────────────────────────────────────────

describe("automationsUsingSocket", () => {
  it("counts a socket referenced by trigger, condition, or action", () => {
    const byTrigger = automation([
      { trigger: { type: "device", socket_id: "s1" }, actions: [] },
    ]);
    const byCondition = automation([
      {
        trigger: { type: "time" },
        conditions: [{ type: "device", socket_id: "s1" }],
        actions: [],
      },
    ]);
    const byAction = automation([
      {
        trigger: { type: "time" },
        actions: [{ target_type: "socket", target_id: "s1", action: "on" }],
      },
    ]);
    expect(automationsUsingSocket([byTrigger, byCondition, byAction], "s1")).toBe(3);
    expect(automationsUsingSocket([byTrigger, byCondition, byAction], "other")).toBe(0);
  });
});

describe("automationsUsingSensor", () => {
  it("counts only sensor-triggered automations for that sensor", () => {
    const a = automation([{ trigger: { type: "sensor", sensor_id: "t1" }, actions: [] }]);
    expect(automationsUsingSensor([a], "t1")).toBe(1);
    expect(automationsUsingSensor([a], "t2")).toBe(0);
  });
});

describe("automationsUsingTarget", () => {
  it("counts group and scene action targets", () => {
    const a = automation([
      {
        trigger: { type: "time" },
        actions: [
          { target_type: "group", target_id: "g1", action: "on" },
          { target_type: "scene", target_id: "sc1", action: "activate" },
        ],
      },
    ]);
    expect(automationsUsingTarget([a], "group", "g1")).toBe(1);
    expect(automationsUsingTarget([a], "scene", "sc1")).toBe(1);
    expect(automationsUsingTarget([a], "group", "sc1")).toBe(0);
  });
});
