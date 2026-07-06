import { vi } from "vitest";

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
