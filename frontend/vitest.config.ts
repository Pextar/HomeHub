import { defineConfig } from "vitest/config";
import { svelte } from "@sveltejs/vite-plugin-svelte";

// Standalone Vitest config (kept separate from vite.config.ts so the PWA
// plugin and dev-server proxy don't load during tests). The Svelte plugin is
// still needed so `.svelte` / `.svelte.ts` modules — which the lib layer
// imports transitively — compile their runes.
export default defineConfig({
  plugins: [svelte()],
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test-setup.ts"],
    include: ["src/**/*.{test,spec}.ts"],
  },
});
