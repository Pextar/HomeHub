<script lang="ts">
    /**
     * The full player's record: the cover at the size the wall can give it,
     * the title and artist under it, the scrub rail, and the row of things
     * you can do to what is playing.
     *
     * The cover is measured rather than declared. Its cap is a *ratio* of
     * the stage rather than a flat number — 340px is the handoff's figure
     * for a 768px wall, and taking it literally made every bigger screen
     * (the desk browser this panel is opened from as readily as the iPad)
     * draw a small record in a large margin. The stage's own size comes in
     * as props because the stage belongs to the screen above; everything
     * derived from it belongs here, including the caption's height, which
     * is measured inside this component and feeds straight back into how
     * tall the cover may be.
     */
    import Icon from "../Icon.svelte";
    import Waveform from "../music/Waveform.svelte";
    import TrackRail from "../music/TrackRail.svelte";
    import { kefSourceLabel } from "../../lib/kef";
    import type { PanelMusicStore, PanelSource } from "../../lib/panel-music.svelte";

    let {
        music,
        featured,
        stageW,
        stageH,
        onOpenArtist,
    }: {
        music: PanelMusicStore;
        featured: PanelSource;
        /** The stage's measured box — the room the record has to fill. */
        stageW: number;
        stageH: number;
        onOpenArtist: () => void;
    } = $props();

    /** How far the ± buttons move the scrub. A rail is imprecise at arm's
     *  length, so the two steps are what actually get used. */
    const SEEK_STEP = 15;
    /** There is a track with a catalog id, and the login may write. */
    const canSave = $derived(!!featured.trackURI && music.canSave);

    /** The in-place confirmation for a radio run, for as long as it is worth
     *  reading. Same instrument as the queued-track line the depth's column
     *  uses, and for the same reason: nothing on screen moves when songs
     *  land at the end of a queue. */
    const NOTE_MS = 6000;

    let noteBeat = $state(0);
    $effect(() => {
        if (!music.lastRadio) return;
        const id = setInterval(() => (noteBeat = Date.now()), 1000);
        return () => clearInterval(id);
    });
    const radioNote = $derived.by(() => {
        void noteBeat;
        const r = music.lastRadio;
        if (!r || Date.now() - r.at > NOTE_MS) return "";
        return r.count > 1
            ? `Queued ${r.count} more like ${r.artist}`
            : `Playing more like ${r.artist}`;
    });

    const railIdle = $derived(!featured?.trackTitle && !featured?.playing);
    const railLabel = $derived(
        featured?.kind === "kef"
            ? `${kefSourceLabel(featured.input) || "Input"} — no track position`
            : featured?.kind === "zone"
              ? "played together — no track position"
              : undefined,
    );

    // Art that 404s (an expired service URL, a proxy that can't reach the
    // speaker) used to leave an empty box on the biggest square on the
    // panel — indistinguishable from a cover still loading. Fall back to
    // §6.7's placeholder, keyed to the URL so the next track gets its own
    // try. Same rule the band's card follows (§16).
    // (the body inside its own padding, which is why there is an element for
    // it: `clientHeight` on the padded body counts the padding as room the
    // cover could have, and it isn't) and the caption riding under the
    // square — then write both axes onto it outright.
    const ART_FLOOR = 160;
    /** The reference wall's stage, and the record the handoff draws in it:
     *  1024×768 less the header and the body's padding is 656 tall, and the
     *  record on it is 340. */
    const REF_STAGE_H = 656;
    const REF_ART = 340;
    /** Where the record stops growing whatever the screen. Past this it is a
     *  poster, not a record. */
    const ART_CEIL = 720;
    /** The gap between the cover and its caption, and between the columns. */
    const HEAD_GAP = 16;
    const COL_GAP = 32;
    /** The controls column: a stated width, not a share of the screen. A
     *  fader stretched across a desk monitor is a worse aim than one at
     *  arm's length on the iPad this is drawn for, and a queue row that wide
     *  parts its title from its duration by half a screen. */
    const SIDE_W = 380;

    /** The record's cap, in proportion to the screen rather than stated flat.
     *  340 is the handoff's number *for a 768px wall*, and a flat 340 read as
     *  the whole rule made every bigger screen — the desk browser this panel
     *  is opened from as readily as the iPad — draw a small record in a large
     *  margin. So the ratio is what carries: on the reference wall this
     *  computes to exactly 340, and a taller screen gets a record in the same
     *  proportion to it. The controls column doesn't grow, because a fader
     *  and a queue row have a size they are best at; a record doesn't. */
    const artCap = (stageH: number) =>
        Math.min(ART_CEIL, Math.max(REF_ART, Math.round(stageH * (REF_ART / REF_STAGE_H))));

    // Landscape is the designed-for shape and the one that measures; the
    // portrait fallback lets CSS size the cover from its width, the one
    // direction every engine agrees on. Mirrors the media query below.
    let landscape = $state(true);
    $effect(() => {
        const mq = window.matchMedia("(orientation: portrait), (max-width: 900px)");
        const apply = () => (landscape = !mq.matches);
        apply();
        mq.addEventListener("change", apply);
        return () => mq.removeEventListener("change", apply);
    });

    /** The art URL that 404'd, so the placeholder shows instead of a broken
     *  image — and so a *new* track's art is tried again rather than being
     *  written off with it. */
    let artFailed = $state<string | null>(null);
    const artSrc = $derived(featured.art && featured.art !== artFailed ? featured.art : null);

    let capH = $state(0);

    const coverPx = $derived.by(() => {
        if (!landscape) return 0; // portrait: CSS sizes it from the width
        const height = stageH - capH - HEAD_GAP;
        const width = stageW - SIDE_W - COL_GAP;
        if (height <= 0 || width <= 0) return 0; // pre-measure, one frame
        return Math.max(ART_FLOOR, Math.min(height, width, artCap(stageH)));
    });
    const coverStyle = $derived(coverPx ? `width:${coverPx}px` : "");
    const artStyle = $derived(coverPx ? `width:${coverPx}px;height:${coverPx}px` : "");
