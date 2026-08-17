import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import PanelResultRow from "./PanelResultRow.svelte";
import type { SpotifyItem } from "../../lib/types";
import type { PanelMusicStore, PanelSource } from "../../lib/panel-music.svelte";

/**
 * The catalog row's contract with the wall.
 *
 * Two of these are rules the row only started owning when it became a
 * component: the dense mode and the full-bleed mode used to reach it as
 * `.kb-open .r-art` from a class two elements up, and they are props now.
 * A row that stopped answering them would look right in every unit test
 * that only asked what it rendered.
 */

const item = (over: Partial<SpotifyItem> = {}): SpotifyItem =>
  ({ kind: "track", uri: "spotify:track:1", name: "Sound and Vision", sub: "David Bowie", ...over }) as SpotifyItem;

const store = (busy: Record<string, boolean> = {}) =>
  ({ busy, enqueue: vi.fn() }) as unknown as PanelMusicStore;

const sonos = { kind: "sonos", id: "kitchen", title: "Kitchen" } as PanelSource;
const kef = { kind: "kef", id: "study", title: "Study" } as PanelSource;

const base = { music: store(), featured: sonos, onOpenArtist: vi.fn(), onPick: vi.fn() };

describe("the panel's catalog row", () => {
  it("plays a song on tap and opens an artist instead", async () => {
    const onPick = vi.fn();
    const onOpenArtist = vi.fn();
    const { unmount } = render(PanelResultRow, { ...base, item: item(), onPick, onOpenArtist });
    // The row's own button, not the queue buttons beside it — their labels
    // name the track too.
    document.querySelector<HTMLButtonElement>(".r-open")!.click();
    expect(onPick).toHaveBeenCalled();
    expect(onOpenArtist).not.toHaveBeenCalled();
    unmount();

    render(PanelResultRow, {
      ...base,
      item: item({ kind: "artist", name: "Bowie", art_url: "https://img/a.jpg" }),
      onPick,
      onOpenArtist,
    });
    document.querySelector<HTMLButtonElement>(".r-open")!.click();
    expect(onOpenArtist).toHaveBeenCalledWith("spotify:track:1", {
      art_url: "https://img/a.jpg",
      round: true,
    });
  });

  it("offers the two queue buttons only where there is a queue", () => {
    const { unmount } = render(PanelResultRow, { ...base, item: item() });
    expect(screen.getByRole("button", { name: /Play .* next/ })).toBeInTheDocument();
    unmount();
    // A KEF speaker has no queue — §15.1: absent, never dead.
    render(PanelResultRow, { ...base, featured: kef, item: item() });
    expect(screen.queryByRole("button", { name: /next/ })).not.toBeInTheDocument();
  });

  it("never offers to queue an artist, which is not a thing to queue", () => {
    render(PanelResultRow, { ...base, item: item({ kind: "artist" }) });
    expect(screen.queryByRole("button", { name: /queue/i })).not.toBeInTheDocument();
  });

  it("disables the row while its own play is in flight, and not another's", () => {
    render(PanelResultRow, { ...base, music: store({ "item:spotify:track:1": true }), item: item() });
    expect(document.querySelector(".r-open")).toBeDisabled();
  });

  it("carries the layout modes on its own root, since scoping severed the old path", () => {
    const { unmount } = render(PanelResultRow, { ...base, item: item(), kbOpen: true, full: true });
    const row = document.querySelector(".row")!;
    expect(row).toHaveClass("kb-open");
    expect(row).toHaveClass("full");
    unmount();
    render(PanelResultRow, { ...base, item: item() });
    expect(document.querySelector(".row")).not.toHaveClass("kb-open");
  });

  it("says what it is and where the tap goes only at full size", () => {
    const { unmount } = render(PanelResultRow, { ...base, item: item(), big: true });
    expect(document.querySelector(".r-line")).toBeInTheDocument();
    unmount();
    render(PanelResultRow, { ...base, item: item() });
    expect(document.querySelector(".r-line")).not.toBeInTheDocument();
  });
});
