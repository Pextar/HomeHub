import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import type { MediaZone, MediaZoneSpeaker } from "../types";

vi.mock("../api", () => ({
  api: {
    mediaZones: vi.fn(async () => zonesFixture),
    mediaEndpoints: vi.fn(async () => []),
  },
}));

const { createZonesBridge } = await import("./zones.svelte");
const { createBusy } = await import("./busy.svelte");

/**
 * The two things the zone layer owes the rest of the module, and neither is
 * about talking to a speaker:
 *
 *   which speakers a playing zone speaks for — Home drops their own cards, so
 *   one piece of music is listed once rather than three times under three
 *   names;
 *
 *   and which zone a start would silence, because the `stream` and `connect`
 *   routes both hold the Spotify account's single active session.
 */
let zonesFixture: MediaZone[] = [];

function speaker(
  over: Partial<MediaZoneSpeaker> & Pick<MediaZoneSpeaker, "id" | "vendor">,
): MediaZoneSpeaker {
  return {
    name: over.id,
    member: `${over.vendor}:${over.id}`,
    capabilities: [],
    ...over,
  } as MediaZoneSpeaker;
}

function playingState(volume = 30) {
  return { state: "playing" as const, volume, muted: false, at: new Date().toISOString() };
}

const make = async () => {
  const zones = createZonesBridge(createBusy());
  await zones.refresh();
  return zones;
};

describe("zone membership and what it stands for", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("claims its speakers while it plays, so their own cards can stand down", async () => {
    zonesFixture = [
      {
        id: "z1",
        name: "Downstairs",
        members: ["sonos:living", "kef:kitchen"],
        route: "stream",
        sync: "buffered",
        speakers: [
          speaker({ id: "living", vendor: "sonos", state: playingState() }),
          speaker({ id: "kitchen", vendor: "kef", state: playingState() }),
        ],
      },
    ];
    const zones = await make();
    expect(zones.playing).toHaveLength(1);
    expect([...zones.playingSonosIds]).toEqual(["living"]);
    expect([...zones.playingKefIds]).toEqual(["kitchen"]);
    expect(zones.isMixed(zones.zones[0])).toBe(true);
  });

  it("claims nothing while it is only paused", async () => {
    zonesFixture = [
      {
        id: "z1",
        name: "Downstairs",
        members: ["sonos:living"],
        speakers: [
          speaker({
            id: "living",
            vendor: "sonos",
            state: { state: "paused", volume: 20, muted: false, at: new Date().toISOString() },
          }),
        ],
      },
    ];
    const zones = await make();
    // A paused zone is not playing, so the room inside it is free to speak
    // for itself again — "Playing now" means playing, literally.
    expect(zones.playing).toHaveLength(0);
    expect(zones.playingSonosIds.size).toBe(0);
  });

  it("names the zone a Spotify-session route would silence", async () => {
    zonesFixture = [
      {
        id: "live",
        name: "Downstairs",
        members: ["sonos:living", "kef:kitchen"],
        route: "stream",
        sync: "buffered",
        speakers: [
          speaker({ id: "living", vendor: "sonos", state: playingState() }),
          speaker({ id: "kitchen", vendor: "kef", state: playingState() }),
        ],
      },
      {
        id: "idle-mixed",
        name: "Upstairs",
        members: ["sonos:bed", "kef:study"],
        route: "stream",
        sync: "buffered",
        speakers: [
          speaker({ id: "bed", vendor: "sonos" }),
          speaker({ id: "study", vendor: "kef" }),
        ],
      },
      {
        id: "idle-sonos",
        name: "Loft",
        members: ["sonos:loft"],
        route: "native",
        sync: "single",
        speakers: [speaker({ id: "loft", vendor: "sonos" })],
      },
    ];
    const zones = await make();
    const byId = (id: string) => zones.zones.find((z) => z.id === id)!;

    // Two streamed zones, one session: starting the second stops the first.
    expect(zones.wouldInterrupt(byId("idle-mixed"))?.name).toBe("Downstairs");
    // A Sonos-only zone plays from the household's own account link and holds
    // nothing, so it interrupts nothing — the rule that keeps cross-vendor
    // playback strictly additive.
    expect(zones.wouldInterrupt(byId("idle-sonos"))).toBeNull();
    // And a zone never reports itself.
    expect(zones.wouldInterrupt(byId("live"))).toBeNull();
  });

  it("shows the mean of its speakers, then the finger's own value", async () => {
    zonesFixture = [
      {
        id: "z1",
        name: "Downstairs",
        members: ["sonos:living", "kef:kitchen"],
        speakers: [
          speaker({ id: "living", vendor: "sonos", state: playingState(20) }),
          speaker({ id: "kitchen", vendor: "kef", state: playingState(40) }),
        ],
      },
    ];
    const zones = await make();
    const z = zones.zones[0];
    // The mean, because a zone-wide set writes one level to every speaker —
    // showing the loudest would jump the fader the moment it was touched.
    expect(zones.shownVolume(z)).toBe(30);

    zones.dragVolume(z, 55);
    expect(zones.shownVolume(z)).toBe(55);
    vi.advanceTimersByTime(4001);
    expect(zones.shownVolume(z)).toBe(30); // the speakers are the authority again
  });

  it("leaves a member whose speaker is gone out of everything but the warning", async () => {
    zonesFixture = [
      {
        id: "z1",
        name: "Downstairs",
        members: ["sonos:living", "sonos:ghost"],
        speakers: [
          speaker({ id: "living", vendor: "sonos", state: playingState() }),
          { member: "sonos:ghost", missing: true } as MediaZoneSpeaker,
        ],
      },
    ];
    const zones = await make();
    const z = zones.zones[0];
    expect(zones.speakersOf(z)).toHaveLength(1);
    expect(zones.memberLine(z)).toBe("living");
  });
});
