<script lang="ts">
    import Icon from "../Icon.svelte";
    import EmptyState from "../EmptyState.svelte";
    import AddDeviceModal from "../../modals/AddDeviceModal.svelte";
    import { data, toasts, session } from "../../lib/stores.svelte";
    import { homeLayout } from "../../lib/home-layout.svelte";
    import { homeTemperature } from "../../lib/home-layout";
    import { api } from "../../lib/api";
    import { describeTarget, haptic } from "../../lib/utils";
    import { openModal } from "../../lib/modal.svelte";
    import { fly } from "svelte/transition";
    import { cubicOut } from "svelte/easing";
    import { Tween } from "svelte/motion";
    import { dur } from "../../lib/motion";

    /**
     * The whole-home hero and the stat strip beside it — the master switch,
     * what the house is drawing, what it is reading, and what happens next.
     *
     * It carries the screen's two other jobs for the same reason it always
     * has: the first-load skeleton and the "no devices at all" empty state
     * both belong to this block of the page, and neither reads as a section
     * you would arrange around.
     */

    /** The clock, ticking in the parent — this block only needs the minute. */
    interface Props {
        nowMinute: number;
    }
    let { nowMinute }: Props = $props();

    const v = $derived(data.value);

    const totalSockets = $derived(v.sockets.length);
    const onSockets = $derived(v.sockets.filter((s) => s.state).length);
    const heroOn = $derived(onSockets > 0);

    // Animated on-count: tween to avoid jarring jumps on toggle.
    const tweenedOn = new Tween(0);
    let _onInit = true;
    $effect(() => {
        const d = _onInit ? 0 : dur(500);
        _onInit = false;
        tweenedOn.set(onSockets, { duration: d, easing: cubicOut });
    });

    const powerSensors = $derived(
        v.sensors.filter((s) => s.kind === "power" && s.last_value != null),
    );
    const hasPower = $derived(powerSensors.length > 0);
    const powerWatts = $derived(
        Math.round(powerSensors.reduce((sum, s) => sum + (s.last_value ?? 0), 0)),
    );

    // Which sensor "inside" means is the user's call (Sensors › Home
    // temperature); with no choice made this is still the house average, but
    // it now says so rather than labelling it with one room's name.
    const temp = $derived(homeTemperature(v.sensors, homeLayout.layout));

    // Desktop stat strip (wide screens only): real metrics that complement the
    // hero — how many automations are active and the next scheduled event.
    const enabledAutomations = $derived(v.automations.filter((a) => a.enabled).length);
    const nextEvent = $derived.by(() => {
        const parse = (s?: string) => {
            if (!s || !/^\d\d:\d\d/.test(s)) return -1;
            const [h, m] = s.split(":").map(Number);
            return h * 60 + m;
        };
        const items = v.schedules
            .filter((s) => s.enabled)
            .map((s) => ({
                min: parse(s.effective_time || s.time),
                time: s.effective_time || s.time,
                label: describeTarget(s.target_type, s.target_id, s.socket_id).label,
            }))
            .filter((x) => x.min >= 0);
        if (items.length === 0) return null;
        const upcoming = items.filter((x) => x.min >= nowMinute).sort((a, b) => a.min - b.min);
        return upcoming[0] ?? items.sort((a, b) => a.min - b.min)[0];
    });

    // The desktop top row is hero + however many stat tiles have real data.
    // Deriving the track list from that count keeps the row filled at every
    // width instead of leaving a hole where a missing tile would have sat.
    const statCount = $derived(
        (temp ? 1 : 0) + (nextEvent ? 1 : 0) + (enabledAutomations > 0 ? 1 : 0),
    );
    const topCols = $derived(statCount > 0 ? `1.6fr ${"1fr ".repeat(statCount).trim()}` : "1fr");

    // ── Bulk actions ────────────────────────────────────────────────────────
    // No confirmation — the master switch is the app's flagship gesture, and a
    // dialog in front of it kills the moment. The action fires immediately and
    // offers Undo instead: every device returns to exactly its prior state.
    async function bulk(on: boolean) {
        haptic();
        const before = new Map(v.sockets.map((s) => [s.id, s.state] as const));
        try {
            const r = on ? await api.allOn() : await api.allOff();
            await data.refresh();
            toasts.show({
                title: on ? "All on" : "All off",
                message: `${r.updated} updated${r.failures.length ? `, ${r.failures.length} failed` : ""}.`,
                tone: "success",
                timeoutMs: 8000,
                action: { label: "Undo", onClick: () => undoBulk(before) },
            });
        } catch (e) {
            toasts.error("Failed", (e as Error).message);
        }
    }
    async function undoBulk(before: ReadonlyMap<string, boolean>) {
        haptic();
        const restore = data.value.sockets.filter(
            (s) => before.has(s.id) && before.get(s.id) !== s.state,
        );
        let failed = 0;
        await Promise.all(
            restore.map(async (s) => {
                try {
                    await (before.get(s.id) ? api.socketOn(s.id) : api.socketOff(s.id));
                } catch {
                    failed++;
                }
            }),
        );
        await data.refresh();
        if (failed) toasts.error("Undo", `${failed} device${failed === 1 ? "" : "s"} didn't respond.`);
    }
