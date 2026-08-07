<script lang="ts">
    /**
     * The panel's third depth: the player with the whole screen
     * (`#/panel?music=1&player=1`, DESIGN.md §16).
     *
     * The dashboard band answers "what's on" while you walk past. The music
     * depth answers "put something on". This one answers **"I am
     * listening"** — you have already started a record and the wall has
     * nothing left to browse for, so the cover goes to the size a record
     * deserves and the space the search results were using goes to what is
     * coming next.
     *
     * It is a screen, not a sheet: a kiosk has no sheets (§16), so it
     * arrives on its own route with a back chip, Escape climbs one level to
     * the depth, and falling asleep drops the whole ladder to the ambient
     * face the same way the depth does.
     */
    import { onMount } from "svelte";
    import { fade } from "svelte/transition";
    import Icon from "../Icon.svelte";
    import Waveform from "../music/Waveform.svelte";
    import Slider from "../music/Slider.svelte";
    import TrackRail from "../music/TrackRail.svelte";
    import QueuePane from "../music/QueuePane.svelte";
    import PanelGroupPane from "./PanelGroupPane.svelte";
    import { kefSourceLabel } from "../../lib/kef";
    import { dur } from "../../lib/motion";
    import { route } from "../../lib/stores.svelte";
    import type { PanelMusicStore } from "../../lib/panel-music.svelte";

    let {
        music,
        onBack,
        onClose,
    }: {
        music: PanelMusicStore;
        /** One rung down the ladder, to the music depth. */
        onBack: () => void;
        /** All the way home, to the dashboard depth. The two are different
         *  answers to different questions — "back to what I was browsing"
         *  and "I'm done listening" — and on a wall the second one is worth
         *  its own target rather than two aims at the first. */
        onClose: () => void;
    } = $props();

    const featured = $derived(music.featured);
    const gs = $derived(featured?.groupState);
    const queueCount = $derived(gs?.queue_length ?? 0);
    /** Only a Sonos group has a queue to give the leftover height to. */
    const hasQueue = $derived(featured?.kind === "sonos");

    /** How far a step moves. Fifteen seconds is the "I missed that" unit —
     *  long enough to be worth a tap, short enough that two taps aren't a
     *  different song. */
    const SEEK_STEP = 15;

    /** The heart renders only where there is something to save: a Spotify
     *  track (radio and line-in carry no catalog id) on a login that may
     *  write to the library. */
    const canSave = $derived(!!featured?.trackURI && music.canSave);

    /** Opening the artist is the depth's job, so it climbs one rung rather
     *  than stacking a screen on a screen. The name is the handle — a
     *  speaker reports what it is playing in words, not in catalog ids —
     *  and the depth resolves it to a page. */
    function openArtist() {
        const name = featured?.trackArtist;
        if (!name) return;
        route.go("panel", { music: "1", artist: name });
    }

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
    let artFailed = $state<string | null>(null);
    const artSrc = $derived(featured?.art && featured.art !== artFailed ? featured.art : null);

    // ── The cover's size, stated in pixels ──────────────────────────────
    // Height is the scarce axis on a wall, so the square is sized from the
    // height it is allowed — and the reading is a measurement, never a
    // chain of ratios (§16). Saying it in CSS meant an `aspect-ratio` on a
    // box whose width was itself shrink-to-fit, so the width came back from
    // the *caption* under the cover and clamped a 700px-tall square to the
    // length of an artist's name: every record on a wide panel rendered as
    // a vertical strip of its own middle. The screen whose entire point is
    // the cover is not the layout to be clever in front of.
    //
    // So: measure the boxes the cover cannot itself size — the *stage*
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

    let stageW = $state(0);
    let stageH = $state(0);
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

    // ── Where the sound goes, and changing it here ──────────────────────
    // This screen never used to name the room it was driving — the chips in
    // the header did, three feet above the transport, and nothing on the
    // card said "and the kitchen too". So the card leads with the
    // destination line, and the line is the door: tapping it swaps the
    // lower half for the grouping pane, which is the one place on the wall
    // where "put this in the kitchen as well" is one tap from the record
    // you are listening to.
    //
    // A swap, not a second screen and certainly not a sheet (§16 has none):
    // the transport and the room fader stay exactly where they were, so
    // pausing mid-thought costs nothing, and the queue comes back when the
    // pane closes. The play modes go with it — grouping is a two-second
    // job and the preferences underneath it are not what anyone is reading
    // while they do it.
    let grouping = $state(false);
    const memberCount = $derived(featured?.members?.length ?? 0);
    // Held against the store rather than the flag alone: splitting the last
    // pair in a one-room house leaves nothing to group, and a pane whose
    // own opener has just vanished is a pane with no way out.
    const groupOpen = $derived(grouping && music.canGroup);

    // A different room featured is a different question; the pane closes
    // rather than re-pointing itself at a group nobody was looking at.
    let groupingFor = "";
    $effect(() => {
        const key = featured?.key ?? "";
        if (key === groupingFor) return;
        groupingFor = key;
        grouping = false;
    });

    // ── The queue arrives already scrolled to what is playing ───────────
    // "The queue, in full, with the row playing marked" is what this screen
    // is for beyond size — and a room forty tracks into a playlist opened
    // on track one, with the mark somewhere below the fold. Nudge only when
    // the row is actually out of view, and only when it changes: this runs
    // beside a five-second poll on an A8X.
    let queueEl = $state<HTMLElement | undefined>();
    let scrolledTo = "";
    $effect(() => {
        const el = queueEl;
        const key = `${featured?.id ?? ""}:${featured?.queueTrack ?? 0}:${music.queue.length}`;
        if (!el || !featured?.queueTrack || key === scrolledTo) return;
        scrolledTo = key;
        requestAnimationFrame(() => {
            const row = el.querySelector<HTMLElement>(".q-row.current");
            if (!row) return;
            const r = row.getBoundingClientRect();
            const box = el.getBoundingClientRect();
            if (r.top >= box.top && r.bottom <= box.bottom) return;
            el.scrollTop += r.top - box.top - 12;
        });
    });

    // Escape climbs one level, the same ladder the back chip walks (§15.6)
    // — and the grouping pane is a rung of it, so the first Escape puts the
    // queue back rather than leaving the screen out from under someone who
    // only meant to close the pane.
    onMount(() => {
        const onKey = (e: KeyboardEvent) => {
            if (e.key !== "Escape") return;
            if (groupOpen) {
                grouping = false;
                return;
            }
            onBack();
        };
        window.addEventListener("keydown", onKey);
        return () => window.removeEventListener("keydown", onKey);
    });
