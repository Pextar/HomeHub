import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import PanelSearchDock from "./PanelSearchDock.svelte";
import type { PanelMusicStore, PanelSource } from "../../lib/panel-music.svelte";

/**
 * The dock's contract while the results have the screen.
 *
 * It cannot simply disappear when the keyboard is up: the queue buttons on
 * the rows stay tappable while typing, and queueing is the one action here
 * that changes nothing else on screen — this strip is where that answer
 * lives. So the rules worth pinning are what it keeps when it shrinks.
 */

const store = (over: Partial<PanelMusicStore> = {}) =>
  ({ busy: {}, skip: vi.fn(), togglePlay: vi.fn(), ...over }) as unknown as PanelMusicStore;

const room = (over: Partial<PanelSource> = {}): PanelSource =>
  ({
    key: "s:kitchen", kind: "sonos", id: "kitchen", title: "Kitchen",
    playing: true, standby: false, volume: 30, muted: false, canSkip: true,
    trackTitle: "Sound and Vision", trackSub: "David Bowie",
    ...over,
  }) as PanelSource;

describe("the panel's search dock", () => {
  it("goes back to the player when the cover is tapped", () => {
    const onBack = vi.fn();
    render(PanelSearchDock, { music: store(), featured: room(), onBack });
    screen.getByRole("button", { name: /Back to the player/ }).click();
    expect(onBack).toHaveBeenCalled();
  });

  it("keeps play/pause when the keyboard takes the room, and drops the skips", () => {
    render(PanelSearchDock, { music: store(), featured: room(), onBack: vi.fn(), kbOpen: true });
    expect(document.querySelector(".b-dock")).toHaveClass("kb-open");
    // Both skips are still in the DOM — the CSS is what shrinks the strip,
    // and the play button is what must survive it.
    expect(screen.getByRole("button", { name: "Pause" })).toBeInTheDocument();
  });

  it("says the freshest true thing: a queued track over the track line", () => {
    const { unmount } = render(PanelSearchDock, {
      music: store(), featured: room(), onBack: vi.fn(),
    });
    expect(document.querySelector(".d-sub")).toHaveTextContent("David Bowie");
    unmount();
    render(PanelSearchDock, {
      music: store(), featured: room(), onBack: vi.fn(), queuedLine: "Queued Heroes",
    });
    const sub = document.querySelector(".d-sub")!;
    expect(sub).toHaveTextContent("Queued Heroes");
    expect(sub).toHaveClass("said");
  });

  it("withholds the skips on a source that cannot skip", () => {
    render(PanelSearchDock, {
      music: store(), featured: room({ canSkip: false }), onBack: vi.fn(),
    });
    expect(screen.queryByRole("button", { name: "Next" })).not.toBeInTheDocument();
  });

  it("says so plainly when no speaker is answering", () => {
    render(PanelSearchDock, { music: store(), featured: undefined, onBack: vi.fn() });
    expect(screen.getByText("No speaker is answering")).toBeInTheDocument();
  });
});
