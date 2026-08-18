import { describe, it, expect, beforeEach, vi } from "vitest";
import { withRoot } from "../../test-runes.svelte";
import { createMusicNav, type MusicNav, type MusicSheet } from "./navigation.svelte";

/**
 * What "back" means in the Music view.
 *
 * Screens stack and sheets swap, and one browser history entry sits over
 * both — three mechanisms that only ever ran inside a mounted component, so
 * none of them had a test. These are the rules that were being maintained by
 * reading: back is up one level and never further, a sheet remembers where it
 * was scrolled to, a screen reached from a player hands back to that player,
 * and the history entry is held exactly while the view is deeper than Home.
 */

const pushState = vi.fn();
const back = vi.fn();
vi.stubGlobal("history", { pushState, back });

vi.mock("./scroll", () => ({
  settleScroll: (el: () => HTMLElement | null, y: number) => scrolled.push(["sheet", y]),
  restoreScroll: (y: number) => scrolled.push(["window", y]),
  toTop: () => scrolled.push(["window", 0]),
}));
vi.mock("../scroll-lock", () => ({
  lockBodyScroll: () => locks.push("lock"),
  unlockBodyScroll: () => locks.push("unlock"),
}));

let scrolled: [string, number][] = [];
let locks: string[] = [];

/** A nav plus everything it asked its host to do. */
function make(opts: { reopens?: boolean } = {}) {
  const left: string[] = [];
  const closed: (MusicSheet | null)[] = [];
  let playerKey: string | null = null;
  let sheetTop = 0;
  const h = withRoot<MusicNav>(() =>
    createMusicNav({
      sheetScrollEl: () => ({ scrollTop: sheetTop }) as HTMLElement,
      playerKey: () => playerKey,
      reopenPlayer: (key) => {
        if (opts.reopens === false) return false;
        playerKey = key;
        return true;
      },
      onLeftScreen: (e) => left.push(e.id),
      onSheetsClosed: (showing) => {
        closed.push(showing);
        // What the view does: a sheet that stopped showing lets go of
        // whatever it was bound to.
        if (showing !== "player") playerKey = null;
      },
    }),
  );
  return {
    h,
    get nav() {
      return h.value;
    },
    left,
    closed,
    setPlayerKey(k: string | null) {
      playerKey = k;
    },
    get playerKey() {
      return playerKey;
    },
    scrollSheetTo(y: number) {
      sheetTop = y;
    },
  };
}

beforeEach(() => {
  scrolled = [];
  locks = [];
  pushState.mockClear();
  back.mockClear();
  window.scrollY = 0;
});

describe("screens", () => {
  it("starts on Home with nothing over it", () => {
    const t = make();
    expect(t.nav.screen).toBe("home");
    expect(t.nav.depth).toBe(0);
    t.h.stop();
  });

  it("goes up one level, not all the way home", () => {
    const t = make();
    t.nav.pushScreen({ id: "browse", scroll: 0 });
    t.nav.pushScreen({ id: "artist", uri: "spotify:artist:1", scroll: 0 });
    t.nav.pushScreen({ id: "context", uri: "spotify:album:1", scroll: 0 });
    t.h.flush();

    t.nav.leaveScreen();
    expect(t.nav.screen).toBe("artist");
    t.nav.leaveScreen();
    expect(t.nav.screen).toBe("browse");
    t.nav.leaveScreen();
    expect(t.nav.screen).toBe("home");
    t.h.stop();
  });

  it("tells the view which screen it just gave up", () => {
    const t = make();
    t.nav.pushScreen({ id: "speakers", scroll: 0 });
    t.nav.leaveScreen();
    expect(t.left).toEqual(["speakers"]);
    t.h.stop();
  });

  it("leaving Home is a no-op rather than an underflow", () => {
    const t = make();
    t.nav.leaveScreen();
    expect(t.nav.screen).toBe("home");
    expect(t.left).toEqual([]);
    t.h.stop();
  });

  it("puts each level back where it was scrolled to", () => {
    const t = make();
    window.scrollY = 400; // read a way down Home
    t.nav.pushScreen({ id: "browse", scroll: 0 });
    window.scrollY = 250; // …and a way down Browse
    t.nav.pushScreen({ id: "artist", scroll: 0 });

    scrolled = [];
    t.nav.leaveScreen();
    expect(scrolled).toEqual([["window", 250]]);
    t.nav.leaveScreen();
    expect(scrolled).toEqual([
      ["window", 250],
      ["window", 400],
    ]);
    t.h.stop();
  });

  it("opens a pushed screen at its top", () => {
    const t = make();
    t.nav.pushScreen({ id: "browse", scroll: 0 });
    expect(scrolled).toEqual([["window", 0]]);
    t.h.stop();
  });
});

