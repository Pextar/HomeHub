<script lang="ts">
    import Icon from "../Icon.svelte";
    import Waveform from "../music/Waveform.svelte";
    import Slider from "../music/Slider.svelte";
    import TrackRail from "../music/TrackRail.svelte";
    import { kefSourceLabel } from "../../lib/kef";
    import { clock } from "../../lib/music/clock.svelte";
    import type { PanelMusicStore } from "../../lib/panel-music.svelte";

    // The featured source's player, shared by both of the panel's depths
    // (DESIGN.md §16). It is a player and only a player: cover, what is
    // playing, scrubber, transport, volume. The room's own settings — the
    // play modes, the sleep timer, per-speaker faders, the KEF input — live
    // on the depth's Rooms pane (PanelRoomSettings) on both shapes: neither
    // the dashboard's one-line band nor a 360px column has the height for
    // them, and the cover pays for all of it either way.
    //
    // Two shapes, and the zone picks (`wide`): the dashboard's band lays
    // everything out on one line — cover, meta and scrubber, transport,
    // destination and fader — and the depth's tall column stacks the same
    // parts. The band is a strip you read in passing and poke at; the
    // column is where the cover gets to be a record (§16).
    //
    // Every capability renders only where the room says it has one (§15):
    // the rail seeks on a Sonos track and is a read-only rail elsewhere,
    // skips appear where a skip would reach something (`canSkip`), play
    // modes are a Sonos coordinator's, and standby is a Wake button rather
    // than a dead label — waking a speaker is the wall's job, not the full
    // view's.
    let {
        music,
        onOpen = undefined,
        onExpand = undefined,
        wide = false,
    }: {
        music: PanelMusicStore;
        /** Given, the cover is a button into the music depth. */
        onOpen?: () => void;
        /** Given, the cover carries a control that takes the player full
         *  screen. Only the depth's column offers it: the dashboard band is
         *  already the biggest thing on its surface, and its cover is the
         *  way *into* the depth. */
        onExpand?: () => void;
        /** Landscape: the cover beside the controls rather than above them.
         *  The dashboard's music zone is a wide, short band, where a stacked
         *  cover is capped by what the controls leave of the height and a
         *  beside-cover is capped by the band's full height instead. Which
         *  way round is bigger is purely the zone's aspect, so the zone
         *  says. */
        wide?: boolean;
    } = $props();

    const featured = $derived(music.featured);

    // ── The cover's size, stated in pixels ──────────────────────────────
    // The cover is a square and height is the scarce axis on a wall, so the
    // square has to be sized from the height it is allowed. It used to say
    // that as a chain — an `aspect-ratio` hanging off a flex item stretched
    // by its row, a `height: 100%` resolved against that, a ratio again on
    // the box inside — and a chain is only as good as its weakest link: an
    // engine that resolves any one of those to `auto` collapses the cover to
    // nothing. On the dashboard that took the artwork *and* the band's only
    // way into the music depth with it, since the cover is the tap-through.
    // So the card measures the room the cover may have and states the square
    // outright: definite on both axes, nothing to resolve.
    const ART_FLOOR = 96;
    const ART_CAP = 420;
    /** The gap between the cover and what rides under it — the meta when
     *  the card is stacked. */
    const HEAD_GAP = 16;
    /** The dashboard band's cover, stated outright. The band is the tallest
     *  zone on the panel and the cover used to take all of it, which made
     *  the surface a poster with a control strip beside it. In the stacked
     *  bands the band is a *strip*: cover, what's playing, transport,
     *  destination, on one line with air above and below it, and the record
     *  gets its full size one depth in where listening is the job (§16). */
    const BAND_ART = 128;

    // Landscape is the designed-for shape and the one that measures; the
    // portrait fallback lets CSS size the cover from its width, which is the
    // direction every engine agrees on. Mirrors the media query below.
    let landscape = $state(true);
    $effect(() => {
        const mq = window.matchMedia("(orientation: portrait), (max-width: 900px)");
        const apply = () => (landscape = !mq.matches);
        apply();
        mq.addEventListener("change", apply);
        return () => mq.removeEventListener("change", apply);
    });

    // Every measurement below is of a box the cover cannot itself size, or
    // the reading would chase its own tail: the scroll region's leftover
    // after the pinned strip, and the meta, whose two lines never wrap.
    let scrollH = $state(0);
    let scrollW = $state(0);
    let metaH = $state(0);

    const coverPx = $derived.by(() => {
        if (!landscape) return 0; // portrait: CSS sizes it from the width
        // The band states its cover; only the depth's column has to measure
        // one, because there the cover is what the column is *for* and what
        // it may have is whatever the controls under it leave.
        if (wide) return BAND_ART;
        const height = scrollH - metaH - HEAD_GAP;
        if (height <= 0 || scrollW <= 0) return 0; // pre-measure, one frame
        return Math.max(ART_FLOOR, Math.min(height, scrollW, ART_CAP));
    });
    const coverStyle = $derived(coverPx ? `width:${coverPx}px;height:${coverPx}px` : "");

    // Art that 404s (the proxy can't reach the speaker, the service expired
    // the URL) left an empty box behind — indistinguishable from a cover
    // that hasn't loaded yet. Fall back to §6.7's placeholder instead, and
    // key the failure to the URL so the next track gets its own try.
    let artFailed = $state<string | null>(null);
    const artSrc = $derived(featured?.art && featured.art !== artFailed ? featured.art : null);
    // A rail gets nothing to say when there is no track loaded to describe.
    const railIdle = $derived(!featured?.trackTitle && !featured?.playing);
    const railLabel = $derived(
        featured?.kind === "kef"
            ? `${kefSourceLabel(featured.input) || "Input"} — no track position`
            : featured?.kind === "zone"
              ? "played together — no track position"
              : undefined, // TrackRail's default: "live stream — no track position"
    );

    // ── The queued confirmation ─────────────────────────────────────────
    // A track dropped into the queue changes nothing on screen — the wall
    // has no dock and no toast to lean on — so the card says so itself for
    // a few seconds, wherever the queueing happened (a search row, an
    // artist page, a record).
    const QUEUED_MS = 5000;
    const queued = $derived.by(() => {
        void clock.beat;
        const q = music.lastQueued;
        if (!q || Date.now() - q.at > QUEUED_MS) return null;
        return q;
    });

    /** How many boxes the transport is driving. The band names its
     *  destination beside the fader, because the fader is the one control
     *  on the strip whose reach isn't obvious from what it looks like. */
    const speakerCount = $derived(featured?.members?.length ?? 1);
