<script lang="ts">
    import { roomKeyOf } from "../../lib/panel-music.svelte";
    import type { PanelMusicStore } from "../../lib/panel-music.svelte";

    /**
     * What this house listens to, at the foot of the depth's Rooms pane
     * (DESIGN.md §16).
     *
     * The pane above it is one room at a time — which one is featured, which
     * rooms are grouped, what each is doing. This is the one picture none of
     * those can give: which rooms do the listening, what the house keeps
     * coming back to, and when in the day it is loud. It belongs here rather
     * than on a screen of its own because it is *about* the room list, and a
     * kiosk does not get a screen for something nobody taps.
     *
     * It is read, not driven — with two exceptions that are worth a tap: a
     * room tallied here is a room you might want featured, and an artist the
     * house plays is a search you were about to type. Everything else is a
     * number to glance at.
     *
     * Every figure is bounded by what the store still remembers (thirty
     * plays per room), so the block says how far back it reaches rather than
     * implying it covers everything.
     */
    let {
        music,
        onArtist,
    }: {
        music: PanelMusicStore;
        /** Open an artist by name — the same door the full player's artist
         *  line uses, since a speaker reports names and not catalog ids. */
        onArtist: (name: string) => void;
    } = $props();

    const data = $derived(music.insights);

    /** The busiest hour, which is what scales the day strip. Zero when the
     *  house has never played anything, and then the strip is not drawn: a
     *  row of empty bars is a chart of nothing. */
    const peak = $derived(Math.max(0, ...(data?.hours ?? [])));
    const nowHour = $derived(new Date().getHours());

    const since = $derived.by(() => {
        const iso = data?.since;
        if (!iso) return "";
        const d = new Date(iso);
        if (Number.isNaN(d.getTime())) return "";
        return d.toLocaleDateString([], { day: "numeric", month: "short" });
    });

    /** A tallied room the panel can actually feature. A room whose speaker
     *  has since gone quiet on the network is still in the tally — it played
     *  those records — but there is nothing to point the panel at, so the row
     *  is a fact rather than a control. */
    function sourceFor(key: string) {
        return music.sources.find((s) => roomKeyOf(s) === key);
    }
</script>

