import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { createThemeStore, theme as moduleStore } from "./theme.svelte";

// The module exports a live singleton, built the moment this file imports it.
// It listens on the same document as the stores these tests build, so it would
// race them for `data-theme`. Nothing here uses it, so unhook it once.
moduleStore.destroy();

type Listener = () => void;

let matches = false; // whether "(prefers-color-scheme: light)" matches
let listeners: Listener[] = [];

/** Stand in for the OS switching appearance, without telling the page. */
function setSystem(scheme: "dark" | "light") {
  matches = scheme === "light";
}
/** …and this is the page being told, which is the part iOS skips. */
function emitChange() {
  for (const l of [...listeners]) l();
}

/** `modern: false` is the Safari < 14 shape — `addListener` and nothing else. */
function installMatchMedia(modern = true) {
  listeners = [];
  const add = (l: Listener) => { listeners.push(l); };
  const remove = (l: Listener) => { listeners = listeners.filter((x) => x !== l); };
  window.matchMedia = ((query: string) => {
    const mql: Record<string, unknown> = {
      get matches() { return matches; },
      media: query,
      onchange: null,
      addListener: add,
      removeListener: remove,
      dispatchEvent: () => true,
    };
    if (modern) {
      mql.addEventListener = (_: string, l: Listener) => add(l);
      mql.removeEventListener = (_: string, l: Listener) => remove(l);
    }
    return mql as unknown as MediaQueryList;
  }) as typeof window.matchMedia;
}

const attr = () => document.documentElement.dataset.theme;
const bar = () => document.querySelector('meta[name="theme-color"]')?.getAttribute("content");

let store: ReturnType<typeof createThemeStore> | null = null;
const make = () => (store = createThemeStore());

beforeEach(() => {
  localStorage.clear();
  document.head.innerHTML = '<meta name="theme-color" content="#14130f" />';
  document.documentElement.dataset.theme = "dark";
  setSystem("dark");
  installMatchMedia();
});
afterEach(() => {
  store?.destroy();
  store = null;
});

describe("theme — auto", () => {
  it("takes the system value at startup", () => {
    setSystem("light");
    const t = make();
    expect(t.mode).toBe("auto");
    expect(t.current).toBe("light");
    expect(attr()).toBe("light");
    expect(bar()).toBe("#f5f1ea");
  });

  it("follows the system when the change event arrives", () => {
    const t = make();
    expect(t.current).toBe("dark");

    setSystem("light");
    emitChange();

    expect(t.current).toBe("light");
    expect(t.system).toBe("light");
    expect(attr()).toBe("light");
    expect(bar()).toBe("#f5f1ea");
  });

  // The bug this module exists for: an installed PWA is frozen behind the home
  // screen, so a switch that happens while it sits there never reaches it as an
  // event. Coming back to the foreground has to re-read the OS itself.
  it("catches up on becoming visible when the change event never came", () => {
    const t = make();
    setSystem("light");
    expect(t.current).toBe("dark"); // still stale — nothing has told it yet

    document.dispatchEvent(new Event("visibilitychange"));

    expect(t.current).toBe("light");
    expect(attr()).toBe("light");
    expect(bar()).toBe("#f5f1ea");
  });

  it("catches up on a back/forward-cache restore", () => {
    const t = make();
    setSystem("light");
    window.dispatchEvent(new Event("pageshow"));
    expect(t.current).toBe("light");
  });

  it("catches up on refocus", () => {
    const t = make();
    setSystem("light");
    window.dispatchEvent(new Event("focus"));
    expect(t.current).toBe("light");
  });

  it("goes back to dark the same way", () => {
    setSystem("light");
    const t = make();
    setSystem("dark");
    window.dispatchEvent(new Event("focus"));
    expect(t.current).toBe("dark");
    expect(attr()).toBe("dark");
    expect(bar()).toBe("#14130f");
  });

  it("stops listening once destroyed", () => {
    const t = make();
    t.destroy();
    store = null;
    setSystem("light");
    emitChange();
    window.dispatchEvent(new Event("focus"));
    expect(t.current).toBe("dark");
  });

  it("uses addListener where addEventListener isn't there (Safari < 14)", () => {
    installMatchMedia(false);
    const t = make();
    setSystem("light");
    emitChange();
    expect(t.current).toBe("light");
  });
});

describe("theme — explicit choice", () => {
  it("ignores the system but still reports what auto would pick", () => {
    const t = make();
    t.setMode("dark");

    setSystem("light");
    emitChange();
    window.dispatchEvent(new Event("focus"));

    expect(t.current).toBe("dark");
    expect(attr()).toBe("dark");
    expect(t.system).toBe("light"); // Settings' "currently light" note
  });

  it("remembers the choice across a reload", () => {
    make().setMode("light");
    expect(localStorage.getItem("theme")).toBe("light");

    store!.destroy();
    const next = make();
    expect(next.mode).toBe("light");
    expect(next.current).toBe("light");
  });

  it("defaults to auto when nothing is stored, and when junk is", () => {
    expect(make().mode).toBe("auto");
    store!.destroy();
    localStorage.setItem("theme", "sepia");
    expect(make().mode).toBe("auto");
  });

  it("cycles dark → light → auto", () => {
    const t = make();
    t.setMode("dark");
    t.cycle();
    expect(t.mode).toBe("light");
    t.cycle();
    expect(t.mode).toBe("auto");
    t.cycle();
    expect(t.mode).toBe("dark");
  });

  it("picks up the system again the moment auto comes back", () => {
    setSystem("light");
    const t = make();
    t.setMode("dark");
    expect(t.current).toBe("dark");

    t.setMode("auto");
    expect(t.current).toBe("light");
    expect(attr()).toBe("light");
  });
});
