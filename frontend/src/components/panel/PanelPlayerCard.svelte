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
    // the dashboard's one-line band nor a 420px column has the height for
    // them, and the cover pays for all of it either way.
    //
    // Two shapes, and the zone picks (`wide`): the dashboard's band lays
    // everything out on one line — cover, meta and scrubber, transport,
    // destination and fader — and the depth's column centres the same parts
    // one under the other. The band is a strip you read in passing and poke
    // at; the column is where the cover gets to be a record, and where it
    // carries the way one depth further in (§16).
    //
    // Neither shape wears a card. Both sit inside a region this surface has
    // already drawn an edge around — the band between two hairlines, the
    // column behind one — and a bordered rectangle inside a bordered region
    // draws the same edge twice. Flat while the room plays, too: §15.2's
    // fill says *this one* is making noise among others that aren't, and
    // there is one player on each of these surfaces.
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
        artMax = 0,
    }: {
        music: PanelMusicStore;
        /** Band only: the cover is a button into the music depth. */
        onOpen?: () => void;
        /** Column only: the cover is a button that takes the player full
         *  screen. The band's cover is the way *into* the depth and can't
         *  also be the way past it. */
        onExpand?: () => void;
        /** Landscape: the cover beside the controls rather than above them.
         *  The dashboard's music zone is a wide, short band; the depth's is a
         *  tall, narrow column. Which way round the parts go is purely the
         *  zone's aspect, so the zone says. */
        wide?: boolean;
        /** Band only: the measured height of the stage the row sits in. The
         *  cover grows into it between BAND_ART_MIN and BAND_ART_CAP. The
         *  band measures rather than the card, because a box the cover can
         *  size is a reading that chases its own tail. */
        artMax?: number;
    } = $props();

    const featured = $derived(music.featured);

    // ── The cover's size, stated in pixels ──────────────────────────────
    // The cover is a square and height is the scarce axis on a wall, so the
    // square is sized from the height it is allowed — and written onto the
    // box outright, definite on both axes. It used to be said as a chain of
    // ratios — an `aspect-ratio` hanging off a flex item stretched by its
    // row, a `height: 100%` resolved against that, a ratio again on the box
    // inside — and a chain is only as good as its weakest link: an engine
    // that resolves any one of those to `auto` collapses the cover to
    // nothing. On the dashboard that took the artwork *and* the band's only
    // way into the music depth with it, since the cover is the tap-through.
    /** The dashboard band's cover. It grows into whatever height the band's
     *  stage has, between these two, and the cap is a *width* decision as
     *  much as a height one: the band's row is 960px on the reference wall
     *  and four columns share it, so every pixel the square takes comes off
     *  the title beside it. Past ~200 the title column is too narrow to
     *  name a song, which is the one thing this band exists to say. The
     *  height the cap leaves is not spent on air — it goes to the shelf at
     *  the foot of the band (PanelBandShelf, §16). */
    const BAND_ART_MIN = 128;
    const BAND_ART_CAP = 200;
    /** The depth's column, stated outright the way the band states its own.
     *  The column is a fixed 420px and everything in it is centred under the
     *  record, so there is nothing left for a measurement to discover: 230
     *  is what a 420px column has room for once the caption, the scrubber,
     *  the transport and the fader have theirs. The card used to measure a
     *  scroll region here because the room's preferences were stacked under
     *  the cover; they moved to the Rooms pane, and the measurement was the
     *  last thing left of that layout (§16). */
    const COL_ART = 230;

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

    const coverPx = $derived.by(() => {
        if (!landscape) return 0; // portrait: CSS sizes it from the width
        // The band's stage is measured by the band (see `artMax`) because
        // the band's height is whatever the strip above and the room row
        // below leave; the depth's column is a stated width with a stated
        // square in it.
        if (!wide) return COL_ART;
        if (artMax <= 0) return BAND_ART_MIN; // pre-measure, one frame
        return Math.max(BAND_ART_MIN, Math.min(artMax, BAND_ART_CAP));
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
                <!-- Opposite the waveform, on the biggest thing in the
                     column. The whole cover is the tap (see below) —
                     listening is a different job from browsing and it wants
                     the whole screen (§16) — so this is the mark that says
                     so, not a second target beside it. -->
                <span class="p-expand" aria-hidden="true"><Icon name="expand" size={18} /></span>
            {/if}
        </span>
    {/if}
{/snippet}

{#snippet meta()}
    {#if featured}
        <span class="p-track">
            <span class="p-title">
                {featured.trackTitle ?? (featured.playing ? "Playing" : "Not playing")}
            </span>
            <span class="p-sub">{featured.trackSub || featured.title}</span>
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
                    <Icon name="skipPrev" size={20} />
                </button>
            {/if}
            <button
                class="t-btn primary"
                class:on={featured.playing}
                aria-label={featured.playing ? "Pause" : "Play"}
                disabled={music.busy["play:" + featured.id]}
                onclick={() => music.togglePlay(featured)}
            >
                <Icon name={featured.playing ? "pause" : "play"} size={24} />
            </button>
            {#if featured.canSkip}
                <button
                    class="t-btn"
                    aria-label="Next track"
                    disabled={music.busy["next:" + featured.id]}
                    onclick={() => music.skip(featured, "next")}
                >
                    <Icon name="skipNext" size={20} />
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
    {@const what = featured.trackTitle ?? (featured.playing ? "playing" : "nothing playing")}
    {@const openLabel = `Open music — ${what} on ${featured.title}`}
    {@const expandLabel = `Full screen player — ${what} on ${featured.title}`}
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
                {@render meta()}
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
            <!-- The depth's column: the record at the top and everything
                 that answers for it under, centred, all of it visible at
                 once. There is no pinned strip here any more and nothing
                 behind a scroll — the two regions existed to hold the
                 transport still while the room's preferences scrolled above
                 it, and the preferences left for the Rooms pane (§16). What
                 is left is a player, and a player fits.
                 The cover is the way one depth further in: it is the biggest
                 and most obviously tappable thing in the column (§15.8), so
                 it carries the tap whole rather than lending a corner of
                 itself to a 44px chip. Transport and volume stay outside the
                 button, so the player still answers here. -->
            {#if onExpand}
                <button class="p-cover p-open" onclick={onExpand} aria-label={expandLabel}>
                    {@render art()}
                </button>
            {:else}
                <div class="p-cover">{@render art()}</div>
            {/if}

            {@render meta()}

            {#if featured.standby}
                {@render standby()}
            {:else}
                {@render queuedLine()}
                <!-- The scrubber belongs to the record, and in one column
                     "under the cover" and "above the transport" are the same
                     place (§16). -->
                <div class="p-rail">{@render rail()}</div>
                {@render transport()}
                {@render volume()}
            {/if}
        {/if}
    </article>
{/if}

<style>
    /* The depth's column. Flat, and centred in the column it was given: no
       border, no fill, and none while the room plays either. §15.2's fill is
       what a hero or a room card wears to say *this one* is making noise
       among others that aren't; there is one player here, its column is
       already divided from the work area by a hairline, and the cover
       carries the waveform. A card inside that column would draw the same
       edge twice — the same rule the band's player and the full player's
       controls follow (§16).
       It owns a scroll as a safety valve, not as a layout: everything in it
       fits the reference wall, and a shorter screen scrolls rather than
       squeezing the record. */
    .p-card {
        flex: 1;
        min-height: 0;
        display: flex;
        flex-direction: column;
        align-items: center;
        justify-content: center;
        gap: var(--space-5);
        overflow-y: auto;
        /* One axis, stated (§12): a fader row lands on a fractional pixel as
           readily as a queue row does, and `overflow-y: auto` alone computes
           the other axis to `auto` too — so the column would pan sideways by
           the pixel the rounding invents. */
        overflow-x: hidden;
    }
    /* The column centres its rows on the record; the ones that are rails
       rather than blocks of text take the column's whole width, because a
       fader you have to aim at from across a room wants every pixel of it.
       The meta stretches for the opposite reason: a title truncates against
       the column that way instead of setting its own width. */
    .p-card:not(.wide) .p-track,
    .p-card:not(.wide) .p-rail,
    .p-card:not(.wide) .p-volume,
    .p-card:not(.wide) .p-queued {
        align-self: stretch;
        min-width: 0;
    }
    .p-card:not(.wide) .p-track {
        text-align: center;
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
        justify-content: flex-start;
        gap: var(--space-5);
        overflow: visible;
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
        width: 268px;
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

    /* The scrubber: beside the record in the band, under it in the column.
       Distance-scaled like the rest of the panel (§16) — the shared rail's
       times are sized for a phone in the hand. The live line is held to one
       line so a KEF's "no track position" can't wrap the row into two. */
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

    /* The tap-through button: reset to a plain flex box so it reads as the
       art it contains, not as a button. */
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
    /* The mark that says the cover leads somewhere. Not a control — the
       cover around it is the control — so it is drawn small and quiet,
       opposite the waveform. */
    .p-expand {
        position: absolute;
        top: var(--space-3);
        right: var(--space-3);
        width: 32px;
        height: 32px;
        display: grid;
        place-items: center;
        border-radius: var(--r-sm);
        background: var(--bg-bar);
        color: var(--text);
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
    /* One size on both shapes. The band and the depth's column say the same
       thing at the same scale — a title is stated large on the full player,
       which is the screen it is the subject of (§16). */
    .p-title {
        font-size: 19px;
        font-weight: 600;
        letter-spacing: -0.02em;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .p-sub {
        font-size: 13.5px;
        color: var(--text-mute);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
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

    /* Transport sized for a wall poke: 48px sides with a 60px centre on both
       of this card's shapes. The band is a strip shared with the meta and the
       fader, and the depth's column is shared with the record — the full
       player is the one screen a transport gets to be the subject of, and it
       is the only one at 64/80 (§16). 48px clears the §2 floor by a
       comfortable margin either way. */
    .p-transport {
        display: flex;
        align-items: center;
        justify-content: center;
        gap: var(--space-4);
        flex-shrink: 0;
    }
    .t-btn {
        width: 48px;
        height: 48px;
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
        width: 60px;
        height: 60px;
        background: var(--on);
        border-color: var(--on);
        color: var(--primary-fg);
    }
    /* On the band the transport is one of four things sharing a 960px row,
       so it holds only the width it needs. */
    .p-card.wide .p-transport {
        flex: none;
        gap: var(--space-3);
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

    /* The reference wall panel is a 1024×768 iPad Air 2 (§16). The column's
       stack — record, meta, scrubber, transport, fader — has room to spare
       there, but a shorter landscape screen doesn't, and the first thing to
       give is the air between the rows rather than the record. Nothing that
       is a touch target shrinks. */
    @media (max-height: 820px) and (orientation: landscape) {
        .p-card {
            gap: var(--space-4);
        }
        .p-card.wide {
            gap: var(--space-5);
        }
    }

    /* Portrait stack: the whole page scrolls, so the card sizes to its
       content and owns no scroll of its own. */
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
            justify-content: flex-start;
            overflow: visible;
        }
        /* Stacked in a page that scrolls, the column is a card again: there
           are no bands here to be its edges, so it has to be its own. */
        .p-card:not(.wide) {
            align-items: stretch;
            padding: var(--space-5);
            border: 1px solid var(--hairline);
            border-radius: var(--r-lg);
            background: var(--card);
        }
    }
</style>
