<script lang="ts">
    import Topbar from "../components/Topbar.svelte";
    import Icon from "../components/Icon.svelte";
    import EmptyState from "../components/EmptyState.svelte";
    import { data } from "../lib/stores.svelte";
    import { fly } from "svelte/transition";
    import { cubicOut } from "svelte/easing";
    import { dur, stagger } from "../lib/motion";
    import type { ActivityEntry } from "../lib/types";

    const v = $derived(data.value);

    type Category = "auto" | "scene" | "manual";
    type Filter = "all" | "auto" | "manual" | "scene" | "error";

    let filter = $state<Filter>("all");

    const FILTERS: { id: Filter; label: string }[] = [
        { id: "all", label: "All" },
        { id: "auto", label: "Automations" },
        { id: "manual", label: "Manual" },
        { id: "scene", label: "Scenes" },
        { id: "error", label: "Errors" },
    ];

    // The activity feed is sourced from real ActivityEntry records. We derive a
    // display category from the entry's source/kind so the timeline can colour
    // and icon each event consistently.
    function categoryOf(e: ActivityEntry): Category {
        if (e.source === "schedule" || e.source === "timer") return "auto";
        if (e.kind === "scene") return "scene";
        return "manual";
    }

    const META: Record<Category, { color: string; icon: "clock" | "scenes" | "socket" }> = {
        auto:   { color: "var(--on)",   icon: "clock" },
        scene:  { color: "#c4a4e0",     icon: "scenes" },
        manual: { color: "var(--cool)", icon: "socket" },
    };

    function matchesFilter(e: ActivityEntry): boolean {
        if (filter === "all") return true;
        if (filter === "error") return e.status === "error";
        return categoryOf(e) === filter;
    }

    const time = (iso: string) =>
        new Date(iso).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });

    function dayLabel(iso: string): string {
        const d = new Date(iso);
        const today = new Date();
        const yesterday = new Date();
        yesterday.setDate(today.getDate() - 1);
        if (d.toDateString() === today.toDateString()) return "Today";
        if (d.toDateString() === yesterday.toDateString()) return "Yesterday";
        return d.toLocaleDateString([], { weekday: "long", month: "short", day: "numeric" });
    }

    function sourceLabel(s: ActivityEntry["source"]): string {
        return s === "schedule" ? "Schedule" : s === "timer" ? "Timer" : "Manual";
    }

    // Group the filtered, newest-first entries into day buckets, preserving order.
    const groups = $derived.by(() => {
        const filtered = v.activity.filter(matchesFilter);
        const out: { label: string; events: ActivityEntry[] }[] = [];
        for (const e of filtered) {
            const label = dayLabel(e.time);
            const last = out[out.length - 1];
            if (last && last.label === label) last.events.push(e);
            else out.push({ label, events: [e] });
        }
        return out;
    });
</script>

<Topbar title="Activity" subtitle="Everything that happened in your home" />

{#if v.activity.length === 0}
    <EmptyState icon="activity" title="No activity yet"
        message="Device changes, schedules and scene runs will show up here as they happen." />
{:else}
    <div class="filters h-scroll">
        {#each FILTERS as f}
            <button class="chip" class:active={filter === f.id} onclick={() => filter = f.id}>
                {f.label}
            </button>
        {/each}
    </div>

    {#if groups.length === 0}
        <p class="field-help">No matching activity.</p>
    {:else}
        <div class="feed">
            {#each groups as g (g.label)}
                <div class="day">
                    <div class="day-label mono">{g.label}</div>
                    <div class="line">
                        {#each g.events as e, i (e.id)}
                            {@const cat = categoryOf(e)}
                            {@const isErr = e.status === "error"}
                            <div class="row"
                                in:fly={{ x: -6, duration: dur(180), delay: stagger(i, 30), easing: cubicOut }}>
                                <div class="bullet" class:err={isErr}
                                    style="color: {isErr ? 'var(--bad)' : META[cat].color}">
                                    <Icon name={isErr ? "close" : META[cat].icon} size={14} />
                                </div>
                                <div class="body">
                                    <div class="top">
                                        <span class="what">{e.label}</span>
                                        <time class="mono when">{time(e.time)}</time>
                                    </div>
                                    <div class="sub">
                                        <span class="who" style="color: {isErr ? 'var(--bad)' : META[cat].color}">{sourceLabel(e.source)}</span>
                                        · <span class="act">{e.action}</span>
                                        {#if e.error}· {e.error}{/if}
                                    </div>
                                </div>
                            </div>
                        {/each}
                    </div>
                </div>
            {/each}
        </div>
    {/if}
{/if}

<style>
    .filters { gap: 6px; padding-bottom: 2px; }

    .feed { display: flex; flex-direction: column; gap: 18px; }
    .day-label {
        color: var(--text-dim);
        font-size: 11.5px;
        letter-spacing: 0.1em;
        text-transform: uppercase;
        margin-bottom: 10px;
    }
    .line { position: relative; }
    /* vertical timeline rail behind the bullets */
    .line::before {
        content: "";
        position: absolute;
        left: 18px; top: 18px; bottom: 18px;
        width: 1px;
        background: var(--hairline);
    }
    .row {
        display: flex;
        gap: 14px;
        padding: 10px 0;
        align-items: flex-start;
        position: relative;
    }
    .bullet {
        width: 36px; height: 36px;
        border-radius: 50%;
        background: var(--card);
        border: 1px solid var(--hairline);
        display: grid; place-items: center;
        flex-shrink: 0;
        z-index: 1;
    }
    .bullet.err { border-color: var(--bad); background: var(--danger-soft); }
    .body { flex: 1; min-width: 0; padding-top: 2px; }
    .top {
        display: flex;
        justify-content: space-between;
        align-items: baseline;
        gap: 8px;
    }
    .what {
        font-size: 13.5px;
        font-weight: 500;
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
    .when { font-size: 11px; color: var(--text-dim); flex-shrink: 0; }
    .sub { color: var(--text-mute); font-size: 12px; margin-top: 2px; }
    .who { font-weight: 500; }
    .act { text-transform: capitalize; }
</style>
