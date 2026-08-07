<script lang="ts">
    /**
     * The queue, as the player's second pane — reached from an "Up next" row
     * that names the actual next track, not as a segmented control: §2 has no
     * exception left to lean on.
     *
     * Rows show a mono track number, replaced by the §6.8 waveform on the one
     * playing, and that row takes the `.tile.on` surface. Tapping a row jumps
     * to it, the trailing X removes it, and "Clear" is destructive enough that
     * the confirm lives with the caller.
     */
    import Icon from "../Icon.svelte";
    import Waveform from "./Waveform.svelte";
    import { trimClock } from "../../lib/music/time";
    import type { SonosQueueItem } from "../../lib/types";

    let {
        items,
        loading = false,
        /** The whole queue's length, which can exceed what was fetched. */
        total,
        /** Queue position of the track playing, if any. */
        currentTrack = undefined,
        /** False while the coordinator is playing something with no queue. */
        playing = false,
        clearBusy = false,
        /** True where no confirm dialog exists (the kiosk panel): the first
         *  tap arms the button for a few seconds, the second clears. */
        confirmClear = false,
        /** Cover art on every row. Off by default — in a phone-width sheet
         *  a thumbnail is bought from the title — and on where the queue is
         *  the point of the screen and has the width for it (the panel's
         *  full player, §16). */
        art = false,
        /** Up/down controls on every row. Off by default: on a phone the
         *  queue is a sheet and the order is set by what you queue next.
         *  On where the queue is the point of the screen and there is width
         *  for two more targets (the panel's full player, §16) — and by tap
         *  rather than by drag, because a hold-then-drag at arm's length
         *  over a five-second poll is the least reliable gesture a wall
         *  could pick (the same argument that made grouping tap-based). */
        reorder = false,
        isBusy,
        onJump,
        onRemove,
        onMove,
        onClear,
    }: {
        items: SonosQueueItem[];
        loading?: boolean;
        total: number;
        currentTrack?: number;
        playing?: boolean;
        clearBusy?: boolean;
        confirmClear?: boolean;
        art?: boolean;
        reorder?: boolean;
        isBusy: (key: string) => boolean;
        onJump: (track: number) => void;
        onRemove: (track: number) => void;
        onMove?: (track: number, dir: -1 | 1) => void;
        onClear: () => void;
    } = $props();

    // Two-tap clear for surfaces without a confirm modal: armed, it says
    // so and resets on its own after a few seconds.
    let armed = $state(false);
    let armTimer: ReturnType<typeof setTimeout> | undefined;
    function clearClick() {
        if (!confirmClear || armed) {
            armed = false;
            clearTimeout(armTimer);
            onClear();
            return;
        }
        armed = true;
        clearTimeout(armTimer);
        armTimer = setTimeout(() => (armed = false), 3000);
    }
</script>

<div class="q-bar">
    <span class="q-total mono">{total} {total === 1 ? "track" : "tracks"}</span>
    <button class="chip" class:on={armed} disabled={clearBusy || items.length === 0} onclick={clearClick}>
        {armed ? "Clear?" : "Clear"}
    </button>
</div>

