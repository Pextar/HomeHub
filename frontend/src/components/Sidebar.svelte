<script lang="ts">
    import Icon from "./Icon.svelte";
    import ConfirmModal from "./ConfirmModal.svelte";
    import { route, theme, data } from "../lib/stores.svelte";
    import { api } from "../lib/api";
    import { openModal } from "../lib/modal.svelte";
    import { fly, fade } from "svelte/transition";
    import { cubicOut } from "svelte/easing";
    import { dur } from "../lib/motion";
    import type { Route } from "../lib/types";

    async function signOut() {
        moreOpen = false;
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Sign out?",
            message: "You'll need to enter your username and password again to get back in.",
            confirmLabel: "Sign out",
        });
        if (!ok) return;
        try { await api.logout(); } catch { /* ignore */ }
        window.location.reload();
    }

    type NavItem = { route: Route; icon: any; label: string };

    // First four are surfaced as primary tabs in the mobile bottom nav.
    // The rest move into the "More" drawer on mobile, but all six show in
    // the desktop sidebar.
    const PRIMARY_COUNT = 4;
    const items: NavItem[] = [
        { route: "dashboard", icon: "home",      label: "Dashboard" },
        { route: "sockets",   icon: "socket",    label: "Devices" },
        { route: "sensors",   icon: "sensor",    label: "Sensors" },
        { route: "schedules", icon: "clock",     label: "Schedules" },
        { route: "scenes",    icon: "scenes",    label: "Scenes" },
        { route: "groups",    icon: "groups",    label: "Groups" },
        { route: "settings",  icon: "settings",  label: "Settings" },
    ];
    const primary = items.slice(0, PRIMARY_COUNT);
    const overflow = items.slice(PRIMARY_COUNT);

    let moreOpen = $state(false);

    // Auto-close the drawer whenever navigation happens.
    $effect(() => {
        // Reading route.current registers the dependency.
        route.current;
        moreOpen = false;
    });

    function toggleTheme() {
        theme.toggle();
    }

    function onKey(e: KeyboardEvent) {
        if (e.key === "Escape" && moreOpen) moreOpen = false;
    }

    // True when the active route is one of the overflow items — used to
    // highlight the "More" tab so the user knows where they are.
    const moreActive = $derived(overflow.some(i => i.route === route.current));

    const healthLabel = $derived(
        data.value.health === "ok" ? "Connected" :
        data.value.health === "error" ? "Backend offline" : "Connecting…"
    );
</script>

<svelte:window onkeydown={onKey} />

