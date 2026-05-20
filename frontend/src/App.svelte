<script lang="ts">
    import { onMount } from "svelte";
    import Sidebar from "./components/Sidebar.svelte";
    import Toasts from "./components/Toasts.svelte";
    import ModalRoot from "./components/ModalRoot.svelte";
    import LoginGate from "./components/LoginGate.svelte";
    import Dashboard from "./views/Dashboard.svelte";
    import FloorPlan from "./views/FloorPlan.svelte";
    import Sockets from "./views/Sockets.svelte";
    import Schedules from "./views/Schedules.svelte";
    import Groups from "./views/Groups.svelte";
    import Scenes from "./views/Scenes.svelte";
    import Sensors from "./views/Sensors.svelte";
    import Users from "./views/Users.svelte";
    import Settings from "./views/Settings.svelte";
    import KidHome from "./views/KidHome.svelte";
    import { data, route, toasts, session } from "./lib/stores.svelte";
    import { fly } from "svelte/transition";
    import { cubicOut } from "svelte/easing";
    import { dur } from "./lib/motion";
    import type { Route } from "./lib/types";

    // PWA SW registration is auth-free — register early so updates are
    // tracked even if the user hasn't logged in yet. `updateSW(true)` reloads
    // the page with the new service worker active.
    let updateSW: ((reload?: boolean) => Promise<void>) | undefined;
    onMount(async () => {
        try {
            const { registerSW } = await import("virtual:pwa-register");
            updateSW = registerSW({
                onRegisteredSW(_url, r) {
                    // Poll for a new SW once an hour while the app stays open.
                    // Without this an iOS PWA left on screen would never notice
                    // an update until the user manually reloaded.
                    if (r) {
                        setInterval(() => { r.update().catch(() => {}); }, 60 * 60 * 1000);
                    }
                },
                onNeedRefresh() {
                    toasts.show({
                        title: "Update ready",
                        message: "A new version is available.",
                        tone: "info",
                        timeoutMs: 0,
                        action: { label: "Refresh", onClick: () => updateSW?.(true) },
                    });
                },
                onOfflineReady() {
                    toasts.success("Ready offline", "The app is installed and works without network.");
                },
            });
        } catch {
            // Service workers might not be available (e.g. in dev or without HTTPS).
        }
    });

    // LoginGate calls onAuthed once it knows the user is signed in. Load the
    // profile first (it decides what's visible), then start the refresh cycle.
    let started = false;
    async function onAuthed() {
        if (started) return;
        started = true;
        await session.load();
        data.refresh();
        data.pingHealth();
        // Polling is the backstop; SSE pushes updates instantly when a socket
        // changes (manual, scheduler, timer — or a physical remote).
        window.setInterval(() => data.refresh(), 30_000);
        window.setInterval(() => data.pingHealth(), 30_000);
        connectEvents();
    }

    // Live updates via Server-Sent Events. The browser auto-reconnects on
    // error, so we just (re)attach handlers. Refreshes are debounced so a
    // burst of changes (e.g. "all off") collapses into a single fetch.
    let refreshTimer: ReturnType<typeof setTimeout> | undefined;
    function connectEvents() {
        try {
            const es = new EventSource("/api/events");
            es.addEventListener("changed", () => {
                clearTimeout(refreshTimer);
                refreshTimer = setTimeout(() => data.refresh(), 250);
            });
        } catch {
            // EventSource unavailable — the polling interval still covers us.
        }
    }

    const views: Record<Route, any> = {
        dashboard: Dashboard,
        floorplan: FloorPlan,
        sockets: Sockets,
        groups: Groups,
        scenes: Scenes,
        schedules: Schedules,
        sensors: Sensors,
        users: Users,
        settings: Settings,
    };

    // Routes a non-admin profile is allowed to open. Everything else is
    // admin-only; deep-linking elsewhere bounces back to the dashboard.
    const ADMIN_ONLY: Route[] = ["floorplan", "groups", "scenes", "schedules", "sensors", "users", "settings"];
    const effectiveRoute = $derived(
        !session.isAdmin && ADMIN_ONLY.includes(route.current) ? "dashboard" : route.current,
    );
    const Current = $derived(views[effectiveRoute]);
</script>

<LoginGate {onAuthed}>
    {#if !session.loaded}
        <div class="boot"></div>
    {:else if session.user?.kid}
        <KidHome />
    {:else}
        <a class="skip-link" href="#main">Skip to main content</a>

        <div class="app">
            <Sidebar />
            <main id="main" class="main" tabindex="-1">
                {#key effectiveRoute}
                    <div class="view" in:fly={{ y: 10, duration: dur(220), easing: cubicOut }}>
                        <Current />
                    </div>
                {/key}
            </main>
        </div>
    {/if}
</LoginGate>

<Toasts />
<ModalRoot />

<style>
    .boot { min-height: 100vh; background: var(--bg); }
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
    }
    .view {
        display: flex;
        flex-direction: column;
        gap: var(--space-6);
    }
    @media (max-width: 900px) {
        .app { grid-template-columns: 1fr; }
        .main {
            padding: var(--space-4);
            padding-bottom: calc(var(--space-4) + 60px + env(safe-area-inset-bottom));
        }
    }
</style>
