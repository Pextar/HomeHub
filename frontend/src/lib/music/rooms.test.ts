import { describe, it, expect, beforeEach, vi } from "vitest";
import type {
  KEFSpeakerView,
  MediaZone,
  MediaZoneSpeaker,
  SonosSpeakerView,
  SonosStatus,
} from "../types";

/**
 * The room model is where Music's three bridges stop being three things, so
 * the two decisions worth pinning down are the ones that used to be spread
 * across four components and got them out of step:
 *
 *   **who owns a speaker.** Exactly one room does. A zone the user built beats
 *   the household's own grouping, which beats a speaker standing alone — get
 *   this wrong and the same Sonos One is on the screen three times under three
 *   names, which is what the redesign existed to fix.
 *
 *   **what a drop means.** One gesture, two mechanisms. Two Sonos rooms group
 *   natively, because that is what the household is for. Anything with a KEF
 *   or an existing zone in it becomes a HomeHub room, because that is the only
 *   thing that can span makes. The user is told which happened; they never
 *   pick.
 */

let sonosFixture: SonosStatus = { speakers: [], groups: [] };
let kefFixture: KEFSpeakerView[] = [];
let zonesFixture: MediaZone[] = [];

const sonosJoin = vi.fn(async (_speakerId: string, _coordinatorId: string) => { });
const mediaCreateZone = vi.fn(async (body: { name: string; members: string[] }) => ({
  id: "new",
  name: body.name,
  members: body.members,
  speakers: [],
}));
const mediaUpdateZone = vi.fn(async (id: string, body: { members?: string[] }) => ({
  ...zonesFixture.find((z) => z.id === id)!,
  members: body.members ?? [],
}));
const mediaDeleteZone = vi.fn(async (_id: string) => { });

vi.mock("../api", () => ({
  api: {
    sonosStatus: vi.fn(async () => sonosFixture),
    sonosFavorites: vi.fn(async () => []),
    kefStatus: vi.fn(async () => ({ speakers: kefFixture })),
    mediaZones: vi.fn(async () => zonesFixture),
    mediaEndpoints: vi.fn(async () => []),
    sonosJoin,
    mediaCreateZone,
    mediaUpdateZone,
    mediaDeleteZone,
  },
}));

const { createSonosBridge } = await import("./sonos.svelte");
const { createKEFBridge } = await import("./kef.svelte");
const { createZonesBridge } = await import("./zones.svelte");
const { createRooms } = await import("./rooms.svelte");
const { createBusy } = await import("./busy.svelte");

function sonosSpeaker(id: string, name = id): SonosSpeakerView {
  return { id, name, reachable: true } as SonosSpeakerView;
}
function kefSpeaker(id: string, name = id): KEFSpeakerView {
  return { id, name, reachable: true } as KEFSpeakerView;
}
function member(id: string, vendor: "sonos" | "kef"): MediaZoneSpeaker {
  return { id, name: id, vendor, member: `${vendor}:${id}`, capabilities: [] } as MediaZoneSpeaker;
}

async function build() {
  const busy = createBusy();
  const sonos = createSonosBridge(busy);
  const kef = createKEFBridge(busy);
  const zones = createZonesBridge(busy);
  await Promise.all([sonos.refresh(), kef.refresh(), zones.refresh()]);
  return { rooms: createRooms(sonos, kef, zones, busy), sonos, kef, zones };
}

beforeEach(() => {
  sonosFixture = { speakers: [], groups: [] };
  kefFixture = [];
  zonesFixture = [];
  sonosJoin.mockClear();
  mediaCreateZone.mockClear();
  mediaUpdateZone.mockClear();
  mediaDeleteZone.mockClear();
});

describe("which room owns a speaker", () => {
  it("lists a Sonos group once, under its own name", async () => {
    sonosFixture = {
      speakers: [sonosSpeaker("living", "Living Room"), sonosSpeaker("kitchen", "Kitchen")],
      groups: [{ coordinator_id: "living", member_ids: ["living", "kitchen"] }],
    };
    const { rooms } = await build();
    expect(rooms.list).toHaveLength(1);
    expect(rooms.list[0].key).toBe("sonos:living");
    expect(rooms.list[0].grouped).toBe(true);
    expect(rooms.list[0].members).toEqual(["Living Room", "Kitchen"]);
  });

  it("lets a zone claim its members, so nothing appears twice", async () => {
    sonosFixture = {
      speakers: [sonosSpeaker("living"), sonosSpeaker("loft")],
      groups: [
        { coordinator_id: "living", member_ids: ["living"] },
        { coordinator_id: "loft", member_ids: ["loft"] },
      ],
    };
    kefFixture = [kefSpeaker("study")];
    zonesFixture = [
      {
        id: "z1",
        name: "Downstairs",
        members: ["sonos:living", "kef:study"],
        speakers: [member("living", "sonos"), member("study", "kef")],
      },
    ];
    const { rooms } = await build();
    // The zone, and the one Sonos speaker it didn't claim. Not the claimed
    // Sonos room, and not the claimed KEF speaker.
    expect(rooms.list.map((r) => r.key)).toEqual(["zone:z1", "sonos:loft"]);
  });

  it("counts speakers that answered nothing as offline rather than as rooms", async () => {
    sonosFixture = { speakers: [sonosSpeaker("ghost")], groups: [] };
    kefFixture = [{ ...kefSpeaker("dead"), reachable: false }];
    const { rooms } = await build();
    expect(rooms.list).toHaveLength(0);
    expect(rooms.offline.map((s) => s.id)).toEqual(["ghost", "dead"]);
  });
});

