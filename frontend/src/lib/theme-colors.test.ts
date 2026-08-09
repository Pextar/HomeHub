import { describe, it, expect } from "vitest";
import css from "../app.css?raw";
import viteConfig from "../../vite.config.ts?raw";
import indexHtml from "../../index.html?raw";
import { BAR_COLOR } from "./theme-colors";

// `--bg` lives in app.css, but three consumers can't resolve a custom property
// and so keep their own copy: the theme store (BAR_COLOR, for the status-bar
// meta tag), the pre-paint script in index.html, and the PWA manifest Vite
// generates from vite.config.ts. The manifest is the one nobody remembers —
// it sets the launch splash and the window chrome, so a stale value there
// shows up as a dark flash on light-mode devices and nowhere else. Read the
// real files back and hold all of them to the stylesheet.

/** `--bg` from the block that opens with the given selector. */
function bgFor(selector: string): string | undefined {
  const start = css.indexOf(selector);
  if (start === -1) return undefined;
  const block = css.slice(start, css.indexOf("}", start));
  return block.match(/--bg:\s*(#[0-9a-fA-F]{3,8})\s*;/)?.[1];
}

describe("chrome colours", () => {
  it("reads both themes out of app.css", () => {
    // Guards the two checks below: a rename of the theme selectors would
    // otherwise quietly turn them into undefined === undefined.
    expect(bgFor('[data-theme="dark"]')).toMatch(/^#[0-9a-f]{6}$/);
    expect(bgFor('[data-theme="light"]')).toMatch(/^#[0-9a-f]{6}$/);
  });

  it("matches what the theme store paints the status bar", () => {
    expect(BAR_COLOR.dark).toBe(bgFor('[data-theme="dark"]'));
    expect(BAR_COLOR.light).toBe(bgFor('[data-theme="light"]'));
  });

  it("matches the manifest's launch colours", () => {
    // The manifest can't carry a media query, so both are deliberately the
    // dark value — the app's resting identity, and what the platform paints
    // before any of our code can have an opinion.
    const declared = viteConfig.match(
      /const BAR_COLOR = \{ dark: "(#[0-9a-f]{6})", light: "(#[0-9a-f]{6})" \};/,
    );
    expect(declared, "vite.config.ts no longer declares BAR_COLOR as expected").not.toBeNull();
    expect(declared![1]).toBe(BAR_COLOR.dark);
    expect(declared![2]).toBe(BAR_COLOR.light);
    expect(viteConfig).toContain("theme_color: BAR_COLOR.dark");
    expect(viteConfig).toContain("background_color: BAR_COLOR.dark");
  });

  it("keeps index.html on placeholders rather than a fourth copy", () => {
    expect(indexHtml).toContain("__THEME_DARK__");
    expect(indexHtml).toContain("__THEME_LIGHT__");
    // Anything the build substitutes can't also be pasted in by hand.
    expect(indexHtml).not.toMatch(/#[0-9a-fA-F]{6}/);
  });
});
