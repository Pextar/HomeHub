<script lang="ts">
    /**
     * The kid music player (DESIGN.md §17): a full-screen takeover from
     * KidHome, speaking the same playful language as the lamps — big emoji,
     * fat targets, two taps for anything destructive — while driving exactly
     * the same speaker brain as the grown-up panel (lib/panel-music), with
     * Sonos as the only make a kid can reach.
     *
     * It is a phone surface and nothing else, so the whole screen is
     * budgeted against one thumb and one fold:
     *
     *   ┌──────────────────────────────┐
     *   │ ‹ Back            🔊 Room ›  │  header — scrolls away
     *   │  ┌────┐ Title                │
     *   │  │art │ artist · album       │  the player: only the things
     *   │  └────┘                      │  a finger touches often
     *   │  ────── rail ──────          │
     *   │     ⏮️   ▶️   ⏭️              │
     *   │  🔊 ──── fader ────  24      │
     *   │  [ 🎛️ More controls ▾ ]      │  modes + per-speaker faders
     *   ├──────────────────────────────┤
     *   │ [🔎 Find][🎶 Up next][🔊 Rooms] │  sticky: the only navigation
     *   │  pane content                │
     *   └──────────────────────────────┘
     *         [ mini bar, once the transport scrolls off ]
     *
     * The panes stay mounted once opened so a search in progress survives a
     * trip to the queue and back.
     */
    import { onMount, tick } from "svelte";
    import { fly } from "svelte/transition";
    import { backOut } from "svelte/easing";
    import { createPanelMusic } from "../lib/panel-music.svelte";
    import { haptic } from "../lib/utils";
    import { fmtSecs } from "../lib/music/time";
    import { repeatLabel } from "../lib/music/sonos.svelte";
    import { dur, reducedMotion } from "../lib/motion";
    import KidSlider from "../components/kid/KidSlider.svelte";
    import KidMusicSearch from "../components/kid/KidMusicSearch.svelte";
    import KidMusicQueue from "../components/kid/KidMusicQueue.svelte";
    import KidMusicRooms from "../components/kid/KidMusicRooms.svelte";

    let { onClose }: { onClose: () => void } = $props();

    const music = createPanelMusic({ sonosOnly: true });

    // The first poll decides between the skeleton and the "no speakers"
    // empty state — without it an empty house would shimmer forever.
    let booted = $state(false);
    onMount(() => {
        void music.refresh().finally(() => {
            booted = true;
        });
    });

    const featured = $derived(music.featured);
    const gs = $derived(featured?.groupState);
    const members = $derived(featured?.members ?? []);
    const multi = $derived(members.length > 1);
    const queueCount = $derived(gs?.queue_length ?? 0);
    /** Anything the disclosure would hold — no toggle when it'd open empty. */
    const hasMore = $derived(!!gs || multi);

    type Pane = "search" | "queue" | "rooms";
    let pane = $state<Pane>("search");

    let rootEl = $state<HTMLElement | null>(null);
    let panesEl = $state<HTMLElement | null>(null);

    const glide: ScrollBehavior = reducedMotion ? "auto" : "smooth";

    /** Put the pane's own top under the sticky chips — where the search box,
     *  the queue head and the rooms hint each start. */
    function scrollToPanes() {
        panesEl?.scrollIntoView({ behavior: glide, block: "start" });
    }
    /** All the way up: the player, the room and the way out. */
    function scrollToTop() {
        rootEl?.scrollTo({ top: 0, behavior: glide });
    }

    function pickPane(p: Pane) {
        haptic();
        // Tapping the chip you are already on means "take me back to the top
        // of this pane" — the phone convention, and the cheapest way back to
        // the search box from thirty results down.
        if (pane === p) {
            scrollToPanes();
            return;
        }
        pane = p;
        // After the swap: scrolling first would aim at a page whose height is
        // about to change, and a short pane would clamp the offset short.
        void tick().then(scrollToPanes);
    }

    function back() {
        haptic();
        onClose();
    }

    /** The room is named in the header, and changing it happens in the Rooms
     *  pane — which is also where rooms are joined together, so there is one
     *  place that owns rooms rather than a chip row duplicating it (§15.5). */
    function goRooms() {
        haptic();
        pane = "rooms";
        void tick().then(scrollToPanes);
    }

    const repeatText = $derived(
        gs?.repeat === "all" ? "Repeat all" : gs?.repeat === "one" ? "Repeat one" : "Repeat",
    );

    // The kid's faders top out here (DESIGN.md §17) — loud enough to enjoy,
    // not enough to upset the house. A speaker already louder (a grown-up
    // turned it up) reads its real number and can only be turned down.
    const VOL_MAX = 50;

    // Play modes and the per-speaker faders are set-and-forget, and together
    // they are half a phone screen. They fold away so the fold holds what a
    // finger actually returns to: the track, the transport, the volume.
    let showMore = $state(false);
    function toggleMore() {
        haptic();
        showMore = !showMore;
    }

    // ── The software keyboard is part of the layout ──────────────────────
    // Measured once, here, because three things answer to it: the results go
    // dense, the sticky chips stand down (they cost 13% of what's left of a
    // phone screen), and the mini bar hides rather than sit behind the keys.
    let kb = $state(0);
    const kbOpen = $derived(kb > 150);
    onMount(() => {
        const vv = window.visualViewport;
        if (!vv) return;
        const measure = () => {
            kb = Math.max(0, Math.round(window.innerHeight - vv.height - vv.offsetTop));
        };
        measure();
        vv.addEventListener("resize", measure);
        vv.addEventListener("scroll", measure);
        return () => {
            vv.removeEventListener("resize", measure);
            vv.removeEventListener("scroll", measure);
        };
    });

    // ── The mini bar: the phone's dock (§15.5's rule, kid form) ─────────
    // A fallback, never a duplicate — so what it watches is the transport
    // itself, not the whole card. The moment the buttons it copies are out
    // of reach it docks at the bottom; while any of them is on screen it
    // stays down. Tapping its text scrolls the player back.
    let transportEl = $state<HTMLElement | null>(null);
    let transportInView = $state(true);

    $effect(() => {
        const el = transportEl;
        if (!el) {
            transportInView = true;
            return;
        }
        const io = new IntersectionObserver(([entry]) => {
            transportInView = entry.isIntersecting;
        });
        io.observe(el);
        return () => io.disconnect();
    });

    const miniUp = $derived(!!featured && !transportInView && !kbOpen);

    // The seek rail: draggable when the track has a length behind it, an
    // honest "live" line when it doesn't (a radio stream has no position).
    const showRail = $derived(!!featured && (featured.playing || !!featured.trackTitle));
    /** Nothing has played here yet — the player says so and points at Find,
     *  rather than showing a transport with nothing behind it. */
    const idle = $derived(!!featured && !featured.playing && !featured.trackTitle);
