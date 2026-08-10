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
    // Vitest stubs CSS imports out to nothing by default, `?raw` included.
    // theme-colors.test.ts reads app.css back to check the palette copies
    // haven't drifted, so let that one file through — processing the rest
    // would only cost time no test spends.
    css: { include: [/app\.css/] },
    setupFiles: ["./src/test-setup.ts"],
    include: ["src/**/*.{test,spec}.ts"],
  },
});