</script>

<section class="fp-record" style={coverStyle} aria-label="Now playing">
    <span class="fp-art" style={artStyle}>
        {#if artSrc}
            <img
                class="fp-cover"
                src={artSrc}
                alt=""
                onerror={() => (artFailed = artSrc)}
            />
        {:else}
            <span class="fp-cover placeholder">[ art ]</span>
        {/if}
        {#if featured.playing}
            <span class="fp-wave"><Waveform /></span>
        {/if}
    </span>
    <!-- The caption: how far through the record we are, and what
         it is. The scrubber belongs to the song and not to the
         controls — at the top of the card it was a hairline in
         a corner, describing something in the other column;
         under the cover it is exactly as wide as the record and
         reads from the sofa. It rides directly under the square
         and the name under it, so the rail touches the thing it
         measures. Both meta lines stay on one line each: the
         cover is measured against this block's height, so a
         caption that wrapped would resize the square that
         decides its width. -->
    <div class="fp-caption" bind:clientHeight={capH}>
        {#if !featured.standby}
            <!-- The rail, with a step either side of it where
                 stepping is possible. Same argument as the
                 volume row's −/+: a rail is an imprecise aim
                 at arm's length, and "back a bit, I missed
                 that" is the one seek anybody makes from a
                 sofa. Absent where there is nothing to seek
                 through — radio has no position to step
                 within (§15.1). -->
            <div class="fp-scrub">
                {#if music.seekable}
                    <button
                        class="s-step"
                        aria-label="Back {SEEK_STEP} seconds"
                        onclick={() =>
                            music.seek(Math.max(0, music.posSec - SEEK_STEP))}
                    >
                        <Icon name="skipPrev" size={15} />
                        <span class="s-num mono">{SEEK_STEP}</span>
                    </button>
                {/if}
                <TrackRail
                    position={music.posSec}
                    duration={music.durSec}
                    seekable={music.seekable}
                    idle={railIdle}
                    liveLabel={railLabel}
                    onSeek={(sec) => music.seek(sec)}
                />
                {#if music.seekable}
                    <button
                        class="s-step"
                        aria-label="Forward {SEEK_STEP} seconds"
                        onclick={() =>
                            music.seek(
                                Math.min(music.durSec, music.posSec + SEEK_STEP),
                            )}
                    >
                        <span class="s-num mono">{SEEK_STEP}</span>
                        <Icon name="skipNext" size={15} />
                    </button>
                {/if}
            </div>
        {/if}
        <div class="fp-meta">
            <h3 class="fp-title">
                {featured.trackTitle ??
                    (featured.playing ? "Playing" : "Not playing")}
            </h3>
            <p class="fp-sub">{featured.trackSub || featured.title}</p>
        </div>
        <!-- What you can do about the *song*, as opposed to
             about the room: keep it, hear more like it, go and
             read about who made it. They ride with the record
             for the same reason the scrubber does — their
             subject is on this side of the screen — and each
             appears only where it can act (§15.1): saving
             needs a catalog id, which radio and line-in don't
             carry, and the rest need an artist to have been
             named at all. -->
        {#if !featured.standby && (canSave || music.canRadio)}
            <div class="fp-acts">
                {#if canSave}
                    <button
                        class="a-heart"
                        class:on={music.saved}
                        aria-pressed={music.saved}
                        aria-label={music.saved
                            ? "Remove from your library"
                            : "Save to your library"}
                        disabled={!!music.busy["save:" + featured.trackURI]}
                        onclick={() => music.toggleSaved()}
                    >
                        <Icon
                            name={music.saved ? "heart" : "heartOutline"}
                            size={19}
                        />
                    </button>
                {/if}
                {#if music.canRadio}
                    <button
                        class="a-chip"
                        disabled={!!music.busy["radio"]}
                        onclick={() => music.startRadio()}
                    >
                        <Icon name="radio" size={15} /><span>More like this</span>
                    </button>
                    <!-- The artist page is the depth's, so
                         this climbs one rung rather than
                         opening a screen on top of a screen.
                         By name, because that is all a
                         speaker reports about what it is
                         playing. -->
                    <button class="a-chip" onclick={onOpenArtist}>
                        <span>{featured.trackArtist}</span>
                        <Icon name="chevronRight" size={15} />
                    </button>
                {/if}
            </div>
        {/if}
        <!-- Queuing a run of songs changes nothing visible, so
             the record says what went in for a few seconds —
             not a toast: a kiosk has nobody to dismiss cards
             (§10, §16). -->
        {#if radioNote}
            <p class="fp-note">{radioNote}</p>
        {/if}
    </div>
</section>

<!-- The controls, capped: a fader stretched across a 1500px
     desk monitor is a worse aim than one at arm's length on the
     iPad this is drawn for, and a queue row that wide parts its
     title from its duration by half a screen. The pair centres
     in whatever is left. -->

<style>
    /* ── The record ──────────────────────────────────────────────────── */
    /* Both boxes are written in pixels once the body has been measured
       (see `coverPx`). The rules here are the portrait fallback and the one
       frame before that measurement lands: a width-led square inside a
       column that already has a width, so there is never a frame where the
       caption decides how wide a record is. */
    .fp-record {
        flex: 0 0 auto;
        align-self: center;
        margin-inline: auto;
        display: flex;
        flex-direction: column;
        gap: var(--space-4); /* HEAD_GAP */
        width: 340px; /* ART_CAP */
        max-width: 100%;
        min-width: 0;
    }
    .fp-art {
        position: relative;
        flex: none;
        width: 100%;
        aspect-ratio: 1;
        min-height: 160px;
        overflow: hidden;
        border-radius: var(--r-lg);
        background: var(--card-2);
    }
    .fp-cover {
        display: block;
        width: 100%;
        height: 100%;
        object-fit: cover;
    }
    span.fp-cover {
        font-size: 12px;
    }
    .fp-wave {
        position: absolute;
        left: var(--space-4);
        bottom: var(--space-4);
        padding: 8px 10px;
        border-radius: var(--r-sm);
        background: var(--bg-bar);
        display: inline-flex;
    }

    /* Name and position, under the record they belong to. */
    .fp-caption {
        flex: none;
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
    }
    /* Distance-scaled like the rest of the panel (§16): the shared rail's
       times are sized for a phone in the hand. */
    .fp-caption :global(.rail-times) {
        font-size: 12.5px;
    }
    .fp-meta {
        min-width: 0;
        text-align: center;
    }
    /* The one place on the panel a track title is the subject: 22px against
       the band's and the depth's 19 (§16). */
    .fp-title {
        margin: 0;
        font-size: 22px;
        font-weight: 600;
        letter-spacing: -0.02em;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .fp-sub {
        margin: 4px 0 0;
        font-size: 13.5px;
        color: var(--text-mute);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    /* The rail with a step either side. The steps hold their size and the
       rail takes what is left, so the record's own width still decides how
       long the scrubber is. */
    .fp-scrub {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        min-width: 0;
    }
    .fp-scrub :global(.rail-box),
    .fp-scrub :global(.rail-live) {
        flex: 1 1 auto;
        min-width: 0;
    }
    .s-step {
        flex: none;
        display: inline-flex;
        align-items: center;
        gap: 2px;
        min-width: 44px;
        height: 44px;
        padding: 0 6px;
        border: 0;
        border-radius: var(--r-sm);
        background: none;
        color: var(--text-mute);
        font-family: inherit;
        cursor: pointer;
        transition:
            color var(--t-fast),
            transform var(--t-fast);
    }
    .s-step:active {
        color: var(--on);
        transform: scale(0.94);
        transition-duration: 80ms;
    }
    .s-step:focus-visible {
        box-shadow: var(--focus-ring);
        outline: none;
    }
    /* The number of seconds is a number (§2). */
    .s-num {
        font-size: 11px;
        letter-spacing: 0.02em;
    }

    /* What you can do about the song. Centred under its name, because that
       is what they are about — the room's controls are the other column's. */
    .fp-acts {
        display: flex;
        align-items: center;
        justify-content: center;
        gap: var(--space-2);
        min-width: 0;
        flex-wrap: wrap;
    }
    .a-heart {
        display: grid;
        place-items: center;
        width: 44px;
        height: 44px;
        border: 1px solid var(--hairline);
        border-radius: var(--r-pill);
        background: var(--card-2);
        color: var(--text-mute);
        cursor: pointer;
        transition:
            color var(--t-fast),
            transform var(--t-fast);
    }
    /* Saved is the accent, not a new colour (§2): the heart is the one
       control here whose state is worth reading from across the room. */
    .a-heart.on {
        color: var(--on);
        border-color: var(--tile-on-border);
    }
    .a-heart:active {
        transform: scale(0.92);
        transition-duration: 80ms;
    }
    .a-heart:disabled {
        opacity: 0.55;
    }
    .a-chip {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        max-width: 100%;
        min-height: 44px;
        padding: 0 var(--space-4);
        border: 1px solid var(--hairline);
        border-radius: var(--r-pill);
        background: var(--card-2);
        color: var(--text-mute);
        font-family: inherit;
        font-size: 13px;
        font-weight: 500;
        cursor: pointer;
        transition: transform var(--t-fast);
    }
    .a-chip span {
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
    .a-chip:active {
        transform: scale(0.96);
        transition-duration: 80ms;
    }
    .a-chip:disabled {
        opacity: 0.55;
    }
    .a-heart:focus-visible,
    .a-chip:focus-visible {
        box-shadow: var(--focus-ring);
        outline: none;
    }

    .fp-note {
        margin: 0;
        text-align: center;
        font-size: 12.5px;
        color: var(--text-dim);
    }

    /* Portrait: the record sits over the controls and CSS sizes it from its
       width — the one direction every engine agrees on — which is the same
       shape `landscape` above switches the measuring off for. */
    @media (orientation: portrait), (max-width: 900px) {
        .fp-record {
            width: min(100%, 340px);
            margin-inline: 0;
        }
    }
</style>