</script>

<div class="km" class:has-mini={miniUp} bind:this={rootEl} in:fly={{ y: 40, duration: dur(320), easing: backOut }}>
    <header class="km-head">
        <button class="km-back" onclick={back} aria-label="Back to my lamps">‹ Back</button>
        <h2 class="km-sr">Music</h2>
        {#if featured}
            {#if music.sources.length > 1}
                <button class="km-room" onclick={goRooms}>
                    <span aria-hidden="true">🔊</span>
                    <span class="km-room-name">{featured.title}</span>
                    <span class="km-room-go" aria-hidden="true">›</span>
                    <span class="km-sr">— change room</span>
                </button>
            {:else}
                <span class="km-room km-room-static">
                    <span aria-hidden="true">🔊</span>
                    <span class="km-room-name">{featured.title}</span>
                </span>
            {/if}
        {/if}
    </header>

    {#if !booted}
        <div class="km-skel-player" aria-hidden="true"></div>
        <div class="km-skel-rows" aria-hidden="true">
            {#each Array(3) as _, i (i)}
                <div class="km-skel-row"></div>
            {/each}
        </div>
    {:else if music.sources.length === 0}
        <div class="km-none">
            <div class="km-none-emoji">🔇</div>
            <p>No speakers yet!<br />Ask a grown-up to add one.</p>
        </div>
    {:else if featured}
        <section class="km-player" class:playing={featured.playing}>
            <div class="km-now">
                <span class="km-artwrap">
                    {#if featured.art}
                        <img class="km-art" src={featured.art} alt="" loading="lazy" />
                    {:else}
                        <span class="km-art km-art-none">🎵</span>
                    {/if}
                </span>
                <span class="km-meta">
                    <span class="km-title">
                        {featured.trackTitle ?? (featured.playing ? "Playing" : "Nothing playing")}
                    </span>
                    {#if featured.trackSub}
                        <span class="km-sub">{featured.trackSub}</span>
                    {/if}
                </span>
            </div>

            {#if showRail}
                {#if music.durSec > 0}
                    <div class="km-rail">
                        <span class="km-time mono">{fmtSecs(music.posSec)}</span>
                        <KidSlider
                            value={music.posSec}
                            max={music.durSec}
                            disabled={!music.seekable}
                            label="Where in the song"
                            valueText="{fmtSecs(music.posSec)} of {fmtSecs(music.durSec)}"
                            onInput={() => {}}
                            onChange={(sec) => music.seek(sec)}
                        />
                        <span class="km-time mono">{fmtSecs(music.durSec)}</span>
                    </div>
                {:else}
                    <p class="km-live">📻 Live radio</p>
                {/if}
            {:else if idle}
                <!-- Nothing has played in this room yet, so the obvious next
                     move is the only thing offered. -->
                <button class="km-find" onclick={() => pickPane("search")}>🔎 Find something to play</button>
            {/if}

            <div class="km-transport" bind:this={transportEl}>
                <button
                    class="km-tbtn"
                    aria-label="Previous song"
                    disabled={music.busy["previous:" + featured.id]}
                    onclick={() => music.skip(featured, "previous")}
                >
                    ⏮️
                </button>
                <button
                    class="km-tbtn km-tplay"
                    aria-label={featured.playing ? "Pause" : "Play"}
                    disabled={music.busy["play:" + featured.id]}
                    onclick={() => music.togglePlay(featured)}
                >
                    {featured.playing ? "⏸️" : "▶️"}
                </button>
                <button
                    class="km-tbtn"
                    aria-label="Next song"
                    disabled={music.busy["next:" + featured.id]}
                    onclick={() => music.skip(featured, "next")}
                >
                    ⏭️
                </button>
            </div>

            <div class="km-volume">
                <button
                    class="km-vol-btn"
                    class:mute={featured.muted}
                    aria-label={featured.muted ? "Unmute" : "Mute"}
                    disabled={music.busy["mute:" + featured.id]}
                    onclick={() => music.toggleMute(featured)}
                >
                    {featured.muted ? "🔇" : "🔊"}
                </button>
                <KidSlider
                    value={Math.min(VOL_MAX, music.vol)}
                    max={VOL_MAX}
                    label="Volume"
                    valueText="{music.vol}%"
                    onInput={(v) => music.dragVolume(featured, v)}
                    onChange={(v) => music.setVolume(featured, v)}
                />
                <span class="km-vol-val mono">{music.vol}</span>
            </div>

            {#if hasMore}
                <button class="km-more" aria-expanded={showMore} onclick={toggleMore}>
                    <span aria-hidden="true">🎛️</span>
                    <span class="km-more-text">More controls</span>
                    <span class="km-more-go" class:open={showMore} aria-hidden="true">▾</span>
                </button>
            {/if}

            {#if showMore && hasMore}
                <div class="km-extra">
                    {#if gs}
                        <div class="km-modes" role="group" aria-label="Play modes">
                            <button
                                class="km-mode"
                                class:on={gs.shuffle}
                                aria-pressed={gs.shuffle}
                                disabled={music.busy["mode:" + featured.id]}
                                onclick={() => music.toggleShuffle()}
                            >
                                🔀 Shuffle
                            </button>
                            <button
                                class="km-mode"
                                class:on={gs.repeat !== "off"}
                                aria-pressed={gs.repeat !== "off"}
                                aria-label={repeatLabel(gs.repeat)}
                                disabled={music.busy["mode:" + featured.id]}
                                onclick={() => music.cycleRepeat()}
                            >
                                {gs.repeat === "one" ? "🔂" : "🔁"} {repeatText}
                            </button>
                            <button
                                class="km-mode"
                                class:on={gs.crossfade}
                                aria-pressed={gs.crossfade}
                                disabled={music.busy["xfade:" + featured.id]}
                                onclick={() => music.toggleCrossfade()}
                            >
                                ✨ Crossfade
                            </button>
                            <button
                                class="km-mode"
                                class:on={!!featured.autoplay}
                                aria-pressed={!!featured.autoplay}
                                disabled={music.busy["autoplay:" + featured.id]}
                                onclick={() => music.toggleAutoplay()}
                            >
                                🎈 Play similar
                            </button>
                        </div>
                        <p class="km-modenote">
                            {featured.autoplay
                                ? "When the songs run out, more like them keep playing 🎶"
                                : "When the songs run out, the music stops."}
                        </p>
                    {/if}

                    {#if multi}
                        <!-- One fader per speaker under the room-wide one — the
                             balance question a group always raises. -->
                        <div class="km-members">
                            {#each members as m (m.id)}
                                <div class="km-member">
                                    <button
                                        class="km-vol-btn small"
                                        class:mute={m.muted}
                                        aria-label="{m.muted ? 'Unmute' : 'Mute'} {m.name}"
                                        disabled={music.busy["mute:" + m.id]}
                                        onclick={() => music.toggleMute(featured, m.id)}
                                    >
                                        {m.muted ? "🔇" : "🔊"}
                                    </button>
                                    <span class="km-member-name">{m.name}</span>
                                    <KidSlider
                                        value={Math.min(VOL_MAX, music.memVol[m.id] ?? m.volume)}
                                        max={VOL_MAX}
                                        label="Volume {m.name}"
                                        valueText="{music.memVol[m.id] ?? m.volume}%"
                                        onInput={(v) => music.dragMemberVolume(m.id, v)}
                                        onChange={(v) => music.setMemberVolume(m.id, v)}
                                    />
                                    <span class="km-vol-val mono">{music.memVol[m.id] ?? m.volume}</span>
                                </div>
                            {/each}
                        </div>
                    {/if}
                </div>
            {/if}
        </section>

        <!-- The one piece of chrome that stays: the module's whole navigation
             in 58px, pinned so it is reachable from anywhere in a long list.
             It stands down while the software keyboard is up. -->
        <nav class="km-panes" class:kb-hidden={kbOpen} bind:this={panesEl} role="group" aria-label="Music sections">
            <button
                class="km-pane-chip"
                class:active={pane === "search"}
                aria-pressed={pane === "search"}
                onclick={() => pickPane("search")}
            >
                🔎 Find
            </button>
            <button
                class="km-pane-chip"
                class:active={pane === "queue"}
                aria-pressed={pane === "queue"}
                onclick={() => pickPane("queue")}
            >
                🎶 Up next{#if queueCount > 0}&nbsp;<span class="mono">{queueCount}</span>{/if}
            </button>
            <button
                class="km-pane-chip"
                class:active={pane === "rooms"}
                aria-pressed={pane === "rooms"}
                onclick={() => pickPane("rooms")}
            >
                🔊 Rooms <span class="mono">{music.sources.length}</span>
            </button>
        </nav>

        <!-- The panes never unmount: a search halfway typed survives a
             peek at the queue, and the rooms pane keeps its two-tap arms
             from resetting on a pane hop. -->
        <div class="km-pane" hidden={pane !== "search"}>
            <KidMusicSearch {music} {kbOpen} />
        </div>
        <div class="km-pane" hidden={pane !== "queue"}>
            <KidMusicQueue {music} onFindMusic={() => pickPane("search")} />
        </div>
        <div class="km-pane" hidden={pane !== "rooms"}>
            <KidMusicRooms {music} />
        </div>
    {/if}

    {#if miniUp && featured}
        <div class="km-mini" class:playing={featured.playing} transition:fly={{ y: 96, duration: dur(260) }}>
            <button class="km-mini-open" onclick={scrollToTop} aria-label="Show the player">
                {#if featured.art}
                    <img class="km-mini-art" src={featured.art} alt="" />
                {:else}
                    <span class="km-mini-art km-mini-art-none" aria-hidden="true">🎵</span>
                {/if}
                <span class="km-mini-meta">
                    <span class="km-mini-title">
                        {featured.trackTitle ?? (featured.playing ? "Playing" : "Nothing playing")}
                    </span>
                    <span class="km-mini-sub">{featured.trackSub || featured.title}</span>
                </span>
            </button>
            <button
                class="km-mini-btn km-mini-play"
                aria-label={featured.playing ? "Pause" : "Play"}
                disabled={music.busy["play:" + featured.id]}
                onclick={() => music.togglePlay(featured)}
            >
                {featured.playing ? "⏸️" : "▶️"}
            </button>
            <button
                class="km-mini-btn"
                aria-label="Next song"
                disabled={music.busy["next:" + featured.id]}
                onclick={() => music.skip(featured, "next")}
            >
                ⏭️
            </button>
        </div>
    {/if}
</div>

<style>
    .km {
        position: fixed;
        inset: 0;
        z-index: var(--z-modal);
        overflow-y: auto;
        /* Its own scroll world: a flick that runs off the end of the queue
           must not drag the lamps page underneath it. */
        overscroll-behavior: contain;
        -webkit-overflow-scrolling: touch;
        background: var(--kid-bg);
        /* The screen's side gutter, published so the two bands that pin to
           the top of the scrollport — the pane chips, and the search box
           while the keyboard is up — can bleed back out to the edges
           without hardcoding it. */
        --km-gutter: var(--space-5);
        /* No top padding on the scroller itself — the sticky chips pin to
           the scrollport edge, and any padding above them would be a gap
           content scrolls through. The header carries it instead. */
        padding: 0 var(--km-gutter) calc(var(--space-7) + env(safe-area-inset-bottom));
    }

    /* Screen-reader-only: the screen's name, which the layout gives to the
       room instead. */
    .km-sr {
        position: absolute;
        width: 1px;
        height: 1px;
        padding: 0;
        margin: -1px;
        overflow: hidden;
        clip-path: inset(50%);
        white-space: nowrap;
    }

    .km-head {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
        /* viewport-fit=cover is on, so a notch would otherwise eat Back. */
        padding-top: calc(var(--space-5) + env(safe-area-inset-top));
        margin-bottom: var(--space-4);
    }
    .km-back {
        font-size: 1.05rem;
        font-weight: 800;
        padding: 12px 18px;
        min-height: 48px;
        border-radius: 999px;
        border: none;
        background: var(--surface-hover);
        color: var(--text);
        cursor: pointer;
        flex-shrink: 0;
        transition: transform 0.12s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .km-back:active { transform: scale(0.93); }

    /* Where the music plays. One room in the house makes it a plain line —
       a control that can't do anything isn't one (DESIGN.md §15.5). */
    .km-room {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        min-width: 0;
        font-size: 1rem;
        font-weight: 800;
        padding: 10px 14px;
        min-height: 48px;
        border-radius: 999px;
        border: 2px solid var(--border);
        background: var(--bg-elevated);
        color: var(--text);
        cursor: pointer;
        transition: transform 0.12s ease, border-color 0.15s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .km-room:active { transform: scale(0.95); border-color: var(--kid-accent); }
    .km-room-static {
        border-color: transparent;
        background: transparent;
        color: var(--text-muted);
        cursor: default;
    }
    .km-room-name {
        min-width: 0;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .km-room-go { color: var(--text-muted); font-size: 1.2rem; flex-shrink: 0; }

    /* Skeletons while the first speaker poll lands. */
    .km-skel-player {
        height: 240px;
        border-radius: var(--radius-xl);
        background: linear-gradient(90deg, var(--surface) 0%, var(--surface-hover) 50%, var(--surface) 100%);
        background-size: 200% 100%;
        animation: shimmer 1.5s linear infinite;
        margin-bottom: var(--space-4);
    }
    .km-skel-rows { display: flex; flex-direction: column; gap: var(--space-3); }
    .km-skel-row {
        height: 76px;
        border-radius: var(--radius-lg);
        background: linear-gradient(90deg, var(--surface) 0%, var(--surface-hover) 50%, var(--surface) 100%);
        background-size: 200% 100%;
        animation: shimmer 1.5s linear infinite;
    }
    @keyframes shimmer {
        0% { background-position: 200% 0; }
        100% { background-position: -200% 0; }
    }

    .km-none {
        text-align: center;
        color: var(--text-muted);
        margin-top: 16vh;
    }
    .km-none-emoji { font-size: 4rem; margin-bottom: var(--space-3); }
    .km-none p { font-size: 1.25rem; font-weight: 700; line-height: 1.5; }

    /* ── The player card ── */
    .km-player {
        position: relative;
        background: var(--bg-elevated);
        border: 3px solid var(--border);
        border-radius: var(--radius-xl);
        padding: var(--space-4) var(--space-5);
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
        margin-bottom: var(--space-4);
        transition: border-color 0.25s ease;
    }
    .km-player.playing { border-color: var(--kid-accent); }
    /* The playing halo breathes on a pseudo-element's opacity rather than
       on the card's own box-shadow: an animated shadow repaints a card the
       size of a phone screen sixty times a second, and this doesn't. */
    .km-player::after {
        content: "";
        position: absolute;
        inset: 0;
        border-radius: inherit;
        pointer-events: none;
        opacity: 0;
        box-shadow: 0 0 0 4px var(--kid-ring), 0 12px 40px var(--kid-glow);
        transition: opacity 0.25s ease;
    }
    .km-player.playing::after {
        opacity: 1;
        animation: km-glow 2.2s ease-in-out infinite;
    }
    @keyframes km-glow {
        0%, 100% { opacity: 0.7; }
        50% { opacity: 1; }
    }

    .km-now {
        display: flex;
        align-items: center;
        gap: var(--space-4);
        min-width: 0;
    }
    .km-artwrap { flex-shrink: 0; }
    .km-art {
        width: 96px;
        height: 96px;
        border-radius: var(--radius-lg);
        object-fit: cover;
        display: block;
    }
    .km-art-none {
        background: var(--surface-hover);
        display: grid;
        place-items: center;
        font-size: 3rem;
    }
    .km-meta {
        display: flex;
        flex-direction: column;
        gap: 4px;
        min-width: 0;
    }
    .km-title {
        font-size: clamp(1.15rem, 4vw, 1.5rem);
        font-weight: 800;
        letter-spacing: -0.02em;
        color: var(--text);
        line-height: 1.15;
        /* Two lines of a long song title, then ellipsis — a phone-width
           title used to push the transport down a row on its own. */
        display: -webkit-box;
        -webkit-line-clamp: 2;
        line-clamp: 2;
        -webkit-box-orient: vertical;
        overflow: hidden;
    }
    .km-sub {
        font-size: 0.95rem;
        font-weight: 600;
        color: var(--text-muted);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    .km-rail {
        display: flex;
        align-items: center;
        gap: var(--space-3);
    }
    .km-time {
        font-family: var(--font-mono);
        font-feature-settings: "tnum" 1;
        font-size: 0.85rem;
        font-weight: 700;
        color: var(--text-muted);
        flex-shrink: 0;
        min-width: 3ch;
        text-align: center;
    }
    .km-live {
        font-size: 0.95rem;
        font-weight: 700;
        color: var(--text-muted);
        text-align: center;
    }
    .km-find {
        font-size: 1.05rem;
        font-weight: 800;
        padding: 14px 22px;
        min-height: 56px;
        border-radius: 999px;
        border: none;
        background: var(--kid-accent-grad);
        color: var(--kid-on-text);
        box-shadow: 0 0 0 4px var(--kid-ring);
        cursor: pointer;
        transition: transform 0.12s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .km-find:active { transform: scale(0.96); }

    .km-transport {
        display: flex;
        align-items: center;
        justify-content: center;
        gap: var(--space-5);
    }
    .km-tbtn {
        width: 62px;
        height: 62px;
        border-radius: 50%;
        border: 2px solid var(--border);
        background: var(--surface);
        font-size: 1.5rem;
        display: grid;
        place-items: center;
        cursor: pointer;
        transition: transform 0.12s ease, border-color 0.15s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .km-tbtn:active { transform: scale(0.9); }
    .km-tbtn:disabled { opacity: 0.5; }
    .km-tplay {
        width: 84px;
        height: 84px;
        font-size: 2.1rem;
        background: var(--kid-accent-grad);
        border-color: var(--kid-accent);
        box-shadow: 0 0 0 4px var(--kid-ring), 0 10px 30px var(--kid-glow);
    }

    /* ── The folded-away half: modes and per-speaker faders ── */
    .km-more {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        justify-content: center;
        font-size: 0.95rem;
        font-weight: 800;
        padding: 10px 16px;
        min-height: 48px;
        border-radius: 999px;
        border: 2px dashed var(--border);
        background: transparent;
        color: var(--text-muted);
        cursor: pointer;
        transition: transform 0.12s ease, border-color 0.15s ease, color 0.15s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .km-more:active { transform: scale(0.97); }
    .km-more[aria-expanded="true"] { border-style: solid; border-color: var(--kid-accent); color: var(--text); }
    .km-more-go { transition: transform 0.2s ease; }
    .km-more-go.open { transform: rotate(180deg); }
    .km-extra {
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
    }

    .km-modes {
        display: flex;
        flex-wrap: wrap;
        gap: var(--space-2);
        justify-content: center;
    }
    .km-mode {
        font-size: 0.95rem;
        font-weight: 800;
        padding: 10px 16px;
        min-height: 48px;
        border-radius: 999px;
        border: 2px solid var(--border);
        background: var(--surface);
        color: var(--text-muted);
        cursor: pointer;
        transition: transform 0.12s ease, border-color 0.15s ease, background 0.15s ease, color 0.15s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .km-mode:active { transform: scale(0.94); }
    .km-mode.on {
        background: var(--kid-accent-soft);
        border-color: var(--kid-accent);
        color: var(--kid-accent);
    }
    .km-mode:disabled { opacity: 0.5; }
    .km-modenote {
        font-size: 0.85rem;
        font-weight: 600;
        color: var(--text-faint);
        text-align: center;
    }

    .km-volume,
    .km-member {
        display: flex;
        align-items: center;
        gap: var(--space-3);
    }
    .km-vol-btn {
        width: 52px;
        height: 52px;
        border-radius: 50%;
        border: 2px solid var(--border);
        background: var(--surface);
        font-size: 1.3rem;
        display: grid;
        place-items: center;
        cursor: pointer;
        flex-shrink: 0;
        transition: transform 0.12s ease, border-color 0.15s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .km-vol-btn.small { width: 46px; height: 46px; font-size: 1.1rem; }
    .km-vol-btn:active { transform: scale(0.9); }
    .km-vol-btn.mute { border-color: var(--kid-pink); }
    .km-member-name {
        width: 88px;
        flex-shrink: 0;
        font-size: 0.9rem;
        font-weight: 800;
        color: var(--text-muted);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .km-members {
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
        border-top: 2px dashed var(--border);
        padding-top: var(--space-3);
    }
    .km-vol-val {
        font-family: var(--font-mono);
        font-feature-settings: "tnum" 1;
        font-size: 0.95rem;
        font-weight: 700;
        color: var(--text-muted);
        min-width: 3ch;
        text-align: right;
        flex-shrink: 0;
    }

    /* ── Pane chips — pinned, because they are the only way between the
         three panes and a results list is longer than a phone ── */
    .km-panes {
        position: sticky;
        top: 0;
        z-index: var(--z-sticky);
        display: flex;
        gap: var(--space-2);
        /* Bleed to the screen edges so nothing scrolls past in the gutters,
           and pin flush to the scrollport with no gap above. */
        margin: 0 calc(-1 * var(--km-gutter)) var(--space-4);
        padding: var(--space-3) var(--km-gutter);
        background: var(--dock-fill);
        backdrop-filter: blur(20px) saturate(1.6);
        -webkit-backdrop-filter: blur(20px) saturate(1.6);
        border-bottom: 1px solid var(--dock-edge);
        /* Content-sized chips that scroll sideways on a narrow phone rather
           than wrapping to two lines, and stretch to fill on wider screens. */
        overflow-x: auto;
        scrollbar-width: none;
    }
    .km-panes::-webkit-scrollbar { display: none; }
    /* Typing: the keyboard has taken half the screen, and the pane you want
       is the one you are already in. */
    .km-panes.kb-hidden { display: none; }
    .km-pane-chip {
        flex: 1 0 auto;
        white-space: nowrap;
        font-size: 1rem;
        font-weight: 800;
        padding: 12px 16px;
        min-height: 54px;
        border-radius: 999px;
        border: 2px solid var(--border);
        background: var(--bg-elevated);
        color: var(--text-muted);
        cursor: pointer;
        transition: transform 0.12s ease, border-color 0.15s ease, background 0.15s ease, color 0.15s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .km-pane-chip:active { transform: scale(0.95); }
    .km-pane-chip.active {
        background: var(--kid-accent-grad);
        border-color: var(--kid-accent);
        color: var(--kid-on-text);
        box-shadow: 0 0 0 3px var(--kid-ring);
    }
    .km-pane[hidden] { display: none; }

    /* ── The mini bar — the phone's now-playing dock ── */
    .km.has-mini {
        padding-bottom: calc(148px + env(safe-area-inset-bottom));
    }
    .km-mini {
        position: fixed;
        z-index: 10;
        left: var(--space-3);
        right: var(--space-3);
        bottom: calc(var(--space-3) + env(safe-area-inset-bottom));
        display: flex;
        align-items: center;
        gap: var(--space-2);
        padding: var(--space-2);
        border-radius: 999px;
        border: 3px solid var(--border);
        background: var(--bg-elevated);
        box-shadow: 0 12px 40px rgba(0, 0, 0, 0.45);
    }
    .km-mini.playing {
        border-color: var(--kid-accent);
        box-shadow: 0 0 0 4px var(--kid-ring), 0 12px 40px var(--kid-glow);
    }
    .km-mini-open {
        flex: 1;
        min-width: 0;
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: 4px;
        border: none;
        border-radius: 999px;
        background: transparent;
        cursor: pointer;
        text-align: left;
        -webkit-tap-highlight-color: transparent;
    }
    .km-mini-art {
        width: 52px;
        height: 52px;
        border-radius: 50%;
        object-fit: cover;
        flex-shrink: 0;
    }
    .km-mini-art-none {
        background: var(--surface-hover);
        display: grid;
        place-items: center;
        font-size: 1.5rem;
    }
    .km-mini-meta {
        display: flex;
        flex-direction: column;
        gap: 1px;
        min-width: 0;
    }
    .km-mini-title {
        font-size: 0.98rem;
        font-weight: 800;
        color: var(--text);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .km-mini-sub {
        font-size: 0.78rem;
        font-weight: 600;
        color: var(--text-muted);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .km-mini-btn {
        width: 54px;
        height: 54px;
        border-radius: 50%;
        border: 2px solid var(--border);
        background: var(--surface);
        font-size: 1.3rem;
        display: grid;
        place-items: center;
        cursor: pointer;
        flex-shrink: 0;
        transition: transform 0.12s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .km-mini-btn:active { transform: scale(0.88); }
    .km-mini-btn:disabled { opacity: 0.5; }
    .km-mini.playing .km-mini-play {
        background: var(--kid-accent-grad);
        border-color: var(--kid-accent);
    }

    @media (min-width: 700px) {
        .km { --km-gutter: var(--space-7); }
        .km-player, .km-pane, .km-head { max-width: 720px; margin-left: auto; margin-right: auto; }
        .km-panes {
            max-width: 720px;
            margin-left: auto;
            margin-right: auto;
            padding-left: 0;
            padding-right: 0;
            border-bottom: none;
        }
    }

    @media (prefers-reduced-motion: reduce) {
        .km-player.playing::after { animation: none; }
        .km-skel-player, .km-skel-row { animation: none; }
        .km-more-go { transition: none; }
    }
</style>
