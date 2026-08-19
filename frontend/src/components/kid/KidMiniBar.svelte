<script lang="ts">
    /**
     * The kid module's dock (DESIGN.md §15.5's rule, kid form): what's on and
     * the two controls worth reaching for, once the player's own transport
     * has scrolled out of reach.
     *
     * A fallback, never a duplicate. It docks exactly when the buttons it
     * copies leave the screen — which is why KidPlayer watches its own
     * transport and reports it, rather than this measuring the whole card —
     * and it stands down entirely while the software keyboard is up, rather
     * than sitting on top of the keys.
     *
     * Tapping the title scrolls the player back rather than opening anything:
     * there is no second screen here to open.
     */
    import { fly } from "svelte/transition";
    import { dur } from "../../lib/motion";
    import type { PanelRooms, PanelSource, PanelTransport } from "../../lib/panel-music/types";

    let {
        music,
        featured,
        /** Take me back to the player. */
        onOpen,
    }: {
        music: PanelRooms & PanelTransport;
        featured: PanelSource;
        onOpen: () => void;
    } = $props();
</script>

<div class="km-mini" class:playing={featured.playing} transition:fly={{ y: 96, duration: dur(260) }}>
    <button class="km-mini-open" onclick={onOpen} aria-label="Show the player">
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

<style>
    .km-mini {
        position: fixed;
        z-index: 10;
        /* Fixed, so it answers to the insets itself — and asymmetry is fine
           here: nothing measures against it. */
        left: max(var(--space-3), env(safe-area-inset-left));
        right: max(var(--space-3), env(safe-area-inset-right));
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
</style>
