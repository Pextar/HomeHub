import { describe, it, expect, beforeEach, vi } from "vitest";
import type { RuleActionDraft, Socket } from "./types";

/**
 * Turning an authored rule into the actions the backend runs.
 *
 * These two shapes deliberately differ. A rule is *written* the way a person
 * thinks about it — "turn the kitchen on, except leave the one over the sink"
 * is one row — and *run* the way devices work, one action per socket. This
 * translation is the only thing that knows both, and it had no test at all
 * while it lived inside a component's module script.
 *
 * The rules worth pinning are the ones a reader can't see from the form:
 * which of level and colour ride along with an action, and which don't.
 */

const store = {
  value: {
    sockets: [] as Socket[],
    groups: [] as { id: string; socket_ids: string[] }[],
    rooms: [] as { id: string; name: string }[],
    scenes: [] as { id: string; name: string }[],
    settings: {},
    sensors: [],
  },
};
vi.mock("./stores.svelte", () => ({ data: store }));

const { compileAction, membersOf, targetsFor, blankRuleAction, firstTargetType } =
  await import("./rules");

const socket = (id: string, over: Partial<Socket> = {}): Socket =>
  ({ id, name: id, protocol: "rf", room: "Kitchen", ...over }) as Socket;

const action = (over: Partial<RuleActionDraft> = {}): RuleActionDraft => ({
  target_type: "socket",
  target_id: "s1",
  action: "on",
  level: 100,
  color: "",
  ...over,
});

beforeEach(() => {
  store.value.sockets = [];
  store.value.groups = [];
  store.value.rooms = [];
  store.value.scenes = [];
});

describe("compiling one action", () => {
  it("passes an ordinary action straight through", () => {
    expect(compileAction(action({ action: "off" }))).toEqual([
      { target_type: "socket", target_id: "s1", action: "off" },
    ]);
  });

  it("calls a scene's action 'activate', whatever the form last had", () => {
    // The form re-seeds this on retarget, but a draft can arrive from a saved
    // rule, and a scene has no other verb.
    const out = compileAction(action({ target_type: "scene", target_id: "sc1", action: "on" }));
    expect(out[0].action).toBe("activate");
  });

  it("always carries a socket's brightness, even at full", () => {
    // A socket action is about one lamp, and its level is part of what the
    // rule says. Colour rides along only when one was chosen.
    expect(compileAction(action({ level: 100 }))[0]).toEqual({
      target_type: "socket",
      target_id: "s1",
      action: "on",
      level: 100,
    });
    expect(compileAction(action({ level: 40, color: "f1c696" }))[0]).toEqual({
      target_type: "socket",
      target_id: "s1",
      action: "on",
      level: 40,
      color: "f1c696",
    });
  });

  it("leaves a group's lighting off an untouched 'on'", () => {
    // "Turn the kitchen on" should not also dictate a brightness nobody set —
    // the lamps come back however they were.
    expect(compileAction(action({ target_type: "group", target_id: "g1", level: 100 }))).toEqual([
      { target_type: "group", target_id: "g1", action: "on" },
    ]);
  });

  it("attaches a group's lighting once it has been moved off default", () => {
    expect(compileAction(action({ target_type: "group", target_id: "g1", level: 40 }))[0]).toEqual({
      target_type: "group",
      target_id: "g1",
      action: "on",
      level: 40,
    });
  });

  it("always carries the level on a 'set', which is what a set means", () => {
    const out = compileAction(
      action({ target_type: "room", target_id: "r1", action: "set", level: 100 }),
    );
    expect(out[0]).toEqual({ target_type: "room", target_id: "r1", action: "set", level: 100 });
  });
});

