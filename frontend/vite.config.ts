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
      includeAssets: ["pwa-icon.svg"],
      manifest: {
        name: "RF Socket Controller",
        short_name: "RF Sockets",
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
      workbox: {
        // Shell-only caching — /api always goes to the network so the app
        // never returns stale or fake socket state. The HTML/CSS/JS shell
        // is cached so the app loads when the Pi is offline.
        navigateFallback: "/index.html",
        navigateFallbackDenylist: [/^\/api/],
        runtimeCaching: [
          {
            urlPattern: ({ url }) => url.pathname.startsWith("/api"),
            handler: "NetworkOnly",
          },
        ],
        globPatterns: ["**/*.{js,css,html,svg,ico,png,webmanifest}"],
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