describe("a screen reached from a player", () => {
  it("hands back to that player when the stack runs out", () => {
    const t = make();
    t.setPlayerKey("sonos:kitchen");
    t.nav.swapSheet("player");
    t.h.flush();

    t.nav.pushScreen({ id: "browse", scroll: 0 }); // Browse, from the player
    expect(t.nav.openSheet).toBeNull(); // the sheet stood down for it

    t.nav.leaveScreen();
    expect(t.playerKey).toBe("sonos:kitchen");
    t.h.stop();
  });

  it("still owes the player after going deeper", () => {
    const t = make();
    t.setPlayerKey("sonos:kitchen");
    t.nav.swapSheet("player");
    t.nav.pushScreen({ id: "browse", scroll: 0 });
    t.nav.pushScreen({ id: "artist", scroll: 0 });

    t.nav.leaveScreen(); // back to Browse — nothing owed yet
    expect(t.playerKey).toBeNull();
    t.nav.leaveScreen(); // now the stack ran out
    expect(t.playerKey).toBe("sonos:kitchen");
    t.h.stop();
  });

  it("falls through to Home when the room went away meanwhile", () => {
    const t = make({ reopens: false });
    t.setPlayerKey("sonos:kitchen");
    t.nav.swapSheet("player");
    t.nav.pushScreen({ id: "browse", scroll: 0 });

    window.scrollY = 0;
    scrolled = [];
    t.nav.leaveScreen();
    expect(t.nav.screen).toBe("home");
    expect(scrolled).toEqual([["window", 0]]); // Home's own offset, restored
    t.h.stop();
  });

  it("owes nothing when the screen was reached from Home", () => {
    const t = make();
    t.nav.pushScreen({ id: "browse", scroll: 0 });
    t.nav.leaveScreen();
    expect(t.playerKey).toBeNull();
    t.h.stop();
  });
});

describe("sheets", () => {
  it("swaps rather than stacking, and comes back to the first", () => {
    const t = make();
    t.nav.swapSheet("player");
    t.nav.swapSheet("room-edit");
    expect(t.nav.openSheet).toBe("room-edit");
    expect(t.nav.sheets.under).toBe("player");

    t.nav.dropSheet();
    expect(t.nav.openSheet).toBe("player");
    t.h.stop();
  });

  it("puts the sheet underneath back where it was scrolled to", () => {
    const t = make();
    t.nav.swapSheet("player");
    t.scrollSheetTo(300); // read down the player
    t.nav.swapSheet("room-edit");

    scrolled = [];
    t.nav.dropSheet();
    expect(scrolled).toEqual([["sheet", 300]]);
    t.h.stop();
  });

  it("opens each sheet at its own top rather than the last one's offset", () => {
    const t = make();
    t.scrollSheetTo(300);
    t.nav.swapSheet("player");
    t.nav.swapSheet("room-edit");
    t.nav.dropSheet(); // back to the player, which was at 300
    scrolled = [];
    t.nav.swapSheet("room-edit"); // …and up again: the editor is at its top
    t.nav.dropSheet();
    expect(scrolled).toEqual([["sheet", 300]]);
    t.h.stop();
  });

  it("says which sheet is showing after one closes", () => {
    const t = make();
    t.nav.swapSheet("player");
    t.nav.swapSheet("room-edit");
    t.nav.dropSheet();
    t.nav.dropSheet();
    expect(t.closed).toEqual(["player", null]);
    t.h.stop();
  });

  it("closes every sheet at once when one is hidden", () => {
    const t = make();
    t.nav.swapSheet("player");
    t.nav.swapSheet("room-edit");
    t.nav.hideSheet();
    expect(t.nav.sheetUp).toBe(false);
    expect(t.closed).toEqual([null]);
    t.h.stop();
  });

  it("locks the body once across a swap, not per sheet", () => {
    const t = make();
    t.nav.swapSheet("player");
    t.h.flush();
    t.nav.swapSheet("room-edit");
    t.h.flush();
    // One lock, still held — an unlock and re-lock here unpins the body for a
    // frame on iOS.
    expect(locks).toEqual(["lock"]);

    t.nav.dropSheet();
    t.nav.dropSheet();
    t.h.flush();
    expect(locks).toEqual(["lock", "unlock"]);
    t.h.stop();
  });
});

describe("the history entry", () => {
  it("is taken once on the way down and given back at Home", () => {
    const t = make();
    t.nav.pushScreen({ id: "browse", scroll: 0 });
    t.h.flush();
    expect(pushState).toHaveBeenCalledTimes(1);

    t.nav.pushScreen({ id: "artist", scroll: 0 });
    t.h.flush();
    expect(pushState).toHaveBeenCalledTimes(1); // still the one entry

    t.nav.leaveScreen();
    t.h.flush();
    expect(back).not.toHaveBeenCalled(); // still deeper than Home
    t.nav.leaveScreen();
    t.h.flush();
    expect(back).toHaveBeenCalledTimes(1);
    t.h.stop();
  });

  it("counts a sheet as depth, so back closes it", () => {
    const t = make();
    t.nav.swapSheet("player");
    t.h.flush();
    expect(t.nav.depth).toBe(1);

    t.nav.onPopState();
    expect(t.nav.sheetUp).toBe(false);
    t.h.stop();
  });

  it("takes the entry again after the browser consumes it", () => {
    const t = make();
    t.nav.pushScreen({ id: "browse", scroll: 0 });
    t.nav.pushScreen({ id: "artist", scroll: 0 });
    t.h.flush();
    pushState.mockClear();

    t.nav.onPopState(); // back to Browse — still deep, so re-take
    t.h.flush();
    expect(t.nav.screen).toBe("browse");
    expect(pushState).toHaveBeenCalledTimes(1);
    t.h.stop();
  });

  it("ignores a pop from Home — that entry is someone else's", () => {
    const t = make();
    t.nav.onPopState();
    expect(t.nav.screen).toBe("home");
    expect(pushState).not.toHaveBeenCalled();
    t.h.stop();
  });

  it("closes the sheet before it starts leaving screens", () => {
    const t = make();
    t.nav.pushScreen({ id: "browse", scroll: 0 });
    t.nav.swapSheet("player");
    t.h.flush();

    t.nav.onPopState();
    expect(t.nav.sheetUp).toBe(false);
    expect(t.nav.screen).toBe("browse"); // the screen is still there
    t.nav.onPopState();
    expect(t.nav.screen).toBe("home");
    t.h.stop();
  });
});
