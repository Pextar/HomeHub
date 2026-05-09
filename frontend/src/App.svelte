<script lang="ts">
    import { onMount } from "svelte";
    import Sidebar from "./components/Sidebar.svelte";
    import Toasts from "./components/Toasts.svelte";
    import ModalRoot from "./components/ModalRoot.svelte";
    import Dashboard from "./views/Dashboard.svelte";
    import Sockets from "./views/Sockets.svelte";
    import Schedules from "./views/Schedules.svelte";
    import Groups from "./views/Groups.svelte";
    import Scenes from "./views/Scenes.svelte";
    import { data, route, toasts } from "./lib/stores";
    import type { Route } from "./lib/types";

    // PWA service-worker auto-update via vite-plugin-pwa.
    // The virtual module is generated at build time.
    onMount(async () => {
        try {
            const { registerSW } = await import("virtual:pwa-register");
            registerSW({
                onNeedRefresh() {
                    toasts.info("Update ready", "A new version is available — reload to apply.");
                },
                onOfflineReady() {
                    toasts.success("Ready offline", "The app is installed and works without network.");
                },
            });
        } catch {
            // Service workers might not be available (e.g. in dev or without HTTPS).
        }

        await Promise.all([data.refresh(), data.pingHealth()]);
        const refresh = window.setInterval(() => data.refresh(), 30_000);
        const ping = window.setInterval(() => data.pingHealth(), 30_000);
        return () => {
            window.clearInterval(refresh);
            window.clearInterval(ping);
        };
    });

    const views: Record<Route, any> = {
        dashboard: Dashboard,
        sockets: Sockets,
        groups: Groups,
        scenes: Scenes,
        schedules: Schedules,
    };
    const Current = $derived(views[route.current]);
</script>

<a class="skip-link" href="#main">Skip to main content</a>

<div class="app">
    <Sidebar />
    <main id="main" class="main" tabindex="-1">
        <Current />
    </main>
</div>

<Toasts />
<ModalRoot />

<style>
    .app {
        display: grid;
        grid-template-columns: 248px 1fr;
        min-height: 100vh;
    }
    .main {
        min-width: 0;
        padding: var(--space-8);
        display: flex;
        flex-direction: column;
        gap: var(--space-6);
    }
    @media (max-width: 900px) {
        .app { grid-template-columns: 1fr; }
        .main { padding: var(--space-5); }
    }
</style>
