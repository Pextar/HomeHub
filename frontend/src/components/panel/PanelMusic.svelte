<script lang="ts">
    import PanelPlayerCard from "./PanelPlayerCard.svelte";
    import PanelRoomChips from "./PanelRoomChips.svelte";
    import PanelBandShelf from "./PanelBandShelf.svelte";
    import PanelAnnounce from "./PanelAnnounce.svelte";
    import Icon from "../Icon.svelte";
    import { route } from "../../lib/stores.svelte";
    import type { PanelMusicStore } from "../../lib/panel-music.svelte";

    // The panel dashboard's music band: what's playing, plus the transport
    // and volume that answer from across the room. It is the widest zone on
    // the surface and the tallest thing on it after the room row, because
    // music is what a wall panel is actually *used* for — the clock and the
    // lights are there to be read, not driven (DESIGN.md §16). Music's second satellite
    // outside its own view (after Home's "Playing now" card, DESIGN.md §6.8).
    // The art taps through to the panel's own music depth — search and the
    // library one level in, without leaving the kiosk (§16). The speaker
    // state itself lives in the shared store the parent panel owns.
    //
    // The band holds its ground when the speakers stop answering: a wall
    // panel that reflows its grid on a dropped packet is worse than one
    // that says "not answering" and stays where it was.
    let { music }: { music: PanelMusicStore } = $props();

    // The way in. The cover is still the tap-through on the card (§16) and
    // it is the bigger, more obvious one — but it is *card* content, and the
    // card is the part of this band that can be absent: no speaker
    // answering, nothing ever played, a cover that failed to draw. The
    // section's own name is here in every one of those states, so that is
    // where the depth's door belongs. It is section navigation, not a
    // second copy of the cover's job.
    const open = () => route.go("panel", { music: "1" });

    /** The stage's measured height — what the player row may have once the
     *  head row and the shelf have taken theirs. The cover is sized from it
     *  (see PanelPlayerCard's `artMax`). */
    let stageH = $state(0);

    /** Calling the house takes the shelf's place at the foot of the band —
     *  a swap, like the full player's grouping pane, because the kiosk has
     *  no sheets and the player above must keep playing and stay touchable
     *  while someone shouts up the stairs (§16). */
    let announcing = $state(false);
    // Only where there is somewhere for it to land. A house whose speakers
    // aren't answering gets no control rather than one that explains itself
    // after the tap (§15.1).
    const canAnnounce = $derived(!!music.announce?.available);

    /** The featured room is going to go quiet on its own, and this band is
     *  the surface anyone actually walks past. The timer was set one depth
     *  in, on the Rooms pane, and until now it was only *readable* there —
     *  so "why is the music fading?" and "how long have I got?" were both
     *  questions the dashboard couldn't answer about its own room. The chip
     *  states the fact and its tap lands where the controls are: the Rooms
     *  pane, not the depth's front door. Absent when nothing is going to
     *  quiet the room, which is most of the time. */
    const sleepLeft = $derived(music.sleepMinutesLeft);
    const openSleep = () => route.go("panel", { music: "1", pane: "rooms" });
</script>

