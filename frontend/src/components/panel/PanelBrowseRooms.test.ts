import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import PanelBrowseRooms from "./PanelBrowseRooms.svelte";
import type { PanelMusicStore, PanelSource } from "../../lib/panel-music.svelte";

/**
 * The Rooms pane's contract.
 *
 * Split is the one destructive gesture on this surface and a kiosk has no
 * confirm modal, so it arms instead — that is the rule this pane exists to
 * get right, and the one a refactor is most likely to quietly drop.
 */

vi.mock("../../lib/kef", () => ({ kefSourceLabel: (s?: string) => s ?? "" }));

const room = (over: Partial<PanelSource> = {}): PanelSource =>
  ({
    key: "s:kitchen", kind: "sonos", id: "kitchen", title: "Kitchen",
    playing: false, standby: false, volume: 30, muted: false, canSkip: true,
    ...over,
  }) as PanelSource;

const store = (sources: PanelSource[], over: Partial<PanelMusicStore> = {}) =>
  ({
    sources, busy: {}, selected: null,
    ungroupFeatured: vi.fn(), joinSource: vi.fn(), moveTo: vi.fn(), leaveMember: vi.fn(),
    timers: [], roomTimers: [], history: [], topPlays: [], insights: null,
    ...over,
  }) as unknown as PanelMusicStore;

describe("the panel's Rooms pane", () => {
  it("lists every room the wall can feature", () => {
    const rooms = [room(), room({ key: "s:bedroom", id: "bedroom", title: "Bedroom" })];
    render(PanelBrowseRooms, {
      music: store(rooms), featured: rooms[0], onArtist: vi.fn(),
    });
    expect(screen.getByRole("button", { name: "Feature Kitchen" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "Feature Bedroom" })).toHaveAttribute("aria-pressed", "false");
  });

  it("arms Split rather than doing it, because a kiosk has no confirm", async () => {
    const ungroupFeatured = vi.fn();
    const grouped = room({ members: [{ id: "kitchen", coordinator: true }, { id: "den" }] as never });
    render(PanelBrowseRooms, {
      music: store([grouped], { ungroupFeatured }), featured: grouped, onArtist: vi.fn(),
    });
    const split = screen.getByRole("button", { name: "Split" });
    split.click();
    expect(ungroupFeatured).not.toHaveBeenCalled();
    // Armed: the label says so, and the second tap is the one that acts.
    const armed = await screen.findByRole("button", { name: "Split?" });
    armed.click();
    expect(ungroupFeatured).toHaveBeenCalledOnce();
  });

  it("offers Join and Move only between two Sonos rooms", () => {
    const kitchen = room();
    const zone = room({ key: "z:1", kind: "zone", id: "1", title: "Downstairs" });
    render(PanelBrowseRooms, {
      music: store([kitchen, zone]), featured: kitchen, onArtist: vi.fn(),
    });
    // A HomeHub zone does not group natively — §15.1: absent, never dead.
    expect(screen.queryByRole("button", { name: /Move the music to Downstairs/ })).not.toBeInTheDocument();
  });
});
