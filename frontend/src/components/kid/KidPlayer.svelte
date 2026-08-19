<script lang="ts">
    /**
     * The kid player's card (DESIGN.md §17): what is on, where it is, and the
     * four things a finger touches often — skip back, play, skip on, volume.
     *
     * The fold is the whole argument for this card's shape. Everything above
     * the "More controls" line is what someone comes back to; everything
     * behind it is set once and forgotten (KidMoreControls). The disclosure
     * isn't drawn at all when there is nothing to put behind it, rather than
     * opening on an empty box.
     *
     * The transport's visibility is watched here rather than by the view,
     * because the element being watched is this component's: the mini bar
     * docks exactly when these buttons go out of reach, and a fallback that
     * measured something else would either double the controls or leave a gap
     * with none.
     */
    import KidSlider from "./KidSlider.svelte";
    import KidVolumeRow from "./KidVolumeRow.svelte";
    import KidMoreControls from "./KidMoreControls.svelte";
    import { fmtSecs } from "../../lib/music/time";
    import type {
        PanelPosition,
        PanelRooms,
        PanelSource,
        PanelTransport,
        PanelVolume,
    } from "../../lib/panel-music/types";

    let {
        music,
        featured,
        /** The kid's ceiling — see KidVolumeRow. */
        volMax,
        /** Nothing has played here yet: point at the one useful next move. */
        onFind,
        /** The transport moved in or out of view — what the mini bar answers
         *  to. Reported rather than exposed as an element, so the rule about
         *  *which* element decides stays with the element. */
        onTransportVisible,
    }: {
        music: PanelRooms & PanelTransport & PanelPosition & PanelVolume;
        featured: PanelSource;
        volMax: number;
        onFind: () => void;
        onTransportVisible: (visible: boolean) => void;
    } = $props();

    const gs = $derived(featured.groupState);
    const multi = $derived((featured.members ?? []).length > 1);
    /** Anything the disclosure would hold — no toggle when it'd open empty. */
    const hasMore = $derived(!!gs || multi);

    let showMore = $state(false);

    // The seek rail: draggable when the track has a length behind it, an
    // honest "live" line when it doesn't (a radio stream has no position).
    const showRail = $derived(featured.playing || !!featured.trackTitle);
    /** Nothing has played here yet — say so and point at Find, rather than
     *  showing a transport with nothing behind it. */
    const idle = $derived(!featured.playing && !featured.trackTitle);

    let transportEl = $state<HTMLElement | null>(null);
    $effect(() => {
        const el = transportEl;
        if (!el) {
            onTransportVisible(true);
            return;
        }
        const io = new IntersectionObserver(([entry]) => onTransportVisible(entry.isIntersecting));
        io.observe(el);
        return () => io.disconnect();
    });
</script>

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
        <button class="km-find" onclick={onFind}>🔎 Find something to play</button>
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

    <KidVolumeRow
        value={Math.min(volMax, music.vol)}
        max={volMax}
        readout={music.vol}
        label="Volume"
        muted={featured.muted}
        muteBusy={music.busy["mute:" + featured.id]}
        muteLabel={featured.muted ? "Unmute" : "Mute"}
        onMute={() => music.toggleMute(featured)}
        onInput={(v) => music.dragVolume(featured, v)}
        onChange={(v) => music.setVolume(featured, v)}
    />

    {#if hasMore}
        <button class="km-more" aria-expanded={showMore} onclick={() => (showMore = !showMore)}>
            <span aria-hidden="true">🎛️</span>
            <span class="km-more-text">More controls</span>
            <span class="km-more-go" class:open={showMore} aria-hidden="true">▾</span>
        </button>
        {#if showMore}
            <KidMoreControls {music} {featured} {volMax} />
        {/if}
    {/if}
</section>

<style>
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

    /* ── The line the folded-away half hangs from ── */
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

    @media (min-width: 700px) {
        .km-player { max-width: 720px; margin-left: auto; margin-right: auto; }
    }

    @media (prefers-reduced-motion: reduce) {
        .km-player.playing::after { animation: none; }
        .km-more-go { transition: none; }
    }
</style>
