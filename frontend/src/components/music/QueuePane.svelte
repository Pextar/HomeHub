<script lang="ts">
    /**
     * The queue, as the player's second pane — reached from an "Up next" row
     * that names the actual next track, not as a segmented control: §2 has no
     * exception left to lean on.
     *
     * **It opens on the track playing, not on track one.** A room forty
     * tracks into a playlist used to answer "up next" with a list of what
     * already went by, and the answer to the question asked was somewhere
     * below the fold. So the pane is cut at the current track — and cut, not
     * trimmed: the last couple of played tracks stay in view above it,
     * because "what was that one I liked?" is asked of this list as often as
     * "what's coming", and it is asked about the song that just ended. Deeper
     * history folds behind one row that says how many.
     *
     * The fold is a disclosure, not a truncation — the queue is still all
     * there, one tap away, played rows dimmed rather than hidden — and
     * expanding it holds the playing row where the eye already is rather
     * than letting the list jump under the finger.
     *
     * Rows show a mono track number, replaced by the §6.8 waveform on the one
     * playing, and that row takes the `.tile.on` surface. Tapping a row jumps
     * to it, the trailing X removes it, and "Clear" is destructive enough that
     * the confirm lives with the caller.
     */
    import { tick } from "svelte";
    import Icon from "../Icon.svelte";
    import Waveform from "./Waveform.svelte";
    import { trimClock } from "../../lib/music/time";
    import { splitQueue, foldEarlier } from "../../lib/music/queue";
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
        /** The way further back than this queue goes — the room's listening
         *  log. Absent on surfaces that have nowhere to send it. */
        onPlayed,
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
        onPlayed?: () => void;
        isBusy: (key: string) => boolean;
        onJump: (track: number) => void;
        onRemove: (track: number) => void;
        onMove?: (track: number, dir: -1 | 1) => void;
        onClear: () => void;
    } = $props();

    // ── Where the list is cut ────────────────────────────────────────────
    // The playing track, and everything after it, is what the pane is for;
    // `splitQueue` owns the arithmetic and the cases where there is no cut
    // to make (radio, line-in, a position past the fetched window). The last
    // couple of played tracks stay above it — `foldEarlier` decides how many
    // — because the song you want back is usually the one that just ended.
    const split = $derived(splitQueue(items, currentTrack));
    const fold = $derived(foldEarlier(split.earlier));

    let showEarlier = $state(false);
    // A different room's queue is a different list, and whatever was unfolded
    // for the last one shouldn't decide how this one opens. The head of the
    // queue is what says "different list": a track advancing, or one removed
    // from the middle, leaves it alone — and neither should fold the list shut
    // under someone who is reading it.
    let foldedFor = "";
    $effect(() => {
        const head = `${items[0]?.track ?? 0}:${items[0]?.title ?? ""}:${items.length === 0}`;
        if (head === foldedFor) return;
        foldedFor = head;
        showEarlier = false;
    });

    /**
     * Unfolding inserts rows *above* the one being read, which on its own
     * throws the playing track down the screen by however many tracks went
     * before it. Measure that row, let the DOM settle, and give the scroll
     * container back the difference so the row doesn't move at all.
     */
    let listEl = $state<HTMLElement | undefined>();
    function scrollParent(el: HTMLElement | null): HTMLElement | null {
        for (let p = el?.parentElement ?? null; p; p = p.parentElement) {
            const oy = getComputedStyle(p).overflowY;
            if ((oy === "auto" || oy === "scroll") && p.scrollHeight > p.clientHeight) return p;
        }
        return null;
    }
    function currentTop(): number | undefined {
        return listEl?.querySelector<HTMLElement>(".q-row.current")?.getBoundingClientRect().top;
    }
    async function toggleEarlier() {
        const before = currentTop();
        showEarlier = !showEarlier;
        await tick();
        const after = currentTop();
        if (before === undefined || after === undefined) return;
        const delta = after - before;
        if (!delta) return;
        const sp = scrollParent(listEl ?? null);
        if (sp) sp.scrollTop += delta;
        else window.scrollBy(0, delta);
    }

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

