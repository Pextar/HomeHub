<script lang="ts">
    /**
     * A room in the Zones grid. One object, two gestures on the same target:
     * tap opens that room's player, drag it onto another room groups them.
     * There is no second control — dragging one thing onto another *is* the
     * grouping gesture, so the select circle that used to sit in the corner
     * has nothing left to say.
     *
     * It also renders the **travelling ghost**, via `ghost`. Same component
     * because it is the same object, and because the ghost needs every one of
     * the base styles below; the alternative was a second file holding a copy
     * of them. The ghost is inert and `position: fixed`, so hit-testing under
     * the finger finds the room beneath it rather than itself — which is also
     * why the caller must render it *outside* the sheet, whose drag transform
     * would otherwise re-anchor it.
     */
    import Icon from "../Icon.svelte";
    import Waveform from "./Waveform.svelte";
    import type { SonosSpeakerView } from "../../lib/types";
    import type { PuckDrag } from "../../lib/music/puck-drag.svelte";

    let {
        ghost = undefined,
        speaker = undefined,
        playing = false,
        /** What the room is playing, or "Idle". */
        sub = "",
        /** No zone means the topology hasn't placed it — nothing to open. */
        openable = true,
        /** Dimmed in place while its ghost does the travelling. */
        lifted = false,
        /** Held by the keyboard, not yet dropped. */
        held = false,
        /** The pointer is over this one. */
        dropping = false,
        /** A candidate while something is held on the keyboard. */
        aiming = false,
        /** The held room's name, for the "Drop X here" copy. */
        grabbedName = "",
        onOpen = undefined,
        onPointerDown = undefined,
        onPointerMove = undefined,
        onPointerUp = undefined,
        onPointerCancel = undefined,
        onClickCapture = undefined,
        onKeyDown = undefined,
    }: {
        ghost?: PuckDrag;
        speaker?: SonosSpeakerView;
        playing?: boolean;
        sub?: string;
        openable?: boolean;
        lifted?: boolean;
        held?: boolean;
        dropping?: boolean;
        aiming?: boolean;
        grabbedName?: string;
        onOpen?: () => void;
        onPointerDown?: (e: PointerEvent) => void;
        onPointerMove?: (e: PointerEvent) => void;
        onPointerUp?: () => void;
        onPointerCancel?: () => void;
        onClickCapture?: (e: MouseEvent) => void;
        onKeyDown?: (e: KeyboardEvent) => void;
    } = $props();
</script>

