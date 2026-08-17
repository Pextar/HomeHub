import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import KidMediaCard from "./KidMediaCard.svelte";
import type { SpotifyItem } from "../../lib/types";

const item = (over: Partial<SpotifyItem> = {}): SpotifyItem =>
  ({ kind: "album", uri: "spotify:album:1", name: "Low", sub: "David Bowie", ...over }) as SpotifyItem;

describe("the kid module's cover card", () => {
  it("is one target, so the whole card is the button", async () => {
    const onPick = vi.fn();
    render(KidMediaCard, { item: item(), onPick });
    const card = screen.getByRole("button");
    expect(card).toHaveTextContent("Low");
    card.click();
    expect(onPick).toHaveBeenCalledWith(item());
  });

  it("stands a kind emoji in for missing art, and hides it from the reader", () => {
    render(KidMediaCard, { item: item({ art_url: undefined }), onPick: vi.fn() });
    const stand = document.querySelector(".kms-card-none");
    expect(stand).toHaveAttribute("aria-hidden", "true");
    expect(stand).toHaveTextContent("💿");
  });

  it("rounds an artist's picture and squares everything else", () => {
    const { unmount } = render(KidMediaCard, {
      item: item({ kind: "artist", art_url: "https://img/a.jpg" }),
      onPick: vi.fn(),
    });
    expect(document.querySelector(".kms-card-art")).toHaveClass("round");
    unmount();
    render(KidMediaCard, { item: item({ art_url: "https://img/b.jpg" }), onPick: vi.fn() });
    expect(document.querySelector(".kms-card-art")).not.toHaveClass("round");
  });
});