</script>

<!-- The art, identical either side of the tap-through button, so the two
     branches can't drift. `.p-artbox` is the square itself — sized in
     pixels from what the card measured — and the waveform and the expand
     control hang off it, so they sit on the cover rather than floating in
     the margin beside it. -->
{#snippet art()}
    {#if featured}
        <span class="p-artbox" style={coverStyle}>
            {#if artSrc}
                <img class="p-art" src={artSrc} alt="" onerror={() => (artFailed = artSrc)} />
            {:else}
                <span class="p-art placeholder">[ art ]</span>
            {/if}
            {#if featured.playing}
                <span class="p-wave"><Waveform /></span>
            {/if}
            {#if onExpand}
                <!-- Opposite the waveform, on the biggest thing on the
                     card. Listening is a different job from browsing,
                     and it wants the whole screen (§16). -->
                <button class="p-expand" aria-label="Full screen player" onclick={onExpand}>
                    <Icon name="expand" size={20} />
                </button>
            {/if}
        </span>
    {/if}
{/snippet}

{#snippet meta(chevron: boolean)}
    {#if featured}
        <span class="p-track" bind:clientHeight={metaH}>
            <span class="p-title">
                {featured.trackTitle ?? (featured.playing ? "Playing" : "Not playing")}
            </span>
            <span class="p-subrow">
                <span class="p-sub">{featured.trackSub || featured.title}</span>
                {#if chevron}
                    <span class="p-go" aria-hidden="true"
                        ><Icon name="chevronRight" size={16} /></span
                    >
                {/if}
            </span>
        </span>
    {/if}
{/snippet}

{#snippet rail()}
    <TrackRail
        position={music.posSec}
        duration={music.durSec}
        seekable={music.seekable}
        idle={railIdle}
        liveLabel={railLabel}
        onSeek={(sec) => music.seek(sec)}
    />
{/snippet}

<!-- The one thing queueing changes that can be seen — so it belongs where
     the eye already is, not at the far end of a scroll it may never
     reach. -->
{#snippet queuedLine()}
    {#if queued}
        <p class="p-queued">
            <Icon name="check" size={14} />
            <span>{queued.next ? "Playing next" : "Added to the queue"} — {queued.title}</span>
        </p>
    {/if}
{/snippet}

<!-- A speaker asleep is a speaker one tap from awake: the wall wakes it
     rather than sending anyone to the full view. -->
{#snippet standby()}
    {#if featured}
        <div class="p-standby">
            <p>In standby</p>
            <button
                class="p-wakebtn"
                disabled={music.busy["power:" + featured.id]}
                onclick={() => music.wake(featured)}
            >
                <Icon name="power" size={18} /><span>Wake {featured.title}</span>
            </button>
        </div>
    {/if}
{/snippet}

{#snippet transport()}
    {#if featured}
        <div class="p-transport">
            {#if featured.canSkip}
                <button
                    class="t-btn"
                    aria-label="Previous track"
                    disabled={music.busy["previous:" + featured.id]}
                    onclick={() => music.skip(featured, "previous")}
                >
                    <Icon name="skipPrev" size={wide ? 20 : 24} />
                </button>
            {/if}
            <button
                class="t-btn primary"
                class:on={featured.playing}
                aria-label={featured.playing ? "Pause" : "Play"}
                disabled={music.busy["play:" + featured.id]}
                onclick={() => music.togglePlay(featured)}
            >
                <Icon name={featured.playing ? "pause" : "play"} size={wide ? 24 : 30} />
            </button>
            {#if featured.canSkip}
                <button
                    class="t-btn"
                    aria-label="Next track"
                    disabled={music.busy["next:" + featured.id]}
                    onclick={() => music.skip(featured, "next")}
                >
                    <Icon name="skipNext" size={wide ? 20 : 24} />
                </button>
            {/if}
        </div>
    {/if}
{/snippet}

{#snippet volume()}
    {#if featured}
        <div class="p-volume">
            <button
                class="v-ico"
                class:mute={featured.muted}
                aria-label={featured.muted ? "Unmute" : "Mute"}
                disabled={music.busy["mute:" + featured.id]}
                onclick={() => music.toggleMute(featured)}
            >
                <Icon name={featured.muted ? "volumeOff" : "volume"} size={18} />
            </button>
            <!-- A fader is an imprecise aim at arm's length, so the wall also
                 gets a discrete step either side of it. -->
            <button
                class="v-step"
                aria-label="Volume down"
                disabled={music.busy["vol:" + featured.id]}
                onclick={() => music.nudgeVolume(featured, -5)}
            >
                <Icon name="minus" size={18} />
            </button>
            <Slider
                value={music.vol}
                label="Volume"
                valueText="{music.vol}%"
                onInput={(v) => music.dragVolume(featured, v)}
                onChange={(v) => music.setVolume(featured, v)}
            />
            <button
                class="v-step"
                aria-label="Volume up"
                disabled={music.busy["vol:" + featured.id]}
                onclick={() => music.nudgeVolume(featured, 5)}
            >
                <Icon name="plus" size={18} />
            </button>
            <span class="v-val mono">{music.vol}</span>
        </div>
    {/if}
{/snippet}

{#if featured}
    {@const openLabel = `Open music — ${featured.trackTitle ?? (featured.playing ? "playing" : "nothing playing")} on ${featured.title}`}
    <article class="p-card" class:playing={featured.playing} class:wide>
        {#if wide}
            <!-- The dashboard band, as one line across the panel: the cover,
                 what is playing with its scrubber under it, the transport,
                 and the room the transport is driving with its fader. Air
                 above and below rather than a cover stretched to fill the
                 band — this strip is what you *read* on the way past and
                 poke in passing, and a record wants the size a record wants
                 one depth in, where listening is the whole job (§16).
                 The cover carries the tap-through: it is the biggest and
                 most obviously tappable thing here (§15.8), and the band's
                 heading carries the same door for the states where there is
                 no card at all. The scrubber rides with the meta, beside the
                 art it describes and out of the transport — a hairline
                 across the control strip described something you weren't
                 looking at. It stays out of the tap-through button too: a
                 scrubber inside a link is not a scrubber. -->
            {#if onOpen}
                <button class="p-cover p-open" onclick={onOpen} aria-label={openLabel}>
                    {@render art()}
                </button>
            {:else}
                <div class="p-cover">{@render art()}</div>
            {/if}

            <div class="p-mid">
                {@render meta(false)}
                {@render queuedLine()}
                {#if !featured.standby}
                    <div class="p-rail">{@render rail()}</div>
                {/if}
            </div>

            {#if featured.standby}
                {@render standby()}
            {:else}
                {@render transport()}
                <!-- The fader is the one control on the strip whose reach
                     isn't obvious from looking at it, so the room it reaches
                     is named directly above it (§15.5). -->
                <div class="p-dest">
                    <span class="p-destchip">
                        <span class="p-destname">{featured.title}</span>
                        {#if speakerCount > 1}
                            <span class="p-destn mono">{speakerCount} spkrs</span>
                        {/if}
                    </span>
                    {@render volume()}
                </div>
            {/if}
        {:else}
            <div class="p-body">
                <!-- The card is two regions, and which one a control is in
                     is the layout decision (§16). This one scrolls: the
                     cover and what is playing. The strip below never does.
                     A wall is read from across the room and tapped in
                     passing, so the tapping half has to be where it was
                     last time. -->
                <div class="p-scroll" bind:clientHeight={scrollH} bind:clientWidth={scrollW}>
                    {#if onOpen}
                        <!-- Transport and volume stay out of the button so
                             the player still answers on the panel itself. -->
                        <button class="p-head p-open" onclick={onOpen} aria-label={openLabel}>
                            {@render art()}
                            {@render meta(true)}
                        </button>
                    {:else}
                        <div class="p-head">
                            {@render art()}
                            {@render meta(false)}
                        </div>
                    {/if}
                </div>

                <!-- The pinned strip: where the track is, play/pause and
                     skip, and how loud. These are what a wall gets walked up
                     to for, so they hold the same place whatever else the
                     room has to show. -->
                <div class="p-controls">
                    {#if featured.standby}
                        {@render standby()}
                    {:else}
                        {@render queuedLine()}
                        <!-- Stacked, the scrubber stays in the strip: there
                             is one column, so "under the cover" and "above
                             the transport" are the same place, and the strip
                             is the half that never moves. -->
                        {@render rail()}
                        {@render transport()}
                        {@render volume()}
                    {/if}
                </div>
            </div>
        {/if}
    </article>
{/if}

<style>
    .p-card {
        flex: 1;
        min-height: 0;
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
        padding: var(--space-5);
        border-radius: var(--r-lg);
        border: 1px solid var(--hairline);
        background: var(--card);
        /* The card itself never scrolls now — .p-scroll inside it does, so
           the strip under it can't be pushed off the panel. */
        overflow: hidden;
        transition:
            background var(--t-med),
            border-color var(--t-med);
    }
    .p-card.playing {
        background: var(--tile-on-gradient);
        border-color: var(--tile-on-border);
    }

    /* The dashboard band: one line, centred in the band's height, with no
       card under it. The stacked bands are the panel's chrome — a hairline
       above and below — so a bordered, filled rectangle inside one of them
       is a card drawn on a card. Flat while it plays, too: there is one
       player on this band and nothing to distinguish it from, and the cover
       already carries the waveform (§16's rule for the full player, and it
       is the same reason). */
    .p-card.wide {
        flex: none;
        flex-direction: row;
        align-items: center;
        gap: var(--space-6);
        padding: 0;
        border: 0;
        border-radius: 0;
        background: none;
        overflow: visible;
    }
    .p-card.wide.playing {
        background: none;
    }
    /* What's playing, and how far through it is — the flexible middle, so
       the title truncates against the row rather than pushing the transport
       off the end of it. */
    .p-mid {
        flex: 1 1 auto;
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
    }
    /* The destination and its fader, as a column at the trailing edge. Wide
       enough for the fader to be an aim rather than a nudge, and narrow
       enough that the middle keeps the row. */
    .p-dest {
        flex: none;
        width: 280px;
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
    }
    /* Not a button: the picker is the band's own chip row, one line up. This
       states which room the fader under it reaches (§15.5). */
    .p-destchip {
        display: inline-flex;
        align-items: center;
        gap: var(--space-2);
        align-self: flex-start;
        max-width: 100%;
        min-height: 34px;
        padding: 0 15px;
        border-radius: var(--r-pill);
        border: 1px solid var(--hairline);
        background: var(--card-2);
        font-size: 13px;
        font-weight: 600;
    }
    .p-destname {
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
    .p-destn {
        flex-shrink: 0;
        font-size: 11.5px;
        font-weight: 500;
        color: var(--text-dim);
    }
    /* The cover slot. It has no size of its own any more: the square inside
       it is stated in pixels (see `coverPx`), so this is just the frame that
       centres it against the band and carries the focus ring. */
    .p-cover {
        flex: none;
        align-self: center;
        display: flex;
        padding: 0;
        border: 0;
        background: none;
        color: inherit;
        font: inherit;
        cursor: pointer;
        border-radius: var(--r-md);
    }
    .p-cover:focus-visible {
        box-shadow: var(--focus-ring);
    }

    /* The scrubber beside the record. Distance-scaled like the rest of the
       panel (§16): the shared rail's times are sized for a phone in the
       hand. The live line is held to one line so a KEF's "no track
       position" can't wrap the row into two. */
    .p-rail {
        flex: none;
        min-width: 0;
    }
    .p-rail :global(.rail-times) {
        font-size: 12.5px;
    }
    .p-rail :global(.rail-live) {
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .p-body {
        flex: 1 1 auto;
        min-width: 0;
        min-height: 0;
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
    }
    /* Everything but the transport strip. It takes the card's leftover
       height and scrolls what doesn't fit — which, past the cover and the
       track, is only ever preferences. */
    .p-scroll {
        flex: 1 1 auto;
        min-height: 0;
        overflow-y: auto;
        /* Down the card, and only down it (§12): nothing in here is wider
           than the column, but a fader row lands on a fractional pixel as
           readily as a queue row does, and the card would pan by the one
           the rounding adds. */
        overflow-x: hidden;
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
    }
    /* The transport strip, pinned. The hairline is the only thing that says
       the card is in two halves — without it the strip reads as the bottom
       of a list that happens to stop there, and there is nothing else on a
       kiosk (no scrollbar, no chrome) to say the half above it moves. */
    .p-controls {
        flex: none;
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
        padding-top: var(--space-3);
        border-top: 1px solid var(--hairline);
    }

    /* Cover + track title, stacked. The pair is sized by the cover, which
       was measured to leave the two lines of meta their room (HEAD_GAP +
       metaH), so nothing here has to be capped or squeezed. */
    .p-head {
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
        flex: 0 0 auto;
        min-width: 0;
    }

    /* The tap-through button: reset to a plain flex column so it reads as
       the art + meta it contains, not as a button. */
    .p-open {
        padding: 0;
        border: 0;
        background: none;
        color: inherit;
        font: inherit;
        text-align: left;
        cursor: pointer;
        border-radius: var(--r-md);
        min-width: 0;
    }
    .p-open:focus-visible {
        box-shadow: var(--focus-ring);
    }

    /* The square itself. In the landscape shapes the card measures what the
       cover may have and writes both axes onto this box in pixels, so there
       is no ratio to resolve and no percentage to chase up through a flex
       chain — the failure that took the artwork and the band's tap-through
       with it. The rule below is the portrait fallback and the single frame
       before the first measurement: a width-led square, which is the one
       direction every engine agrees on. */
    .p-artbox {
        position: relative;
        flex: none;
        width: 100%;
        aspect-ratio: 1;
        margin-inline: auto;
        min-height: 96px;
        overflow: hidden;
        border-radius: var(--r-md);
        background: var(--card-2);
    }
    .p-art {
        display: block;
        width: 100%;
        height: 100%;
        object-fit: cover;
    }
    span.p-art {
        font-size: 11px;
    }
    .p-expand {
        position: absolute;
        top: var(--space-3);
        right: var(--space-3);
        width: 44px;
        height: 44px;
        display: grid;
        place-items: center;
        border: 0;
        border-radius: var(--r-sm);
        background: var(--bg-bar);
        color: var(--text);
        cursor: pointer;
        transition: transform var(--t-fast);
    }
    .p-expand:active {
        transform: scale(0.92);
        transition-duration: 80ms;
    }
    .p-expand:focus-visible {
        box-shadow: var(--focus-ring);
    }

    .p-wave {
        position: absolute;
        left: var(--space-3);
        bottom: var(--space-3);
        padding: 6px 8px;
        border-radius: var(--r-sm);
        background: var(--bg-bar);
        display: inline-flex;
    }

    .p-track {
        display: flex;
        flex-direction: column;
        gap: 4px;
        min-width: 0;
        flex-shrink: 0;
    }
    .p-title {
        font-size: 21px;
        font-weight: 600;
        letter-spacing: -0.02em;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    /* On the band the title shares a line with everything else the strip
       has to say, so it is stated at the band's scale rather than the
       column's — the record is where a title gets to be large (§16). */
    .p-card.wide .p-title {
        font-size: 19px;
    }
    .p-card.wide .p-sub {
        font-size: 13.5px;
    }
    .p-subrow {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-2);
        min-width: 0;
    }
    .p-sub {
        font-size: 14px;
        color: var(--text-mute);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .p-go {
        color: var(--text-dim);
        flex-shrink: 0;
        display: inline-flex;
    }

    .p-standby {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: var(--space-3);
        padding: var(--space-4) 0;
        flex-shrink: 0;
    }
    .p-standby p {
        margin: 0;
        font-size: 13px;
        color: var(--text-dim);
    }
    .p-wakebtn {
        display: inline-flex;
        align-items: center;
        gap: 8px;
        min-height: 48px;
        padding: 0 var(--space-5);
        border-radius: var(--r-pill);
        border: 1px solid var(--border-strong);
        background: var(--card-2);
        color: var(--text);
        font-family: inherit;
        font-size: 14px;
        font-weight: 500;
        cursor: pointer;
        transition:
            background var(--t-fast),
            transform var(--t-fast);
    }
    .p-wakebtn:active {
        transform: scale(0.96);
        transition-duration: 80ms;
    }
    .p-wakebtn:disabled {
        opacity: 0.55;
    }

    /* Transport sized for a wall poke: 64px sides, 80px centre on the
       depth's column, one size down on the dashboard band — the band is a
       strip shared with the meta and the fader, and the full player is the
       screen a transport gets to be the subject of (§16). 48px still clears
       the §2 floor by a comfortable margin. */
    .p-transport {
        display: flex;
        align-items: center;
        justify-content: center;
        gap: var(--space-5);
        flex-shrink: 0;
    }
    .t-btn {
        width: 64px;
        height: 64px;
        border-radius: 50%;
        border: 1px solid var(--hairline);
        background: var(--card-2);
        color: var(--text);
        display: grid;
        place-items: center;
        cursor: pointer;
        transition:
            background var(--t-fast),
            color var(--t-fast),
            border-color var(--t-fast),
            transform var(--t-fast);
    }
    .t-btn:active {
        transform: scale(0.94);
        transition-duration: 80ms;
    }
    .t-btn:disabled {
        opacity: 0.5;
    }
    .t-btn.primary {
        width: 80px;
        height: 80px;
        background: var(--on);
        border-color: var(--on);
        color: var(--primary-fg);
    }
    .p-card.wide .p-transport {
        flex: none;
        gap: var(--space-3);
    }
    .p-card.wide .t-btn {
        width: 48px;
        height: 48px;
    }
    .p-card.wide .t-btn.primary {
        width: 60px;
        height: 60px;
    }

    /* Queued: the only visible trace an untouched player leaves. */
    .p-queued {
        display: flex;
        align-items: center;
        gap: 8px;
        margin: 0;
        padding: 10px var(--space-3);
        border-radius: var(--r-md);
        background: var(--on-soft);
        color: var(--on);
        font-size: 12.5px;
        flex-shrink: 0;
    }
    .p-queued span {
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .p-volume {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        flex-shrink: 0;
    }
    /* The band's fader keeps every control at its full 44px: the trailing
       column was widened to take them rather than the controls shrunk to
       fit it. A wall's volume is aimed at from across a room. */
    .p-card.wide .v-ico {
        margin-left: -8px;
    }
    .v-ico {
        width: 44px;
        height: 44px;
        display: grid;
        place-items: center;
        border: 0;
        background: none;
        color: var(--text-mute);
        cursor: pointer;
        border-radius: var(--r-sm);
        flex-shrink: 0;
        margin-left: -10px;
    }
    .v-ico.mute {
        color: var(--bad);
    }
    .v-ico:disabled {
        opacity: 0.5;
    }
    /* The ± steps: same hit area as the mute button, quieter ink. */
    .v-step {
        width: 44px;
        height: 44px;
        display: grid;
        place-items: center;
        flex-shrink: 0;
        border: 1px solid var(--hairline);
        border-radius: 50%;
        background: var(--card-2);
        color: var(--text-mute);
        cursor: pointer;
        transition:
            background var(--t-fast),
            transform var(--t-fast);
    }
    .v-step:active {
        transform: scale(0.92);
        transition-duration: 80ms;
    }
    .v-step:disabled {
        opacity: 0.5;
    }
    .v-val {
        font-size: 13px;
        color: var(--text-mute);
        width: 3ch;
        text-align: right;
        flex-shrink: 0;
    }

    /* The reference wall panel is a 1024×768 iPad Air 2 (§16), where the
       card's whole stack — cover, meta, scrubber, transport, play modes,
       volume — lands within a dozen pixels of the column it has to fit.
       Tightening the card's own spacing there is what keeps the cover off
       its floor, and keeps the depth's extra rows from sitting as far
       under the fold. Nothing that is a touch target shrinks. */
    @media (max-height: 820px) and (orientation: landscape) {
        .p-card,
        .p-body,
        .p-scroll,
        .p-controls {
            gap: var(--space-3);
        }
        .p-card {
            padding: var(--space-4);
        }
        .p-transport {
            gap: var(--space-4);
        }
    }

    /* Portrait stack: the whole page scrolls, so the card sizes to its
       content and neither region owns a scroll of its own — a pinned strip
       inside a page that already scrolls is just a shorter card. */
    @media (orientation: portrait), (max-width: 900px) {
        /* Landscape needs landscape. Stacked again, the band's markup falls
           out as cover-over-controls on its own — .p-cover is simply the
           first row of the column instead of the first column of the row,
           and the trailing destination column becomes the last row of it.
           `coverPx` stands down here (see `landscape`), so the cover is the
           width-led square the base rule describes, capped so it can't take
           a phone's whole screen. The card gets its chrome back here too:
           in a page that scrolls there are no bands to be edges, so the
           player has to be its own. */
        .p-card.wide {
            flex-direction: column;
            align-items: stretch;
            padding: var(--space-5);
            border: 1px solid var(--hairline);
            border-radius: var(--r-lg);
            background: var(--card);
        }
        .p-card.wide.playing {
            background: var(--tile-on-gradient);
            border-color: var(--tile-on-border);
        }
        .p-dest {
            width: 100%;
        }
        .p-cover {
            align-self: auto;
            width: 100%;
            max-width: 280px;
            margin-inline: auto;
        }
        .p-artbox {
            max-width: 280px;
        }
        .p-card {
            flex: none;
            overflow: visible;
        }
        .p-scroll {
            flex: none;
            overflow: visible;
        }
    }
</style>
