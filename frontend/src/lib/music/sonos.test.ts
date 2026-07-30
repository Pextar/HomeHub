import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import type { SonosStatus, SonosSpeakerView } from "../types";

vi.mock("../api", () => ({
  api: {
    sonosStatus: vi.fn(async () => statusFixture),
    sonosSetVolume: vi.fn(async () => {}),
    sonosFavorites: vi.fn(async () => []),
  },
}));

const { createSonosBridge } = await import("./sonos.svelte");
const { createBusy } = await import("./busy.svelte");

/**
 * The Sonos fader's local-value window — the same contract KEF's has
 * (`kef.test.ts`), and for a while the one bridge that didn't keep it.
 *
 * A drag stamps "this value is the user's, not the poll's". Without the
 * stamp the next status poll wrote the speaker's last-read volume straight
 * back over the finger's position, so the slider sprang backwards mid-drag
 * once a second and the room never followed the gesture.
 */
let statusFixture: SonosStatus;

function speaker(id: string, volume: number): SonosSpeakerView {
  return {
    id,
    name: id,
    reachable: true,
    state: { volume, muted: false, playing: false },
  } as SonosSpeakerView;
}

function status(...vols: number[]): SonosStatus {
  const speakers = vols.map((v, i) => speaker(`sp${i}`, v));
  return {
    speakers,
    groups: [{ coordinator_id: "sp0", member_ids: speakers.map((x) => x.id) }],
  } as SonosStatus;
}

describe("Sonos volume", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    statusFixture = status(30, 30);
  });
  afterEach(() => vi.useRealTimers());

  const make = () => createSonosBridge(createBusy());

  it("shows the polled volume before anything is dragged", async () => {
    const sonos = make();
    await sonos.refresh();
    expect(sonos.shownVolume(statusFixture.speakers[0])).toBe(30);
  });

  it("holds the dragged value against a poll that lands mid-drag", async () => {
    const sonos = make();
    await sonos.refresh();

    sonos.dragVolume("sp0", 55);
    expect(sonos.shownVolume(statusFixture.speakers[0])).toBe(55);

    // The speaker hasn't caught up yet, and reports the old level.
    await sonos.refresh();
    expect(sonos.shownVolume(statusFixture.speakers[0])).toBe(55);
  });

  it("holds a dragged group value against a poll that lands mid-drag", async () => {
    const sonos = make();
    await sonos.refresh();

    sonos.dragGroupVolume("sp0", 70);
    expect(sonos.shownGroupVolume("sp0")).toBe(70);

    await sonos.refresh();
    expect(sonos.shownGroupVolume("sp0")).toBe(70);
  });

  it("sends while the finger is still down", async () => {
    const { api } = await import("../api");
    const sonos = make();
    await sonos.refresh();

    sonos.dragVolume("sp0", 40);
    sonos.dragVolume("sp0", 45);
    await vi.advanceTimersByTimeAsync(200);

    expect(api.sonosSetVolume).toHaveBeenCalled();
  });

  it("lets the poll take over once the drag is over", async () => {
    const sonos = make();
    await sonos.refresh();

    sonos.dragVolume("sp0", 55);
    await vi.advanceTimersByTimeAsync(3001);

    // Someone turned it up on the speaker itself. It is the authority on its
    // own volume; the local value was only ever a bridge across the drag.
    statusFixture = status(70, 70);
    await sonos.refresh();
    expect(sonos.shownVolume(statusFixture.speakers[0])).toBe(70);
  });
});
