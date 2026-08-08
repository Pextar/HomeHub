import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import type {
  SonosStatus,
  SonosSpeakerView,
  SonosQueueItem,
  KEFStatus,
  KEFSpeakerView,
  MediaZone,
  MediaHistory,
  MediaTopPlays,
  Listening,
  MusicTimerView,
  SonosFavorite,
  AnnounceStatus,
  SpotifyItem,
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
let historyFixture: MediaHistory;
let topFixture: MediaTopPlays;
let insightsFixture: Listening;
let timersFixture: MusicTimerView[];
let favoritesFixture: SonosFavorite[];
let announceFixture: AnnounceStatus;
let similarFixture: SpotifyItem[];
let savedFixture = false;
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
    sonosGroup: vi.fn(async () => {}),
    sonosSettings: vi.fn(async () => ({ sleep_minutes: 0 })),
    sonosFavorites: vi.fn(async () => favoritesFixture),
    sonosPlayFavorite: vi.fn(async () => {}),
    sonosSetVolume: vi.fn(async () => {}),
    sonosSetMute: vi.fn(async () => {}),
    sonosPause: vi.fn(async () => {}),
    kefPause: vi.fn(async () => {}),
    mediaZonePause: vi.fn(async () => {}),
    sonosQueueMove: vi.fn(async () => {}),
    // The reads the store makes on its own behalf as soon as it exists: what
    // this login may do with the library, what the room played before, and
    // whether there is anywhere to announce to. All three degrade to "no
    // control" rather than failing, which is what the fixtures below say.
    spotifyStatus: vi.fn(async () => ({ connected: true, library: true })),
    spotifySaved: vi.fn(async () => ({ saved: savedFixture })),
    spotifySetSaved: vi.fn(async () => {}),
    spotifySimilar: vi.fn(async () => similarFixture),
    sonosQueueAdd: vi.fn(async () => ({ track: 1, length: 1 })),
    sonosQueueAddMany: vi.fn(async () => ({ track: 1, length: 1, added: 1 })),
    mediaForgetPlay: vi.fn(async () => {}),
    mediaHistory: vi.fn(async () => historyFixture),
    mediaTopPlays: vi.fn(async () => topFixture),
    mediaInsights: vi.fn(async () => insightsFixture),
    musicTimers: vi.fn(async () => timersFixture),
    musicSleep: vi.fn(async () => ({ timer: {}, quiet_at: "" })),
    musicCreateTimer: vi.fn(async () => ({})),
    musicUpdateTimer: vi.fn(async () => ({})),
    musicDeleteTimer: vi.fn(async () => {}),
    musicCancelFade: vi.fn(async () => ({ cancelled: true })),
    announceStatus: vi.fn(async () => announceFixture),
    announce: vi.fn(async () => ({
      rooms: ["Kitchen"],
      unreachable: [],
      spoken: true,
      duration_ms: 3000,
    })),
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
  historyFixture = { plays: [], household: false };
  topFixture = { plays: [], by_hour: false, hour: 0 };
  insightsFixture = {
    plays: 0,
    items: 0,
    rooms: [],
    artists: [],
    top: [],
    hours: new Array(24).fill(0),
  };
  timersFixture = [];
  favoritesFixture = [];
  announceFixture = {
    available: true,
    rooms: [{ id: "kitchen", name: "Kitchen" }],
    voice: true,
    max_text: 200,
  };
  similarFixture = [];
  savedFixture = false;
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

    // One request, and the whole card in it: the hub walks the household
    // through the joins in order, which is what stops a wall that sleeps
    // mid-gesture from leaving the house half grouped.
    expect(api.sonosGroup).toHaveBeenCalledWith("kitchen", { join: ["bedroom", "study"] });
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

    expect(api.sonosGroup).toHaveBeenCalledWith("kitchen", { join: ["bedroom", "study"] });
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

    expect(api.sonosGroup).not.toHaveBeenCalled();
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

    expect(api.sonosGroup).toHaveBeenCalledWith("kitchen", { leave: ["bedroom", "study"] });
    // The coordinator carries the queue and the stream; it never leaves.
    expect(vi.mocked(api.sonosGroup).mock.calls[0][1].leave).not.toContain("kitchen");
    h.stop();
  });
});

