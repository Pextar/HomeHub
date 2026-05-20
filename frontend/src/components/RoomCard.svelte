<!--
  Compact room card. Two-row layout:
    [name (full width)           ]
    ["n/m on" meta] [On] [Off]

  On mobile, the on/off buttons grow to 44 px tall to hit the iOS HIG
  minimum touch target, and the card itself has a coarser active state.
-->
<script lang="ts">
    import Icon from "./Icon.svelte";
    import { api } from "../lib/api";
    import { runAction } from "../lib/utils";
    import type { RoomSummary } from "../lib/types";

    interface Props { room: RoomSummary; }
    let { room }: Props = $props();

    const anyOn = $derived(room.on > 0);
    const allOn = $derived(room.on === room.sockets && room.sockets > 0);
</script>

<div class="room" class:on={anyOn}>
    <div class="name" title={room.name}>{room.name}</div>
    <div class="bottom">
        <div class="meta">
            <span class="count" class:dim={!anyOn}>
                {room.on}<span class="slash">/{room.sockets}</span>
            </span>
            <span class="status">{allOn ? "all on" : anyOn ? "on" : "off"}</span>
        </div>
        <div class="actions">
            <button class="act-btn on-btn" title="Turn all on" aria-label="Turn all on"
                onclick={() => runAction(() => api.roomOn(room.name), `${room.name} on`)}>
                <Icon name="sun" size={16} />
            </button>
            <button class="act-btn off-btn" title="Turn all off" aria-label="Turn all off"
                onclick={() => runAction(() => api.roomOff(room.name), `${room.name} off`)}>
                <Icon name="moon" size={16} />
            </button>
        </div>
    </div>
</div>

<style>
    .room {
        background: var(--surface);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        padding: var(--space-3);
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
        min-width: 0;
        transition: border-color var(--t-fast), background var(--t-fast);
    }
    .room.on {
        border-color: var(--success);
        box-shadow: inset 3px 0 0 var(--success);
    }
    @media (hover: hover) {
        .room:hover { border-color: var(--border-strong); background: var(--bg-elevated); }
        .room.on:hover { border-color: var(--success); }
    }

    .name {
        font-weight: 600;
        font-size: 14px;
        line-height: 1.2;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
    .bottom {
        display: flex;
        align-items: center;
        gap: var(--space-2);
    }
    .meta {
        display: flex;
        align-items: baseline;
        gap: 6px;
        flex: 1;
        font-size: 11px;
    }
    .count {
        font-variant-numeric: tabular-nums;
        font-weight: 600;
        color: var(--success);
    }
    .count.dim { color: var(--text-faint); }
    .slash { color: var(--text-faint); font-weight: 400; }
    .status {
        color: var(--text-muted);
        text-transform: uppercase;
        letter-spacing: 0.04em;
        font-size: 10px;
    }

    .actions {
        display: flex;
        gap: 4px;
        flex-shrink: 0;
    }

    /* Base — 34 px visible, same as before */
    .act-btn {
        all: unset;
        display: grid;
        place-items: center;
        width: 34px; height: 34px;
        border-radius: var(--radius-sm);
        cursor: pointer;
        color: var(--text-muted);
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        touch-action: manipulation;
        transition: background 0.15s, color 0.15s, border-color 0.15s, transform 0.1s;
    }
    .act-btn:hover { color: var(--text); border-color: var(--border-strong); }
    .act-btn:active { transform: scale(0.90); }
    .act-btn:focus-visible { outline: 2px solid var(--primary); outline-offset: 2px; }
    .on-btn:hover  { color: var(--success); border-color: var(--success); background: var(--success-soft); }
    .off-btn:hover { color: var(--danger);  border-color: var(--danger);  background: var(--danger-soft); }

    /* Touch screens: expand to 44 × 44 minimum target */
    @media (pointer: coarse) {
        .room { padding: var(--space-3) var(--space-3); }
        .name { font-size: 15px; }
        .act-btn {
            width: 44px; height: 44px;
            border-radius: var(--radius-md);
        }
        .meta { font-size: 12px; }
    }
</style>