{#if data && data.plays > 0}
    <section class="in" aria-label="What this house listens to">
        <h3 class="s-label">What this house listens to</h3>
        <p class="in-stat">
            <span class="mono">{data.plays}</span> plays ·
            <span class="mono">{data.items}</span>
            different things{#if since}&nbsp;· since {since}{/if}
        </p>

        {#if peak > 0}
            <!-- The day, in twenty-four bars. Not a chart anyone reads a
                 value off — it answers "when is this house loud" at a
                 glance, from across a room, which is the only question a
                 wall panel asks of a histogram. The hour it is now is
                 marked, so the shape has a place in it. -->
            <div class="in-day" aria-hidden="true">
                {#each data.hours as n, h (h)}
                    <span
                        class="in-bar"
                        class:now={h === nowHour}
                        class:empty={n === 0}
                        style:height="{Math.max(3, Math.round((n / peak) * 100))}%"
                    ></span>
                {/each}
            </div>
            <!-- Ends and middle, so the strip has a scale. The hour it is
                 now is marked on the bars themselves — putting it in the
                 labels too would say the same thing twice and move a label
                 that ought to hold still. -->
            <p class="in-daylabel mono">
                <span>00:00</span><span>12:00</span><span>23:00</span>
            </p>
        {/if}

        {#if data.rooms.length > 0}
            <p class="in-sub mono">Rooms</p>
            <div class="in-rooms">
                {#each data.rooms as r (r.key)}
                    {@const src = sourceFor(r.key)}
                    <button
                        class="in-room"
                        disabled={!src}
                        aria-label={src ? `Feature ${r.name}` : `${r.name} — ${r.plays} plays`}
                        onclick={() => src && (music.selected = src.key)}
                    >
                        <span class="in-rname">{r.name}</span>
                        <span class="in-track">
                            <span
                                class="in-fill"
                                style:width="{Math.round((r.plays / data.rooms[0].plays) * 100)}%"
                            ></span>
                        </span>
                        <span class="in-rn mono">{r.plays}</span>
                    </button>
                {/each}
            </div>
        {/if}

        {#if data.artists.length > 0}
            <p class="in-sub mono">Artists</p>
            <div class="in-chips">
                {#each data.artists as a (a.key)}
                    <button class="in-chip" onclick={() => onArtist(a.name)}>
                        <span>{a.name}</span>
                        <span class="mono">{a.plays}</span>
                    </button>
                {/each}
            </div>
        {/if}
    </section>
{/if}

<style>
    .in {
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
        margin-top: var(--space-5);
        padding-top: var(--space-4);
        border-top: 1px solid var(--hairline);
    }
    .s-label {
        margin: 0;
        font-family: var(--font-mono);
        font-size: 11px;
        font-weight: 500;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-dim);
    }
    .in-stat {
        margin: 0;
        font-size: 12.5px;
        color: var(--text-mute);
    }

    /* The day strip: twenty-four bars on a shared baseline. Height only —
       no gradient, no glow (§2), and nothing that animates. */
    .in-day {
        display: flex;
        align-items: flex-end;
        gap: 2px;
        height: 44px;
        margin-top: var(--space-2);
    }
    .in-bar {
        flex: 1;
        min-width: 0;
        border-radius: 2px 2px 0 0;
        background: var(--border-strong);
    }
    .in-bar.empty {
        background: var(--hairline);
    }
    .in-bar.now {
        background: var(--on);
    }
    .in-daylabel {
        display: flex;
        justify-content: space-between;
        margin: 0;
        font-size: 10px;
        color: var(--text-dim);
    }

    .in-sub {
        margin: var(--space-3) 0 0;
        font-size: 10.5px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-dim);
    }
    .in-rooms {
        display: flex;
        flex-direction: column;
        gap: 4px;
    }
    /* A room row is a tap that features it — the same thing the list above
       does, said with the number that made you want to. A room with nothing
       answering keeps the row and loses the tap (§15.1). */
    .in-room {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        min-height: 32px;
        padding: 0 var(--space-1);
        border: 0;
        border-radius: var(--r-sm);
        background: none;
        color: var(--text-mute);
        font: inherit;
        font-size: 12.5px;
        text-align: left;
        cursor: pointer;
    }
    .in-room:disabled {
        cursor: default;
        opacity: 0.7;
    }
    .in-rname {
        width: 108px;
        flex-shrink: 0;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .in-track {
        flex: 1;
        min-width: 0;
        height: 6px;
        border-radius: var(--r-pill);
        background: var(--card-2);
        overflow: hidden;
    }
    .in-fill {
        display: block;
        height: 100%;
        border-radius: var(--r-pill);
        background: var(--border-strong);
    }
    .in-rn {
        width: 3ch;
        flex-shrink: 0;
        text-align: right;
        font-size: 11.5px;
        color: var(--text-dim);
    }

    .in-chips {
        display: flex;
        flex-wrap: wrap;
        gap: var(--space-2);
    }
    .in-chip {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        padding: 7px 12px;
        border-radius: var(--r-pill);
        border: 1px solid var(--hairline);
        background: var(--card-2);
        color: var(--text-mute);
        font-family: inherit;
        font-size: 12.5px;
        font-weight: 500;
        cursor: pointer;
        transition:
            background var(--t-fast),
            color var(--t-fast),
            transform var(--t-fast);
    }
    .in-chip .mono {
        color: var(--text-dim);
        font-size: 11px;
    }
    .in-chip:active {
        transform: scale(0.95);
        transition-duration: 80ms;
    }
    @media (hover: hover) {
        .in-chip:hover {
            color: var(--text);
            background: var(--card-3);
        }
        .in-room:not(:disabled):hover .in-rname {
            color: var(--text);
        }
    }

    @media (pointer: coarse) {
        .in-chip {
            min-height: 44px;
            padding-inline: 16px;
        }
        .in-room {
            min-height: 44px;
        }
    }
</style>
