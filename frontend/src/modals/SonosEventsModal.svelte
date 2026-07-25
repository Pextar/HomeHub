<script lang="ts">
    // "Live updates" — the surface that explains how speaker state reaches
    // the app, and what to do when it can't.
    //
    // The GENA subscriptions behind it (backend: internal/sonos/monitor.go)
    // are invisible when they work and baffling when they don't: the app just
    // feels slow, with nothing on screen admitting why. So this sheet answers
    // three questions in order — is push working, which speaker isn't, and
    // what would fix it — and carries the one action that can fix it.
    import { onMount } from "svelte";
    import Modal from "../components/Modal.svelte";
    import Icon from "../components/Icon.svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { toasts } from "../lib/stores.svelte";
    import { copyText } from "../lib/clipboard";
    import { formatAgo } from "../lib/utils";
    import type { SonosEventHealth } from "../lib/types";

    // Re-read while the sheet is open: after a retry the interesting moment
    // is the flip to live, which lands a second or two later.
    const POLL_MS = 4000;

    let health = $state<SonosEventHealth | null>(null);
    let loaded = $state(false);
    let retrying = $state(false);
    // Ticks once a second purely so the "4m ago" labels stay honest while
    // somebody watches the sheet.
    let now = $state(Date.now());

    async function refresh() {
        try {
            health = await api.sonosEventHealth();
        } catch (e) {
            // Non-fatal: the sheet is diagnostic, so a failed read shows the
            // last good answer rather than replacing it with an error.
            if (!loaded) toasts.error("Couldn't read status", (e as Error).message);
        } finally {
            loaded = true;
        }
    }

    onMount(() => {
        void refresh();
        const poll = setInterval(refresh, POLL_MS);
        const clock = setInterval(() => (now = Date.now()), 1000);
        return () => {
            clearInterval(poll);
            clearInterval(clock);
        };
    });

    async function retry() {
        retrying = true;
        try {
            await api.sonosEventRetry();
            // Subscribing is a network round-trip per service per speaker, so
            // the answer isn't ready the instant the POST returns. Give the
            // watchers a moment, then read.
            await new Promise((r) => setTimeout(r, 1200));
            await refresh();
            if (health?.live) toasts.success("Live updates on", subtitle);
        } catch (e) {
            toasts.error("Couldn't retry", (e as Error).message);
        } finally {
            retrying = false;
        }
    }

    async function copyCallback() {
        if (!health?.callback) return;
        const ok = await copyText(health.callback);
        if (ok) toasts.success("Copied", "Callback address");
        else toasts.warn("Couldn't copy", "Long-press the address to select it.");
    }

    // ── The headline ─────────────────────────────────────────────────────
    // Four states, each with different advice, so they get different copy
    // rather than one "live/not live" boolean the user has to interpret.
    type Tone = "live" | "partial" | "off";

    const tone = $derived<Tone>(
        !health || !health.live
            ? "off"
            : health.subscribed < health.total
              ? "partial"
              : "live",
    );

    const headline = $derived(
        !health || health.total === 0
            ? "No speakers yet"
            : tone === "live"
              ? "Live updates on"
              : tone === "partial"
                ? "Partly live"
                : health.running
                  ? "Polling instead"
                  : "Push isn't running",
    );

    const subtitle = $derived.by(() => {
        if (!health || health.total === 0) return "Add a speaker to get started.";
        if (tone === "live")
            return "Your speakers tell HomeHub the moment anything changes.";
        if (tone === "partial")
            return `${health.subscribed} of ${health.total} speakers are pushing — the rest are being polled.`;
        if (!health.running)
            return "The hub isn't watching for speaker events. Restarting HomeHub should bring it back.";
        return "No speaker could reach HomeHub back, so the app asks them every few seconds instead.";
    });

    // What the user actually gets, in plain terms. This is the part that
    // makes the difference legible: "under a second" vs "up to 5 seconds".
    const effect = $derived(
        tone === "off"
            ? "Changes made on a speaker or in the Sonos app show up here within about 5 seconds."
            : "Changes made on a speaker or in the Sonos app show up here in well under a second.",
    );

    function speakerLine(sp: SonosEventHealth["speakers"][number]): string {
        if (sp.subscribed) {
            const ago = sp.last_event ? formatAgo(sp.last_event) : "";
            if (sp.events === 0) return "Subscribed — waiting for the first update";
            return `Last update ${ago}`;
        }
        if (!sp.reachable) return "Not answering — check the speaker is on";
        return sp.error || "Not subscribed";
    }

    // Referenced so the per-second tick invalidates the "ago" labels.
    const speakers = $derived.by(() => {
        void now;
        return health?.speakers ?? [];
    });
