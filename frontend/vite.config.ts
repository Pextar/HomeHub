import { defineConfig, type Plugin } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import { VitePWA } from "vite-plugin-pwa";

// `--bg` per theme, again — the config can't import from src/ without pulling
// this file into the app's TS project, and the manifest below needs the value
// at build time. `src/lib/theme-colors.test.ts` holds this copy, the one in
// src/, and app.css itself to the same values, so the duplication can't rot.
const BAR_COLOR = { dark: "#14130f", light: "#f5f1ea" };

// index.html has to settle the theme before the first paint, which means the
// two chrome colours appear in a plain <script> that can't import anything.
// Substituting them keeps that script honest instead of leaving yet another
// hand-maintained copy of the palette in a file nobody diffs.
function themeColors(): Plugin {
  return {
    name: "homehub:theme-colors",
    transformIndexHtml(html) {
      return html
        .replaceAll("__THEME_DARK__", BAR_COLOR.dark)
        .replaceAll("__THEME_LIGHT__", BAR_COLOR.light);
    },
  };
}

// During `vite dev` we proxy /api to the Go backend running on :8080 so the
// dev server (5173) and the API can co-exist. In production, the Go server
// serves the built dist/ directly and /api hits the same origin.
export default defineConfig({
  plugins: [
    svelte(),
    themeColors(),
    VitePWA({
      // "prompt" instead of "autoUpdate": we want onNeedRefresh to fire so the
      // app can show a "Refresh" toast button. autoUpdate skips the waiting
      // phase, which means the prompt never shows on an always-open PWA.
      registerType: "prompt",
      // injectManifest lets us write a custom service worker (sw.ts) so we
      // can handle the Web Push `push` and `notificationclick` events
      // alongside the standard Workbox precaching and offline logic.
      strategies: "injectManifest",
      srcDir: "src",
      filename: "sw.ts",
      injectManifest: {
        // Cache the same file types as the previous generateSW setup.
        globPatterns: ["**/*.{js,css,html,svg,ico,png,webmanifest}"],
      },
      includeAssets: ["pwa-icon.svg"],
      manifest: {
        name: "HomeHub",
        short_name: "HomeHub",
        description: "Control 433MHz RF sockets from anywhere",
        // A manifest can't carry a media query, so these two are necessarily
        // one theme's values: they set the launch splash and the window chrome
        // the platform paints *before* any of our code runs. Dark is the app's
        // resting identity, so it wins the tie. Once the document is up, the
        // `theme-color` meta tag the theme store maintains overrides
        // `theme_color` — the manifest is the cold-start default, not the
        // live value.
        theme_color: BAR_COLOR.dark,
        background_color: BAR_COLOR.dark,
        display: "standalone",
        orientation: "any",
        start_url: "/",
        scope: "/",
        icons: [
          { src: "pwa-icon.svg", sizes: "any", type: "image/svg+xml", purpose: "any" },
          { src: "pwa-icon.svg", sizes: "any", type: "image/svg+xml", purpose: "maskable" },
        ],
        shortcuts: [
          { name: "Sockets", short_name: "Sockets", url: "/#/sockets" },
          { name: "Scenes", short_name: "Scenes", url: "/#/scenes" },
          { name: "Schedules", short_name: "Schedules", url: "/#/schedules" },
          { name: "Panel", short_name: "Panel", url: "/#/panel" },
        ],
      },
    }),
  ],
  server: {
    port: 5173,
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
  build: {
    outDir: "dist",
    sourcemap: false,
  },
});
