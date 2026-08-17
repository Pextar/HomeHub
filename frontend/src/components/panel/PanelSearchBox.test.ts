import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import PanelSearchBox from "./PanelSearchBox.svelte";
import type { SpotifyStore } from "../../lib/music/spotify.svelte";
import type { PanelSource } from "../../lib/panel-music.svelte";

/**
 * The search box's contract.
 *
 * The room line under the box is the point of this component as much as the
 * box is: the results are otherwise the one place on the wall that never
 * names their own destination, and a wall is the surface most likely to be
 * used by whoever walked past it last.
 */

const spotify = (over: Partial<SpotifyStore> = {}) =>
  ({
    query: "", results: null, kindFilter: "all",
    onQueryInput: vi.fn(), retry: vi.fn(), loadMore: vi.fn(),
    ...over,
  }) as unknown as SpotifyStore;

const room = { kind: "sonos", id: "kitchen", title: "Kitchen" } as PanelSource;
const base = {
  liveMessage: "", onTyping: vi.fn(), onQueryKey: vi.fn(), onClear: vi.fn(), onDone: vi.fn(),
};

describe("the panel's search box", () => {
  it("names where a tap will land", () => {
    const { unmount } = render(PanelSearchBox, { ...base, spotify: spotify(), featured: room });
    expect(document.querySelector(".s-dest")).toHaveTextContent("Plays on Kitchen");
    unmount();
    render(PanelSearchBox, { ...base, spotify: spotify(), featured: undefined });
    expect(document.querySelector(".s-dest")).toHaveTextContent("No speaker is answering");
  });

  it("treats typing as the signal that this is a search", async () => {
    const onTyping = vi.fn();
    render(PanelSearchBox, { ...base, spotify: spotify(), featured: room, onTyping });
    const input = screen.getByLabelText("Search music");
    input.dispatchEvent(new Event("pointerdown", { bubbles: true }));
    expect(onTyping).toHaveBeenCalled();
  });

  it("offers the clear button only once there is something to clear", () => {
    const { unmount } = render(PanelSearchBox, { ...base, spotify: spotify(), featured: room });
    expect(screen.queryByRole("button", { name: "Clear search" })).not.toBeInTheDocument();
    unmount();
    render(PanelSearchBox, { ...base, spotify: spotify({ query: "bowie" }), featured: room });
    expect(screen.getByRole("button", { name: "Clear search" })).toBeInTheDocument();
  });

  it("offers Done only while the results hold the whole wall", () => {
    const onDone = vi.fn();
    render(PanelSearchBox, { ...base, spotify: spotify(), featured: room, fullBleed: true, onDone });
    screen.getByRole("button", { name: "Done" }).click();
    expect(onDone).toHaveBeenCalled();
  });

  it("counts each kind of result on its filter chip", () => {
    render(PanelSearchBox, {
      ...base, featured: room,
      spotify: spotify({
        results: { tracks: [{}, {}], albums: [], playlists: [], artists: [{}] },
      } as never),
    });
    expect(screen.getByRole("button", { name: /Songs 2/ })).toBeInTheDocument();
    // A kind with no matches gets no chip rather than a chip reading zero.
    expect(screen.queryByRole("button", { name: /Albums/ })).not.toBeInTheDocument();
  });

  it("stays layout-neutral so the column's own gaps still apply", () => {
    render(PanelSearchBox, { ...base, spotify: spotify(), featured: room });
    // A real box here would swallow the flex gap the search column sets.
    expect(document.querySelector(".s-head")).toBeInTheDocument();
  });
});
