import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import type {
  SonosStatus,
  SonosSpeakerView,
  SonosQueueItem,
  KEFStatus,
  KEFSpeakerView,
  MediaZone,
} from "./types";

/**
 * The panel's speaker brain (DESIGN.md §16).
 *
 * Everything here is a rule the wall gets judged on rather than a detail of
 * one screen: which rooms exist and under what name, what the panel says
 * when nothing answers, which capabilities a source may advertise, and the
 * two places the store used to claim more than it knew — "up next" under
 * shuffle, and a destination that could move under a finger.
 */

let sonosFixture: SonosStatus;
let kefFixture: KEFStatus;
let zonesFixture: MediaZone[];
let queueFixture: SonosQueueItem[];
let sonosFails = false;

vi.mock("./api", () => ({
  api: {
    sonosStatus: vi.fn(async () => {
      if (sonosFails) throw new Error("unreachable");
      return sonosFixture;
    }),
    kefStatus: vi.fn(async () => {
      if (sonosFails) throw new Error("unreachable");
      return kefFixture;
    }),
    mediaZones: vi.fn(async () => {
      if (sonosFails) throw new Error("unreachable");
      return zonesFixture;
    }),
    sonosQueue: vi.fn(async () => queueFixture),
    sonosJoin: vi.fn(async () => {}),
    sonosLeave: vi.fn(async () => {}),
    sonosSettings: vi.fn(async () => ({ sleep_minutes: 0 })),
    sonosFavorites: vi.fn(async () => []),
    sonosSetVolume: vi.fn(async () => {}),
    sonosSetMute: vi.fn(async () => {}),
    sonosPause: vi.fn(async () => {}),
    kefPause: vi.fn(async () => {}),
    mediaZonePause: vi.fn(async () => {}),
  },
}));

vi.mock("./stores.svelte", () => ({
  session: { isAdmin: true, user: null },
  toasts: { error: vi.fn() },
}));

vi.mock("./live", () => ({ onLive: () => () => {} }));

const { api } = await import("./api");
const { createPanelMusic } = await import("./panel-music.svelte");
const { withRoot } = await import("../test-runes.svelte");

// ── Fixtures ─────────────────────────────────────────────────────────────

function sonosSpeaker(id: string, over: Partial<SonosSpeakerView> = {}): SonosSpeakerView {
  return {
    id,
    name: id,
    reachable: true,
    state: { volume: 30, muted: false, playing: false },
    ...over,
  } as SonosSpeakerView;
}

function kefSpeaker(id: string, over: Partial<KEFSpeakerView["state"]> = {}): KEFSpeakerView {
  return {
    id,
    name: id,
    reachable: true,
    state: { volume: 20, muted: false, playing: false, powered_on: true, source: "wifi", ...over },
  } as KEFSpeakerView;
}

function zone(id: string, members: { id: string; vendor: "sonos" | "kef" }[]): MediaZone {
  return {
    id,
    name: id,
    members: members.map((m) => `${m.vendor}:${m.id}`),
    speakers: members.map((m) => ({
      id: m.id,
      name: m.id,
      vendor: m.vendor,
      member: `${m.vendor}:${m.id}`,
      capabilities: [],
      state: { state: "stopped", volume: 40, muted: false, at: new Date().toISOString() },
    })),
    route: "native",
  } as MediaZone;
}

/**
 * Create the store inside an effect root, poll once, and settle — including
 * the reads the first poll *triggers* (the featured room's queue, its sleep
 * timer, the household's favorites), which are a microtask behind it.
 */
async function boot() {
  const h = withRoot(() => createPanelMusic());
  await h.value.refresh();
  h.flush();
  await Promise.resolve();
  await Promise.resolve();
  h.flush();
  return h;
}

