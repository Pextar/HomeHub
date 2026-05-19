<!--
  Compact room pill. Single row on every breakpoint:
    [name | "n/m on" meta] [On] [Off]

  Designed to fit two-up on phones without dominating the dashboard the way
  the previous "card with stacked buttons" layout did. The whole row also
  acts as a "navigate to this room's devices" hint via the press state —
  but actions are kept on dedicated buttons so accidental taps don't blast
  power changes.
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
    <div class="info">
        <div class="name" title={room.name}>{room.name}</div>
        <div class="meta">
            <span class="count" class:dim={!anyOn}>
                {room.on}<span class="slash">/{room.sockets}</span>
            </span>
            <span class="status">{allOn ? "all on" : anyOn ? "on" : "off"}</span>
        </div>
    </div>
    <div class="actions">
        <button class="icon-btn on-btn" title="Turn all on" aria-label="Turn all on"
            onclick={() => runAction(() => api.roomOn(room.name), `${room.name} on`)}>
            <Icon name="sun" size={16} />
        </button>
        <button class="icon-btn off-btn" title="Turn all off" aria-label="Turn all off"
            onclick={() => runAction(() => api.roomOff(room.name), `${room.name} off`)}>
            <Icon name="moon" size={16} />
        </button>
    </div>
</div>

<style>
    .room {
        background: var(--surface);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        padding: var(--space-2) var(--space-3);
        display: flex;
        align-items: center;
        gap: var(--space-2);
        min-height: 56px;
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

    .info { flex: 1; min-width: 0; }
    .name {
        font-weight: 600;
        font-size: 14px;
        line-height: 1.2;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
    .meta {
        display: flex;
        align-items: baseline;
        gap: 6px;
        margin-top: 2px;
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
    .icon-btn {
        all: unset;
        display: grid;
        place-items: center;
        width: 34px; height: 34px;
        border-radius: var(--radius-sm);
        cursor: pointer;
        color: var(--text-muted);
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        transition: background 0.15s, color 0.15s, border-color 0.15s;
    }
    .icon-btn:hover { color: var(--text); border-color: var(--border-strong); }
    .icon-btn:focus-visible { outline: 2px solid var(--accent, #60a5fa); outline-offset: 1px; }
    .icon-btn:active { transform: scale(0.94); }
    .on-btn:hover  { color: var(--success); border-color: var(--success); }
    .off-btn:hover { color: var(--danger);  border-color: var(--danger);  }
</style>
