<script lang="ts">
    import Waveform from "../music/Waveform.svelte";
    import TrackRail from "../music/TrackRail.svelte";
    import PanelMarquee from "./PanelMarquee.svelte";
    import type { PanelNowPlaying } from "../../lib/panel";

    /**
     * The ambient face's listening subject (DESIGN.md §16): what the wall
     * rests on while a record is on.
     *
     * The two parts are the cover and a column beside it, and the column is
     * where this component earns its keep. A wall showing one record for
     * three minutes has the space and the attention to say everything about
     * it that can be said without a tap — so it does, in one order, most
     * important first:
     *
     *   clock · title · artist & album · how far through · which room ·
     *   what's next · what else is on · the house's own two figures
     *
     * Everything below the title is conditional, and absent rather than
     * empty when the room can't answer it (§15.1): radio has no position
     * and no queue, a shuffled group has no knowable next track, and a
     * house with one room playing has no "also playing". A quiet evening
     * with a station on collapses back to almost exactly the face this
     * replaced.
     *
     * The face renders as two roots so the parent's `.face` stays the flex
     * container that decides row-or-column: the cover and the column are
     * its two items, and the panel keeps the drift, the fade and the night
     * dimming that belong to the face as a whole.
     */
    let {
        playing,
        /** Live, off the store: seconds into the track and its length. Zero
         *  duration means the source reported none — radio, line-in, a KEF
         *  or a streamed zone — and the rail is absent rather than
         *  fabricated (§15.5). */
        position,
        duration,
        timeLabel,
        statusLine,
    }: {
        playing: PanelNowPlaying;
        position: number;
        duration: number;
        timeLabel: string;
        statusLine: string;
    } = $props();

    // Art that 404s — the proxy can't reach the speaker, the service
    // expired the URL — left an empty box that read as one still loading.
    // Keyed to the URL so the next track gets its own try.
    let artFailed = $state<string | null>(null);
    const artSrc = $derived(playing.art && playing.art !== artFailed ? playing.art : null);
</script>

<div class="l-art">
    {#if artSrc}
        <img src={artSrc} alt="" onerror={() => (artFailed = artSrc)} />
    {:else}
        <span class="placeholder">[ art ]</span>
    {/if}
</div>

<div class="l-meta">
    <!-- Still a wall panel: the time is what you glance up for, so it keeps
         the top of the column — one size down from the clock face, where it
         is no longer the subject, and one size down again from where it sat
         before the column had this much else to say. -->
    <div class="l-clock mono">{timeLabel}</div>

    <div class="l-track"><PanelMarquee text={playing.title} /></div>
    {#if playing.sub}
        <!-- Truncated rather than rolling, deliberately: one thing moving on
             a resting screen reads as the record announcing itself, two
             read as a ticker. The title is the one you came for. -->
        <div class="l-sub">{playing.sub}</div>
    {/if}

    {#if duration > 0}
        <div class="l-rail"><TrackRail {position} {duration} large /></div>
    {/if}

    <div class="l-room">
        <Waveform />
        <span class="l-room-name">{playing.room}</span>
        {#if playing.queueTrack && playing.queueLength}
            <span class="l-qpos mono">{playing.queueTrack} / {playing.queueLength}</span>
        {/if}
    </div>

    {#if playing.next}
        <div class="l-next">
            <span class="l-label">Up next</span>
            <span class="l-next-title">{playing.next.title}</span>
            {#if playing.next.sub}<span class="l-next-sub">{playing.next.sub}</span>{/if}
        </div>
    {/if}

    {#if playing.elsewhere.length}
        <div class="l-also">
            <span class="l-label">Also playing</span>
            <span class="l-also-rooms">{playing.elsewhere.join(" · ")}</span>
        </div>
    {/if}

    <div class="l-status mono">{statusLine}</div>
</div>

<style>
    /* The cover is sized in viewport units and capped, not measured: this
       face has nothing else competing for the space, so there is no reading
       that could chase its own tail (contrast the band and the full player,
       which both measure — §16). The cap came down when the column grew:
       a record and a column of facts share a 768px wall, and the column is
       the half that got longer. */
    .l-art {
        flex: none;
        width: clamp(200px, 50vh, 560px);
        height: clamp(200px, 50vh, 560px);
        border-radius: var(--r-lg);
        overflow: hidden;
        background: var(--card-2);
        border: 1px solid var(--hairline);
        display: grid;
        place-items: center;
    }
    .l-art img {
        width: 100%;
        height: 100%;
        object-fit: cover;
        display: block;
    }
    .l-art .placeholder {
        width: 100%;
        height: 100%;
        display: grid;
        place-items: center;
        font-size: 12px;
        color: var(--text-dim);
    }

    /* A stated width rather than a shrink-to-fit column: the marquee
       measures its overrun against this box, so the box may not be sized
       by the text inside it — that is the same trap the full player's
       record fell into, one column over (§16). */
    .l-meta {
        min-width: 0;
        width: clamp(300px, 44vw, 620px);
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
    }
    .l-clock {
        font-size: clamp(48px, 9vw, 112px);
        font-weight: 500;
        letter-spacing: -0.03em;
        line-height: 1;
    }
    .l-track {
        font-size: clamp(24px, 3.4vw, 44px);
        font-weight: 600;
        letter-spacing: -0.02em;
        line-height: 1.15;
    }
    .l-sub {
        font-size: clamp(15px, 1.9vw, 23px);
        color: var(--text-mute);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .l-rail {
        margin-top: var(--space-1);
    }

    /* Which room is making the noise — the waveform rather than a dot,
       because audio is moving (§6.8). */
    .l-room {
        display: flex;
        align-items: center;
        gap: 10px;
        min-width: 0;
    }
    .l-room-name {
        font-size: 17px;
        font-weight: 600;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .l-qpos {
        font-size: 15px;
        color: var(--text-dim);
        flex-shrink: 0;
    }

    /* Up next / Also playing: a mono label over the answer, the way the
       strip states a reading. Both are absent when the room can't answer. */
    .l-label {
        font-family: var(--font-mono);
        font-size: 11.5px;
        font-weight: 500;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-dim);
    }
    .l-next,
    .l-also {
        display: flex;
        flex-direction: column;
        gap: 3px;
        min-width: 0;
        margin-top: var(--space-2);
    }
    .l-next-title {
        font-size: 19px;
        font-weight: 500;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .l-next-sub,
    .l-also-rooms {
        font-size: 15px;
        color: var(--text-mute);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .l-status {
        margin-top: var(--space-3);
        font-size: 15px;
        color: var(--text-dim);
    }

    /* Portrait: the cover gives way first — the fallback the rest of the
       panel already takes (§16). The column centres with it. */
    @media (orientation: portrait), (max-width: 900px) {
        .l-art {
            width: min(60vw, 380px);
            height: min(60vw, 380px);
        }
        .l-meta {
            width: min(84vw, 520px);
            text-align: center;
        }
        /* Centred by the text, never by `align-items` on the column: that
           shrink-wraps every row to its content, and the rail — a bar with
           a time at each end and nothing in the middle — collapses to the
           width of the two times, which then read as one number. */
        .l-room {
            justify-content: center;
        }
        .l-next,
        .l-also {
            align-items: center;
        }
    }
</style>