</script>

<div class="fp" in:fade={{ duration: dur(160) }}>
    <!-- The same 72px band the depth wears, and all this one has to carry is
         which screen you are on and the two ways off it: back climbs one rung
         to the music depth, the × drops the whole ladder to the dashboard.
         The room chips are not here — this screen is one room, and the
         control column below names it and is the door to changing it. -->
    <header class="fp-head">
        <button class="back" onclick={onBack} aria-label="Back to music">
            <Icon name="chevronLeft" size={18} />
        </button>
        <h2 class="fp-where-am-i">Now playing</h2>
        <button class="back fp-close" onclick={onClose} aria-label="Close the player">
            <Icon name="close" size={18} />
        </button>
    </header>

    {#if featured}
        <!-- The body pads; the stage inside it is what the two columns
             actually have, and what the record is measured against. Two
             elements because one can't be both: `clientHeight` on a padded
             box counts the padding as room. -->
        <div class="fp-body">
            <div class="fp-stage" bind:clientWidth={stageW} bind:clientHeight={stageH}>
                <!-- The record: the cover at the size a wall can read from the
                     sofa, with the name of what is on it directly underneath —
                     the two together are one object, and putting the name in the
                     controls column left this side of the screen half empty.
                     The column is exactly as wide as the square, so the caption
                     truncates against the cover rather than the cover against
                     the caption. -->
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
                                    <button class="a-chip" onclick={openArtist}>
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
                <section
                    class="fp-side"
                    class:hollow={(!hasQueue && !groupOpen) || featured.standby}
                    aria-label="Player controls"
                >
                    <!-- The destination, and the way to change it. It shows up
                         only where it can do something: a Sonos room with
                         somewhere to join from or more than one speaker to
                         split. A lone KEF has nothing to group with and gets no
                         control that would be refused (§15.1). -->
                    {#if music.canGroup}
                        <button
                            class="fp-where"
                            class:open={groupOpen}
                            aria-expanded={groupOpen}
                            onclick={() => (grouping = !grouping)}
                        >
                            <span class="w-title">{featured.title}</span>
                            <span class="w-tail">
                                {#if memberCount > 1}
                                    <span class="w-count mono">{memberCount} spkrs</span>
                                {/if}
                                <span class="w-go" aria-hidden="true"
                                    ><Icon name="chevronRight" size={16} /></span
                                >
                            </span>
                        </button>
                    {:else}
                        <!-- Nothing to group with — a lone KEF, a zone — so the
                             row is a statement rather than a control, and the
                             screen still names the room it is driving. -->
                        <p class="fp-where flat">
                            <span class="w-title">{featured.title}</span>
                        </p>
                    {/if}

                    {#if featured.standby}
                        <div class="fp-standby">
                            <p>In standby</p>
                            <button
                                class="fp-wake"
                                disabled={music.busy["power:" + featured.id]}
                                onclick={() => music.wake(featured)}
                            >
                                <Icon name="power" size={18} /><span>Wake {featured.title}</span>
                            </button>
                        </div>
                    {:else}
                        <div class="fp-controls">
                            <div class="fp-transport">
                                {#if featured.canSkip}
                                    <button
                                        class="t-btn"
                                        aria-label="Previous track"
                                        disabled={music.busy["previous:" + featured.id]}
                                        onclick={() => music.skip(featured, "previous")}
                                    >
                                        <Icon name="skipPrev" size={22} />
                                    </button>
                                {/if}
                                <button
                                    class="t-btn primary"
                                    aria-label={featured.playing ? "Pause" : "Play"}
                                    disabled={music.busy["play:" + featured.id]}
                                    onclick={() => music.togglePlay(featured)}
                                >
                                    <Icon name={featured.playing ? "pause" : "play"} size={28} />
                                </button>
                                {#if featured.canSkip}
                                    <button
                                        class="t-btn"
                                        aria-label="Next track"
                                        disabled={music.busy["next:" + featured.id]}
                                        onclick={() => music.skip(featured, "next")}
                                    >
                                        <Icon name="skipNext" size={22} />
                                    </button>
                                {/if}
                            </div>

                            <div class="fp-volume">
                                <button
                                    class="v-ico"
                                    class:mute={featured.muted}
                                    aria-label={featured.muted ? "Unmute" : "Mute"}
                                    disabled={music.busy["mute:" + featured.id]}
                                    onclick={() => music.toggleMute(featured)}
                                >
                                    <Icon
                                        name={featured.muted ? "volumeOff" : "volume"}
                                        size={18}
                                    />
                                </button>
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
                        </div>

                        <!-- The rule that says the transport is one thing and the
                             queue under it is another. It is the only edge drawn
                             in this column: the column itself is flat (§16). -->
                        <div class="fp-rule"></div>

                        <!-- What this screen is for beyond size: the queue, in
                             full, with the row playing marked — the one thing
                             the depth's column never had the height to show. A
                             room with no queue behind it says so in one quiet
                             line and gives the height back to the transport
                             rather than holding open a scroll region with a
                             sentence at the top of it. -->
                        {#if groupOpen}
                            <PanelGroupPane {music} />
                        {:else if hasQueue}
                            <div class="fp-queue" bind:this={queueEl}>
                                <QueuePane
                                    art
                                    items={music.queue}
                                    loading={music.queueLoading}
                                    total={queueCount || music.queue.length}
                                    currentTrack={featured.queueTrack}
                                    playing={featured.playing}
                                    confirmClear
                                    clearBusy={!!music.busy["qclear:" + featured.id]}
                                    isBusy={(k) => !!music.busy[k]}
                                    reorder
                                    onJump={(t) => music.jumpTo(t)}
                                    onRemove={(t) => music.removeQueued(t)}
                                    onMove={(t, dir) => music.moveQueued(t, dir)}
                                    onClear={() => music.clearQueue()}
                                />
                            </div>
                        {:else}
                            <p class="fp-noqueue">
                                {featured.kind === "kef"
                                    ? "A KEF speaker plays its input — there is no queue to show."
                                    : "This room is played together — its queue lives with whatever is streaming to it."}
                            </p>
                        {/if}
                    {/if}
                </section>
            </div>
        </div>
    {:else}
        <div class="fp-nosrc">
            <Icon name="speaker" size={28} />
            <p>No speakers reachable</p>
        </div>
    {/if}
</div>

<style>
    .fp {
        grid-row: 1 / -1;
        display: flex;
        flex-direction: column;
        min-height: 0;
        min-width: 0;
        /* Edge to edge, like the dashboard's bands and the depth: the header
           draws its own rule and the body carries its own padding, because a
           wall panel has no page around it to show (§16). */
    }

    /* The same 72px band the music depth wears, so the two depths' chrome
       lands in the same place as you climb. */
    .fp-head {
        height: 72px;
        flex-shrink: 0;
        display: flex;
        align-items: center;
        gap: var(--space-4);
        min-width: 0;
        padding: 0 var(--space-8);
        border-bottom: 1px solid var(--hairline);
    }
    .fp-where-am-i {
        margin: 0;
        font-size: 14px;
        font-weight: 600;
        color: var(--text-mute);
    }
    .back {
        width: 40px;
        height: 40px;
        flex-shrink: 0;
        display: grid;
        place-items: center;
        border-radius: 50%;
        border: 1px solid var(--hairline);
        background: var(--card-2);
        color: var(--text);
        cursor: pointer;
        transition:
            background var(--t-fast),
            color var(--t-fast),
            transform var(--t-fast);
    }
    .back:active {
        transform: scale(0.94);
        transition-duration: 80ms;
    }
    .fp-close {
        margin-left: auto;
    }

    /* Two columns, neither of which grows past what it can use: the controls
       hold a stated 380px and the record stops at its cap, and the slack goes
       to the margins — the record centres in whatever the controls leave (see
       the auto margins below). A fader stretched across a desk monitor is a
       worse aim than one at arm's length on the iPad this is drawn for. */
    .fp-body {
        flex: 1;
        min-height: 0;
        display: flex;
        padding: var(--space-7) var(--space-8);
    }
    .fp-stage {
        flex: 1;
        min-width: 0;
        min-height: 0;
        display: flex;
        gap: var(--space-8); /* COL_GAP */
    }

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

    /* ── The controls, and what's next ───────────────────────────────── */
    /* Flat, and without a card of its own: this column is already a region
       of a surface that draws its own edges, and §15.2's fill is what a hero
       or a room card wears to say *this one* is making noise among others
       that aren't. Here there is one room and nothing to distinguish it
       from, the record beside it carries the waveform, and the one thing on
       screen that still needs the ON gradient is the queue row that is
       playing — which it cannot have against a card wearing the same
       gradient. The app's own full player is flat for the same reason. */
    .fp-side {
        flex: 0 0 380px; /* SIDE_W */
        overflow: hidden;
        min-height: 0;
        display: flex;
        flex-direction: column;
        gap: var(--space-5);
    }
    /* Nothing below the transport to grow into — a room without a queue, or
       one asleep. Then the column is as tall as what it has to say and rides
       beside the record rather than stretching to the screen: a KEF's
       controls held open to a full-height panel were four rows at the top
       of an empty rectangle, which reads as something that failed to load
       rather than as a speaker with no queue. */
    .fp-side.hollow {
        align-self: center;
    }
    .fp-rule {
        flex: none;
        height: 1px;
        background: var(--hairline);
    }

    /* The destination line: a full-width target because it is the one
       control on this card whose job is to be found from across the room,
       and quiet because it is a statement first and a button second. */
    .fp-where {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-2);
        width: 100%;
        min-height: 52px;
        margin: 0;
        flex: none;
        padding: 0 var(--space-4);
        border-radius: var(--r-pill);
        border: 1px solid var(--hairline);
        background: var(--card-2);
        color: var(--text);
        font-family: inherit;
        text-align: left;
        cursor: pointer;
        transition:
            background var(--t-fast),
            border-color var(--t-fast),
            transform var(--t-fast);
    }
    /* Not a control where there is nothing to group with (§15.1) — then the
       row only states which room the transport is driving. */
    .fp-where.flat {
        cursor: default;
    }
    .fp-where:not(.flat):active {
        transform: scale(0.99);
        transition-duration: 80ms;
    }
    /* Open, it reads as the head of the pane below it rather than as one
       more control floating over the queue. */
    .fp-where.open {
        border-color: var(--border-strong);
        background: var(--surface);
    }
    .w-title {
        min-width: 0;
        font-size: 14px;
        font-weight: 600;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .w-tail {
        display: inline-flex;
        align-items: center;
        gap: var(--space-2);
        flex-shrink: 0;
    }
    .w-count {
        font-size: 11.5px;
        font-weight: 500;
        color: var(--text-dim);
    }
    /* The chevron is the whole affordance: the row opens the grouping pane
       in place of the queue, and turns a quarter to say the pane is what is
       under it now. Transform only, which is all an A8X should be asked for
       (§16). */
    .w-go {
        display: inline-flex;
        color: var(--text-dim);
        transition: transform var(--t-med);
    }
    .fp-where.open .w-go {
        transform: rotate(90deg);
        color: var(--on);
    }

    .fp-controls {
        flex: none;
        display: flex;
        flex-direction: column;
        gap: var(--space-5);
    }

    /* Transport, one size up from the band's and the depth's: this is the
       one screen a transport is the subject of, and the one you are furthest
       from (§16). */
    .fp-transport {
        display: flex;
        align-items: center;
        justify-content: center;
        gap: var(--space-4);
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

    .fp-volume {
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
        transition: transform var(--t-fast);
    }
    .v-step:active {
        transform: scale(0.92);
        transition-duration: 80ms;
    }
    .v-ico:disabled,
    .v-step:disabled {
        opacity: 0.5;
    }
    .v-val {
        font-size: 14px;
        color: var(--text-mute);
        width: 3ch;
        text-align: right;
        flex-shrink: 0;
    }

    /* Every control on the wall answers a keyboard too — the panel is
       reached from a desk browser as often as from the iPad it is drawn
       for, and a focus ring is the only thing that says where a tap would
       land there. */
    .back:focus-visible,
    .fp-where:focus-visible,
    .t-btn:focus-visible,
    .v-ico:focus-visible,
    .v-step:focus-visible,
    .fp-wake:focus-visible {
        outline: none;
        box-shadow: var(--focus-ring);
    }

    /* The queue takes whatever the controls leave and owns its own scroll —
       the page never does (§16). */
    .fp-queue {
        flex: 1 1 auto;
        min-height: 0;
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
        overflow-y: auto;
        /* One axis, stated. `overflow-y: auto` alone computes the *other*
           axis to `auto` as well, so a row whose content lands on a
           fractional pixel — a queue row is a flex line of text, and text
           measures at 25.97px as readily as at 26 — makes the pane
           scrollable sideways by the 1px that rounding adds. Nothing is
           actually too wide: every row's box ends exactly where the pane's
           does. But the wall doesn't know that, and it wobbles under a
           finger. `.s-results` in the depth's search column has said this
           for the same reason. */
        overflow-x: hidden;
    }
    /* How long the queue is, and Clear, stay put while it scrolls: the pane
       opens at the track playing (see the effect above), so a bar that
       scrolled with the list would start out of sight — and Clear is the
       one control here you must never have to hunt for. The top padding
       rides on the bar rather than on the scrollport, or rows would show
       through the gap above it. */
    .fp-queue :global(.q-bar) {
        position: sticky;
        top: 0;
        z-index: 1;
        padding-bottom: var(--space-2);
        /* The column is flat now, so the bar hides the rows behind it
           against the panel's own surface rather than a card's. */
        background: var(--bg);
    }
    .fp-noqueue {
        margin: 0;
        flex: none;
        font-size: 13px;
        line-height: 1.5;
        text-align: center;
        color: var(--text-dim);
    }

    .fp-standby {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: var(--space-4);
    }
    .fp-standby p {
        margin: 0;
        font-size: 14px;
        color: var(--text-dim);
    }
    .fp-wake {
        display: inline-flex;
        align-items: center;
        gap: 8px;
        min-height: 52px;
        padding: 0 var(--space-5);
        border-radius: var(--r-pill);
        border: 1px solid var(--border-strong);
        background: var(--card-2);
        color: var(--text);
        font-family: inherit;
        font-size: 15px;
        font-weight: 500;
        cursor: pointer;
    }

    .fp-nosrc {
        flex: 1;
        display: flex;
        flex-direction: column;
        align-items: center;
        justify-content: center;
        gap: var(--space-3);
        border: 1px dashed var(--border);
        border-radius: var(--r-lg);
        color: var(--text-dim);
        font-size: 14px;
    }
    .fp-nosrc p {
        margin: 0;
    }

    /* Every icon-only control on the wall clears the §2 floor: the header's
       two chips are drawn at 40 and grow to it on a touch screen. */
    @media (pointer: coarse) {
        .back {
            width: 44px;
            height: 44px;
        }
    }

    /* The reference wall is a 1024×768 iPad (§16), where the controls
       column lands within a few pixels of the height it has. Tightening the
       spacing there is what keeps the cover off its floor; nothing that is
       a touch target shrinks. */
    @media (max-height: 820px) and (orientation: landscape) {
        .fp-body {
            padding: var(--space-5) var(--space-6);
        }
        .fp-side,
        .fp-controls {
            gap: var(--space-4);
        }
        /* The destination row gives back the pixels it can; it stays a 44px
           target, which is the floor and not a preference. */
        .fp-where {
            min-height: 44px;
        }
    }

    /* Portrait: the record over the controls, and the page scrolls. */
    @media (orientation: portrait), (max-width: 900px) {
        .fp-body {
            padding: var(--space-5);
        }
        .fp-stage {
            flex-direction: column;
            align-items: center;
        }
        .fp-record {
            width: min(100%, 340px);
            margin-inline: 0;
        }
        .fp-side {
            flex: none;
            width: 100%;
            max-width: 560px;
            overflow: visible;
        }
        .fp-queue {
            flex: none;
            overflow: visible;
        }
    }
</style>