{#if ghost}
    <div
        class="puck puck-ghost"
        class:playing={ghost.playing}
        aria-hidden="true"
        style:width="{ghost.w}px"
        style:height="{ghost.h}px"
        style:left="{ghost.x}px"
        style:top="{ghost.y}px"
    >
        <span class="puck-icon">
            {#if ghost.playing}<Waveform ink />{:else}<Icon name="speaker" size={16} />{/if}
        </span>
        <span class="puck-body">
            <span class="puck-name">{ghost.name}</span>
            <span class="puck-sub">{ghost.sub}</span>
        </span>
    </div>
{:else if speaker}
    <button
        class="puck"
        class:playing
        class:held={held || lifted}
        class:lifted
        class:drop={dropping}
        class:aiming
        data-speaker={speaker.id}
        disabled={!openable}
        aria-keyshortcuts="g"
        aria-label={aiming
            ? `Group ${grabbedName} with ${speaker.name}`
            : `${speaker.name} — open player, or press G to pick it up for grouping`}
        onpointerdown={onPointerDown}
        onpointermove={onPointerMove}
        onpointerup={onPointerUp}
        onpointercancel={onPointerCancel}
        onclickcapture={onClickCapture}
        onkeydown={onKeyDown}
        onclick={onOpen}
    >
        <span class="puck-icon">
            <!-- On the filled amber tile the bars take the tile's ink; amber
                 on amber would be invisible. -->
            {#if playing}<Waveform ink />{:else}<Icon name="speaker" size={16} />{/if}
        </span>
        <!-- Says "this object moves", on hover only and to a pointer only:
             touch has the press-and-hold to discover, and a mouse has nothing
             but the cursor otherwise. Not a control — it takes no pointer
             events, so it can't be mistaken for the select circle that used
             to sit here. -->
        <span class="puck-grip" aria-hidden="true"><Icon name="grip" size={14} /></span>
        <span class="puck-body">
            <span class="puck-name">{speaker.name}</span>
            <span class="puck-sub">
                {#if aiming}
                    Drop {grabbedName} here
                {:else if held || lifted}
                    Held
                {:else}
                    {sub}
                {/if}
            </span>
        </span>
    </button>
{/if}

<style>
    /* One element, one target: tap opens the room, drag groups it. The select
       circle that used to sit in the corner is gone, so the padding that
       reserved its space goes with it. */
    .puck {
        position: relative;
        width: 100%;
        display: flex; flex-direction: column; gap: 10px;
        padding: 14px;
        background: var(--card); border: 1px solid var(--hairline);
        border-radius: var(--r-lg);
        color: var(--text); text-align: left; cursor: grab; font: inherit;
        /* Vertical panning still belongs to the sheet — a puck only lifts on
           a press that stays put (see HOLD_MS). */
        touch-action: pan-y;
        -webkit-user-select: none; user-select: none;
        transition: border-color var(--t-fast), box-shadow var(--t-fast),
            opacity var(--t-fast), transform var(--t-fast);
    }
    .puck.playing { background: var(--tile-on-gradient); border-color: var(--tile-on-border); }
    .puck:active { transform: scale(0.98); }
    .puck:disabled { opacity: 0.6; cursor: default; }
    /* The one being carried: dimmed in place, so the grid keeps its shape
       while the ghost does the travelling. */
    .puck.lifted { opacity: 0.35; cursor: grabbing; transform: none; }
    .puck.held:not(.lifted) { border-color: var(--on); }
    /* Where it would land. Same ring for the pointer's drop target and the
       keyboard's candidates, because they mean the same thing. */
    .puck.drop {
        border-color: var(--on);
        box-shadow: 0 0 0 2px var(--on), 0 0 22px -4px var(--on-glow);
    }
    .puck.aiming { border-color: var(--tile-on-border); }
    .puck.aiming:focus-visible { box-shadow: var(--focus-ring), 0 0 0 2px var(--on); }
    .puck-grip {
        position: absolute; top: 10px; right: 10px;
        display: flex;
        color: var(--text-dim);
        opacity: 0;
        pointer-events: none;
        transition: opacity var(--t-fast);
    }
    @media (hover: hover) {
        /* Not while lifted: the pointer is captured, so the source puck stays
           :hover for the whole drag and would keep the grip lit under the
           ghost that is already carrying it. */
        .puck:not(:disabled):not(.lifted):hover .puck-grip { opacity: 0.7; }
    }
    /* The travelling copy. */
    .puck-ghost {
        position: fixed; z-index: 200;
        pointer-events: none;
        box-shadow: var(--shadow-lg);
        /* Fully opaque and slightly lifted: at 94% the room underneath read
           through the ghost and both sets of text fought. */
        opacity: 1;
        transform: scale(1.03);
        transition: none;
    }
    .puck-icon {
        width: 34px; height: 34px; border-radius: var(--r-md);
        display: grid; place-items: center;
        background: var(--card-3); color: var(--text-mute);
    }
    .puck.playing .puck-icon { background: var(--on); color: var(--primary-fg); }
    .puck-body { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
    /* Clear of the hover grip in the corner. */
    .puck-name { font-size: 14px; font-weight: 600; padding-right: 20px; }
    .puck-sub {
        font-size: 11.5px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    @media (prefers-reduced-motion: reduce) {
        .puck { transition-duration: 0.001ms; }
    }
</style>
