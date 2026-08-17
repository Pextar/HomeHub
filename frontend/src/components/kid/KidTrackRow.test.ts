import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import KidTrackRow from "./KidTrackRow.svelte";
import type { SpotifyItem } from "../../lib/types";
import type { PanelMusicStore } from "../../lib/panel-music.svelte";

/**
 * The kid row's contract (DESIGN.md §17).
 *
 * Queueing has to *say so*: the queue is a pane away and the count changes
 * silently, which a kid cannot be expected to go and check. And the dense
 * mode is a prop now rather than a `.kb-open .kms-art` descendant, so it is
 * worth pinning that the row still answers it.
 */

const item = (over: Partial<SpotifyItem> = {}): SpotifyItem =>
  ({ kind: "track", uri: "spotify:track:1", name: "Heroes", sub: "David Bowie", ...over }) as SpotifyItem;

const store = (busy: Record<string, boolean> = {}) => ({ busy }) as unknown as PanelMusicStore;
const base = {
  music: store(), onPick: vi.fn(), onToggleQueue: vi.fn(), onQueue: vi.fn(),
};

describe("the kid module's song row", () => {
  it("plays on tap", () => {
    const onPick = vi.fn();
    render(KidTrackRow, { ...base, item: item(), onPick });
    document.querySelector<HTMLButtonElement>(".kms-main")!.click();
    expect(onPick).toHaveBeenCalledWith(item());
  });

  it("opens the queue choices behind the ＋ rather than queueing blind", () => {
    const onToggleQueue = vi.fn();
    const onQueue = vi.fn();
    const { unmount } = render(KidTrackRow, { ...base, item: item(), onToggleQueue, onQueue });
    screen.getByRole("button", { name: /Queue/ }).click();
    expect(onToggleQueue).toHaveBeenCalledWith("spotify:track:1");
    expect(onQueue).not.toHaveBeenCalled();
    unmount();

    render(KidTrackRow, { ...base, item: item(), queueOpen: true, onQueue });
    screen.getByText(/Play next/).click();
    expect(onQueue).toHaveBeenCalledWith(item(), true);
  });

  it("says out loud that the queue took it — the count is a pane away", () => {
    const { unmount } = render(KidTrackRow, { ...base, item: item() });
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    unmount();
    render(KidTrackRow, { ...base, item: item(), flashed: true });
    expect(screen.getByRole("status")).toHaveTextContent("Added to the queue 🎉");
  });

  it("carries the dense mode on its own root", () => {
    render(KidTrackRow, { ...base, item: item(), kbOpen: true });
    expect(document.querySelector(".kms-row")).toHaveClass("kb-open");
  });

  it("shows a track number only where the list is numbered", () => {
    const { unmount } = render(KidTrackRow, { ...base, item: item(), num: 3 });
    expect(document.querySelector(".kms-num")).toHaveTextContent("3");
    unmount();
    render(KidTrackRow, { ...base, item: item() });
    expect(document.querySelector(".kms-num")).toBeNull();
  });

  it("disables itself while its own play is in flight", () => {
    render(KidTrackRow, { ...base, music: store({ "item:spotify:track:1": true }), item: item() });
    expect(document.querySelector(".kms-main")).toBeDisabled();
  });
});
