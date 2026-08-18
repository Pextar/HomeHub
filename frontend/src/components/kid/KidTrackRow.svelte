<script lang="ts">
    /**
     * The one song row for every list in the kid module (DESIGN.md §17):
     * tap plays it, the big ＋ queues it without stopping what's on.
     *
     * Queueing says so with a 🎉 rather than leaving it to a count that
     * changes three taps away — a kid can't watch that happen.
     *
     * The dense mode arrives as a prop rather than being inherited from an
     * ancestor's class: a component's styles are scoped, so the pane's
     * `.kb-open .kms-art` would no longer reach in here.
     */
    import { rowSub } from "../../lib/music/catalog";
    import { fmtMs } from "../../lib/music/format";
    import type { PanelRooms } from "../../lib/panel-music.svelte";
    import type { SpotifyItem } from "../../lib/types";

    let {
        item,
        num = null,
        music,
        kbOpen = false,
        queueOpen = false,
        flashed = false,
        onPick,
        onToggleQueue,
        onQueue,
    }: {
        item: SpotifyItem;
        /** Track number, where the list is a numbered one. */
        num?: number | null;
        music: PanelRooms;
        /** The software keyboard is up, so the row goes dense. */
        kbOpen?: boolean;
        /** This row's queue choices are showing. */
        queueOpen?: boolean;
        /** This row was the last thing queued — say so. */
        flashed?: boolean;
        onPick: (item: SpotifyItem) => void;
        onToggleQueue: (uri: string) => void;
        onQueue: (item: SpotifyItem, next: boolean) => void;
    } = $props();
</script>

<div class="kms-row" class:kb-open={kbOpen}>
    <button
        class="kms-main"
        class:starting={!!music.busy["item:" + item.uri]}
        disabled={!!music.busy["item:" + item.uri]}
        onclick={() => onPick(item)}
    >
        {#if num !== null}
            <span class="kms-num mono">{num}</span>
        {/if}
        {#if item.art_url}
            <img class="kms-art" src={item.art_url} alt="" loading="lazy" />
        {:else}
            <span class="kms-art kms-art-none" aria-hidden="true">🎵</span>
        {/if}
        <span class="kms-names">
            <span class="kms-name">{item.name}</span>
            {#if rowSub(item)}
                <span class="kms-sub">{rowSub(item)}</span>
            {/if}
        </span>
        {#if item.duration_ms}
            <span class="kms-dur mono">{fmtMs(item.duration_ms)}</span>
        {/if}
    </button>
    <button
        class="kms-plus"
        class:open={queueOpen}
        aria-label="Queue “{item.name}”"
        aria-expanded={queueOpen}
        onclick={() => onToggleQueue(item.uri)}
    >
        ＋
    </button>
</div>
{#if queueOpen}
    <div class="kms-qactions" role="group" aria-label="Queue options">
        <button class="kms-qbtn" onclick={() => onQueue(item, true)}>▶️ Play next</button>
        <button class="kms-qbtn" onclick={() => onQueue(item, false)}>➕ Add to the end</button>
    </div>
{/if}
{#if flashed}
    <p class="kms-flash" role="status">Added to the queue 🎉</p>
{/if}

<style>
    /* ── Track rows ── */
    .kms-row {
        display: flex;
        align-items: stretch;
        gap: var(--space-2);
    }
    .kms-main {
        flex: 1;
        min-width: 0;
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: var(--space-2) var(--space-3);
        min-height: 64px;
        border-radius: var(--radius-lg);
        border: 2px solid var(--border);
        background: var(--bg-elevated);
        cursor: pointer;
        text-align: left;
        transition: transform 0.12s ease, border-color 0.15s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .kms-main:active { transform: scale(0.98); border-color: var(--kid-accent); }
    .kms-main:disabled { opacity: 0.75; }
    /* A tapped song can take a moment to reach the speaker — the cover
       breathes until the player (or the mini bar) names it. */
    .kms-main.starting .kms-art {
        animation: kms-start 0.9s ease-in-out infinite;
    }
    @keyframes kms-start {
        0%, 100% { opacity: 1; }
        50% { opacity: 0.4; }
    }
    .kms-num {
        font-family: var(--font-mono);
        font-feature-settings: "tnum" 1;
        font-size: 0.95rem;
        font-weight: 700;
        color: var(--text-faint);
        width: 2ch;
        text-align: right;
        flex-shrink: 0;
    }
    .kms-art {
        width: 48px;
        height: 48px;
        border-radius: var(--radius-md);
        object-fit: cover;
        flex-shrink: 0;
    }
    .kms-art-none {
        background: var(--surface-hover);
        display: grid;
        place-items: center;
        font-size: 1.4rem;
    }
    .kms-names {
        flex: 1;
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 2px;
    }
    .kms-name {
        font-size: 1rem;
        font-weight: 800;
        color: var(--text);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .kms-sub {
        font-size: 0.82rem;
        font-weight: 600;
        color: var(--text-muted);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .kms-dur {
        font-family: var(--font-mono);
        font-feature-settings: "tnum" 1;
        font-size: 0.85rem;
        font-weight: 700;
        color: var(--text-muted);
        flex-shrink: 0;
    }
    /* On a phone the length was being paid for by the title and artist you
       choose a song by — the trade DESIGN.md §15.9 already settled for the
       app's rows, applied to the kid's. */
    @media (max-width: 480px) {
        .kms-dur { display: none; }
    }
    .kms-plus {
        width: 52px;
        min-height: 52px;
        border-radius: var(--radius-lg);
        border: 2px solid var(--border);
        background: var(--bg-elevated);
        color: var(--kid-accent);
        font-size: 1.4rem;
        font-weight: 800;
        cursor: pointer;
        flex-shrink: 0;
        align-self: center;
        transition: transform 0.12s ease, border-color 0.15s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .kms-plus:active { transform: scale(0.9); }
    .kms-plus.open { border-color: var(--kid-accent); background: var(--kid-accent-soft); }

    .kms-qactions {
        display: flex;
        flex-wrap: wrap;
        gap: var(--space-2);
        padding-left: var(--space-3);
    }
    .kms-qbtn {
        /* Both on one line where they fit; stacked full-width on a narrow
           phone rather than wrapping "Add to the end" onto two lines. */
        flex: 1 1 190px;
        font-size: 1rem;
        font-weight: 800;
        padding: 12px 16px;
        min-height: 52px;
        border-radius: var(--radius-lg);
        border: 2px solid var(--kid-accent);
        background: var(--kid-accent-soft);
        color: var(--kid-accent);
        cursor: pointer;
        -webkit-tap-highlight-color: transparent;
    }
    .kms-qbtn:active { transform: scale(0.96); }
    .kms-flash {
        font-size: 0.9rem;
        font-weight: 800;
        color: var(--kid-green);
        padding-left: var(--space-3);
    }

    /* Dense mode: the keyboard is up and the row gives back what it took.
       Scoped to the row now that it is a component. */
    .kms-row.kb-open .kms-main {
        min-height: 52px;
    }
    .kms-row.kb-open .kms-art {
        width: 36px;
        height: 36px;
    }
    .kms-row.kb-open .kms-sub,
    .kms-row.kb-open .kms-dur {
        display: none;
    }

    @media (prefers-reduced-motion: reduce) {
        .kms-main.starting .kms-art {
            animation: none;
        }
    }
</style>