<aside class="sidebar" aria-label="Primary">
    <div class="brand">
        <div class="mark" aria-hidden="true">
            <Icon name="bolt" size={20} />
        </div>
        <div>
            <div class="name">HomeHub</div>
            <div class="sub">Smart Home</div>
        </div>
    </div>

    <!-- Desktop: full list. Mobile: only the primary slice (the rest live in
         the More drawer). -->
    <nav class="nav nav-desktop" aria-label="Sections">
        {#each items as item (item.route)}
            <a
                href="#/{item.route}"
                class="nav-item"
                aria-current={route.current === item.route ? "page" : undefined}
            >
                <Icon name={item.icon} size={18} />
                <span class="nav-label">{item.label}</span>
            </a>
        {/each}
    </nav>

    <nav class="nav nav-mobile" aria-label="Sections">
        {#each primary as item (item.route)}
            <a
                href="#/{item.route}"
                class="nav-item"
                aria-current={route.current === item.route ? "page" : undefined}
            >
                <Icon name={item.icon} size={20} />
                <span class="nav-label">{item.label}</span>
            </a>
        {/each}
        <button
            class="nav-item more-btn"
            aria-haspopup="menu"
            aria-expanded={moreOpen}
            aria-current={moreActive && !moreOpen ? "page" : undefined}
            onclick={() => (moreOpen = !moreOpen)}
        >
            <Icon name="more" size={20} />
            <span class="nav-label">More</span>
        </button>
    </nav>

    <div class="footer">
        <button class="theme-toggle" aria-label="Toggle theme" onclick={toggleTheme}>
            <Icon name={theme.current === "dark" ? "moon" : "sun"} size={14} />
            <span>Theme</span>
        </button>
        <button class="theme-toggle" aria-label="Sign out" onclick={signOut}>
            <Icon name="logout" size={14} />
            <span>Sign out</span>
        </button>
        <div class="health" aria-live="polite">
            <span class="dot" data-state={data.value.health}></span>
            <span class="label">{healthLabel}</span>
        </div>
    </div>
</aside>

<!-- Mobile-only overflow drawer (bottom sheet). -->
{#if moreOpen}
    <div
        class="drawer-root"
        role="presentation"
        onclick={(e) => { if (e.target === e.currentTarget) moreOpen = false; }}
        transition:fade={{ duration: dur(150) }}
    >
        <div
            class="drawer"
            role="menu"
            aria-label="More options"
            transition:fly={{ y: 24, duration: dur(220), easing: cubicOut }}
        >
            <div class="drawer-handle" aria-hidden="true"></div>

            <div class="drawer-section" aria-label="Sections">
                {#each overflow as item (item.route)}
                    <a
                        href="#/{item.route}"
                        class="drawer-item"
                        role="menuitem"
                        aria-current={route.current === item.route ? "page" : undefined}
                    >
                        <span class="drawer-icon"><Icon name={item.icon} size={20} /></span>
                        <span class="drawer-label">{item.label}</span>
                    </a>
                {/each}
            </div>

            <div class="drawer-section" aria-label="Settings">
                <button class="drawer-item" role="menuitem" onclick={() => { toggleTheme(); }}>
                    <span class="drawer-icon">
                        <Icon name={theme.current === "dark" ? "sun" : "moon"} size={20} />
                    </span>
                    <span class="drawer-label">
                        {theme.current === "dark" ? "Light theme" : "Dark theme"}
                    </span>
                </button>
                <button class="drawer-item danger" role="menuitem" onclick={signOut}>
                    <span class="drawer-icon"><Icon name="logout" size={20} /></span>
                    <span class="drawer-label">Sign out</span>
                </button>
            </div>

            <div class="drawer-health" aria-live="polite">
                <span class="dot" data-state={data.value.health}></span>
                <span>{healthLabel}</span>
            </div>
        </div>
    </div>
{/if}

<style>
    .sidebar {
        background: var(--bg-elevated);
        border-right: 1px solid var(--border);
        padding: var(--space-6) var(--space-4);
        display: flex;
        flex-direction: column;
        position: sticky;
        top: 0;
        height: 100vh;
    }
    .brand {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: 0 var(--space-2) var(--space-6);
        border-bottom: 1px solid var(--border);
        margin-bottom: var(--space-4);
    }
    .mark {
        width: 36px; height: 36px;
        border-radius: var(--radius-md);
        background: var(--gradient-brand);
        display: grid; place-items: center;
        color: #fff;
        box-shadow: 0 4px 12px var(--primary-glow);
        flex-shrink: 0;
    }
    .name {
        font-weight: 800;
        letter-spacing: -0.02em;
        background: var(--gradient-brand);
        -webkit-background-clip: text;
        -webkit-text-fill-color: transparent;
        background-clip: text;
    }
    .sub { font-size: 12px; color: var(--text-faint); }

    .nav { display: flex; flex-direction: column; gap: 2px; }
    .nav-mobile { display: none; }
    .nav-item {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: 10px var(--space-3);
        border-radius: var(--radius-md);
        color: var(--text-muted);
        transition: background var(--t-fast), color var(--t-fast);
        cursor: pointer;
        background: transparent;
        border: none;
        text-align: left;
        font: inherit;
        width: 100%;
    }
    .nav-item:hover { background: var(--surface-hover); color: var(--text); }
    .nav-item[aria-current="page"] {
        background: var(--primary-soft);
        color: var(--primary);
        font-weight: 600;
        box-shadow: inset 3px 0 0 var(--primary);
    }
    .nav-item[aria-current="page"] :global(svg) { color: var(--primary); }

    .footer {
        margin-top: auto;
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
        padding-top: var(--space-4);
        border-top: 1px solid var(--border);
    }
    .theme-toggle {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: 8px var(--space-3);
        border: 1px solid var(--border);
        background: transparent;
        border-radius: var(--radius-md);
        cursor: pointer;
        color: var(--text-muted);
        transition: background var(--t-fast), color var(--t-fast);
    }
    .theme-toggle:hover { background: var(--surface-hover); color: var(--text); }

    .health {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        color: var(--text-muted);
        font-size: 12px;
        padding: 0 var(--space-3);
    }
    .dot {
        width: 8px; height: 8px; border-radius: 50%;
        background: var(--text-faint);
        flex-shrink: 0;
        transition: background var(--t-med), box-shadow var(--t-med);
    }
    .dot[data-state="ok"] {
        background: var(--success);
        box-shadow: 0 0 0 3px var(--success-soft);
        animation: pulse-dot 2.5s ease-in-out infinite;
    }
    .dot[data-state="error"] { background: var(--danger); box-shadow: 0 0 0 3px var(--danger-soft); }
    @keyframes pulse-dot {
        0%, 100% { box-shadow: 0 0 0 3px var(--success-soft); }
        50%       { box-shadow: 0 0 0 5px var(--success-soft); }
    }

    /* ---------- Mobile bottom nav — iOS tab bar ---------- */
    @media (max-width: 900px) {
        .sidebar {
            position: fixed;
            bottom: 0; left: 0; right: 0;
            top: auto;
            height: auto;
            flex-direction: row;
            align-items: stretch;
            border-right: none;
            /* Frosted-glass iOS tab bar */
            background: var(--bg-bar);
            backdrop-filter: saturate(180%) blur(24px);
            -webkit-backdrop-filter: saturate(180%) blur(24px);
            /* iOS hairline separator — no heavy shadow */
            border-top: 0.5px solid var(--separator);
            box-shadow: none;
            padding: 0;
            padding-bottom: env(safe-area-inset-bottom);
            z-index: 100;
            gap: 0;
        }
        .brand { display: none; }
        .footer { display: none; }
        .nav-desktop { display: none; }
        .nav-mobile { display: flex; flex: 1; flex-direction: row; gap: 0; }
        .nav-mobile .nav-item {
            flex: 1;
            flex-direction: column;
            align-items: center;
            justify-content: center;
            gap: 3px;
            padding: 8px 4px;
            border-radius: 0;
            font-size: 10px;    /* iOS tab label size */
            min-height: 49px;   /* iOS standard tab bar height */
            color: var(--text-faint);
            text-align: center;
            width: auto;
        }
        /* Scale up icons slightly in the tab bar */
        .nav-mobile .nav-item :global(svg) { width: 24px; height: 24px; }
        /* iOS: active = tint color only, no indicator line */
        .nav-mobile .nav-item[aria-current="page"] {
            background: transparent;
            color: var(--primary);
            box-shadow: none;
        }
        .nav-mobile .nav-item[aria-current="page"] :global(svg) {
            color: var(--primary);
        }
        .nav-mobile .nav-label {
            line-height: 1;
            font-weight: 400;       /* iOS uses regular weight for tab labels */
            letter-spacing: 0;
        }
        .nav-mobile .nav-item[aria-current="page"] .nav-label {
            font-weight: 500;       /* Slightly heavier on active — subtle cue */
        }
    }

    /* ---------- Drawer (bottom sheet) ---------- */
    .drawer-root {
        position: fixed; inset: 0;
        background: rgba(8, 11, 22, 0.5);
        backdrop-filter: blur(3px);
        z-index: 120;
        display: flex;
        align-items: flex-end;
        justify-content: center;
    }
    :global([data-theme="light"]) .drawer-root {
        background: rgba(20, 24, 38, 0.35);
    }
    .drawer {
        width: 100%;
        background: var(--bg-bar);
        backdrop-filter: saturate(180%) blur(24px);
        -webkit-backdrop-filter: saturate(180%) blur(24px);
        border-top: 0.5px solid var(--separator);
        border-top-left-radius: var(--radius-xl);
        border-top-right-radius: var(--radius-xl);
        padding: var(--space-3) var(--space-4)
                 calc(var(--space-4) + 56px + env(safe-area-inset-bottom));
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
        box-shadow: var(--shadow-lg);
    }
    .drawer-handle {
        width: 36px;
        height: 5px;
        border-radius: 999px;
        background: var(--border-strong);
        margin: 4px auto var(--space-2);
    }
    .drawer-section {
        display: flex;
        flex-direction: column;
        gap: 2px;
        padding: var(--space-1) 0;
    }
    .drawer-section + .drawer-section {
        border-top: 1px solid var(--border);
        padding-top: var(--space-2);
        margin-top: var(--space-1);
    }
    .drawer-item {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: 14px var(--space-3);
        border-radius: var(--radius-md);
        color: var(--text);
        background: transparent;
        border: none;
        cursor: pointer;
        font: inherit;
        text-align: left;
        width: 100%;
        transition: background var(--t-fast), color var(--t-fast);
    }
    .drawer-item:hover { background: var(--surface-hover); }
    .drawer-item:active { background: var(--surface); }
    .drawer-item[aria-current="page"] {
        background: var(--surface);
        color: var(--primary);
        font-weight: 600;
    }
    .drawer-item[aria-current="page"] :global(svg) { color: var(--primary); }
    .drawer-item.danger { color: var(--danger); }
    .drawer-icon {
        width: 28px;
        display: grid;
        place-items: center;
        color: var(--text-muted);
    }
    .drawer-item[aria-current="page"] .drawer-icon,
    .drawer-item.danger .drawer-icon { color: inherit; }
    .drawer-label { font-size: 15px; }
    .drawer-health {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        color: var(--text-muted);
        font-size: 12px;
        padding: var(--space-2) var(--space-3) 0;
        border-top: 1px solid var(--border);
        margin-top: var(--space-1);
    }

    /* Hide the drawer entirely on desktop — it's a mobile-only affordance. */
    @media (min-width: 901px) {
        .drawer-root { display: none; }
    }
</style>
