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
    //
    // Everything on it is stated at the size it deserves and no larger. The
    // strip is 80px and the clock inside it is 32px, because in the stacked
    // bands the clock is a fact you glance at, not the panel's subject — the
    // ambient face is where the clock is the subject, and it is five times
    // this size there. Nothing here wears a card: the stats are bare figures
    // with a label under them, the way a dashboard states a reading.
    let {
        timeLabel,
        dateLabel,
        lightsOn,
        lightsTotal,
        insideTemp,
        onExit,
    }: {
        timeLabel: string;
        dateLabel: string;
        lightsOn: number;
        lightsTotal: number;
        insideTemp: number | null;
        /** The one way out of the kiosk (§16) — it rides the strip's
         *  trailing edge, which is the only chrome the panel has. */
        onExit: () => void;
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

<section class="strip" aria-label="Clock and home status">
    <div class="when">
        <div class="clock mono">{timeLabel}</div>
        <div class="date">{dateLabel}</div>
    </div>

    <div class="stat">
        <div class="v mono">{lightsOn}<span>/{lightsTotal}</span></div>
        <div class="l">lights on</div>
    </div>
    {#if insideTemp != null}
        <div class="stat">
            <div class="v mono">{insideTemp}°</div>
            <div class="l">inside</div>
        </div>
    {/if}

    <!-- The master gesture as a pill, pinned right. Its dot is the state:
         amber and ringed while anything is lit, quiet when the house is
         dark — so the button says what it will do *and* what is true. -->
    <button class="master" disabled={busy || lightsTotal === 0} onclick={toggleAll}>
        <span class="dot" class:on={anyOn}></span>
        <span>{anyOn ? "All off" : "All on"}</span>
    </button>

    <button class="exit" onclick={onExit}>
        <Icon name="close" size={13} /><span>Exit</span>
    </button>
</section>

<style>
    .strip {
        height: 80px;
        flex-shrink: 0;
        display: flex;
        align-items: center;
        gap: var(--space-8);
        min-width: 0;
        padding: 0 var(--space-8);
        border-bottom: 1px solid var(--hairline);
    }
    .when {
        display: flex;
        flex-direction: column;
        min-width: 0;
    }

    .clock {
        font-size: 32px;
        font-weight: 600;
        letter-spacing: -0.02em;
        line-height: 1;
        flex-shrink: 0;
    }
    .date {
        font-size: 12.5px;
        color: var(--text-mute);
        margin-top: 4px;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    /* Bare readings — figure over label, no card. Two facts that qualify
       the home, sized so the clock stays the biggest thing on the strip. */
    .stat {
        display: flex;
        flex-direction: column;
        gap: 2px;
        min-width: 0;
    }
    .stat .v {
        font-size: 19px;
        font-weight: 600;
        line-height: 1.1;
    }
    .stat .v span {
        color: var(--text-dim);
        font-weight: 500;
    }
    .stat .l {
        font-size: 11px;
        color: var(--text-mute);
        white-space: nowrap;
    }

    .master {
        margin-left: auto;
        display: inline-flex;
        align-items: center;
        gap: 9px;
        flex-shrink: 0;
        min-height: 44px;
        padding: 0 var(--space-5);
        border-radius: var(--r-pill);
        border: 1px solid var(--hairline);
        background: var(--card-2);
        color: var(--text);
        font-family: inherit;
        font-size: 13.5px;
        font-weight: 600;
        white-space: nowrap;
        cursor: pointer;
        transition:
            background var(--t-fast),
            border-color var(--t-fast),
            transform var(--t-fast);
    }
    .master:active {
        transform: scale(0.96);
        transition-duration: 80ms;
    }
    .master:disabled {
        opacity: 0.55;
        cursor: default;
    }
    .dot {
        width: 6px;
        height: 6px;
        border-radius: 50%;
        flex-shrink: 0;
        background: var(--text-dim);
        transition:
            background var(--t-med),
            box-shadow var(--t-med);
    }
    .dot.on {
        background: var(--on);
        box-shadow: 0 0 0 4px var(--on-soft);
    }

    /* The one way out — quiet, trailing the strip, big enough for a wall
       poke (§16). */
    .exit {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        flex-shrink: 0;
        min-height: 44px;
        padding: 0 var(--space-4);
        border-radius: var(--r-pill);
        border: 1px solid var(--hairline);
        background: none;
        color: var(--text-dim);
        font-family: inherit;
        font-size: 12.5px;
        font-weight: 500;
        cursor: pointer;
        transition:
            color var(--t-fast),
            border-color var(--t-fast),
            transform var(--t-fast);
    }
    .exit:active {
        transform: scale(0.95);
        transition-duration: 80ms;
    }

    /* Portrait / narrow: back to a stacked block — a one-line strip needs
       landscape width to be one line. */
    @media (orientation: portrait), (max-width: 900px) {
        .strip {
            height: auto;
            flex-wrap: wrap;
            gap: var(--space-4);
            padding: var(--space-5) var(--space-5);
        }
        .when {
            width: 100%;
        }
        .clock {
            font-size: 56px;
        }
        .master {
            margin-left: auto;
        }
    }
</style>
