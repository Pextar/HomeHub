<script lang="ts">
    import PanelPlayerCard from "./PanelPlayerCard.svelte";
    import PanelRoomChips from "./PanelRoomChips.svelte";
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
        {#if music.unreachable}
            <div class="m-out">
                <Icon name="speaker" size={26} />
                <p class="m-outline">No speaker is answering</p>
                <p class="m-outsub">The panel keeps trying — nothing here has been lost.</p>
            </div>
        {:else}
            <PanelPlayerCard {music} wide onOpen={open} />
        {/if}
    </section>
{/if}

<style>
    /* The middle band. It takes every row the strip above and the room row
       below don't claim (§16) and spends most of it on air: the head row
       states the section and where the sound goes, and the player is one
       line centred in what's left. A wall panel is read from across a room,
       and a strip with room to breathe reads from further away than a
       rectangle packed to its edges. */
    .music {
        display: flex;
        flex-direction: column;
        justify-content: center;
        gap: var(--space-7);
        min-height: 0;
        min-width: 0;
        padding: var(--space-5) var(--space-8);
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
