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
    import { createSoftKeyboard } from "../lib/music/keyboard.svelte";
    import { haptic } from "../lib/utils";
    import { dur, reducedMotion } from "../lib/motion";
    import KidPlayer from "../components/kid/KidPlayer.svelte";
    import KidMiniBar from "../components/kid/KidMiniBar.svelte";
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
    const queueCount = $derived(featured?.groupState?.queue_length ?? 0);

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

    // The kid's faders top out here (DESIGN.md §17) — loud enough to enjoy,
    // not enough to upset the house. A speaker already louder (a grown-up
    // turned it up) reads its real number and can only be turned down.
    const volMax = 50;

    // ── The software keyboard is part of the layout ──────────────────────
    // Measured once, here, because three things answer to it: the results go
    // dense, the sticky chips stand down (they cost 13% of what's left of a
    // phone screen), and the mini bar hides rather than sit behind the keys.
    const keyboard = createSoftKeyboard();
    const kbOpen = $derived(keyboard.open);

    // ── The mini bar: the phone's dock (§15.5's rule, kid form) ─────────
    // A fallback, never a duplicate — so what it watches is the transport
    // itself, not the whole card. The moment the buttons it copies are out
    // of reach it docks at the bottom; while any of them is on screen it
    // stays down. Tapping its text scrolls the player back.
    // The player watches its own transport and says when it leaves the
    // screen; the bar docks for exactly as long as it is gone.
    let transportInView = $state(true);
    const miniUp = $derived(!!featured && !transportInView && !kbOpen);
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
        <KidPlayer
            {music}
            {featured}
            {volMax}
            onFind={() => pickPane("search")}
            onTransportVisible={(v) => (transportInView = v)}
        />

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
        <KidMiniBar {music} {featured} onOpen={scrollToTop} />
    {/if}
</div>

<style>
    .km {
        position: fixed;
        inset: 0;
        z-index: var(--z-modal);
        overflow-y: auto;
        /* One axis (§12). The two bands that bleed back out to the edges do
           it by exactly this scroller's gutter — out to the padding box,
           which is where a hidden x axis clips — so they are untouched,
           and a row of song text can no longer pan the screen sideways by
           the pixel its rounding invents. */
        overflow-x: hidden;
        /* Its own scroll world: a flick that runs off the end of the queue
           must not drag the lamps page underneath it. */
        overscroll-behavior: contain;
        -webkit-overflow-scrolling: touch;
        background: var(--kid-bg);
        /* The screen's side gutter, published so the two bands that pin to
           the top of the scrollport — the pane chips, and the search box
           while the keyboard is up — can bleed back out to the edges
           without hardcoding it. It clears a landscape notch, and takes the
           same value on both sides so that bleed stays symmetric: a gutter
           that differed left from right would need two negative margins and
           would read as a mistake on the side without the notch. */
        --km-gutter: max(var(--space-5), env(safe-area-inset-left), env(safe-area-inset-right));
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
    @media (min-width: 700px) {
        .km { --km-gutter: max(var(--space-7), env(safe-area-inset-left), env(safe-area-inset-right)); }
        .km-pane, .km-head { max-width: 720px; margin-left: auto; margin-right: auto; }
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
        .km-skel-player, .km-skel-row { animation: none; }
    }
</style>
