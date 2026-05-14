<script lang="ts">
    import Icon from "./Icon.svelte";
    import ConfirmModal from "./ConfirmModal.svelte";
    import { route, theme, data } from "../lib/stores.svelte";
    import { api } from "../lib/api";
    import { openModal } from "../lib/modal.svelte";
    import type { Route } from "../lib/types";

    async function signOut() {
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Sign out?",
            message: "You'll need to enter your username and password again to get back in.",
            confirmLabel: "Sign out",
        });
        if (!ok) return;
        try { await api.logout(); } catch { /* ignore */ }
        // Reload so LoginGate re-checks auth and shows the login form.
        window.location.reload();
    }

    const items: { route: Route; icon: any; label: string }[] = [
        { route: "dashboard", icon: "dashboard", label: "Dashboard" },
        { route: "sockets",   icon: "socket",    label: "Sockets" },
        { route: "groups",    icon: "groups",    label: "Groups" },
        { route: "scenes",    icon: "scenes",    label: "Scenes" },
        { route: "schedules", icon: "clock",     label: "Schedules" },
    ];
</script>

<aside class="sidebar" aria-label="Primary">
    <div class="brand">
        <div class="mark" aria-hidden="true">
            <Icon name="bolt" size={22} />
        </div>
        <div>
            <div class="name">RF Sockets</div>
            <div class="sub">Controller</div>
        </div>
    </div>

    <nav class="nav" aria-label="Sections">
        {#each items as item (item.route)}
            <a
                href="#/{item.route}"
                class="nav-item"
                aria-current={route.current === item.route ? "page" : undefined}
            >
                <Icon name={item.icon} size={18} />
                {item.label}
            </a>
        {/each}
    </nav>

    <div class="footer">
        <button class="theme-toggle" aria-label="Toggle theme" onclick={() => theme.toggle()}>
            <Icon name={theme.current === "dark" ? "moon" : "sun"} size={14} />
            <span>Theme</span>
        </button>
        <button class="theme-toggle" aria-label="Sign out" onclick={signOut}>
            <Icon name="logout" size={14} />
            <span>Sign out</span>
        </button>
        <div class="health" aria-live="polite">
            <span class="dot" data-state={data.value.health}></span>
            <span class="label">
                {data.value.health === "ok" ? "Connected" :
                 data.value.health === "error" ? "Backend offline" : "Connecting…"}
            </span>
        </div>
    </div>
</aside>

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
        background: linear-gradient(135deg, var(--primary), #7b2cbf);
        display: grid; place-items: center;
        color: #fff;
    }
    .name { font-weight: 700; letter-spacing: -0.01em; }
    .sub { font-size: 12px; color: var(--text-muted); }

    .nav { display: flex; flex-direction: column; gap: 2px; }
    .nav-item {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: 10px var(--space-3);
        border-radius: var(--radius-md);
        color: var(--text-muted);
        transition: background var(--t-fast), color var(--t-fast);
        cursor: pointer;
    }
    .nav-item:hover { background: var(--surface-hover); color: var(--text); }
    .nav-item[aria-current="page"] {
        background: var(--surface);
        color: var(--text);
        font-weight: 600;
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
    }
    .dot[data-state="ok"] { background: var(--success); box-shadow: 0 0 0 3px var(--success-soft); }
    .dot[data-state="error"] { background: var(--danger); box-shadow: 0 0 0 3px var(--danger-soft); }

    @media (max-width: 900px) {
        .sidebar {
            position: fixed;
            bottom: 0; left: 0; right: 0;
            top: auto;
            height: auto;
            flex-direction: row;
            align-items: stretch;
            border-right: none;
            border-top: 1px solid var(--border);
            padding: 0;
            padding-bottom: env(safe-area-inset-bottom);
            z-index: 100;
            gap: 0;
        }
        .brand { display: none; }
        .nav { flex-direction: row; flex: 1; gap: 0; }
        .nav-item {
            flex-direction: column;
            align-items: center;
            justify-content: center;
            gap: 3px;
            padding: 8px 4px;
            border-radius: 0;
            font-size: 11px;
            flex: 1;
            min-height: 52px;
        }
        .footer {
            flex-direction: row;
            align-items: stretch;
            margin-top: 0;
            margin-left: 0;
            border-top: 0;
            padding-top: 0;
            gap: 0;
        }
        .health { display: none; }
        .theme-toggle {
            flex-direction: column;
            gap: 3px;
            padding: 8px var(--space-2);
            border: none;
            border-radius: 0;
            font-size: 11px;
            min-width: 52px;
            min-height: 52px;
            justify-content: center;
        }
    }
</style>
