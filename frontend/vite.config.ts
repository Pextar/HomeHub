import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import { VitePWA } from "vite-plugin-pwa";

// During `vite dev` we proxy /api to the Go backend running on :8080 so the
// dev server (5173) and the API can co-exist. In production, the Go server
// serves the built dist/ directly and /api hits the same origin.
export default defineConfig({
  plugins: [
    svelte(),
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
        theme_color: "#0b1020",
        background_color: "#0b1020",
        display: "standalone",
        orientation: "any",
        start_url: "/",
        scope: "/",
        icons: [
          { src: "pwa-icon.svg", sizes: "any", type: "image/svg+xml", purpose: "any" },
          { src: "pwa-icon.svg", sizes: "any", type: "image/svg+xml", purpose: "maskable" },
        ],
        shortcuts: [
          { name: "Sockets",   short_name: "Sockets",   url: "/#/sockets" },
          { name: "Scenes",    short_name: "Scenes",    url: "/#/scenes" },
          { name: "Schedules", short_name: "Schedules", url: "/#/schedules" },
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
