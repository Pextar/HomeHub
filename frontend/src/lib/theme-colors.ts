/**
 * The two chrome colours, in the one place everything reads them from.
 *
 * These are `--bg` for each theme (app.css §3). They have to be restated
 * outside CSS because three consumers can't reach a custom property: the
 * `theme-color` meta tag that paints the status bar, the pre-paint script in
 * index.html, and the web app manifest that Vite generates — and the manifest
 * is the copy nobody remembers exists. `theme-colors.test.ts` reads the values
 * back out of app.css and fails if this file has drifted from it.
 */
export const BAR_COLOR = {
  dark: "#14130f",
  light: "#f5f1ea",
} as const;

export type Resolved = keyof typeof BAR_COLOR;
