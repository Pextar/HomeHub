import { describe, it, expect } from "vitest";
import {
  complete,
  deviceInRoom,
  extractAction,
  hostOf,
  norm,
  resolveGroup,
  resolveRoom,
  resolveTarget,
  roomNames,
  WHOLE,
  type Vocabulary,
} from "./console-language";
import type { Group, Socket } from "./types";

/**
 * What the console understands.
 *
 * These are the rules a person notices being right or wrong — whether "the
 * kitchen" finds the kitchen, whether "lamp off" works as well as "turn off
 * lamp", whether naming a thing exactly beats a longer name containing it.
 * All of it lived inside a view and had no test.
 */

const socket = (name: string, room = "Kitchen", over: Partial<Socket> = {}): Socket =>
  ({ id: name, name, room, protocol: "rf", ...over }) as Socket;
const group = (name: string, socket_ids: string[] = []): Group =>
  ({ id: name, name, socket_ids }) as Group;

const house = (over: Partial<Vocabulary> = {}): Vocabulary => ({
  sockets: [],
  groups: [],
  scenes: [],
  ...over,
});

describe("naming a device", () => {
  it("is room/device, hyphenated — a name you can type back in", () => {
    expect(hostOf(socket("Reading Lamp", "Living Room"))).toBe("living-room/reading-lamp");
  });

  it("files a socket with no room under 'unassigned'", () => {
    expect(hostOf(socket("Fan", ""))).toBe("unassigned/fan");
  });
});

describe("resolving a name", () => {
  const v = house({
    sockets: [socket("Lamp", "Kitchen"), socket("Kitchen Fan", "Utility")],
    groups: [group("Kitchen Lights")],
  });

  it("finds a device by its plain name", () => {
    expect(resolveTarget(v, "Lamp")).toEqual({ kind: "device", socket: v.sockets[0] });
  });

  it("finds a device by its room/device name", () => {
    expect(resolveTarget(v, "kitchen/lamp")).toEqual({ kind: "device", socket: v.sockets[0] });
  });

  it("ignores a leading 'the' or 'my'", () => {
    expect(resolveTarget(v, "the Lamp")).toEqual({ kind: "device", socket: v.sockets[0] });
    expect(resolveTarget(v, "my Lamp")).toEqual({ kind: "device", socket: v.sockets[0] });
  });

  it("lets an exact name beat a longer one that merely contains it", () => {
    // "Kitchen" is a room exactly; it is also inside the device "Kitchen Fan"
    // and the group "Kitchen Lights". The exact pass has to run first or the
    // room becomes unreachable by its own name.
    expect(resolveTarget(v, "Kitchen")).toEqual({ kind: "room", name: "Kitchen" });
  });

  it("falls back to a substring, still device before group before room", () => {
    expect(resolveTarget(v, "fan")).toEqual({ kind: "device", socket: v.sockets[1] });
    expect(resolveTarget(v, "lights")).toEqual({ kind: "group", group: v.groups[0] });
  });

  it("answers nothing rather than guessing", () => {
    expect(resolveTarget(v, "greenhouse")).toBeUndefined();
    expect(resolveTarget(v, "   ")).toBeUndefined();
  });
});

describe("resolving a room", () => {
  const sockets = [socket("A", "Living Room"), socket("B", "Kitchen")];

  it("lists each room once", () => {
    expect(roomNames([...sockets, socket("C", "Kitchen")])).toEqual(["Living Room", "Kitchen"]);
  });

  it("matches in both directions, since a room's name and its nickname differ", () => {
    expect(resolveRoom(sockets, "living room")).toBe("Living Room");
    expect(resolveRoom(sockets, "living")).toBe("Living Room");
  });

  it("finds a device inside one room and not the same name elsewhere", () => {
    const both = [socket("Lamp", "Kitchen"), socket("Lamp", "Study")];
    expect(deviceInRoom(both, "Lamp", "Study")?.room).toBe("Study");
    expect(deviceInRoom(both, "Lamp", "Attic")).toBeUndefined();
  });
});

describe("resolving a group", () => {
  const groups = [group("Lights"), group("Kitchen Lights")];

  it("prefers the exact name over one containing it", () => {
    expect(resolveGroup(groups, "Lights")?.name).toBe("Lights");
  });

  it("falls back to a substring", () => {
    expect(resolveGroup(groups, "kitchen")?.name).toBe("Kitchen Lights");
  });
});

describe("reading a sentence", () => {
  it("drops filler that never changes what a line does", () => {
    expect(norm("Turn off the lamp, please.")).toBe("turn off lamp");
  });

  it("keeps the words that do — 'in' separates a device from its room", () => {
    expect(norm("turn on the lamp in the study")).toBe("turn on lamp in study");
  });

  it("takes the verb from the front", () => {
    expect(extractAction(["turn", "off", "lamp"])).toEqual({ action: "off", rest: ["lamp"] });
    expect(extractAction(["off", "lamp"])).toEqual({ action: "off", rest: ["lamp"] });
    expect(extractAction(["toggle", "lamp"])).toEqual({ action: "toggle", rest: ["lamp"] });
  });

  it("takes it from the back too, because people write both ways round", () => {
    expect(extractAction(["lamp", "off"])).toEqual({ action: "off", rest: ["lamp"] });
    expect(extractAction(["turn", "lamp", "off"])).toEqual({ action: "off", rest: ["lamp"] });
  });

  it("won't read 'turn' on its own as a verb", () => {
    // "turn the lamp" says nothing about which way, and guessing would be a
    // console that switches things off when you didn't ask.
    expect(extractAction(["turn", "lamp"])).toBeNull();
    expect(extractAction(["lamp"])).toBeNull();
  });

  it("knows the words that mean 'all of it'", () => {
    expect(WHOLE.has("everything")).toBe(true);
    expect(WHOLE.has("lights")).toBe(true);
    expect(WHOLE.has("lamp")).toBe(false);
  });
});

describe("what Tab offers", () => {
  const v = house({
    sockets: [socket("Lamp"), socket("Ceiling")],
    groups: [group("Lights")],
    scenes: [{ name: "Evening" }],
  });

  it("offers verbs before names on a bare line", () => {
    const c = complete(v, "to");
    expect(c.head).toBe("");
    expect(c.matches).toEqual(["toggle "]);
  });

  it("offers only names once a verb has been typed", () => {
    const c = complete(v, "turn off l");
    expect(c.head).toBe("turn off ");
    // Every name starting with "l" — and no verbs, since none can follow.
    expect(c.matches).toEqual(["Lamp", "Lights"]);
  });

  it("keeps what was typed as the head, so completing never eats the verb", () => {
    expect(complete(v, "  toggle ceil").head).toBe("  toggle ");
  });

  it("offers nothing for a fragment that is already a whole name", () => {
    // Completing it to itself looks like Tab doing nothing, and cycling on to
    // a longer name would take away what was typed.
    expect(complete(v, "toggle Lamp").matches).toEqual([]);
  });

  it("reaches rooms and scenes, not just devices and groups", () => {
    expect(complete(v, "scene ").matches).toContain("Evening");
    expect(complete(v, "on ").matches).toContain("Kitchen");
  });
});