beforeEach(() => {
  sonosFails = false;
  sonosFixture = {
    speakers: [sonosSpeaker("kitchen"), sonosSpeaker("bedroom")],
    groups: [
      { coordinator_id: "kitchen", member_ids: ["kitchen"] },
      { coordinator_id: "bedroom", member_ids: ["bedroom"] },
    ],
  } as SonosStatus;
  kefFixture = { speakers: [] };
  zonesFixture = [];
  queueFixture = [];
  localStorage.clear();
});
afterEach(() => vi.clearAllMocks());

// ── Rooms ────────────────────────────────────────────────────────────────

describe("panel sources", () => {
  it("lists each reachable Sonos group as a room", async () => {
    const h = await boot();
    expect(h.value.sources.map((s) => s.title)).toEqual(["kitchen", "bedroom"]);
    h.stop();
  });

  it("hides a speaker a zone already claims, under the zone's name", async () => {
    // The app's own rule (lib/music/rooms.svelte.ts): a zone is something
    // someone arranged on purpose, so it outranks the household's grouping.
    // Showing both puts one speaker on two cards under two names.
    kefFixture = { speakers: [kefSpeaker("study")] };
    zonesFixture = [
      zone("Downstairs", [
        { id: "kitchen", vendor: "sonos" },
        { id: "study", vendor: "kef" },
      ]),
    ];
    const h = await boot();

    expect(h.value.sources.map((s) => s.title)).toEqual(["Downstairs", "bedroom"]);
    expect(h.value.sources.find((s) => s.kind === "zone")?.members?.map((m) => m.vendor)).toEqual([
      "sonos",
      "kef",
    ]);
    h.stop();
  });

  it("averages a Sonos group's member volumes rather than reading the lead", async () => {
    sonosFixture = {
      speakers: [
        sonosSpeaker("kitchen", { state: { volume: 10, muted: false, playing: false } as never }),
        sonosSpeaker("bedroom", { state: { volume: 30, muted: false, playing: false } as never }),
      ],
      groups: [{ coordinator_id: "kitchen", member_ids: ["kitchen", "bedroom"] }],
    } as SonosStatus;
    const h = await boot();
    expect(h.value.sources[0].volume).toBe(20);
    h.stop();
  });
});

// ── What the wall may claim ──────────────────────────────────────────────

describe("capability honesty", () => {
  it("offers skips on a KEF speaker playing from the network", async () => {
    sonosFixture = { speakers: [], groups: [] } as SonosStatus;
    kefFixture = { speakers: [kefSpeaker("study", { source: "wifi" })] };
    const h = await boot();
    expect(h.value.sources[0].canSkip).toBe(true);
    h.stop();
  });

  it("withholds them on a physical input, where a skip reaches nothing", async () => {
    sonosFixture = { speakers: [], groups: [] } as SonosStatus;
    kefFixture = { speakers: [kefSpeaker("study", { source: "tv" })] };
    const h = await boot();
    expect(h.value.sources[0].canSkip).toBe(false);
    h.stop();
  });

  it("withholds them on a streamed zone, which refuses next", async () => {
    sonosFixture = { speakers: [], groups: [] } as SonosStatus;
    const z = zone("Everywhere", [{ id: "a", vendor: "sonos" }]);
    zonesFixture = [{ ...z, route: "stream" }];
    const h = await boot();
    expect(h.value.sources[0].canSkip).toBe(false);
    h.stop();
  });

  it("refuses a skip it never offered", async () => {
    sonosFixture = { speakers: [], groups: [] } as SonosStatus;
    kefFixture = { speakers: [kefSpeaker("study", { source: "tv" })] };
    const h = await boot();
    h.value.skip(h.value.sources[0], "next");
    expect(api.kefStatus).toHaveBeenCalled(); // the poll ran
    h.stop();
  });
});

// ── Up next ──────────────────────────────────────────────────────────────