describe("compiling a per-lamp group action", () => {
  beforeEach(() => {
    store.value.sockets = [
      socket("ceiling", { protocol: "matter" }),
      socket("sink", { protocol: "matter" }),
      socket("fan", { protocol: "rf" }),
    ];
    store.value.groups = [{ id: "g1", socket_ids: ["ceiling", "sink", "fan"] }];
  });

  const perLamp = (over: Record<string, Partial<{ state: string; level: number; color: string }>>) =>
    action({
      target_type: "group",
      target_id: "g1",
      perLamp: {
        ceiling: { state: "on", level: 100, color: "", ...over.ceiling },
        sink: { state: "on", level: 100, color: "", ...over.sink },
        fan: { state: "on", level: 100, color: "", ...over.fan },
      } as RuleActionDraft["perLamp"],
    });

  it("becomes one socket action per member, not a group action", () => {
    const out = compileAction(perLamp({}));
    expect(out.map((a) => [a.target_type, a.target_id])).toEqual([
      ["socket", "ceiling"],
      ["socket", "sink"],
      ["socket", "fan"],
    ]);
  });

  it("drops a lamp marked unchanged — the whole point of authoring per lamp", () => {
    const out = compileAction(perLamp({ sink: { state: "ignore" } }));
    expect(out.map((a) => a.target_id)).toEqual(["ceiling", "fan"]);
  });

  it("gives lighting only to the members that can take it", () => {
    // The RF socket in the group is switched, not dimmed; sending it a level
    // would be a setting it has nowhere to put.
    const out = compileAction(perLamp({ ceiling: { level: 40, color: "f1c696" } }));
    expect(out.find((a) => a.target_id === "ceiling")).toEqual({
      target_type: "socket",
      target_id: "ceiling",
      action: "on",
      level: 40,
      color: "f1c696",
    });
    expect(out.find((a) => a.target_id === "fan")).toEqual({
      target_type: "socket",
      target_id: "fan",
      action: "on",
    });
  });

  it("carries no lighting on a lamp being turned off", () => {
    const out = compileAction(perLamp({ sink: { state: "off", level: 40 } }));
    expect(out.find((a) => a.target_id === "sink")).toEqual({
      target_type: "socket",
      target_id: "sink",
      action: "off",
    });
  });

  it("ignores the map on anything but a group or room 'on'", () => {
    const set = perLamp({});
    set.action = "set";
    expect(compileAction(set)).toEqual([
      { target_type: "group", target_id: "g1", action: "set", level: 100 },
    ]);
  });
});

describe("what a rule can point at", () => {
  it("matches a room's members by name, since sockets name their room", () => {
    store.value.sockets = [
      socket("a", { room: "Kitchen" }),
      socket("b", { room: "Hall" }),
      socket("c", { room: "Kitchen" }),
    ];
    store.value.rooms = [{ id: "r1", name: "Kitchen" }];
    expect(membersOf(action({ target_type: "room", target_id: "r1" })).map((s) => s.id)).toEqual([
      "a",
      "c",
    ]);
  });

  it("has no members for a socket or a scene", () => {
    expect(membersOf(action({ target_type: "socket" }))).toEqual([]);
    expect(membersOf(action({ target_type: "scene" }))).toEqual([]);
  });

  it("sorts rooms by name — the only list a person scans alphabetically", () => {
    store.value.rooms = [
      { id: "r2", name: "Study" },
      { id: "r1", name: "Attic" },
    ];
    expect(targetsFor("room").map((t) => t.label)).toEqual(["Attic", "Study"]);
  });

  it("starts a new row on the first kind of thing the house actually has", () => {
    store.value.scenes = [{ id: "sc1", name: "Evening" }];
    expect(firstTargetType()).toBe("scene");
    expect(blankRuleAction()).toEqual({
      target_type: "scene",
      target_id: "sc1",
      action: "activate",
      level: 100,
      color: "",
    });
  });

  it("still yields a usable row in an empty house", () => {
    // Nothing to point at, but the row has to exist for the form to draw.
    expect(blankRuleAction().target_id).toBe("");
  });
});