describe("moving the music", () => {
  // "Put this in the kitchen as well" is grouping; "take it with me" is a
  // move, and it used to cost two aims at a wall. The order is the whole
  // rule: the destination joins while the old room is still coordinating,
  // so the handover doesn't drop the music between two calls.
  it("joins the destination before the old room leaves", async () => {
    sonosFixture = {
      speakers: [sonosSpeaker("kitchen"), sonosSpeaker("study")],
      groups: [
        { coordinator_id: "kitchen", member_ids: ["kitchen"] },
        { coordinator_id: "study", member_ids: ["study"] },
      ],
    } as SonosStatus;
    const h = await boot();
    h.value.selected = "s:kitchen";
    h.flush();

    h.value.moveTo(h.value.sources.find((s) => s.key === "s:study")!);
    h.flush();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();

    // The move is two calls and a re-read before the store follows the
    // sound; a macrotask drains all of it.
    await new Promise((r) => setTimeout(r, 0));
    h.flush();

    // The pair is stated in one request and the hub keeps the order: the
    // destination joins while the old room is still coordinating, so the
    // queue and the stream are handed over before anything steps out.
    expect(api.sonosGroup).toHaveBeenCalledWith("kitchen", {
      join: ["study"],
      leave: ["kitchen"],
    });
    // The wall follows the sound.
    expect(h.value.selected).toBe("s:study");
    h.stop();
  });

  it("has nothing to move to on a room that doesn't group", async () => {
    kefFixture = { speakers: [kefSpeaker("study")] };
    sonosFixture = {
      speakers: [sonosSpeaker("kitchen")],
      groups: [{ coordinator_id: "kitchen", member_ids: ["kitchen"] }],
    } as SonosStatus;
    const h = await boot();
    h.value.selected = "k:study";
    h.flush();

    h.value.moveTo(h.value.sources.find((s) => s.key === "s:kitchen")!);
    h.flush();
    await Promise.resolve();

    expect(api.sonosGroup).not.toHaveBeenCalled();
    h.stop();
  });
});

describe("the queue's order", () => {
  it("moves a track one place and re-reads rather than guessing", async () => {
    queueFixture = [
      { track: 1, title: "One" },
      { track: 2, title: "Two" },
      { track: 3, title: "Three" },
    ];
    const h = await boot();

    h.value.moveQueued(2, 1);
    h.flush();
    await Promise.resolve();
    await Promise.resolve();

    expect(api.sonosQueueMove).toHaveBeenCalledWith("kitchen", 2, 3);
    expect(api.sonosQueue).toHaveBeenCalled();
    h.stop();
  });

  // The ends of a list are where an off-by-one shows up as a speaker error.
  it("refuses to move past either end", async () => {
    queueFixture = [
      { track: 1, title: "One" },
      { track: 2, title: "Two" },
    ];
    const h = await boot();

    h.value.moveQueued(1, -1);
    h.value.moveQueued(2, 1);
    h.flush();
    await Promise.resolve();

    expect(api.sonosQueueMove).not.toHaveBeenCalled();
    h.stop();
  });
});

describe("saving what's playing", () => {
  it("only offers the heart where there is a track to save", async () => {
    sonosFixture = {
      speakers: [
        sonosSpeaker("kitchen", {
          state: {
            volume: 30,
            muted: false,
            playing: true,
            track: { title: "Radio 3", artist: "Live" },
          },
        } as Partial<SonosSpeakerView>),
      ],
      groups: [{ coordinator_id: "kitchen", member_ids: ["kitchen"] }],
    } as SonosStatus;
    const h = await boot();

    // Radio carries an artist line and no catalog id, so there is nothing
    // the library could be asked about.
    expect(h.value.featured?.trackURI).toBeUndefined();
    h.value.toggleSaved();
    h.flush();
    await Promise.resolve();
    expect(api.spotifySetSaved).not.toHaveBeenCalled();
    h.stop();
  });

  it("reads the saved state for a Spotify track and writes the flip", async () => {
    savedFixture = false;
    sonosFixture = {
      speakers: [
        sonosSpeaker("kitchen", {
          state: {
            volume: 30,
            muted: false,
            playing: true,
            track: { title: "Kaos", artist: "Bo Kaspers", spotify_uri: "spotify:track:abc" },
          },
        } as Partial<SonosSpeakerView>),
      ],
      groups: [{ coordinator_id: "kitchen", member_ids: ["kitchen"] }],
    } as SonosStatus;
    const h = await boot();
    await Promise.resolve();
    h.flush();

    expect(api.spotifySaved).toHaveBeenCalledWith("spotify:track:abc");

    h.value.toggleSaved();
    h.flush();
    // Optimistic: the heart is the confirmation, since a wall has nothing
    // else to show while the round trip completes.
    expect(h.value.saved).toBe(true);
    await Promise.resolve();
    expect(api.spotifySetSaved).toHaveBeenCalledWith("spotify:track:abc", true);
    h.stop();
  });
});

