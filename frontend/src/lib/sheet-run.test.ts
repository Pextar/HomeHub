import { describe, it, expect } from "vitest";
import { closed, raise, swapTo, dismiss, closeAll, isUp } from "./sheet-run";

type Sheet = "player" | "search" | "zones";

const run = (open: Sheet | null, under: Sheet | null = null) => ({ open, under });

describe("sheet-run", () => {
  it("starts bare", () => {
    expect(closed<Sheet>()).toEqual(run(null));
    expect(isUp(closed<Sheet>())).toBe(false);
  });

  it("raises a sheet over the bare page", () => {
    const r = raise(closed<Sheet>(), "search");
    expect(r).toEqual(run("search"));
    expect(isUp(r)).toBe(true);
  });

  it("raising over an open sheet replaces it without remembering it", () => {
    // "Manage" from Home while Search happens to be up is a fresh start, not
    // a drill-down — dismissing Zones must not drop the user back into Search.
    expect(raise(run("search"), "zones")).toEqual(run("zones"));
  });

  it("swaps, remembering the sheet underneath", () => {
    expect(swapTo(run("zones"), "player")).toEqual(run("player", "zones"));
  });

  it("swapping from the bare page is just a raise", () => {
    expect(swapTo(closed<Sheet>(), "player")).toEqual(run("player"));
  });

  it("swapping to the sheet already open changes nothing", () => {
    const r = run("player", "zones");
    expect(swapTo(r, "player")).toBe(r);
  });

  it("never chains what to come back to", () => {
    // Zones → player → search must remember only the player. Three levels of
    // "back" is a navigation stack, and screens are what those are for.
    const r = swapTo(swapTo(run("zones"), "player"), "search");
    expect(r).toEqual(run("search", "player"));
  });

  it("dismisses back to the sheet underneath, then leaves for good", () => {
    const swapped = swapTo(run("zones"), "player");
    const back = dismiss(swapped);
    expect(back).toEqual(run("zones"));
    expect(dismiss(back)).toEqual(run(null));
  });

  it("dismisses a lone sheet straight to the bare page", () => {
    expect(dismiss(run("search"))).toEqual(run(null));
  });

  it("dismissing the bare page is a no-op", () => {
    const bare = closed<Sheet>();
    expect(dismiss(bare)).toBe(bare);
  });

  it("closeAll leaves sheets entirely, forgetting what was underneath", () => {
    expect(closeAll(swapTo(run("zones"), "player"))).toEqual(run(null));
  });

  it("closeAll on the bare page keeps the same value", () => {
    const bare = closed<Sheet>();
    expect(closeAll(bare)).toBe(bare);
  });

  it("stays up across a swap, so the scroll lock is never released mid-run", () => {
    const zones = raise(closed<Sheet>(), "zones");
    const player = swapTo(zones, "player");
    const back = dismiss(player);
    expect([zones, player, back].map(isUp)).toEqual([true, true, true]);
    expect(isUp(dismiss(back))).toBe(false);
  });

  it("only ever holds one sheet on screen", () => {
    const states = [
      raise(closed<Sheet>(), "zones"),
      swapTo(raise(closed<Sheet>(), "zones"), "player"),
      dismiss(swapTo(raise(closed<Sheet>(), "zones"), "player")),
    ];
    for (const s of states) {
      expect(typeof s.open).toBe("string");
      expect(s.open).not.toBe(s.under);
    }
  });
});