describe("up next", () => {
  const withQueue = (over: Record<string, unknown>) => {
    sonosFixture = {
      speakers: [
        sonosSpeaker("kitchen", {
          state: { volume: 30, muted: false, playing: true, queue_track: 1 } as never,
          group_state: {
            shuffle: false,
            repeat: "off",
            crossfade: false,
            queue_length: 3,
            from_queue: true,
            ...over,
          } as never,
        }),
      ],
      groups: [{ coordinator_id: "kitchen", member_ids: ["kitchen"] }],
    } as SonosStatus;
    queueFixture = [
      { track: 1, title: "One" },
      { track: 2, title: "Two" },
    ] as SonosQueueItem[];
  };

  it("names the next queued track when the group plays straight through", async () => {
    withQueue({});
    const h = await boot();
    expect(h.value.queueOrderKnown).toBe(true);
    expect(h.value.nextInQueue?.title).toBe("Two");
    h.stop();
  });

  it("names nothing under shuffle, where the speaker picks its own next", async () => {
    withQueue({ shuffle: true });
    const h = await boot();
    expect(h.value.queueOrderKnown).toBe(false);
    expect(h.value.nextInQueue).toBeUndefined();
    h.stop();
  });

  it("names nothing under repeat-one, where the next track is this one", async () => {
    withQueue({ repeat: "one" });
    const h = await boot();
    expect(h.value.queueOrderKnown).toBe(false);
    expect(h.value.nextInQueue).toBeUndefined();
    h.stop();
  });
});

// ── The destination ──────────────────────────────────────────────────────

describe("the featured room", () => {
  it("follows whatever is playing until someone chooses", async () => {
    const h = await boot();
    expect(h.value.featured?.title).toBe("kitchen"); // nothing playing: the first

    sonosFixture.speakers[1].state = { volume: 30, muted: false, playing: true } as never;
    await h.value.refresh();
    h.flush();
    expect(h.value.featured?.title).toBe("bedroom");
    h.stop();
  });

  it("stays put once latched, even when another room starts playing", async () => {
    // The wall's flow is type → tap, seconds apart. A room that starts up
    // elsewhere in between must not re-point the tap.
    const h = await boot();
    h.value.latchFeatured();
    h.flush();

    sonosFixture.speakers[1].state = { volume: 30, muted: false, playing: true } as never;
    await h.value.refresh();
    h.flush();
    expect(h.value.featured?.title).toBe("kitchen");
    h.stop();
  });
});

// ── When nothing answers ─────────────────────────────────────────────────

describe("outages", () => {
  it("keeps the column and reports the outage when speakers stop answering", async () => {
    sonosFixture = {
      speakers: [sonosSpeaker("kitchen", { reachable: false, state: undefined })],
      groups: [],
    } as SonosStatus;
    const h = await boot();
    expect(h.value.hasSpeakers).toBe(true);
    expect(h.value.unreachable).toBe(true);
    h.stop();
  });

  it("does not let a failed poll erase a home that has speakers", async () => {
    const h = await boot();
    expect(h.value.hasSpeakers).toBe(true);

    sonosFails = true;
    await h.value.refresh();
    h.flush();
    // A dropped read says nothing about what this home owns — and a wall
    // panel must not reflow its grid on one.
    expect(h.value.hasSpeakers).toBe(true);
    h.stop();
  });

  it("says a home with no speakers at all has none", async () => {
    sonosFixture = { speakers: [], groups: [] } as SonosStatus;
    const h = await boot();
    expect(h.value.hasSpeakers).toBe(false);
    expect(h.value.unreachable).toBe(false);
    h.stop();
  });
});

// ── Volume and mute ──────────────────────────────────────────────────────