describe("more like this", () => {
  it("fills the queue behind what's playing on a Sonos room", async () => {
    similarFixture = [
      { kind: "track", uri: "spotify:track:1", name: "One" },
      { kind: "track", uri: "spotify:track:2", name: "Two" },
    ];
    sonosFixture = {
      speakers: [
        sonosSpeaker("kitchen", {
          state: {
            volume: 30,
            muted: false,
            playing: true,
            track: { title: "Kaos", artist: "Bo Kaspers" },
          },
        } as Partial<SonosSpeakerView>),
      ],
      groups: [{ coordinator_id: "kitchen", member_ids: ["kitchen"] }],
    } as SonosStatus;
    const h = await boot();

    expect(h.value.canRadio).toBe(true);
    h.value.startRadio();
    h.flush();
    for (let i = 0; i < 6; i++) await Promise.resolve();

    expect(api.spotifySimilar).toHaveBeenCalledWith("Bo Kaspers", 8);
    // The whole run in one request, forwards. This used to be one call per
    // track sent *backwards* — Sonos resolves each "play next" against the
    // queue as it is at that moment, so a reversed loop happened to come out
    // in order. The hub deals them into consecutive slots now, so the array
    // is simply the order they land in.
    expect(api.sonosQueueAddMany).toHaveBeenCalledTimes(1);
    const [room, run, next] = vi.mocked(api.sonosQueueAddMany).mock.calls[0];
    expect(room).toBe("kitchen");
    expect(run.map((i) => i.uri)).toEqual(["spotify:track:1", "spotify:track:2"]);
    expect(next).toBe(true);
    h.stop();
  });

  it("has nothing to seed from in a silent room", async () => {
    const h = await boot();
    expect(h.value.canRadio).toBe(false);
    h.value.startRadio();
    h.flush();
    await Promise.resolve();
    expect(api.spotifySimilar).not.toHaveBeenCalled();
    h.stop();
  });
});

describe("what the room played before", () => {
  it("asks per room, under the key the media layer files plays under", async () => {
    kefFixture = { speakers: [kefSpeaker("study")] };
    const h = await boot();
    h.value.selected = "k:study";
    h.flush();
    await Promise.resolve();
    await Promise.resolve();

    expect(api.mediaHistory).toHaveBeenCalledWith("kef:study", 12);
    h.stop();
  });

  // A favorite is replayed the way it was started. One that has since been
  // deleted from the household simply stops being offered — better than a
  // tile that fails.
  it("replays a Sonos favorite through the favorites path", async () => {
    historyFixture = {
      plays: [{ provider: "sonos", uri: "x-sonosapi-stream:p3", title: "P3", at: "" }],
      household: false,
    };
    favoritesFixture = [
      { id: "fv:1", title: "P3", uri: "x-sonosapi-stream:p3" } as SonosFavorite,
    ];
    const h = await boot();
    for (let i = 0; i < 4; i++) await Promise.resolve();
    h.flush();

    h.value.playFromHistory(h.value.history[0]);
    h.flush();
    await Promise.resolve();

    expect(api.sonosPlayFavorite).toHaveBeenCalledWith("kitchen", favoritesFixture[0]);
    h.stop();
  });

  it("drops a favorite the household has since deleted", async () => {
    historyFixture = {
      plays: [{ provider: "sonos", uri: "x-sonosapi-stream:gone", title: "Gone", at: "" }],
      household: false,
    };
    const h = await boot();
    for (let i = 0; i < 4; i++) await Promise.resolve();
    h.flush();

    h.value.playFromHistory(h.value.history[0]);
    h.flush();
    await Promise.resolve();

    expect(api.sonosPlayFavorite).not.toHaveBeenCalled();
    h.stop();
  });

  // The counterweight to a ranked shelf. What a room keeps coming back to
  // leads the wall, and a record started by mistake is exactly what gets
  // replayed — because it is the tile in the first slot.
  it("forgets one play from the featured room, and re-reads the shelf", async () => {
    historyFixture = {
      plays: [{ provider: "spotify", uri: "spotify:track:oops", title: "Oops", at: "" }],
      household: false,
    };
    const h = await boot();
    for (let i = 0; i < 4; i++) await Promise.resolve();
    h.flush();

    expect(h.value.canForget).toBe(true);
    h.value.forgetPlay(h.value.history[0]);
    h.flush();
    for (let i = 0; i < 4; i++) await Promise.resolve();

    expect(api.mediaForgetPlay).toHaveBeenCalledWith("sonos:kitchen", "spotify:track:oops");
    // The shelf is re-read rather than spliced: forgetting one entry can
    // change the ranking of everything left, and the ranking is the point.
    expect(api.mediaHistory).toHaveBeenCalledTimes(2);
    h.stop();
  });

  // The fallback shelf is other rooms' plays. One room is not the place to
  // edit them, so the control is absent rather than refused (§15.1).
  it("never forgets from the household's fallback list", async () => {
    historyFixture = {
      plays: [{ provider: "spotify", uri: "spotify:track:x", title: "X", at: "" }],
      household: true,
    };
    const h = await boot();
    for (let i = 0; i < 4; i++) await Promise.resolve();
    h.flush();

    expect(h.value.canForget).toBe(false);
    h.value.forgetPlay(h.value.history[0]);
    h.flush();
    await Promise.resolve();

    expect(api.mediaForgetPlay).not.toHaveBeenCalled();
    h.stop();
  });
});

