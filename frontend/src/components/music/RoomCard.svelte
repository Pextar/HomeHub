<script lang="ts">
    /**
     * A room, as one card.
     *
     * This replaces four things that all described the same object at
     * different sizes and in different places: the "Playing now" card, the
     * zone card, the small room chip, and the draggable puck one sheet down.
     * Having four of them is why the same speaker could appear three times on
     * one screen under three names, and why grouping lived somewhere the rooms
     * weren't.
     *
     * One card, one place, and every gesture on the same target:
     *
     *   tap        — focus this room (the hero above switches to it)
     *   tap again  — open its player
     *   drag       — drop it on another room to play them together
     *   play       — start or stop it, without leaving the grid
     *
     * It also renders the **travelling ghost**, via `ghost`. Same component
     * because it is the same object; the ghost is inert and `position: fixed`,
     * so hit-testing under the finger finds the card beneath it rather than
     * itself.
     */
    import Icon from "../Icon.svelte";
    import Waveform from "./Waveform.svelte";
    import ProgressLine from "./ProgressLine.svelte";
    import type { Room, RoomsModel } from "../../lib/music/rooms.svelte";
    import type { RoomDrag } from "../../lib/music/room-drag.svelte";

    let {
        ghost = undefined,
        room = undefined,
        rooms = undefined,
        /** The one the hero is showing. */
        focused = false,
        /** Dimmed in place while its ghost does the travelling. */
        lifted = false,
        /** Held by the keyboard, not yet dropped. */
        held = false,
        /** The pointer is over this one and it would take the drop. */
        dropping = false,
        /** A candidate while something is held on the keyboard. */
        aiming = false,
        /** The held room's name, for the "Drop X here" copy. */
        grabbedName = "",
        onSelect = undefined,
        onToggle = undefined,
        onPointerDown = undefined,
        onPointerMove = undefined,
        onPointerUp = undefined,
        onPointerCancel = undefined,
        onClickCapture = undefined,
        onKeyDown = undefined,
    }: {
        ghost?: RoomDrag;
        room?: Room;
        rooms?: RoomsModel;
        focused?: boolean;
        lifted?: boolean;
        held?: boolean;
        dropping?: boolean;
        aiming?: boolean;
        grabbedName?: string;
        onSelect?: () => void;
        onToggle?: () => void;
        onPointerDown?: (e: PointerEvent) => void;
        onPointerMove?: (e: PointerEvent) => void;
        onPointerUp?: () => void;
        onPointerCancel?: () => void;
        onClickCapture?: (e: MouseEvent) => void;
        onKeyDown?: (e: KeyboardEvent) => void;
    } = $props();

    const playing = $derived(!!room && !!rooms && rooms.isPlaying(room));
    const art = $derived(room && rooms ? rooms.art(room) : undefined);
    const now = $derived(room && rooms ? rooms.nowLine(room) : "");
    const sub = $derived(room && rooms ? rooms.subLine(room) : "");
    /** The second line: what it is made of, or what it is playing. */
    const memberLine = $derived(room && rooms ? rooms.memberLine(room) : "");
</script>

