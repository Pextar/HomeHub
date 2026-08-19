import { describe, it, expect, beforeEach, vi } from "vitest";
import { withRoot } from "../../test-runes.svelte";
import { createSoftKeyboard, type SoftKeyboard } from "./keyboard.svelte";

/**
 * How much of the screen the keyboard has taken, as the two surfaces that
 * lay themselves out around it need to know it.
 *
 * The case worth pinning is the first one: a surface can mount with the
 * keyboard already up — on the wall, where the music depth is a route away,
 * that is the ordinary way back into it — and the copy that didn't measure on
 * mount reported nothing until the next resize.
 */

type Listener = () => void;

/** A stand-in visualViewport the test can move. */
function viewport(height: number) {
  const listeners: Listener[] = [];
  const vv = {
    height,
    offsetTop: 0,
    addEventListener: (_: string, fn: Listener) => listeners.push(fn),
    removeEventListener: (_: string, fn: Listener) => {
      const i = listeners.indexOf(fn);
      if (i >= 0) listeners.splice(i, 1);
    },
  };
  return {
    vv,
    /** The keyboard opens (or closes), and the viewport says so. */
    resizeTo(h: number) {
      vv.height = h;
      for (const fn of [...listeners]) fn();
    },
    get listenerCount() {
      return listeners.length;
    },
  };
}

beforeEach(() => {
  vi.stubGlobal("window", { innerHeight: 1000, visualViewport: undefined });
});

/** Mount a keyboard over a viewport of `height`, as if the page just loaded. */
function mount(height: number) {
  (window as unknown as { visualViewport: unknown }).visualViewport = viewport(height).vv;
  return withRoot<SoftKeyboard>(() => createSoftKeyboard());
}

describe("the software keyboard", () => {
  it("takes a reading on mount, for a surface that arrives with it already up", () => {
    const port = viewport(600); // 400px of a 1000px window is gone
    (window as unknown as { visualViewport: unknown }).visualViewport = port.vv;
    const h = withRoot<SoftKeyboard>(() => createSoftKeyboard());
    expect(h.value.height).toBe(400);
    expect(h.value.open).toBe(true);
    h.stop();
  });

  it("reports nothing on a screen with no keyboard covering it", () => {
    const h = mount(1000);
    expect(h.value.height).toBe(0);
    expect(h.value.open).toBe(false);
    h.stop();
  });

  it("follows the keyboard up and down", () => {
    const port = viewport(1000);
    (window as unknown as { visualViewport: unknown }).visualViewport = port.vv;
    const h = withRoot<SoftKeyboard>(() => createSoftKeyboard());
    expect(h.value.open).toBe(false);

    port.resizeTo(650);
    h.flush();
    expect(h.value.height).toBe(350);
    expect(h.value.open).toBe(true);

    port.resizeTo(1000);
    h.flush();
    expect(h.value.open).toBe(false);
    h.stop();
  });

  it("won't call a toolbar a keyboard", () => {
    // A collapsing URL bar takes a slice off the viewport too, and a layout
    // that went dense for it would flinch on every scroll.
    const port = viewport(1000);
    (window as unknown as { visualViewport: unknown }).visualViewport = port.vv;
    const h = withRoot<SoftKeyboard>(() => createSoftKeyboard());
    port.resizeTo(900); // 100px
    h.flush();
    expect(h.value.height).toBe(100);
    expect(h.value.open).toBe(false);
    h.stop();
  });

  it("degrades to zero where the browser has no visual viewport at all", () => {
    const h = withRoot<SoftKeyboard>(() => createSoftKeyboard());
    expect(h.value.height).toBe(0);
    expect(h.value.open).toBe(false);
    h.stop();
  });

  it("lets go of its listeners when the surface goes", () => {
    const port = viewport(1000);
    (window as unknown as { visualViewport: unknown }).visualViewport = port.vv;
    const h = withRoot<SoftKeyboard>(() => createSoftKeyboard());
    expect(port.listenerCount).toBe(2);
    h.stop();
    expect(port.listenerCount).toBe(0);
  });
});
