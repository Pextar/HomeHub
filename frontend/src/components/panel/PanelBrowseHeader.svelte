<script lang="ts">
    /**
     * The music depth's own band (DESIGN.md §16), drawn the way the
     * dashboard's status strip is: a fixed 72px row, edge to edge, divided
     * from the body by a hairline rather than floated over it.
     *
     * It carries everything about the surface that isn't the work itself —
     * the way back, where a tap plays, and which pane the work area is
     * showing. Two of those three used to live in the work column, and both
     * were paying for it out of the results: the room chips cost three rows
     * of the cover's height stacked in the player column, and the pane
     * switcher was a band above the results, where on a 656px column every
     * band above the results is a result you can't see.
     *
     * On the header they cost the column nothing and hold the same place
     * whichever pane is up — which matters most for the chips, since where a
     * tap plays must not move because a search started.
     */
    import Icon from "../Icon.svelte";
    import PanelRoomChips from "./PanelRoomChips.svelte";
    import type { PanelMusicStore, PanelSource } from "../../lib/panel-music/types";

    export type Pane = "search" | "queue" | "rooms";

    let {
        music,
        featured,
        pane,
        queueCount,
        onBack,
        onPane,
    }: {
        music: PanelMusicStore;
        featured: PanelSource | undefined;
        pane: Pane;
        queueCount: number;
        onBack: () => void;
        onPane: (p: Pane) => void;
    } = $props();
</script>

<header class="b-head">
    <button class="back" onclick={onBack} aria-label="Back to the panel">
        <Icon name="chevronLeft" size={18} />
    </button>
    <h2 class="sr-only">Music</h2>
    <PanelRoomChips {music} />
    <div class="p-panes" role="group" aria-label="Music panes">
        <button
            class="p-pane"
            class:active={pane === "search"}
            aria-pressed={pane === "search"}
            onclick={() => onPane("search")}
        >
            Search
        </button>
        <button
            class="p-pane"
            class:active={pane === "queue"}
            aria-pressed={pane === "queue"}
            onclick={() => onPane("queue")}
        >
            Queue{#if featured?.kind === "sonos" && queueCount > 0}
                <span class="mono">{queueCount}</span>{/if}
        </button>
        <button
            class="p-pane"
            class:active={pane === "rooms"}
            aria-pressed={pane === "rooms"}
            onclick={() => onPane("rooms")}
        >
            Rooms <span class="mono">{music.sources.length}</span>
        </button>
    </div>
</header>

<style>
    .b-head {
        height: 72px;
        flex-shrink: 0;
        display: flex;
        align-items: center;
        gap: var(--space-4);
        min-width: 0;
        padding: 0 var(--space-8);
        border-bottom: 1px solid var(--hairline);
    }
    /* The chip row keeps its own one-line, shrink-then-scroll behaviour
       (PanelRoomChips) on every surface that carries it; here it just takes
       the space between the back chip and the pane switcher. */
    .b-head :global(.p-sources) {
        flex: 1 1 auto;
    }
    /* The way back to the dashboard depth: a round chevron chip on the
       leading edge, the same shape the full player's header wears. */
    .back {
        width: 40px;
        height: 40px;
        flex-shrink: 0;
        display: grid;
        place-items: center;
        border-radius: 50%;
        border: 1px solid var(--hairline);
        background: var(--card-2);
        color: var(--text);
        cursor: pointer;
        transition:
            background var(--t-fast),
            color var(--t-fast),
            transform var(--t-fast);
    }
    .back:active {
        transform: scale(0.94);
        transition-duration: 80ms;
    }

    /* The pane switcher as one segmented control: a track around the three,
       so they read as one choice rather than as three chips that happen to
       sit together. */
    .p-panes {
        display: flex;
        gap: 6px;
        flex-shrink: 0;
        padding: 4px;
        border-radius: var(--r-pill);
        border: 1px solid var(--hairline);
        background: var(--card-2);
    }
    .p-pane {
        display: inline-flex;
        align-items: center;
        gap: 5px;
        min-height: 36px;
        padding: 0 16px;
        border: 0;
        border-radius: var(--r-pill);
        background: none;
        color: var(--text-mute);
        font-family: inherit;
        font-size: 13px;
        font-weight: 600;
        cursor: pointer;
        transition:
            background var(--t-fast),
            color var(--t-fast);
    }
    .p-pane.active {
        background: var(--text);
        color: var(--bg);
    }
    .p-pane:focus-visible {
        outline: none;
        box-shadow: var(--focus-ring);
    }

    /* Distance-scaled targets (§2's floor, §16's reason): this is a wall, so
       the depth's own controls clear 44px rather than inheriting a phone's
       sizing. */
    @media (pointer: coarse) {
        .back {
            width: 44px;
            height: 44px;
        }
        .p-pane {
            min-height: 44px;
        }
    }

    /* Portrait / narrow fallback: the band wraps rather than squeezing the
       chips off the end (the panel is designed landscape-first). */
    @media (orientation: portrait), (max-width: 760px) {
        .b-head {
            height: auto;
            flex-wrap: wrap;
            padding: var(--space-4) var(--space-5);
        }
    }
</style>
