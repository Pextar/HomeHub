<script lang="ts">
    import Waveform from "../music/Waveform.svelte";
    import type { PanelRooms } from "../../lib/panel-music.svelte";

    // Where the sound goes. This used to sit inside the player card, in a
    // column narrow enough that three rooms wrapped to two rows and six to
    // three — 150px of a 720px panel spent on the control you touch least,
    // taken from the cover you look at most. On the surface's own header it
    // is one line, and the card's height goes to the cover instead (§16).
    let { music }: { music: PanelRooms } = $props();

    const featured = $derived(music.featured);

    /** How a chip names its make, for the ones that aren't obvious. */
    function chipTitle(kind: string): string {
        return kind === "zone" ? "HomeHub room" : kind === "kef" ? "KEF speaker" : "Sonos room";
    }
</script>

{#if music.sources.length > 1}
    <div class="p-sources" role="group" aria-label="Room">
        {#each music.sources as s (s.key)}
            {@const chosen = featured?.key === s.key}
            {@const speakers = s.members?.length ?? 1}
            <button
                class="p-chip"
                class:active={chosen}
                aria-pressed={chosen}
                title={chipTitle(s.kind)}
                onclick={() => (music.selected = s.key)}
            >
                <!-- Which room is playing, readable from across the room —
                     without having to select each one to find out. -->
                {#if s.playing}<span class="p-chipwave"><Waveform /></span>{/if}
                <span class="p-chipname">{s.title}</span>
                <!-- How far the chosen room reaches. Only on the chosen one:
                     it is the room every other control on the surface is
                     pointed at, and a count on every chip is a column of
                     numbers nobody is comparing. -->
                {#if chosen && speakers > 1}
                    <span class="p-chipn mono">{speakers} spkrs</span>
                {/if}
            </button>
        {/each}
    </div>
{/if}

<style>
    /* One line on every surface that carries this row: past what fits, it
       scrolls sideways — one axis, and the part-chip left showing at the
       edge is what says there is more. It used to wrap, which on the
       dashboard band cost the music band a row of its height for the control
       touched least, and made the same row behave one way on the band and
       another on the depth's header.
       The chips do not shrink to fit. A room grid's tiles share the width
       down to a floor because they are equal cells (§16); a chip is its own
       name, and seven of them shrunk to fit a header read "Kit…", "O…",
       "P…" — a row of rooms nobody can tell apart is worse than a row you
       have to push. So every chip on screen is legible and the rest are one
       swipe away. */
    .p-sources {
        display: flex;
        flex-wrap: nowrap;
        gap: var(--space-2);
        min-width: 0;
        overflow-x: auto;
        overflow-y: hidden;
        scrollbar-width: none;
        /* Room for the focus ring, which an overflow scroller would
           otherwise clip against the chips' own edge. */
        padding-block: 3px;
        margin-block: -3px;
    }
    .p-sources::-webkit-scrollbar {
        display: none;
    }
    .p-chip {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        flex: 0 0 auto;
        min-width: 0;
        padding: 8px 14px;
        border-radius: var(--r-pill);
        border: 1px solid var(--hairline);
        background: var(--card);
        color: var(--text-mute);
        font-family: inherit;
        font-size: 13px;
        font-weight: 500;
        white-space: nowrap;
        cursor: pointer;
        transition:
            background var(--t-fast),
            color var(--t-fast),
            border-color var(--t-fast),
            transform var(--t-fast);
    }
    .p-chip:active {
        transform: scale(0.95);
        transition-duration: 80ms;
    }
    /* Chosen is a ring and a brighter ink, which is how the panel says
       *chosen* everywhere else (§16: the Rooms pane's chosen room gets a
       ring, and playing gets the gradient). The chips used to invert to
       solid `--text`, which is the register the pane switcher beside them
       uses for *its* active segment — two different pickers on one header
       shouting the same way. Playing is still the waveform, and the two
       states stay independent. */
    .p-chip.active {
        background: var(--card-2);
        color: var(--text);
        border-color: var(--border-strong);
        box-shadow: inset 0 0 0 1px var(--border-strong);
    }
    .p-chipwave {
        display: inline-flex;
        margin-right: 1px;
        flex-shrink: 0;
    }
    /* Only a genuinely long name gives, and only against itself: "Upstairs
       landing and hall" is one room's problem, not the row's. */
    .p-chipname {
        min-width: 0;
        max-width: 22ch;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .p-chipn {
        flex-shrink: 0;
        font-size: 11.5px;
        font-weight: 500;
        color: var(--text-dim);
    }
    .p-chip:focus-visible {
        outline: none;
        box-shadow: var(--focus-ring);
    }

    /* Distance-scaled targets: this is a wall, so every chip on it clears
       the §2 floor rather than inheriting a phone's sizing. */
    @media (pointer: coarse) {
        .p-chip {
            min-height: 44px;
            padding-inline: 16px;
        }
    }
</style>