describe("calling the house", () => {
  it("reports where an announcement would land", async () => {
    const h = await boot();
    await Promise.resolve();
    h.flush();

    expect(h.value.announce?.available).toBe(true);
    expect(h.value.announce?.voice).toBe(true);

    h.value.sendAnnouncement("Dinner's ready");
    h.flush();
    await Promise.resolve();
    await Promise.resolve();

    expect(api.announce).toHaveBeenCalledWith("Dinner's ready", undefined);
    expect(h.value.lastAnnounce?.rooms).toEqual(["Kitchen"]);
    h.stop();
  });
});

// ── What a room comes back to ────────────────────────────────────────────
// The plain history says what was on last; this says what the room keeps
// choosing, and at an hour it has a habit at, what it plays *then*. The
// rules worth guarding are the honest ones: the shelf may never claim a
// habit the backend didn't report, and a ranked answer is always this
// room's own — it never softens into the household's the way the plain
// list may.

describe("what this room comes back to", () => {
  it("ranks the room's plays at the hour it currently is", async () => {
    topFixture = {
      plays: [{ provider: "spotify", uri: "spotify:track:a", title: "Morning", at: "", count: 6 }],
      by_hour: true,
      hour: 8,
    };
    const h = await boot();
    for (let i = 0; i < 4; i++) await Promise.resolve();
    h.flush();

    expect(api.mediaTopPlays).toHaveBeenCalledWith("sonos:kitchen", { limit: 8, hour: "now" });
    expect(h.value.topPlays).toHaveLength(1);
    expect(h.value.topPlaysByHour).toBe(true);
    expect(h.value.topPlaysHour).toBe(8);
    h.stop();
  });

  // A room with no habit at this hour is answered with its favourites
  // overall, and the flag says so — the label above the shelf is the only
  // thing that separates the two claims.
  it("says when the ranking is not about this hour", async () => {
    topFixture = {
      plays: [{ provider: "spotify", uri: "spotify:track:b", title: "Anytime", at: "", count: 3 }],
      by_hour: false,
      hour: 20,
    };
    const h = await boot();
    for (let i = 0; i < 4; i++) await Promise.resolve();
    h.flush();

    expect(h.value.topPlaysByHour).toBe(false);
    h.stop();
  });

  // Two reads, two answers: a room with nothing ranked still has whatever
  // the plain shelf found, including the household's fallback, and the
  // ranked list stays empty rather than borrowing it.
  it("keeps the ranked list empty when the room has no habit of its own", async () => {
    historyFixture = {
      plays: [{ provider: "spotify", uri: "spotify:track:c", title: "Elsewhere", at: "" }],
      household: true,
    };
    const h = await boot();
    for (let i = 0; i < 4; i++) await Promise.resolve();
    h.flush();

    expect(h.value.topPlays).toEqual([]);
    expect(h.value.history).toHaveLength(1);
    expect(h.value.historyHousehold).toBe(true);
    h.stop();
  });
});

// ── Sleep and wake ───────────────────────────────────────────────────────
// HomeHub's own timers, which is what lets the wall set one on a KEF
// speaker or a HomeHub room at all — the sleep timer it used to set was a
// Sonos group's and reached neither.

