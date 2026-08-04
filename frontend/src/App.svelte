<script lang="ts">
    import { onMount } from "svelte";
    import Sidebar from "./components/Sidebar.svelte";
    import Toasts from "./components/Toasts.svelte";
    import ModalRoot from "./components/ModalRoot.svelte";
    import LoginGate from "./components/LoginGate.svelte";
    import Dashboard from "./views/Dashboard.svelte";
    import Rooms from "./views/Rooms.svelte";
    import FloorPlan from "./views/FloorPlan.svelte";
    import Sockets from "./views/Sockets.svelte";
    import Music from "./views/Music.svelte";
    import Automations from "./views/Automations.svelte";
    import Groups from "./views/Groups.svelte";
    import Scenes from "./views/Scenes.svelte";
    import Sensors from "./views/Sensors.svelte";
    import Insights from "./views/Insights.svelte";
    import Activity from "./views/Activity.svelte";
    import Users from "./views/Users.svelte";
    import Settings from "./views/Settings.svelte";
    import Console from "./views/Console.svelte";
    import KidHome from "./views/KidHome.svelte";
    import Panel from "./views/Panel.svelte";
    import AssistantLauncher from "./components/AssistantLauncher.svelte";
    import { data, route, toasts, session, uiPrefs } from "./lib/stores.svelte";
    import { onLive } from "./lib/live";
    import { panelIdleMs } from "./lib/panel";
    import { closeModal, modalStack } from "./lib/modal.svelte";
    import { pullToRefresh } from "./lib/pull-refresh";
    import { fly, fade } from "svelte/transition";
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
                        setInterval(
                            () => {
                                r.update().catch(() => {});
                            },
                            60 * 60 * 1000,
                        );
                        // iOS suspends a backgrounded home-screen PWA's timers
                        // entirely, so the hourly poll above never fires across
                        // a close/reopen — the app can sit on a stale build for
                        // days. Check the moment it's looked at again instead of
                        // waiting on a timer that was never running.
                        document.addEventListener("visibilitychange", () => {
                            if (document.visibilityState === "visible") {
                                r.update().catch(() => {});
                            }
                        });
                    }
                },
                onNeedRefresh() {
                    // A panel-homed device is a wall kiosk: no chrome, and
                    // nobody standing at it to tap a "Refresh" toast — it
                    // would just keep running whatever build it booted with
                    // indefinitely. Update it itself; a reload it drives on
                    // its own is far cheaper than a fix that never arrives.
                    if (uiPrefs.panelHome) {
                        void updateSW?.(true);
                        return;
                    }
                    toasts.show({
                        title: "Update ready",
                        message: "A new version is available.",
                        tone: "info",
                        timeoutMs: 0,
                        action: { label: "Refresh", onClick: () => updateSW?.(true) },
                    });
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

    // Live updates via Server-Sent Events (lib/live.ts owns the connection).
    // Refreshes are debounced so a burst of changes (e.g. "all off")
    // collapses into a single fetch. Music has its own topic and its own
    // subscribers, so speaker chatter never lands here.
    let refreshTimer: ReturnType<typeof setTimeout> | undefined;
    function connectEvents() {
        onLive("changed", () => {
            clearTimeout(refreshTimer);
            refreshTimer = setTimeout(() => data.refresh(), 250);
        });
    }

    const views: Record<Route, any> = {
        dashboard: Dashboard,
        rooms: Rooms,
        floorplan: FloorPlan,
        sockets: Sockets,
        music: Music,
        groups: Groups,
        scenes: Scenes,
        schedules: Automations,
        automations: Automations,
        sensors: Sensors,
        insights: Insights,
        activity: Activity,
        users: Users,
        settings: Settings,
        console: Console,
        panel: Panel,
    };

    // Routes a non-admin profile is allowed to open. Everything else is
    // admin-only; deep-linking elsewhere bounces back to the dashboard.
    const ADMIN_ONLY: Route[] = [
        "rooms",
        "floorplan",
        "music",
        "groups",
        "scenes",
        "schedules",
        "automations",
        "sensors",
        "insights",
        "activity",
        "users",
        "settings",
        "console",
    ];
    const effectiveRoute = $derived(
        !session.isAdmin && ADMIN_ONLY.includes(route.current) ? "dashboard" : route.current,
    );
    const Current = $derived(views[effectiveRoute]);

    // ── Kiosk coherence (DESIGN.md §16) ─────────────────────────────────
    // Entering the panel marks this device as panel-homed: the dashboard
    // route renders the panel in its place, and idling on any other route
    // walks the device back home. The panel's Exit chip clears the mark.
    const panelHome = $derived(uiPrefs.panelHome && !session.user?.kid);
    const showPanel = $derived(
        effectiveRoute === "panel" || (panelHome && effectiveRoute === "dashboard"),
    );

    // Idle auto-return. Armed only while panel-homed and away from the
    // panel; any touch re-arms it. When it fires, open modals are dismissed
    // first — a kiosk must never strand a sheet over its home screen — and
    // it arrives on the ambient face (?idle=1) rather than waking the UI.
    let lastActive = Date.now();
    $effect(() => {
        if (!panelHome || showPanel) return;
        lastActive = Date.now();
        const onTouch = () => {
            lastActive = Date.now();
        };
        window.addEventListener("pointerdown", onTouch, { passive: true });
        const id = setInterval(() => {
            if (Date.now() - lastActive > panelIdleMs()) {
                while (modalStack().length > 0) closeModal();
                route.go("panel", { idle: "1" });
            }
        }, 1000);
        return () => {
            window.removeEventListener("pointerdown", onTouch);
            clearInterval(id);
        };
    });

    // Reset scroll position to the top whenever the user navigates to a
    // different page, so the new view always starts at the top. Also reset
    // horizontal scroll: `scrollTo` leaves an unspecified axis untouched, so
    // any sideways offset picked up on the previous view (e.g. an h-scroll
    // strip rubber-banding the whole page) would otherwise carry over
    // permanently, since nothing else ever resets it.
    $effect(() => {
        effectiveRoute;
        window.scrollTo({ top: 0, left: 0, behavior: "instant" });
    });
</script>

<LoginGate {onAuthed}>
    {#if !session.loaded}
        <div class="boot"></div>
    {:else if session.user?.kid}
        <KidHome />
    {:else if showPanel}
        <!-- The panel is a kiosk surface, not a view inside the shell: no
             sidebar, tab dock, pull-to-refresh or assistant FAB (DESIGN.md §16). -->
        <Panel />
    {:else}
        <a class="skip-link" href="#main">Skip to main content</a>

        <div class="app">
            <Sidebar />
            <main id="main" class="main" tabindex="-1">
                <div class="view-stack" use:pullToRefresh={() => data.refresh()}>
                    {#key effectiveRoute}
                        <div
                            class="view"
                            in:fly={{ y: 10, duration: dur(240), easing: cubicOut }}
                            out:fade={{ duration: dur(140) }}
                        >
                            <Current />
                        </div>
                    {/key}
                </div>
            </main>
        </div>
        <AssistantLauncher />
    {/if}
</LoginGate>

<Toasts />
<ModalRoot />

<style>
    .boot {
        min-height: 100vh;
        background: var(--bg);
    }
    .app {
        /* Flex instead of grid so the sidebar's CSS width transition
           naturally pushes the main content — no grid-template-columns
           animation needed (which browsers don't support anyway). */
        display: flex;
        min-height: 100vh;
    }
    .main {
        flex: 1;
        min-width: 0;
        padding: 28px 36px;
        display: flex;
        flex-direction: column;
    }
    /* Single-cell grid so the outgoing and incoming views overlap during a
       route change instead of stacking and doubling the page height.
       minmax(0, 1fr) caps the column at available width — an implicit `auto`
       column would size to max-content and overflow the container on narrow
       screens when the topbar holds multiple non-shrinkable buttons. */
    .view-stack {
        display: grid;
        grid-template-columns: minmax(0, 1fr);
        min-width: 0;
    }
    .view {
        grid-area: 1 / 1;
        display: flex;
        flex-direction: column;
        gap: var(--space-6);
    }
    @media (max-width: 900px) {
        .main {
            padding: var(--space-4);
            padding-bottom: calc(var(--space-4) + var(--nav-clear));
        }
    }
</style>