</script>

{#if !v.loaded}
    <!-- First load: shimmer placeholders instead of blank sections (§10). -->
    <div class="top-grid" aria-hidden="true">
        <div class="hero tile skel-hero">
            <div class="skeleton skel-line lg"></div>
            <div class="skeleton skel-line sm"></div>
        </div>
    </div>
{:else if totalSockets === 0}
    <!-- First run: no devices at all — point at the add-device flow. -->
    <EmptyState
        fill
        icon="socket"
        title="No devices yet"
        message="Add your first RF socket or smart light to start controlling your home."
    >
        {#if session.isAdmin}
            <button class="btn btn-primary" onclick={() => openModal(AddDeviceModal, {})}>
                Add device
            </button>
        {/if}
    </EmptyState>
{:else}
    <div class="top-grid" style="--top-cols: {topCols}">
        <div class="hero tile" class:on={heroOn} in:fly={{ y: 14, duration: dur(280), easing: cubicOut }}>
            <div class="hero-top">
                <div class="hero-lead">
                    <div class="hero-eyebrow mono">Whole home</div>
                    <div class="hero-count">
                        <span class="num-display">{Math.round(tweenedOn.current)}</span>
                        <span class="hero-of">of {totalSockets} on</span>
                    </div>
                </div>
                <button
                    class="sw-big"
                    class:on={heroOn}
                    onclick={() => bulk(!heroOn)}
                    aria-label={heroOn ? "Turn all devices off" : "Turn all devices on"}
                    aria-pressed={heroOn}
                ></button>
            </div>
            {#if hasPower || temp}
                <div class="hero-meta">
                    {#if hasPower}
                        <span class="hero-stat">
                            <Icon name="bolt" size={13} />
                            <span class="mono hero-em">{powerWatts} W</span> now
                        </span>
                    {/if}
                    {#if hasPower && temp}<span class="hero-sep">·</span>{/if}
                    {#if temp}
                        <span class="hero-stat">
                            <span class="mono hero-em">{Math.round(temp.value)}°</span>
                            <!-- A named sensor says where; an average says the
                                 old word, since "average of 3" is a caption for
                                 the tile below and not a place in the house. -->
                            <span class="hero-where">{temp.named ? temp.label : "inside"}</span>
                        </span>
                    {/if}
                </div>
            {/if}
        </div>

        {#if temp}
            <div class="stat tile">
                <div class="stat-eyebrow mono">Temperature</div>
                <div class="stat-value cool">{Math.round(temp.value)}°</div>
                <div class="stat-sub" title={temp.label}>{temp.label}</div>
            </div>
        {/if}
        {#if nextEvent}
            <div class="stat tile">
                <div class="stat-eyebrow mono">Next event</div>
                <div class="stat-value">{nextEvent.time}</div>
                <div class="stat-sub">{nextEvent.label}</div>
            </div>
        {/if}
        {#if enabledAutomations > 0}
            <div class="stat tile">
                <div class="stat-eyebrow mono">Automations</div>
                <div class="stat-value">{enabledAutomations}</div>
                <div class="stat-sub">active</div>
            </div>
        {/if}
    </div>
{/if}

<style>
    /* ── First-load skeleton ────────────────────────── */
    .skel-hero {
        min-height: 120px;
        justify-content: center;
        gap: 10px;
    }
    .skel-line { height: 14px; border-radius: var(--r-sm); width: 60%; }
    .skel-line.lg { height: 26px; width: 40%; }
    .skel-line.sm { width: 25%; }

    /* Hero spans full width on phones with the stat tiles as a compact strip
       beneath it; on desktop they share one row, matching the mockup. */
    .top-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: var(--space-3); }
    .hero { grid-column: 1 / -1; }
    .stat {
        padding: 12px 14px;
        flex-direction: column;
        justify-content: space-between;
        gap: 4px;
        min-width: 0;
    }
    @media (min-width: 1024px) {
        /* One track per tile that actually rendered (--top-cols), so the row
           always spans the full width whatever the home has to report. */
        .top-grid { grid-template-columns: var(--top-cols); align-items: stretch; gap: var(--space-4); }
        .hero { grid-column: auto; }
        .stat { padding: 18px; gap: 8px; }
    }
    .stat-eyebrow {
        color: var(--text-mute);
        font-size: 10px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .stat-value {
        font-size: 20px;
        font-weight: 600;
        letter-spacing: -0.02em;
        font-family: var(--font-mono);
        font-feature-settings: "tnum" 1;
    }
    @media (min-width: 1024px) {
        .stat-eyebrow { font-size: 11px; }
        .stat-value { font-size: 32px; }
    }
    .stat-value.cool { color: var(--cool); }
    .stat-sub { color: var(--text-mute); font-size: 11px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    @media (min-width: 1024px) {
        .stat-sub { font-size: 12px; }
    }

    /* ── Whole-home hero ────────────────────────────── */
    .hero { padding: 20px; gap: 16px; transition: box-shadow var(--t-med); }
    /* When the home is lit the hero carries the room's glow with it — the
       "lit from within" beat, not just a warmer card. */
    .hero.tile.on { box-shadow: 0 12px 48px -14px var(--on-glow); }
    .hero-top {
        display: flex;
        align-items: flex-start;
        justify-content: space-between;
        gap: 12px;
    }
    .hero-lead { min-width: 0; }
    .hero-eyebrow {
        color: var(--on);
        font-size: 11px;
        letter-spacing: 0.1em;
        text-transform: uppercase;
    }
    .hero-count {
        margin-top: 8px;
        display: flex;
        align-items: baseline;
        gap: 10px;
        white-space: nowrap;
    }
    .hero-count .num-display { font-size: 56px; }
    .hero-of { color: var(--text-mute); font-size: 14px; }
    .hero .sw-big {
        flex-shrink: 0;
        border: 0; padding: 0;
        appearance: none; -webkit-appearance: none;
        cursor: pointer;
    }
    .hero .sw-big:focus-visible { box-shadow: var(--focus-ring); }
    .hero-meta {
        display: flex;
        align-items: center;
        gap: 8px;
        color: var(--text-mute);
        font-size: 12px;
        white-space: nowrap;
    }
    .hero-stat { display: inline-flex; align-items: center; gap: 6px; min-width: 0; }
    .hero-where { overflow: hidden; text-overflow: ellipsis; }
    .hero-stat :global(svg) { color: var(--on); }
    .hero-em { color: var(--text); }
    .hero-sep { color: var(--text-dim); }
</style>
