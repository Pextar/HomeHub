<script lang="ts">
    import Icon from "../Icon.svelte";
    import { api } from "../../lib/api";
    import { data, toasts } from "../../lib/stores.svelte";
    import { haptic } from "../../lib/utils";

    // The panel's left column: the clock the room reads from a distance,
    // the two stats that qualify the home, and the master gesture. All
    // display values arrive as props — Panel owns the single clock tick.
    let {
        timeLabel,
        dateLabel,
        lightsOn,
        lightsTotal,
        insideTemp,
    }: {
        timeLabel: string;
        dateLabel: string;
        lightsOn: number;
        lightsTotal: number;
        insideTemp: number | null;
    } = $props();

    const anyOn = $derived(lightsOn > 0);

    // The flagship gesture from the dashboard, unchanged: one tap, action
    // fires immediately, the toast reports the outcome.
    let busy = $state(false);
    async function toggleAll() {
        if (busy) return;
        busy = true;
        haptic();
        const on = !anyOn;
        try {
            const r = on ? await api.allOn() : await api.allOff();
            await data.refresh();
            toasts.show({
                title: on ? "All on" : "All off",
                message: `${r.updated} updated${r.failures.length ? `, ${r.failures.length} failed` : ""}.`,
                tone: "success",
            });
        } catch (e) {
            toasts.error("Failed", (e as Error).message);
        } finally {
            busy = false;
        }
    }
</script>

<section class="hero" aria-label="Clock and home status">
    <div class="clock mono">{timeLabel}</div>
    <div class="date">{dateLabel}</div>

    <div class="stats">
        <div class="stat">
            <span class="stat-ico" class:on={anyOn}><Icon name="light" size={20} /></span>
            <span class="stat-val mono">{lightsOn}<span class="dim"> / {lightsTotal}</span></span>
            <span class="stat-label">lights on</span>
        </div>
        {#if insideTemp != null}
            <div class="stat">
                <span class="stat-ico"><Icon name="temperature" size={20} /></span>
                <span class="stat-val mono">{insideTemp}°</span>
                <span class="stat-label">inside</span>
            </div>
        {/if}
    </div>

    <button
        class="master"
        class:on={anyOn}
        disabled={busy || lightsTotal === 0}
        onclick={toggleAll}
    >
        <span class="m-ico"><Icon name="power" size={22} /></span>
        <span class="m-text">
            <span class="m-title">{anyOn ? "All off" : "All on"}</span>
            <span class="m-sub mono">{lightsOn} of {lightsTotal} on</span>
        </span>
    </button>
</section>

<style>
    .hero {
        display: flex;
        flex-direction: column;
        min-height: 0;
    }

    .clock {
        font-size: 76px;
        font-weight: 500;
        letter-spacing: -0.03em;
        line-height: 1;
    }
    .date {
        margin-top: var(--space-2);
        font-size: 15px;
        color: var(--text-mute);
    }

    .stats {
        display: flex;
        gap: var(--space-3);
        margin-top: var(--space-6);
    }
    .stat {
        flex: 1;
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 6px;
        padding: var(--space-3) var(--space-4);
        background: var(--card);
        border: 1px solid var(--hairline);
        border-radius: var(--r-md);
    }
    .stat-ico {
        color: var(--text-dim);
        display: inline-flex;
    }
    .stat-ico.on {
        color: var(--on);
    }
    .stat-val {
        font-size: 22px;
        font-weight: 500;
    }
    .stat-val .dim {
        color: var(--text-dim);
        font-size: 15px;
    }
    .stat-label {
        font-family: var(--font-mono);
        font-size: 10.5px;
        font-weight: 500;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-dim);
    }

    /* Master gesture — pinned to the bottom of the column, the one control
       every panel user reaches for. ON wears the sanctioned gradient. */
    .master {
        margin-top: auto;
        display: flex;
        align-items: center;
        gap: var(--space-4);
        width: 100%;
        min-height: 68px;
        padding: var(--space-3) var(--space-5);
        border-radius: var(--r-lg);
        border: 1px solid var(--hairline);
        background: var(--card);
        color: var(--text);
        font-family: inherit;
        text-align: left;
        cursor: pointer;
        transition:
            background var(--t-med),
            border-color var(--t-med),
            transform var(--t-fast);
    }
    .master:active {
        transform: scale(0.98);
        transition-duration: 80ms;
    }
    .master:disabled {
        opacity: 0.6;
        cursor: default;
    }
    .master.on {
        background: var(--tile-on-gradient);
        border-color: var(--tile-on-border);
    }
    .m-ico {
        width: 48px;
        height: 48px;
        border-radius: var(--r-md);
        display: grid;
        place-items: center;
        background: var(--surface);
        color: var(--text-mute);
        flex-shrink: 0;
        transition:
            color var(--t-med),
            background var(--t-med),
            box-shadow var(--t-med);
    }
    .master.on .m-ico {
        background: var(--on-soft);
        color: var(--on);
        box-shadow: 0 0 20px 2px var(--on-glow);
    }
    .m-text {
        display: flex;
        flex-direction: column;
        gap: 2px;
        min-width: 0;
    }
    .m-title {
        font-size: 19px;
        font-weight: 600;
        letter-spacing: -0.02em;
    }
    .m-sub {
        font-size: 12.5px;
        color: var(--text-mute);
    }
</style>