</script>

<Modal
    title="Live updates"
    subtitle="How speaker changes reach this app"
    size="wide"
    guardUnsaved={false}
>
    {#snippet body()}
        {#if !loaded}
            <div class="skeleton sk-hero"></div>
            <div class="skeleton sk-row"></div>
            <div class="skeleton sk-row"></div>
        {:else}
            <!-- The answer, before any detail. -->
            <div class="hero" class:live={tone === "live"} class:partial={tone === "partial"}>
                <span class="hero-ico">
                    <Icon name={tone === "off" ? "radio" : "bolt"} size={20} />
                </span>
                <span class="hero-meta">
                    <span class="hero-title">{headline}</span>
                    <span class="hero-sub">{subtitle}</span>
                </span>
                {#if health && health.total > 0}
                    <span class="hero-count mono" aria-hidden="true">
                        {health.subscribed}/{health.total}
                    </span>
                {/if}
            </div>

            {#if health && health.total > 0}
                <p class="effect">{effect}</p>

                <!-- Per speaker: which ones push, and what the ones that
                     don't are complaining about. -->
                <div class="block">
                    <span class="field-label">Speakers</span>
                    {#each speakers as sp (sp.id)}
                        <div class="row">
                            <span class="dot" class:on={sp.subscribed} aria-hidden="true"></span>
                            <span class="r-meta">
                                <span class="r-name">{sp.name || sp.id}</span>
                                <span class="r-sub" class:bad={!sp.subscribed && !!sp.error}>
                                    {speakerLine(sp)}
                                </span>
                            </span>
                            <span class="r-right mono">
                                {#if sp.subscribed}
                                    {sp.events}
                                {:else}
                                    —
                                {/if}
                            </span>
                        </div>
                    {/each}
                    <!-- Only worth explaining once there is a number to
                         explain; every row reads "—" until then. -->
                    {#if health.subscribed > 0}
                        <div class="field-help">
                            The number is how many updates that speaker has
                            pushed since HomeHub started.
                        </div>
                    {/if}
                </div>

                <!-- Only shown when it's actionable. When everything is live
                     the address is trivia; when it isn't, it's the fix. -->
                {#if tone !== "live"}
                    <div class="block">
                        <div class="block-head">
                            <span class="field-label">Speakers post back to</span>
                            {#if health.callback}
                                <button class="act-btn" onclick={copyCallback}>Copy</button>
                            {/if}
                        </div>
                        {#if health.callback}
                            <code class="addr mono">{health.callback}</code>
                        {:else}
                            <div class="r-sub bad">
                                {health.callback_error ||
                                    "HomeHub couldn't work out an address your speakers can reach."}
                            </div>
                        {/if}
                        <div class="field-help">
                            Each speaker has to be able to open a plain
                            <code>http</code> connection to this address. It
                            fails when the speakers are on a different subnet
                            or VLAN, when a firewall blocks the hub's port, or
                            when HomeHub is in a container without host
                            networking.
                        </div>
                    </div>

                    <p class="reassure">
                        Nothing is broken in the meantime — HomeHub falls back
                        to asking every speaker every few seconds, exactly as
                        it did before push existed. It's just slower and
                        chattier on your network.
                    </p>
                {/if}
            {/if}
        {/if}
    {/snippet}

    {#snippet actions()}
        {#if health && health.total > 0 && tone !== "live"}
            <button class="btn btn-ghost" onclick={retry} disabled={retrying}>
                {retrying ? "Checking…" : "Try again"}
            </button>
        {/if}
        <button class="btn btn-primary" onclick={() => closeModal()}>Done</button>
    {/snippet}
</Modal>

<style>
    /* ── Headline ─────────────────────────────────────────────────────── */
    .hero {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: var(--space-4);
        border: 1px solid var(--border);
        border-radius: var(--radius-lg);
        background: var(--surface);
    }
    /* Live is the "on" state, so it takes the sanctioned warm treatment
       rather than a colour of its own. */
    .hero.live {
        background: var(--tile-on-gradient);
        border-color: var(--tile-on-border);
    }
    .hero.partial {
        border-color: color-mix(in srgb, var(--warn) 32%, var(--border));
    }

    .hero-ico {
        flex-shrink: 0;
        width: 38px;
        height: 38px;
        display: grid;
        place-items: center;
        border-radius: var(--radius-md);
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        color: var(--text-muted);
    }
    .hero.live .hero-ico {
        color: var(--primary);
        border-color: var(--tile-on-border);
        box-shadow: 0 0 18px 2px var(--primary-glow);
    }
    .hero.partial .hero-ico { color: var(--warn); }

    .hero-meta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
    .hero-title { font-size: 15px; font-weight: 600; letter-spacing: -0.01em; }
    .hero-sub { font-size: 12.5px; color: var(--text-muted); line-height: 1.45; }
    .hero-count {
        flex-shrink: 0;
        font-size: 13px;
        font-weight: 500;
        color: var(--text-muted);
        font-feature-settings: "tnum" 1;
    }
    .hero.live .hero-count { color: var(--primary); }

    .effect {
        margin: var(--space-4) 0 0;
        font-size: 13px;
        line-height: 1.55;
        color: var(--text-muted);
    }

    /* ── Blocks ───────────────────────────────────────────────────────── */
    .block {
        margin-top: var(--space-4);
        display: flex;
        flex-direction: column;
        gap: 6px;
        padding: var(--space-3);
        background: var(--surface);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
    }
    .block-head { display: flex; align-items: center; justify-content: space-between; }

    .row {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: 8px 0;
        min-height: 44px;
    }
    .row + .row { border-top: 1px solid var(--border); }

    /* §6.6 status dot — amber with a soft halo when subscribed. */
    .dot {
        flex-shrink: 0;
        width: 6px;
        height: 6px;
        border-radius: 50%;
        background: var(--text-faint);
    }
    .dot.on {
        background: var(--primary);
        box-shadow: 0 0 0 4px var(--primary-soft);
    }

    .r-meta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
    .r-name {
        font-weight: 500;
        font-size: 13.5px;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
    .r-sub { font-size: 12px; color: var(--text-muted); line-height: 1.4; }
    .r-sub.bad { color: var(--danger); }
    .r-right {
        flex-shrink: 0;
        font-size: 12.5px;
        color: var(--text-muted);
        font-feature-settings: "tnum" 1;
    }

    .addr {
        font-family: var(--font-mono);
        font-size: 12px;
        word-break: break-all;
        color: var(--text);
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        border-radius: var(--radius-sm);
        padding: 8px 10px;
        user-select: all;
        -webkit-user-select: all;
    }

    .field-help code {
        font-family: var(--font-mono);
        font-size: 0.92em;
        background: var(--bg-elevated);
        padding: 1px 5px;
        border-radius: var(--radius-sm);
    }

    .reassure {
        margin: var(--space-4) 0 0;
        font-size: 12.5px;
        line-height: 1.55;
        color: var(--text-faint);
    }

    .act-btn {
        font: inherit;
        font-size: 12px;
        font-weight: 600;
        padding: 5px 10px;
        border-radius: var(--radius-sm);
        border: 1px solid var(--border-strong);
        background: var(--bg-elevated);
        color: var(--text);
        cursor: pointer;
        transition: background var(--t-fast);
    }
    .act-btn:hover { background: var(--surface-hover); }

    /* Skeletons, not spinners (DESIGN.md §10). */
    .sk-hero { height: 76px; border-radius: var(--radius-lg); }
    .sk-row { height: 52px; border-radius: var(--radius-md); margin-top: var(--space-3); }

    @media (pointer: coarse) {
        .act-btn { padding: 10px 14px; font-size: 13px; }
    }
</style>