describe("music timers", () => {
  function sleepIn(minutes: number, fadeMinutes: number, room = "sonos:kitchen"): MusicTimerView {
    return {
      id: "mt_1",
      room,
      room_name: room,
      action: "stop",
      enabled: true,
      fires_at: new Date(Date.now() + minutes * 60_000).toISOString(),
      fade_minutes: fadeMinutes,
      fading: false,
    };
  }

  it("sets a sleep timer on whichever room is featured, KEF included", async () => {
    sonosFixture = { speakers: [], groups: [] } as unknown as SonosStatus;
    kefFixture = { speakers: [kefSpeaker("study")] } as KEFStatus;
    const h = await boot();
    for (let i = 0; i < 4; i++) await Promise.resolve();
    h.flush();

    h.value.setSleepIn(40);
    h.flush();
    await Promise.resolve();

    expect(api.musicSleep).toHaveBeenCalledWith({ room: "kef:study", minutes: 40 });
    h.stop();
  });

  // The timer fires when the *fade* starts, so the minutes left are counted
  // to the silence at the end of it. Reading the fire time as the answer
  // would tell someone the room goes quiet five minutes before it does.
  it("counts down to the silence, not to the start of the fade", async () => {
    timersFixture = [sleepIn(30, 6)];
    const h = await boot();
    for (let i = 0; i < 4; i++) await Promise.resolve();
    h.flush();

    expect(h.value.sleepTimer?.id).toBe("mt_1");
    expect(h.value.sleepMinutesLeft).toBe(36);
    h.stop();
  });

  // A recurring stop is a standing instruction, not the "quiet in forty
  // minutes" someone just tapped — and another room's timer is not this
  // room's at all.
  it("takes only this room's one-shot stop as its sleep timer", async () => {
    timersFixture = [
      { ...sleepIn(30, 5, "sonos:bedroom"), id: "mt_other" },
      {
        id: "mt_nightly",
        room: "sonos:kitchen",
        room_name: "kitchen",
        action: "stop",
        enabled: true,
        time: "23:00",
        fading: false,
      },
    ];
    const h = await boot();
    for (let i = 0; i < 4; i++) await Promise.resolve();
    h.flush();

    expect(h.value.sleepTimer).toBeUndefined();
    expect(h.value.roomTimers.map((t) => t.id)).toEqual(["mt_nightly"]);
    h.stop();
  });

  // "I'm still up": the ramp stops, the volume goes back and the music
  // keeps playing — which is why it is a different call from deleting.
  it("reports a ramp in flight and cancels it by room", async () => {
    timersFixture = [{ ...sleepIn(2, 5), fading: true }];
    const h = await boot();
    for (let i = 0; i < 4; i++) await Promise.resolve();
    h.flush();

    expect(h.value.fading).toBe(true);

    h.value.cancelFade();
    h.flush();
    await Promise.resolve();

    expect(api.musicCancelFade).toHaveBeenCalledWith("sonos:kitchen");
    h.stop();
  });

  it("sets a wake-up with the room, the days and something to play", async () => {
    const h = await boot();
    for (let i = 0; i < 4; i++) await Promise.resolve();
    h.flush();

    h.value.setWake({
      time: "06:45",
      days: [1, 2, 3, 4, 5],
      volume: 20,
      fadeMinutes: 10,
      item: { uri: "spotify:playlist:x", title: "Mornings" },
    });
    h.flush();
    await Promise.resolve();

    expect(api.musicCreateTimer).toHaveBeenCalledWith(
      expect.objectContaining({
        room: "sonos:kitchen",
        action: "start",
        time: "06:45",
        days: [1, 2, 3, 4, 5],
        volume: 20,
        fade_minutes: 10,
      }),
    );
    h.stop();
  });

  // An alarm with nothing to put on is refused here rather than by the
  // backend: the panel offers the times only where it has something to
  // point them at, and this is the guard behind that.
  it("refuses a wake-up with nothing to play", async () => {
    const h = await boot();
    for (let i = 0; i < 4; i++) await Promise.resolve();
    h.flush();

    h.value.setWake({ time: "06:45", days: [], item: { title: "Nothing" } });
    h.flush();
    await Promise.resolve();

    expect(api.musicCreateTimer).not.toHaveBeenCalled();
    h.stop();
  });

  // The kid surface may only drive Sonos and the timer endpoints are
  // admin-only, so asking would be a guaranteed 403 on every load of a
  // screen with no control to draw with the answer.
  it("never asks for timers or insights on the kid surface", async () => {
    const h = withRoot(() => createPanelMusic({ sonosOnly: true }));
    await h.value.refresh();
    h.flush();
    for (let i = 0; i < 4; i++) await Promise.resolve();
    h.flush();

    expect(api.musicTimers).not.toHaveBeenCalled();
    expect(api.mediaInsights).not.toHaveBeenCalled();
    h.stop();
  });
});