describe("what each room can actually do", () => {
  it("gives a Sonos room a queue and a seek, and a KEF speaker neither", async () => {
    sonosFixture = {
      speakers: [sonosSpeaker("living")],
      groups: [{ coordinator_id: "living", member_ids: ["living"] }],
    };
    kefFixture = [kefSpeaker("study")];
    const { rooms } = await build();
    const sonosRoom = rooms.byKey("sonos:living")!;
    const kefRoom = rooms.byKey("kef:study")!;

    expect(sonosRoom.canQueue).toBe(true);
    expect(sonosRoom.canSeek).toBe(true);
    expect(sonosRoom.canPlayMode).toBe(true);

    expect(kefRoom.canQueue).toBe(false);
    expect(kefRoom.canSeek).toBe(false);
    expect(kefRoom.canPickInput).toBe(true);
    expect(kefRoom.canSkip).toBe(true);
  });

  it("takes the skips off a room HomeHub is streaming to", async () => {
    zonesFixture = [
      {
        id: "z1",
        name: "Downstairs",
        members: ["sonos:living"],
        route: "stream",
        speakers: [member("living", "sonos")],
      },
      {
        id: "z2",
        name: "Loft",
        members: ["sonos:loft"],
        route: "native",
        speakers: [member("loft", "sonos")],
      },
    ];
    const { rooms } = await build();
    // Mid-stream, `next` sent to a speaker is a call it refuses — so the
    // control isn't offered rather than offered and rejected.
    expect(rooms.byKey("zone:z1")!.canSkip).toBe(false);
    expect(rooms.byKey("zone:z2")!.canSkip).toBe(true);
  });
});

describe("dropping one room on another", () => {
  it("refuses a drop that would change nothing", async () => {
    sonosFixture = {
      speakers: [sonosSpeaker("living"), sonosSpeaker("kitchen")],
      groups: [{ coordinator_id: "living", member_ids: ["living", "kitchen"] }],
    };
    zonesFixture = [];
    const { rooms } = await build();
    const room = rooms.list[0];
    expect(rooms.canGroup(room, room)).toBe(false);
  });

  it("groups two Sonos rooms natively, carrying every speaker in the one dragged", async () => {
    sonosFixture = {
      speakers: [sonosSpeaker("a"), sonosSpeaker("b"), sonosSpeaker("c")],
      groups: [
        { coordinator_id: "a", member_ids: ["a", "b"] },
        { coordinator_id: "c", member_ids: ["c"] },
      ],
    };
    const { rooms } = await build();
    const source = rooms.byKey("sonos:a")!;
    const target = rooms.byKey("sonos:c")!;

    await rooms.group(source, target);
    // The whole card moved: what was dragged was a room, and leaving half of
    // it behind is not what the gesture said.
    expect(sonosJoin.mock.calls).toEqual([
      ["a", "c"],
      ["b", "c"],
    ]);
    expect(mediaCreateZone).not.toHaveBeenCalled();
  });

  it("builds a HomeHub room when a KEF is involved, because Sonos can't hold one", async () => {
    sonosFixture = {
      speakers: [sonosSpeaker("living", "Living Room")],
      groups: [{ coordinator_id: "living", member_ids: ["living"] }],
    };
    kefFixture = [kefSpeaker("study", "Study")];
    const { rooms } = await build();
    const source = rooms.byKey("kef:study")!;
    const target = rooms.byKey("sonos:living")!;

    const said = await rooms.group(source, target);
    expect(sonosJoin).not.toHaveBeenCalled();
    // Named for the card that stayed put under the finger.
    expect(mediaCreateZone).toHaveBeenCalledWith({
      name: "Living Room",
      members: ["sonos:living", "kef:study"],
    });
    expect(said).toContain("Living Room");
  });

  it("adds to the room it was dropped on, rather than making a second one", async () => {
    sonosFixture = {
      speakers: [sonosSpeaker("loft")],
      groups: [{ coordinator_id: "loft", member_ids: ["loft"] }],
    };
    zonesFixture = [
      {
        id: "z1",
        name: "Downstairs",
        members: ["sonos:living", "kef:study"],
        speakers: [member("living", "sonos"), member("study", "kef")],
      },
    ];
    const { rooms } = await build();

    await rooms.group(rooms.byKey("sonos:loft")!, rooms.byKey("zone:z1")!);
    expect(mediaUpdateZone).toHaveBeenCalledWith("z1", {
      members: ["sonos:living", "kef:study", "sonos:loft"],
    });
    expect(mediaCreateZone).not.toHaveBeenCalled();
  });

  it("merges two built rooms into the one dropped on, and retires the other", async () => {
    zonesFixture = [
      {
        id: "z1",
        name: "Downstairs",
        members: ["sonos:living"],
        speakers: [member("living", "sonos")],
      },
      {
        id: "z2",
        name: "Upstairs",
        members: ["kef:study"],
        speakers: [member("study", "kef")],
      },
    ];
    const { rooms } = await build();

    await rooms.group(rooms.byKey("zone:z2")!, rooms.byKey("zone:z1")!);
    expect(mediaUpdateZone).toHaveBeenCalledWith("z1", {
      members: ["sonos:living", "kef:study"],
    });
    // The emptied one would otherwise sit there as a second name over the
    // same speakers — exactly the duplication rooms exist to prevent.
    expect(mediaDeleteZone).toHaveBeenCalledWith("z2");
  });
});
