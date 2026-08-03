<script lang="ts">
    import Icon from "../Icon.svelte";
    import { api } from "../../lib/api";
    import { data, toasts } from "../../lib/stores.svelte";
    import { haptic } from "../../lib/utils";

    // The panel's status strip, across the top: the clock the room reads
    // from a distance, the two stats that qualify the home, and the master
    // gesture. It runs horizontally because it is a thing to *read* — the
    // height it used to take as a left-hand column now belongs to the music
    // band, which is the thing a wall is used to drive (DESIGN.md §16). All
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
            await (on ? api.allOn() : api.allOff());
            await data.refresh();
        } catch (e) {
            toasts.error("Failed", (e as Error).message);
        } finally {
            busy = false;
        }
    }
</script>

<section class="hero" aria-label="Clock and home status">
    <div class="when">
        <div class="clock mono">{timeLabel}</div>
        <div class="date">{dateLabel}</div>
    </div>

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
        align-items: center;
        gap: var(--space-5);
        min-width: 0;
    }
    .when {
        display: flex;
        align-items: baseline;
        gap: var(--space-3);
        min-width: 0;
    }

    .clock {
        font-size: 54px;
        font-weight: 500;
        letter-spacing: -0.03em;
        line-height: 1;
        flex-shrink: 0;
    }
    .date {
        font-size: 15px;
        color: var(--text-mute);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    /* The stats sit between the clock and the master button and give up
       the leftover width, so the strip stays one line. */
    .stats {
        display: flex;
        gap: var(--space-3);
        margin-left: auto;
    }
    .stat {
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 4px;
        padding: var(--space-2) var(--space-4);
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

    /* Master gesture — the trailing end of the strip, the one control every
       panel user reaches for. ON wears the sanctioned gradient. */
    .master {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        flex-shrink: 0;
        min-height: 64px;
        padding: var(--space-2) var(--space-4);
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
        width: 44px;
        height: 44px;
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
        font-size: 17px;
        font-weight: 600;
        letter-spacing: -0.02em;
    }
    .m-sub {
        font-size: 12.5px;
        color: var(--text-mute);
    }

    /* Portrait / narrow: back to a stacked block — a one-line strip needs
       landscape width to be one line. */
    @media (orientation: portrait), (max-width: 900px) {
        .hero {
            flex-direction: column;
            align-items: stretch;
            gap: var(--space-4);
        }
        .clock {
            font-size: 72px;
        }
        .stats {
            margin-left: 0;
        }
        .stat {
            flex: 1;
        }
    }
</style>
