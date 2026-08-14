<script lang="ts">
    /**
     * What this room has been heard playing — the player's third pane, and
     * the only surface in Music that outlives a queue.
     *
     * The queue pane keeps what it has already passed, which answers "what
     * was that?" right up until someone puts a different record on. This
     * doesn't lose it: the hub writes a row whenever a track has actually
     * been playing for a while, in any room, whether it was started here, in
     * the Sonos app, or by the station's own scheduler.
     *
     * Rows are grouped by day, because the question is asked in days ("that
     * thing from last night") and a bare stack of times is not an answer.
     * Tapping a row plays it again where a service URI came with it, and says
     * plainly that it can't where one didn't — radio names its songs but
     * hands over nothing to play, and a row that failed on tap would be worse
     * than a row that never offered.
     */
    import Icon from "../Icon.svelte";
    import { formatAgo } from "../../lib/utils";
    import type { HeardTrack } from "../../lib/types";

    let {
        tracks,
        loading = false,
        /** True when this is the household's listening, not this room's. */
        household = false,
        /** The room these rows would play back into. */
        roomName,
        /** In flight, keyed like everything else in the module. */
        isBusy,
        /** Absent where nothing here can be played again (a KEF, a zone). */
        onPlay,
        onClear,
        clearBusy = false,
    }: {
        tracks: HeardTrack[];
        loading?: boolean;
        household?: boolean;
        roomName: string;
        isBusy: (key: string) => boolean;
        onPlay?: (t: HeardTrack) => void;
        onClear: () => void;
        clearBusy?: boolean;
    } = $props();

    /** Rows in days, newest day first, each day's rows already newest first. */
    const days = $derived.by(() => {
        const out: { label: string; key: string; rows: HeardTrack[] }[] = [];
        for (const t of tracks) {
            const key = dayKey(t.at);
            const last = out[out.length - 1];
            if (last && last.key === key) last.rows.push(t);
            else out.push({ key, label: dayLabel(t.at), rows: [t] });
        }
        return out;
    });

    function dayKey(at: string | Date): string {
        const d = at instanceof Date ? at : new Date(at);
        return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`;
    }

    /** Today and yesterday by name; anything older by date. */
    function dayLabel(at: string): string {
        const d = new Date(at);
        const now = new Date();
        // Yesterday from the calendar rather than from 24 hours of
        // milliseconds: an hour of it goes missing twice a year.
        const yesterday = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1);
        if (dayKey(d) === dayKey(now)) return "Today";
        if (dayKey(d) === dayKey(yesterday)) return "Yesterday";
        return d.toLocaleDateString(undefined, {
            weekday: "short",
            day: "numeric",
            month: "short",
        });
    }

    /** The clock time it played at — the anchor a memory actually uses. */
    function clock(at: string): string {
        return new Date(at).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
    }

    /** Row identity for the busy key: the URI, else where it sits in time. */
    const rowKey = (t: HeardTrack) => "heard:" + (t.uri || t.title + t.at);
</script>

<div class="q-bar">
    <span class="q-total mono">
        {tracks.length}
        {tracks.length === 1 ? "track" : "tracks"}
    </span>
    <button class="chip" disabled={clearBusy || tracks.length === 0 || household} onclick={onClear}>
        Clear
    </button>
</div>

{#if loading}
    <div class="skeleton h-skeleton"></div>
{:else if tracks.length === 0}
    <p class="h-none">
        Nothing heard yet. Once music has been playing here for a while it lands in this list —
        whatever started it, and whether or not it was HomeHub.
    </p>
{:else}
    {#if household}
        <!-- Never let a list imply this room played something it didn't: say
             whose listening this is, and every row names its own room. -->
        <p class="h-note">
            Nothing heard in {roomName} yet — this is what the house has been playing.
        </p>
    {/if}

    <div class="h-days">
        {#each days as day (day.key)}
            <div class="h-day">
                <div class="eyrow">{day.label}</div>
                {#each day.rows as t (rowKey(t) + t.at)}
                    <div class="h-row">
                        {#if t.uri && onPlay}
                            <button
                                class="h-open"
                                disabled={isBusy(rowKey(t))}
                                onclick={() => onPlay?.(t)}
                            >
                                {@render body(t)}
                                <span class="h-go" aria-hidden="true">
                                    <Icon name="play" size={14} />
                                </span>
                            </button>
                        {:else}
                            <!-- Nothing to hand back, so nothing to tap: the row
                                 keeps its shape and loses only the affordance. -->
                            <div class="h-open flat">{@render body(t)}</div>
                        {/if}
                    </div>
                {/each}
            </div>
        {/each}
    </div>

    <p class="h-hint">
        {#if onPlay}
            Tap anything with a play mark to hear it again in {roomName}. Radio and line-in are
            named here but can't be handed back — nothing came with them to play.
        {:else}
            These are names only: {roomName} plays what it is handed, so there is nothing here to start
            again from a row.
        {/if}
    </p>
{/if}

{#snippet body(t: HeardTrack)}
    {#if t.art_uri}
        <img class="h-art" src={t.art_uri} alt="" loading="lazy" />
    {:else}
        <span class="h-art placeholder"></span>
    {/if}
    <span class="h-meta">
        <span class="h-title">{t.title}</span>
        <span class="h-sub">
            {#if t.artist}{t.artist}{/if}
            {#if household && t.room_name}<span class="h-room">· {t.room_name}</span>{/if}
        </span>
    </span>
    <span class="h-when">
        <span class="h-clock mono">{clock(t.at)}</span>
        <span class="h-ago mono">{formatAgo(t.at)}</span>
    </span>
{/snippet}

<style>
    .q-bar {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
    }
    .q-total {
        font-size: 11px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-mute);
    }
    .h-skeleton {
        height: 220px;
        border-radius: var(--r-md);
    }
    .h-none,
    .h-note {
        font-size: 12.5px;
        color: var(--text-mute);
        line-height: 1.5;
        margin: 0;
    }

    .h-days {
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
    }
    .h-day {
        display: flex;
        flex-direction: column;
        gap: 2px;
    }
    .h-day .eyrow {
        margin-bottom: var(--space-1);
    }

    .h-row {
        display: flex;
        align-items: center;
        border-radius: var(--r-md);
        transition: background 150ms ease;
    }
    @media (hover: hover) {
        .h-row:hover {
            background: var(--card-2);
        }
    }
    .h-open {
        flex: 1;
        min-width: 0;
        display: flex;
        align-items: center;
        gap: var(--space-3);
        min-height: 52px;
        padding: 6px var(--space-2);
        background: transparent;
        border: 0;
        border-radius: var(--r-md);
        color: var(--text);
        cursor: pointer;
        text-align: left;
        font: inherit;
    }
    /* A row with nothing to play is still a row worth reading, so it keeps
       its shape and loses only the affordance. */
    .h-open.flat {
        cursor: default;
    }
    .h-open:disabled {
        opacity: 0.5;
        cursor: default;
    }
    .h-art {
        width: 40px;
        height: 40px;
        flex-shrink: 0;
        object-fit: cover;
        border-radius: var(--r-sm);
        background: var(--card-2);
    }
    .h-meta {
        flex: 1;
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 1px;
    }
    .h-title {
        font-size: 13.5px;
        font-weight: 500;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
    .h-sub {
        font-size: 11.5px;
        color: var(--text-mute);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
    .h-room {
        color: var(--text-dim);
    }
    .h-when {
        display: flex;
        flex-direction: column;
        align-items: flex-end;
        gap: 1px;
        flex-shrink: 0;
    }
    .h-clock {
        font-size: 11.5px;
        color: var(--text-dim);
    }
    .h-ago {
        font-size: 10px;
        color: var(--text-dim);
        opacity: 0.75;
    }
    .h-go {
        display: flex;
        flex-shrink: 0;
        color: var(--text-mute);
    }
    .h-hint {
        font-size: 11.5px;
        color: var(--text-dim);
        line-height: 1.5;
        margin: 0;
    }

    @media (pointer: coarse) {
        .h-open {
            min-height: 56px;
        }
    }
    @media (prefers-reduced-motion: reduce) {
        .h-row {
            transition-duration: 0.001ms;
        }
    }
</style>