{#if music.hasSpeakers}
    <section class="music" aria-label="Now playing">
        <header class="m-head">
            <h2>
                <button class="m-open" onclick={open}>
                    <span>Music</span>
                    <Icon name="chevronRight" size={18} />
                </button>
            </h2>
            <!-- The destination picker rides here, where the row is as wide
                 as the panel: in the card it wrapped to two and three lines
                 and took that height off the cover (§16). -->
            <PanelRoomChips {music} />
            {#if sleepLeft > 0}
                <!-- The one thing on this band that is about to happen
                     without anyone touching it. It rides with the two
                     whole-house taps because it is the same kind of fact:
                     read from a step away, acted on somewhere else. While
                     the ramp is actually walking, the icon breathes — the
                     volume is moving on its own over minutes with no other
                     tell, which is the same licence §6.8's waveform has. -->
                <button class="m-pauseall m-sleep" class:fading={music.fading} onclick={openSleep}>
                    <span class="m-sleepico" class:live={music.fading}>
                        <Icon name={music.fading ? "activity" : "moon"} size={14} />
                    </span>
                    <span>{music.fading ? "Fading" : "Quiet in"}</span>
                    <span class="mono">{sleepLeft}m</span>
                </button>
            {/if}
            {#if canAnnounce}
                <!-- The one control on this wall that makes a sound in a
                     room you are not standing in. It rides beside Pause all
                     because both are whole-house taps aimed at from a step
                     away, and neither is about the room the chips name. -->
                <button
                    class="m-pauseall"
                    class:on={announcing}
                    aria-pressed={announcing}
                    onclick={() => (announcing = !announcing)}
                >
                    <Icon name="megaphone" size={14} /><span>Announce</span>
                </button>
            {/if}
            {#if music.anyPlaying}
                <!-- The tap a wall gets asked for on the way to bed, and
                     had no button for: everything, quiet, at once. -->
                <button
                    class="m-pauseall"
                    disabled={music.busy["pauseall"]}
                    onclick={() => music.pauseAll()}
                >
                    <Icon name="pause" size={14} /><span>Pause all</span>
                </button>
            {/if}
        </header>
        <!-- The stage: the height the band has left once the head row and
             the shelf have taken theirs. It is measured here rather than
             inside the card because the cover is sized from it, and a box
             the cover can size is a reading that chases its own tail (§16).
             A flex item with `min-height: 0` takes its height from the band
             and never from what is in it, so the loop can't close. -->
        <div class="m-stage" bind:clientHeight={stageH}>
            {#if music.unreachable}
                <div class="m-out">
                    <Icon name="speaker" size={26} />
                    <p class="m-outline">No speaker is answering</p>
                    <p class="m-outsub">The panel keeps trying — nothing here has been lost.</p>
                </div>
            {:else}
                <PanelPlayerCard {music} wide artMax={stageH} onOpen={open} />
            {/if}
        </div>
        <!-- What to put on next, at the foot of the band. The player is a
             strip and cannot use the band's height on its own; this is what
             that height is for (§16). -->
        {#if announcing}
            <PanelAnnounce {music} onClose={() => (announcing = false)} />
        {:else if !music.unreachable}
            <PanelBandShelf {music} />
        {/if}
    </section>
{/if}

<style>
    /* The middle band. It takes every row the strip above and the room row
       below don't claim (§16) and spends them in this order: the head row
       states the section and where the sound goes, the shelf at the foot
       says what could go on next, and the player row in between takes what
       is left — which is what sizes the cover. */
    .music {
        display: flex;
        flex-direction: column;
        gap: var(--space-5);
        min-height: 0;
        min-width: 0;
        padding: var(--space-5) var(--space-8);
    }
    /* The player's room. `min-height: 0` is what makes the measurement safe:
       the stage is sized by the band, never by the cover inside it. */
    .m-stage {
        flex: 1 1 auto;
        min-height: 0;
        min-width: 0;
        display: flex;
        align-items: center;
    }
    .m-stage :global(.p-card.wide) {
        flex: 1 1 auto;
        min-width: 0;
    }
    .m-head {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        flex-shrink: 0;
    }
    /* The chips take the middle; Pause all keeps the trailing edge. */
    .m-head :global(.p-sources) {
        flex: 1 1 auto;
    }
    h2 {
        margin: 0;
        flex-shrink: 0;
        font-size: 17px;
        font-weight: 600;
        letter-spacing: -0.02em;
    }
    /* The heading is the door, so it is drawn as the heading and not as a
       chip: only the chevron says it leads anywhere. A wall poke needs the
       whole 44px, hence the negative inline margin — the text still lines
       up with the band under it. */
    .m-open {
        display: inline-flex;
        align-items: center;
        gap: var(--space-1);
        min-height: 44px;
        margin-inline: calc(var(--space-2) * -1);
        padding-inline: var(--space-2);
        border: 0;
        border-radius: var(--r-sm);
        background: none;
        color: inherit;
        font: inherit;
        letter-spacing: inherit;
        cursor: pointer;
        transition: color var(--t-fast);
    }
    .m-open :global(svg) {
        color: var(--text-dim);
    }
    .m-open:active {
        color: var(--on);
    }
    .m-pauseall {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        min-height: 36px;
        padding: 0 var(--space-3);
        border-radius: var(--r-pill);
        border: 1px solid var(--hairline);
        background: var(--card-2);
        color: var(--text-mute);
        font-family: inherit;
        font-size: 12.5px;
        font-weight: 500;
        cursor: pointer;
        flex-shrink: 0;
        transition:
            background var(--t-fast),
            transform var(--t-fast);
    }
    .m-pauseall:active {
        transform: scale(0.95);
        transition-duration: 80ms;
    }
    .m-pauseall:disabled {
        opacity: 0.55;
    }
    /* Open is a ring and a brighter ink, which is how the panel says
       "chosen" everywhere else (§16) — never the ON gradient, which on this
       surface belongs to a room that is lit. */
    .m-pauseall.on {
        border-color: var(--on);
        color: var(--text);
    }

    /* A statement first and a target second, so it is drawn quieter than the
       two taps beside it — until the ramp is in flight, when it is the only
       thing on the panel that says why the volume is dropping. */
    .m-sleep .mono {
        color: var(--text);
    }
    .m-sleep.fading {
        border-color: var(--on);
        color: var(--text);
    }
    .m-sleepico {
        display: inline-flex;
    }
    /* The one loop this band is allowed, and only while something is
       genuinely happening right now (§16). Opacity on a 14px icon. */
    .m-sleepico.live {
        animation: breathe 2.4s ease-in-out infinite;
        color: var(--on);
    }
    @keyframes breathe {
        0%,
        100% {
            opacity: 1;
        }
        50% {
            opacity: 0.4;
        }
    }
    /* Reduced motion stops a loop outright rather than slowing it: a loop is
       the one kind of motion someone who asked for less of it keeps seeing. */
    @media (prefers-reduced-motion: reduce) {
        .m-sleepico.live {
            animation: none;
        }
    }

    /* Nothing answered — said in place, at the column's own size, so the
       grid doesn't move while the network sorts itself out. */
    .m-out {
        flex: 1;
        display: flex;
        flex-direction: column;
        align-items: center;
        justify-content: center;
        gap: var(--space-2);
        padding: var(--space-5);
        border: 1px dashed var(--border);
        border-radius: var(--r-lg);
        color: var(--text-dim);
        text-align: center;
    }
    .m-outline {
        margin: 0;
        font-size: 15px;
        color: var(--text-mute);
    }
    .m-outsub {
        margin: 0;
        font-size: 12.5px;
    }

    @media (pointer: coarse) {
        .m-pauseall {
            min-height: 44px;
        }
    }
</style>
