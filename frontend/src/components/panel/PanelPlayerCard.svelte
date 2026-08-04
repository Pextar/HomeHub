<script lang="ts">
    import Icon from "../Icon.svelte";
    import Waveform from "../music/Waveform.svelte";
    import Slider from "../music/Slider.svelte";
    import TrackRail from "../music/TrackRail.svelte";
    import { kefSourceLabel } from "../../lib/kef";
    import { repeatLabel } from "../../lib/music/sonos.svelte";
    import { clock } from "../../lib/music/clock.svelte";
    import type { PanelMusicStore } from "../../lib/panel-music.svelte";

    // The featured source's player, shared by both of the panel's depths
    // (DESIGN.md §16). It is a player and only a player: cover, what is
    // playing, scrubber, transport, volume. The room's own settings — play
    // modes on the depth, the sleep timer, per-speaker faders, the KEF
    // input — live on the depth's Rooms pane (PanelRoomSettings), because
    // stacked under the cover in a 360px column they cost more height than
    // the column had and the cover paid for all of it.
    //
    // Two shapes, and the zone picks (`wide`): the dashboard's wide band
    // puts the cover beside the controls, the depth's tall column above
    // them.
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
    const gs = $derived(featured?.groupState);

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
    /** The gap between the cover and the meta under it, stacked. */
    const HEAD_GAP = 16;

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
    // the reading would chase its own tail: the card's width, the body's
    // stretched height (the card's height is the band's, not its content's),
    // the scroll region's leftover after the pinned strip, and the meta,
    // whose two lines never wrap.
    let cardW = $state(0);
    let bodyH = $state(0);
    let scrollH = $state(0);
    let scrollW = $state(0);
    let metaH = $state(0);

    const coverPx = $derived.by(() => {
        if (!landscape) return 0; // portrait: CSS sizes it from the width
        const height = wide ? bodyH : scrollH - metaH - HEAD_GAP;
        // Beside the controls the cover may take about half the band; above
        // them it may take the column.
        const width = wide ? cardW * 0.45 : scrollW;
        if (height <= 0 || width <= 0) return 0; // pre-measure, one frame
        return Math.max(ART_FLOOR, Math.min(height, width, ART_CAP));
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

    const repeatText = $derived(
        gs?.repeat === "all" ? "Repeat all" : gs?.repeat === "one" ? "Repeat one" : "Repeat",
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

{#if featured}
    {@const openLabel = `Open music — ${featured.trackTitle ?? (featured.playing ? "playing" : "nothing playing")} on ${featured.title}`}
    <article class="p-card" class:playing={featured.playing} class:wide bind:clientWidth={cardW}>
        {#if wide}
            <!-- Landscape: the cover takes the band's whole height and the
                 controls sit beside it, so the two stop competing for the
                 one axis a 768px wall is short of. The cover carries the
                 tap-through on its own here — it is the biggest and most
                 obviously tappable thing on the card (§15.8), and a second
                 button around the meta would only say the same thing
                 twice. The band's header carries the way in as well, but
                 that is section navigation and it is there whether or not
                 there is a card under it (§16). -->
            {#if onOpen}
                <button class="p-cover p-open" onclick={onOpen} aria-label={openLabel}>
                    {@render art()}
                </button>
            {:else}
                <div class="p-cover">{@render art()}</div>
            {/if}
        {/if}

        <div class="p-body" bind:clientHeight={bodyH}>
            <!-- The card is two regions, and which one a control is in is
                 the layout decision (§16). This one scrolls: what is
                 playing and the room's preferences — plus the cover, when
                 the card is stacked. The strip below never does. A wall is
                 read from across the room and tapped in passing, so the
                 tapping half has to be where it was last time. -->
            <div class="p-scroll" bind:clientHeight={scrollH} bind:clientWidth={scrollW}>
                {#if wide}
                    {@render meta(false)}
                {:else if onOpen}
                    <!-- Transport and volume stay out of the button so the
                         player still answers on the panel itself. -->
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

                {#if wide && !featured.standby && gs}
                    <!-- Play modes ride with the player only where the card
                         is a wide band and has the room for them. In the
                         depth's column they belong with the room, on the
                         Rooms pane (PanelRoomSettings) — stacked under the
                         cover they cost more height than the column had. -->
                    <div class="p-modes">
                        <button
                            class="p-mode"
                            class:on={gs.shuffle}
                            aria-pressed={gs.shuffle}
                            disabled={music.busy["mode:" + featured.id]}
                            onclick={() => music.toggleShuffle()}
                        >
                            <Icon name="shuffle" size={16} /><span>Shuffle</span>
                        </button>
                        <button
                            class="p-mode"
                            class:on={gs.repeat !== "off"}
                            aria-pressed={gs.repeat !== "off"}
                            aria-label={repeatLabel(gs.repeat)}
                            disabled={music.busy["mode:" + featured.id]}
                            onclick={() => music.cycleRepeat()}
                        >
                            <Icon
                                name={gs.repeat === "one" ? "repeatOne" : "repeat"}
                                size={16}
                            /><span>{repeatText}</span>
                        </button>
                        <button
                            class="p-mode"
                            class:on={gs.crossfade}
                            aria-pressed={gs.crossfade}
                            disabled={music.busy["xfade:" + featured.id]}
                            onclick={() => music.toggleCrossfade()}
                        >
                            <Icon name="activity" size={16} /><span>Crossfade</span>
                        </button>
                        <!-- What happens after the last queued song: carry on
                             with the queue, or keep the room going with music
                             like it (§15.5). The hub's preference, not the
                             speaker's, but it reads as one more play mode. -->
                        <button
                            class="p-mode"
                            class:on={!!featured.autoplay}
                            aria-pressed={!!featured.autoplay}
                            disabled={music.busy["autoplay:" + featured.id]}
                            onclick={() => music.toggleAutoplay()}
                        >
                            <Icon name="assistant" size={16} /><span>Play similar</span>
                        </button>
                    </div>
                {/if}
            </div>

            <!-- The pinned strip: where the track is, play/pause and skip, and
             how loud. These are what a wall gets walked up to for, so they
             hold the same place whatever else the room has to show. -->
            <div class="p-controls">
                {#if featured.standby}
                    <!-- A speaker asleep is a speaker one tap from awake: the
                     wall wakes it rather than sending anyone to the full
                     view. -->
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
                {:else}
                    {#if queued}
                        <!-- The one thing queueing changes that can be seen —
                         so it belongs where the eye already is, not at the
                         far end of a scroll it may never reach. -->
                        <p class="p-queued">
                            <Icon name="check" size={14} />
                            <span
                                >{queued.next ? "Playing next" : "Added to the queue"} — {queued.title}</span
                            >
                        </p>
                    {/if}

                    <TrackRail
                        position={music.posSec}
                        duration={music.durSec}
                        seekable={music.seekable}
                        idle={railIdle}
                        liveLabel={railLabel}
                        onSeek={(sec) => music.seek(sec)}
                    />

                    <div class="p-transport">
                        {#if featured.canSkip}
                            <button
                                class="t-btn"
                                aria-label="Previous track"
                                disabled={music.busy["previous:" + featured.id]}
                                onclick={() => music.skip(featured, "previous")}
                            >
                                <Icon name="skipPrev" size={24} />
                            </button>
                        {/if}
                        <button
                            class="t-btn primary"
                            class:on={featured.playing}
                            aria-label={featured.playing ? "Pause" : "Play"}
                            disabled={music.busy["play:" + featured.id]}
                            onclick={() => music.togglePlay(featured)}
                        >
                            <Icon name={featured.playing ? "pause" : "play"} size={30} />
                        </button>
                        {#if featured.canSkip}
                            <button
                                class="t-btn"
                                aria-label="Next track"
                                disabled={music.busy["next:" + featured.id]}
                                onclick={() => music.skip(featured, "next")}
                            >
                                <Icon name="skipNext" size={24} />
                            </button>
                        {/if}
                    </div>

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
                        <!-- A fader is an imprecise aim at arm's length, so the
                         wall also gets a discrete step either side of it. -->
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
            </div>
        </div>
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

    /* Landscape. The zone decides (see the `wide` prop): a wide, short band
       makes the cover as tall as the band, where stacking would cap it at
       whatever the controls left of the height — on a 1024x768 wall that is
       the difference between a 360px cover and a 160px one. */
    .p-card.wide {
        flex-direction: row;
        align-items: stretch;
        gap: var(--space-5);
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
    .p-body {
        flex: 1 1 auto;
        min-width: 0;
        min-height: 0;
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
    }
    /* Beside the cover the column is sized by the cover, not by its own
       content, so it can have slack — a room in standby has a title and a
       Wake button and nothing else. Centre the pair rather than pinning the
       strip to a bottom edge it shares with nothing. */
    .p-card.wide .p-body {
        justify-content: center;
    }
    .p-card.wide .p-scroll {
        flex: 0 1 auto;
    }

    /* Everything but the transport strip. It takes the card's leftover
       height and scrolls what doesn't fit — which, past the cover and the
       track, is only ever preferences. */
    .p-scroll {
        flex: 1 1 auto;
        min-height: 0;
        overflow-y: auto;
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

    /* Transport sized for a wall poke: 64px sides, 80px centre. */
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

    .p-modes {
        display: flex;
        flex-wrap: wrap; /* four chips don't fit the 352px column on one line */
        justify-content: center;
        gap: var(--space-2);
    }
    .p-mode {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        padding: 8px 12px;
        border-radius: var(--r-pill);
        border: 1px solid var(--hairline);
        background: var(--card-2);
        color: var(--text-mute);
        font-family: inherit;
        font-size: 12.5px;
        font-weight: 500;
        cursor: pointer;
        transition:
            background var(--t-fast),
            color var(--t-fast),
            border-color var(--t-fast),
            transform var(--t-fast);
    }
    .p-mode:active {
        transform: scale(0.95);
        transition-duration: 80ms;
    }
    .p-mode.on {
        background: var(--on-soft);
        border-color: var(--tile-on-border);
        color: var(--on);
    }
    .p-mode:disabled {
        opacity: 0.55;
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

    /* Names the block the room-wide fader used to introduce by sitting
       right above it. Mono uppercase micro-label, per §4. */

    /* Distance-scaled targets: this is a wall, so every chip on it clears
       the §2 floor rather than inheriting a phone's sizing. */
    @media (pointer: coarse) {
        .p-mode {
            min-height: 44px;
            padding-inline: 16px;
        }
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
        /* Landscape needs landscape. Stacked again, the `wide` markup falls
           out as cover-over-controls on its own — .p-cover is simply the
           first row of the column instead of the first column of the row.
           `coverPx` stands down here (see `landscape`), so the cover is the
           width-led square the base rule describes, capped so it can't take
           a phone's whole screen. */
        .p-card.wide {
            flex-direction: column;
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