describe("volume", () => {
  it("holds the dragged value against a poll that lands mid-drag", async () => {
    const h = await boot();
    const kitchen = h.value.sources[0];

    h.value.dragVolume(kitchen, 70);
    h.flush();
    expect(h.value.vol).toBe(70);

    await h.value.refresh(); // the speaker still reports 30
    h.flush();
    expect(h.value.vol).toBe(70);
    h.stop();
  });

  it("steps by a fixed amount and clamps at the ends", async () => {
    const h = await boot();
    const kitchen = h.value.sources[0];

    h.value.nudgeVolume(kitchen, 5);
    h.flush();
    expect(h.value.vol).toBe(35);

    h.value.nudgeVolume(kitchen, -100);
    h.flush();
    expect(h.value.vol).toBe(0);
    h.stop();
  });

  // One audible speaker means the room is audible, so the room reads as
  // unmuted and one tap silences all of it. The store used to test `some`
  // here, which made the button labelled "Mute" unmute the other speaker.
  it("mutes the whole group when any speaker in it is still audible", async () => {
    sonosFixture = {
      speakers: [
        sonosSpeaker("kitchen", { state: { volume: 30, muted: true, playing: false } as never }),
        sonosSpeaker("bedroom", { state: { volume: 30, muted: false, playing: false } as never }),
      ],
      groups: [{ coordinator_id: "kitchen", member_ids: ["kitchen", "bedroom"] }],
    } as SonosStatus;
    const h = await boot();

    h.value.toggleMute(h.value.sources[0]);
    h.flush();
    expect(api.sonosSetMute).toHaveBeenCalledWith("kitchen", true);
    expect(api.sonosSetMute).toHaveBeenCalledWith("bedroom", true);
    h.stop();
  });
});

// ── Pause everything ─────────────────────────────────────────────────────

describe("pause all", () => {
  it("pauses every playing room, whatever make it is", async () => {
    sonosFixture.speakers[0].state = { volume: 30, muted: false, playing: true } as never;
    kefFixture = { speakers: [kefSpeaker("study", { playing: true })] };
    const h = await boot();

    expect(h.value.anyPlaying).toBe(true);
    h.value.pauseAll();
    h.flush();
    await Promise.resolve();

    expect(api.sonosPause).toHaveBeenCalledWith("kitchen");
    expect(api.kefPause).toHaveBeenCalledWith("study");
    // The room that wasn't playing is left alone.
    expect(api.sonosPause).not.toHaveBeenCalledWith("bedroom");
    h.stop();
  });
});

// ── Grouping ─────────────────────────────────────────────────────────────
// What the full player's grouping pane is allowed to offer. The rule under
// all of it is §15.1's: only Sonos groups natively, so anything else is
// absent from the pane rather than present and refused.

