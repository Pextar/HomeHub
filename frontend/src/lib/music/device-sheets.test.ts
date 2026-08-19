import { describe, it, expect, beforeEach, vi } from "vitest";
import type { AirPlaySpeakerView, KEFSpeakerView, SonosSpeakerView, UPnPRenderer } from "../types";

/**
 * What has to be re-read after a device sheet closes.
 *
 * This is the part that was written out at each call site and had already
 * gone its own way. The rule is not "refresh the bridge you edited": removing
 * a speaker cascades out of the rooms that held it, and the room editor's
 * picker offers a list that just changed — so the zones and the endpoints are
 * owed on every device change, and forgetting either leaves a room claiming a
 * speaker that is gone.
 *
 * The other half is the negative: a sheet that changed nothing costs no
 * reads at all.
 */

/** What the sheet answers. Set per test. */
let sheetResult: unknown = false;
const opened: string[] = [];

vi.mock("../modal.svelte", () => ({
  openModal: (component: { name?: string }, props: Record<string, unknown>) => {
    opened.push(String(props.brand ?? component?.name ?? "sheet"));
    return Promise.resolve(sheetResult);
  },
}));
vi.mock("../../modals/SpeakerModal.svelte", () => ({ default: { name: "speaker" } }));
vi.mock("../../modals/SonosEventsModal.svelte", () => ({ default: { name: "events" } }));
vi.mock("../../modals/MusicQualityModal.svelte", () => ({ default: { name: "quality" } }));
vi.mock("../../modals/SpotifyConnectModal.svelte", () => ({ default: { name: "connect" } }));
vi.mock("../../modals/QobuzConnectModal.svelte", () => ({ default: { name: "qobuz" } }));

const { createDeviceSheets } = await import("./device-sheets");

function make() {
  const reads: string[] = [];
  const bridge = (name: string) => ({ refresh: () => void reads.push(name) });
  const kefEdited: string[] = [];
  const sheets = createDeviceSheets({
    sonos: bridge("sonos"),
    kef: bridge("kef"),
    airplay: bridge("airplay"),
    upnp: bridge("upnp"),
    zones: {
      refresh: () => void reads.push("zones"),
      loadEndpoints: () => void reads.push("endpoints"),
    },
    onKefEdited: (id) => kefEdited.push(id),
  });
  return { sheets, reads, kefEdited };
}

const sonosSpeaker = { id: "sp1" } as SonosSpeakerView;
const kefSpeaker = { id: "kef1" } as KEFSpeakerView;
const airplaySpeaker = { id: "ap1" } as AirPlaySpeakerView;
const renderer = { id: "up1" } as UPnPRenderer;

beforeEach(() => {
  sheetResult = false;
  opened.length = 0;
});

describe("a device sheet that changed something", () => {
  beforeEach(() => (sheetResult = true));

  it("always re-reads the rooms and the picker's list, not just the bridge", async () => {
    const t = make();
    await t.sheets.openUPnP(renderer);
    expect(t.reads).toEqual(["upnp", "zones", "endpoints"]);
  });

  it("re-reads every brand after the add sheet, which could have made any", async () => {
    const t = make();
    await t.sheets.openSpeaker();
    expect(t.reads).toEqual(["sonos", "kef", "airplay", "upnp", "zones", "endpoints"]);
  });

  it("reads UPnP after an AirPlay edit — one box, two advertisements", async () => {
    const t = make();
    await t.sheets.openAirPlay(airplaySpeaker);
    expect(t.reads).toEqual(["airplay", "upnp", "zones", "endpoints"]);
  });

  it("closes the settings pane of the KEF speaker that was edited", async () => {
    const t = make();
    await t.sheets.openKEF(kefSpeaker);
    expect(t.kefEdited).toEqual(["kef1"]);
    expect(t.reads).toEqual(["kef", "zones", "endpoints"]);
  });

  it("edits under the brand it was opened for", async () => {
    const t = make();
    await t.sheets.openKEF(kefSpeaker);
    await t.sheets.openAirPlay(airplaySpeaker);
    await t.sheets.openUPnP(renderer);
    await t.sheets.openSpeaker(sonosSpeaker);
    expect(opened).toEqual(["kef", "airplay", "upnp", "sonos"]);
  });
});

describe("a device sheet that changed nothing", () => {
  it("costs no reads at all", async () => {
    const t = make();
    await t.sheets.openSpeaker();
    await t.sheets.openKEF(kefSpeaker);
    await t.sheets.openAirPlay(airplaySpeaker);
    await t.sheets.openUPnP(renderer);
    expect(t.reads).toEqual([]);
    expect(t.kefEdited).toEqual([]);
  });
});

describe("the quality sheets", () => {
  it("re-read the zones, because what a zone reports depends on them", async () => {
    // Not the device bridges: nothing was registered or removed. What
    // changed is what each zone can decode, or where the account is playing.
    for (const open of ["openQuality", "openConnect", "openQobuz"] as const) {
      const t = make();
      await t.sheets[open]();
      expect(t.reads, open).toEqual(["zones"]);
    }
  });

  it("re-reads Sonos after the push-status sheet, which can turn events on", async () => {
    const t = make();
    await t.sheets.openEvents();
    expect(t.reads).toEqual(["sonos"]);
  });
});