<!-- Where the queue's own memory ends. Going back through a queue is going up
     it, so the way past its beginning is a row above the top of it: this
     queue, then everything before this queue. It stays for an empty queue as
     well — a queue that was just replaced is exactly when the log is the only
     thing left that knows what played. -->
{#if onPlayed}
    <button class="q-earlier q-before" onclick={onPlayed}>
        <span class="q-earlier-icon"><Icon name="clock" size={14} /></span>
        <span class="q-earlier-label">Played before this</span>
        <span class="q-before-go" aria-hidden="true"><Icon name="chevronLeft" size={14} /></span>
    </button>
{/if}

{#if loading}
    <div class="skeleton q-skeleton"></div>
{:else if items.length === 0}
    <p class="q-none">
        Nothing queued. Play a favorite or a Spotify result and it lands here — radio and
        line-in play straight through without a queue.
    </p>
{:else}
    <div class="q-list" bind:this={listEl}>
        <!-- The deep history, folded into one row. Chevron up to reach back
             for it, chevron down to put it away again: the direction the
             list moves, not an abstract disclosure triangle. -->
        {#if fold.hidden.length > 0}
            <button class="q-earlier" aria-expanded={showEarlier} onclick={toggleEarlier}>
                <span class="q-earlier-icon">
                    <Icon name={showEarlier ? "chevronDown" : "chevronUp"} size={14} />
                </span>
                <span class="q-earlier-label">
                    {#if showEarlier}
                        Fold the rest back up
                    {:else}
                        <span class="mono">{fold.hidden.length}</span>
                        {fold.hidden.length === 1 ? "track" : "tracks"} before that
                    {/if}
                </span>
            </button>
            {#if showEarlier}
                {#each fold.hidden as item (item.track)}
                    {@render row(item)}
                {/each}
            {/if}
        {/if}

        <!-- What just played, kept in view: the song worth asking about is
             usually the one that just ended, and it is a row you can tap to
             hear again rather than a name you have to go looking for. -->
        {#each fold.shown as item (item.track)}
            {@render row(item)}
        {/each}

        {#each split.ahead as item (item.track)}
            {@render row(item)}
            <!-- The line between "this" and "what's after this", drawn once
                 and only where there is something after it to name. -->
            {#if item.track === currentTrack && split.upNext > 0}
                <div class="q-split">
                    <span class="eyrow">Up next</span>
                    <span class="q-split-n mono">{split.upNext}</span>
                </div>
            {/if}
        {/each}

        {#if split.currentIdx >= 0 && split.upNext === 0 && total <= items.length}
            <p class="q-end">Last track in the queue.</p>
        {/if}
    </div>
    {#if total > items.length}
        <div class="q-more mono">showing the first {items.length} of {total}</div>
    {/if}
{/if}

{#snippet row(item: SonosQueueItem)}
    {@const current = item.track === currentTrack}
    <div class="q-row" class:current class:past={currentTrack !== undefined && item.track < currentTrack}>
        <button class="q-open" disabled={isBusy("jump:" + item.track)} onclick={() => onJump(item.track)}>
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
{/snippet}

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

    /* The fold. A row, not a chip: it sits in the list's column and stands in
       for the rows it holds, so it takes their width and their shape. */
    .q-earlier {
        display: flex; align-items: center; gap: var(--space-2);
        width: 100%; min-height: 40px; padding: 6px var(--space-2);
        background: transparent; border: 0; border-radius: var(--r-md);
        color: var(--text-mute); cursor: pointer; text-align: left; font: inherit;
        font-size: 12px;
        transition: background 150ms ease, color 150ms ease;
    }
    @media (hover: hover) { .q-earlier:hover { background: var(--card-2); color: var(--text); } }
    .q-earlier-icon {
        width: 26px; flex-shrink: 0;
        display: flex; align-items: center; justify-content: center;
        color: var(--text-dim);
    }
    .q-earlier-label .mono { font-size: 11.5px; color: var(--text); }
    /* The door out of the queue and into the log. Same row shape as the
       fold above it — they are the same gesture, one queue apart. */
    .q-before { color: var(--text-mute); }
    .q-before .q-earlier-label { flex: 1; }
    .q-before-go { display: flex; transform: rotate(180deg); color: var(--text-dim); }
    /* Played, and dimmer for it — still a target, just not the subject. */
    .q-row.past .q-title { color: var(--text-mute); }
    .q-row.past .q-num, .q-row.past .q-sub { color: var(--text-dim); }
    .q-row.past .q-art { opacity: 0.65; }

    /* Where "playing" stops and "next" starts. */
    .q-split {
        display: flex; align-items: baseline; gap: var(--space-2);
        padding: var(--space-3) var(--space-2) var(--space-1);
    }
    .q-split-n { font-size: 11px; color: var(--text-dim); }
    .q-end {
        font-size: 11.5px; color: var(--text-dim);
        padding: var(--space-2) var(--space-2) 0; margin: 0;
    }
    .q-rm { width: 36px; height: 36px; flex-shrink: 0; margin-right: 4px; color: var(--text-mute); }
    .q-rm:disabled { opacity: 0.4; }
    .q-mv { width: 32px; height: 36px; flex-shrink: 0; color: var(--text-dim); }
    .q-mv:disabled { opacity: 0.3; }
    .q-more { font-size: 10.5px; color: var(--text-dim); text-align: center; }

    @media (pointer: coarse) {
        .q-earlier { min-height: 44px; }
        .q-rm { width: 44px; height: 44px; }
        /* Narrower than the 44 floor on purpose: the row itself is 48 tall,
           so the hit area clears it vertically, and three full-width targets
           in a 380px column would leave the title nothing. The pair reads as
           one control and is aimed at as one. */
        .q-mv { width: 38px; height: 44px; }
    }
    @media (prefers-reduced-motion: reduce) {
        .q-row, .q-earlier { transition-duration: 0.001ms; }
    }
</style>