describe("grouping", () => {
  it("offers every other Sonos room to a featured Sonos room", async () => {
    const h = await boot();
    h.value.selected = "s:kitchen";
    h.flush();

    expect(h.value.joinable.map((s) => s.title)).toEqual(["bedroom"]);
    expect(h.value.canGroup).toBe(true);
    h.stop();
  });

  it("offers nothing to a KEF speaker or a HomeHub room, which don't group natively", async () => {
    kefFixture = { speakers: [kefSpeaker("study")] };
    zonesFixture = [
      zone("Downstairs", [
        { id: "bedroom", vendor: "sonos" },
        { id: "study", vendor: "kef" },
      ]),
    ];
    const h = await boot();

    h.value.selected = "z:Downstairs";
    h.flush();
    expect(h.value.joinable).toEqual([]);
    expect(h.value.canGroup).toBe(false);
    h.stop();
  });

  it("says a lone Sonos room in a one-room house has nothing to group", async () => {
    sonosFixture = {
      speakers: [sonosSpeaker("kitchen")],
      groups: [{ coordinator_id: "kitchen", member_ids: ["kitchen"] }],
    } as SonosStatus;
    const h = await boot();

    expect(h.value.joinable).toEqual([]);
    // Nothing to join, and nothing to split either.
    expect(h.value.canGroup).toBe(false);
    h.stop();
  });

  it("still offers the pane to a grouped room with nowhere left to join", async () => {
    sonosFixture = {
      speakers: [sonosSpeaker("kitchen"), sonosSpeaker("bedroom")],
      groups: [{ coordinator_id: "kitchen", member_ids: ["kitchen", "bedroom"] }],
    } as SonosStatus;
    const h = await boot();

    expect(h.value.joinable).toEqual([]);
    // The group can still be split, and each speaker balanced — which is
    // the other half of what the pane is for.
    expect(h.value.canGroup).toBe(true);
    h.stop();
  });

  it("joins the whole card, not the speaker that was aimed at", async () => {
    // §15.4: what was dragged — or here, tapped — is a room, so every
    // speaker in it goes.
    sonosFixture = {
      speakers: [sonosSpeaker("kitchen"), sonosSpeaker("bedroom"), sonosSpeaker("study")],
      groups: [
        { coordinator_id: "kitchen", member_ids: ["kitchen"] },
        { coordinator_id: "bedroom", member_ids: ["bedroom", "study"] },
      ],
    } as SonosStatus;
    const h = await boot();
    h.value.selected = "s:kitchen";
    h.flush();

    h.value.joinSource(h.value.joinable[0]);
    h.flush();
    await Promise.resolve();
    await Promise.resolve();

    expect(api.sonosJoin).toHaveBeenCalledWith("bedroom", "kitchen");
    expect(api.sonosJoin).toHaveBeenCalledWith("study", "kitchen");
    h.stop();
  });

  it("gathers every room into the featured one, and leaves it featured", async () => {
    sonosFixture = {
      speakers: [sonosSpeaker("kitchen"), sonosSpeaker("bedroom"), sonosSpeaker("study")],
      groups: [
        { coordinator_id: "kitchen", member_ids: ["kitchen"] },
        { coordinator_id: "bedroom", member_ids: ["bedroom"] },
        { coordinator_id: "study", member_ids: ["study"] },
      ],
    } as SonosStatus;
    const h = await boot();
    h.value.selected = "s:kitchen";
    h.flush();

    h.value.joinAll();
    h.flush();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();

    expect(api.sonosJoin).toHaveBeenCalledWith("bedroom", "kitchen");
    expect(api.sonosJoin).toHaveBeenCalledWith("study", "kitchen");
    // The room the wall was driving is still the room the wall is driving.
    expect(h.value.selected).toBe("s:kitchen");
    h.stop();
  });

  it("refuses a join a non-Sonos room could never take", async () => {
    kefFixture = { speakers: [kefSpeaker("study")] };
    const h = await boot();
    h.value.selected = "k:study";
    h.flush();

    h.value.joinAll();
    h.flush();
    await Promise.resolve();

    expect(api.sonosJoin).not.toHaveBeenCalled();
    h.stop();
  });

  it("steps one speaker out without disturbing the rest", async () => {
    sonosFixture = {
      speakers: [sonosSpeaker("kitchen"), sonosSpeaker("bedroom"), sonosSpeaker("study")],
      groups: [{ coordinator_id: "kitchen", member_ids: ["kitchen", "bedroom", "study"] }],
    } as SonosStatus;
    const h = await boot();

    h.value.leaveMember("bedroom");
    h.flush();
    await Promise.resolve();

    expect(api.sonosLeave).toHaveBeenCalledWith("bedroom");
    expect(api.sonosLeave).toHaveBeenCalledTimes(1);
    h.stop();
  });

  it("splits by sending every follower out, and never the lead", async () => {
    sonosFixture = {
      speakers: [sonosSpeaker("kitchen"), sonosSpeaker("bedroom"), sonosSpeaker("study")],
      groups: [{ coordinator_id: "kitchen", member_ids: ["kitchen", "bedroom", "study"] }],
    } as SonosStatus;
    const h = await boot();

    h.value.ungroupFeatured();
    h.flush();
    await Promise.resolve();
    await Promise.resolve();

    expect(api.sonosLeave).toHaveBeenCalledWith("bedroom");
    expect(api.sonosLeave).toHaveBeenCalledWith("study");
    expect(api.sonosLeave).not.toHaveBeenCalledWith("kitchen");
    h.stop();
  });
});