{#if ghost}
    <div
        class="rc rc-ghost"
        class:playing={ghost.playing}
        aria-hidden="true"
        style:width="{ghost.w}px"
        style:height="{ghost.h}px"
        style:left="{ghost.x}px"
        style:top="{ghost.y}px"
    >
        <div class="rc-top">
            <span class="rc-art placeholder">
                {#if ghost.playing}<Waveform ink />{:else}<Icon name="speaker" size={17} />{/if}
            </span>
            <span class="rc-id">
                <span class="rc-name">{ghost.name}</span>
                <span class="rc-line">{ghost.sub}</span>
            </span>
        </div>
    </div>
{:else if room && rooms}
    <div
        class="rc"
        class:playing
        class:focused
        class:held={held || lifted}
        class:lifted
        class:drop={dropping}
        class:aiming
        data-room={room.key}
    >
        <!-- The whole card is the target for tap and for drag; the play button
             below sits above it and stops its own clicks. -->
        <button
            class="rc-hit"
            aria-keyshortcuts="g"
            aria-label={aiming
                ? `Play ${grabbedName} with ${room.name}`
                : focused
                  ? `Open ${room.name}`
                  : `${room.name} — ${now}. Tap to focus, or press G to pick it up for grouping.`}
            onpointerdown={onPointerDown}
            onpointermove={onPointerMove}
            onpointerup={onPointerUp}
            onpointercancel={onPointerCancel}
            onclickcapture={onClickCapture}
            onkeydown={onKeyDown}
            onclick={onSelect}
        ></button>

        <div class="rc-top">
            {#if art}
                <img class="rc-art" src={art} alt="" loading="lazy" />
            {:else}
                <span class="rc-art placeholder">
                    <Icon name={room.kind === "zone" ? "groups" : "speaker"} size={17} />
                </span>
            {/if}
            <span class="rc-id">
                <span class="rc-name">{room.name}</span>
                <span class="rc-line">
                    {#if room.grouped}
                        <span class="rc-grouped"><Icon name="groups" size={11} /></span>
                        <span class="mono">{room.members.length}</span> speakers · {memberLine}
                    {:else}
                        {memberLine}
                    {/if}
                </span>
            </span>
            <!-- Above the hit layer, so a tap on it plays rather than focuses. -->
            <button
                class="rc-play"
                class:on={playing}
                disabled={rooms.playBusy(room) || !room.reachable}
                aria-label={playing ? `Pause ${room.name}` : `Play ${room.name}`}
                onclick={(e) => {
                    e.stopPropagation();
                    onToggle?.();
                }}
            >
                <Icon name={playing ? "pause" : "play"} size={16} />
            </button>
        </div>

        <div class="rc-now">
            {#if aiming}
                <span class="rc-aim">Drop {grabbedName} here</span>
            {:else if held || lifted}
                <span class="rc-aim">Held — drop it on another room</span>
            {:else}
                {#if playing}<Waveform ink={playing} />{/if}
                <span class="rc-track">{now}</span>
                {#if sub}<span class="rc-artist">{sub}</span>{/if}
            {/if}
        </div>

        <ProgressLine value={rooms.progress(room)} />
    </div>
{/if}

<style>
    .rc {
        position: relative;
        overflow: hidden;
        display: flex; flex-direction: column; gap: 10px;
        padding: 14px;
        background: var(--card); border: 1px solid var(--hairline);
        border-radius: var(--r-lg);
        color: var(--text);
        /* Vertical panning still belongs to the page — a card only lifts on a
           press that stays put (see HOLD_MS in room-drag). */
        touch-action: pan-y;
        -webkit-user-select: none; user-select: none;
        transition: border-color var(--t-fast), box-shadow var(--t-fast),
            opacity var(--t-fast), transform var(--t-fast);
    }
    .rc.playing { background: var(--tile-on-gradient); border-color: var(--tile-on-border); }
    /* The one the hero is showing. A ring rather than a fill: "you are looking
       at this" is a different claim from "this is playing", and both can be
       true of the same card at once. */
    .rc.focused { border-color: var(--on); }
    .rc.focused::after {
        content: "";
        position: absolute; inset: 0;
        border-radius: inherit;
        box-shadow: inset 0 0 0 1px var(--on);
        pointer-events: none;
    }

    /* The tap-and-drag layer: the whole card, under everything else. */
    .rc-hit {
        position: absolute; inset: 0;
        background: none; border: 0; padding: 0; margin: 0;
        cursor: grab;
        border-radius: inherit;
    }
    /* Inset, because the card clips its own overflow to keep the progress
       hairline inside its corners — an outer ring would be shaved off. */
    .rc-hit:focus-visible {
        box-shadow: inset 0 0 0 3px var(--on-glow);
        outline: none;
    }
    .rc:active { transform: scale(0.99); }
    .rc.lifted { opacity: 0.35; transform: none; }
    .rc.lifted .rc-hit { cursor: grabbing; }
    .rc.held:not(.lifted) { border-color: var(--on); }
    /* Where it would land. Same ring for the pointer's drop target and the
       keyboard's candidates, because they mean the same thing. */
    .rc.drop {
        border-color: var(--on);
        box-shadow: 0 0 0 2px var(--on), 0 0 22px -4px var(--on-glow);
    }
    .rc.aiming { border-color: var(--tile-on-border); }

    /* The travelling copy. */
    .rc-ghost {
        position: fixed; z-index: 200;
        pointer-events: none;
        box-shadow: var(--shadow-lg);
        opacity: 1;
        transform: scale(1.03);
        transition: none;
    }

    /* Everything above the hit layer is inert but the play button, so a tap on
       the name or the art still lands on the card's own gesture. */
    .rc-top {
        position: relative;
        display: flex; align-items: center; gap: var(--space-3);
        pointer-events: none;
    }
    .rc-art {
        width: 44px; height: 44px; border-radius: var(--r-md); flex-shrink: 0;
        object-fit: cover; background: var(--card-3);
    }
    span.rc-art { display: grid; place-items: center; color: var(--text-mute); }
    .rc.playing span.rc-art { background: var(--on); color: var(--primary-fg); }
    .rc-id { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
    .rc-name {
        font-size: 15px; font-weight: 600; letter-spacing: -0.01em;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .rc-line {
        display: flex; align-items: center; gap: 5px;
        font-size: 11.5px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .rc-grouped { display: flex; color: var(--text-dim); flex-shrink: 0; }

    .rc-play {
        position: relative;
        pointer-events: auto;
        width: 40px; height: 40px; flex-shrink: 0;
        display: grid; place-items: center;
        background: var(--card-3); border: 1px solid var(--hairline);
        border-radius: 50%;
        color: var(--text); cursor: pointer;
        transition: background var(--t-fast), color var(--t-fast), box-shadow var(--t-fast);
    }
    .rc-play.on {
        background: var(--on); border-color: transparent; color: var(--primary-fg);
        box-shadow: 0 0 18px -4px var(--on-glow);
    }
    .rc-play:disabled { opacity: 0.5; cursor: default; }
    @media (hover: hover) {
        .rc-play:not(:disabled):hover { border-color: var(--border-strong); }
        .rc-play.on:not(:disabled):hover { background: var(--primary-hover); }
    }
    @media (pointer: coarse) {
        .rc-play { width: 44px; height: 44px; }
    }

    .rc-now {
        position: relative;
        display: flex; align-items: baseline; gap: 7px;
        min-height: 17px;
        font-size: 12.5px;
        pointer-events: none;
    }
    .rc-track {
        color: var(--text);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
        flex-shrink: 1;
    }
    .rc-artist {
        color: var(--text-mute); font-size: 11.5px;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
        flex-shrink: 2;
    }
    .rc-aim { color: var(--on); font-size: 12.5px; }

    @media (prefers-reduced-motion: reduce) {
        .rc { transition-duration: 0.001ms; }
    }
</style>
