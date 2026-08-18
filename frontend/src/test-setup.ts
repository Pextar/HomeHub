// Global test setup, for both layers of the suite.
//
// The lib layer tests stores and pure functions; the component layer mounts
// real components into jsdom and queries them the way a reader would (by
// role, by label), so an assertion fails for the same reason a person would
// notice — a control that lost its name, a state that stopped being drawn.
//
// Two jsdom gaps are patched below. Neither is a workaround for a bug in the
// app: jsdom simply implements less of the browser than Svelte uses.
//
// Writing a component test:
//   import { render, screen } from "@testing-library/svelte";
//   render(MyComponent, { ...props });
//   screen.getByRole("button", { name: "Play" }).click();
//
// Runes do not work inside a .test.ts file — those are not run through the
// Svelte compiler. Use a plain object for props a component mutates, or
// src/test-runes.svelte.ts when a store's own reactivity is the subject.

import { vi, afterEach } from "vitest";
import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/svelte";

// Every component test unmounts what it mounted. Without this a test that
// queries by role finds the previous test's DOM as well as its own.
afterEach(cleanup);

// jsdom doesn't implement matchMedia, but the theme store reads it at import
// time to pick an initial theme. Provide a minimal stub so the lib module
// graph loads under test.
if (!window.matchMedia) {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }));
}

// jsdom implements no Web Animations API, and Svelte's transitions drive
// every in:/out: through `element.animate`. Any component with a transition
// on it would throw on mount rather than fail an assertion. The stub is a
// finished animation: the DOM lands in its final state immediately, which is
// what a test wants to query anyway — and matches how the app behaves under
// `prefers-reduced-motion`, which the design system already collapses to.
if (!Element.prototype.animate) {
  Element.prototype.animate = function animate() {
    return {
      cancel() {},
      finish() {},
      pause() {},
      play() {},
      reverse() {},
      addEventListener() {},
      removeEventListener() {},
      currentTime: 0,
      playState: "finished",
      finished: Promise.resolve(),
    } as unknown as Animation;
  };
}