{#if loading}
    <div class="skeleton q-skeleton"></div>
{:else if items.length === 0}
    <p class="q-none">
        Nothing queued. Play a favorite or a Spotify result and it lands here — radio and
        line-in play straight through without a queue.
    </p>
{:else}
    <div class="q-list">
        {#each items as item (item.track)}
            {@const current = item.track === currentTrack}
            <div class="q-row" class:current>
                <button
                    class="q-open"
                    disabled={isBusy("jump:" + item.track)}
                    onclick={() => onJump(item.track)}
                >
                    <span class="q-num mono">
                        {#if current && playing}
                            <Waveform />
                        {:else}
                            {item.track}
                        {/if}
                    </span>
                    {#if art}
                        {#if item.art_uri}
                            <img class="q-art" src={item.art_uri} alt="" loading="lazy" />
                        {:else}
                            <span class="q-art placeholder"></span>
                        {/if}
                    {/if}
                    <span class="q-meta">
                        <span class="q-title">{item.title || "Unknown track"}</span>
                        {#if item.artist}<span class="q-sub">{item.artist}</span>{/if}
                    </span>
                    {#if item.duration}
                        <span class="q-dur mono">{trimClock(item.duration)}</span>
                    {/if}
                </button>
                {#if reorder && onMove}
                    <!-- Disabled at the ends rather than hidden: a control
                         that appears and disappears as rows move is a moving
                         target, and the row above is where the finger is
                         already aimed. -->
                    <button
                        class="icon-btn q-mv"
                        aria-label="Move {item.title || 'track ' + item.track} up"
                        disabled={item.track <= 1 || isBusy("qmv:" + item.track)}
                        onclick={() => onMove(item.track, -1)}
                    >
                        <Icon name="chevronUp" size={14} />
                    </button>
                    <button
                        class="icon-btn q-mv"
                        aria-label="Move {item.title || 'track ' + item.track} down"
                        disabled={item.track >= items.length || isBusy("qmv:" + item.track)}
                        onclick={() => onMove(item.track, 1)}
                    >
                        <Icon name="chevronDown" size={14} />
                    </button>
                {/if}
                <button
                    class="icon-btn q-rm"
                    aria-label="Remove {item.title || 'track ' + item.track} from the queue"
                    disabled={isBusy("qrm:" + item.track)}
                    onclick={() => onRemove(item.track)}
                >
                    <Icon name="close" size={14} />
                </button>
            </div>
        {/each}
    </div>
    {#if total > items.length}
        <div class="q-more mono">showing the first {items.length} of {total}</div>
    {/if}
{/if}

<style>
    .q-bar { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); }
    .q-total {
        font-size: 11px; letter-spacing: 0.08em; text-transform: uppercase;
        color: var(--text-mute);
    }
    .q-skeleton { height: 220px; border-radius: var(--r-md); }
    .q-none { font-size: 12.5px; color: var(--text-mute); line-height: 1.5; }
    .q-list { display: flex; flex-direction: column; gap: 2px; }
    .q-row {
        display: flex; align-items: center; gap: var(--space-1);
        border-radius: var(--r-md);
        transition: background 150ms ease;
    }
    @media (hover: hover) { .q-row:hover { background: var(--card-2); } }
    .q-row.current { background: var(--tile-on-gradient); }
    .q-open {
        flex: 1; min-width: 0;
        display: flex; align-items: center; gap: var(--space-3);
        min-height: 48px; padding: 6px var(--space-2);
        background: transparent; border: 0; border-radius: var(--r-md);
        color: var(--text); cursor: pointer; text-align: left; font: inherit;
    }
    .q-open:disabled { opacity: 0.5; cursor: default; }
    .q-num {
        width: 26px; flex-shrink: 0;
        display: flex; align-items: center; justify-content: center;
        font-size: 11.5px; color: var(--text-dim);
    }
    .q-row.current .q-num { color: var(--on); }
    .q-art {
        width: 36px; height: 36px; flex-shrink: 0;
        object-fit: cover; border-radius: var(--r-sm); background: var(--card-2);
    }
    .q-meta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
    .q-title {
        font-size: 13.5px; font-weight: 500;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .q-row.current .q-title { color: var(--on); }
    .q-sub {
        font-size: 11.5px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .q-dur { font-size: 11px; color: var(--text-dim); flex-shrink: 0; }
    .q-rm { width: 36px; height: 36px; flex-shrink: 0; margin-right: 4px; color: var(--text-mute); }
    .q-rm:disabled { opacity: 0.4; }
    .q-mv { width: 32px; height: 36px; flex-shrink: 0; color: var(--text-dim); }
    .q-mv:disabled { opacity: 0.3; }
    .q-more { font-size: 10.5px; color: var(--text-dim); text-align: center; }

    @media (pointer: coarse) {
        .q-rm { width: 44px; height: 44px; }
        /* Narrower than the 44 floor on purpose: the row itself is 48 tall,
           so the hit area clears it vertically, and three full-width targets
           in a 380px column would leave the title nothing. The pair reads as
           one control and is aimed at as one. */
        .q-mv { width: 38px; height: 44px; }
    }
    @media (prefers-reduced-motion: reduce) {
        .q-row { transition-duration: 0.001ms; }
    }
</style>
