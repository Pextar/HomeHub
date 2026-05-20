<!--
  Room card — two-part layout:
    ┌──────────────────────────┐
    │▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓│  ← 3 px accent bar (colour from room name hash)
    │ Entré                    │
    │ 0/2 · off                │
    │ [☀ On    ] [ 🌙 Off   ] │
    └──────────────────────────┘
  The two text+icon buttons span the full card width and are 44px tall —
  easy to tap, visually balanced, and clearly labelled.
-->
<script lang="ts">
    import Icon from "./Icon.svelte";
    import { api } from "../lib/api";
    import { runAction } from "../lib/utils";
    import type { RoomSummary } from "../lib/types";

    interface Props { room: RoomSummary; }
    let { room }: Props = $props();

    const anyOn  = $derived(room.on > 0);
    const allOn  = $derived(room.on === room.sockets && room.sockets > 0);
    const allOff = $derived(room.on === 0);

    // Eight vivid accent colours that work well on the dark background.
    // A tiny djb2 hash of the room name makes each room's colour stable
    // even if rooms are reordered or new rooms are added.
    const ACCENTS = [
        "#818cf8", // indigo
        "#34d399", // emerald
        "#fb923c", // orange
        "#60a5fa", // sky
        "#f472b6", // pink
        "#c084fc", // violet
        "#fbbf24", // amber
        "#4ade80", // lime
    ];

    function nameHash(s: string): number {
        let h = 5381;
        for (let i = 0; i < s.length; i++) h = ((h << 5) + h) ^ s.charCodeAt(i);
        return Math.abs(h);
    }

    const accent = $derived(ACCENTS[nameHash(room.name) % ACCENTS.length]);
</script>

<div class="room" class:on={anyOn} style="--room-accent: {accent}">
    <div class="body">
        <div class="name" title={room.name}>{room.name}</div>
        <div class="meta">
            <span class="count" class:dim={!anyOn}>{room.on}/{room.sockets}</span>
            <span class="status-label">{allOn ? "all on" : anyOn ? "on" : "off"}</span>
        </div>
    </div>
    <div class="actions">
        <button class="act-btn on-btn" aria-label="Turn {room.name} on"
            onclick={() => runAction(() => api.roomOn(room.name), `${room.name} on`)}>
            <Icon name="sun" size={14} />
            <span>On</span>
        </button>
        <button class="act-btn off-btn" aria-label="Turn {room.name} off"
            onclick={() => runAction(() => api.roomOff(room.name), `${room.name} off`)}>
            <Icon name="moon" size={14} />
            <span>Off</span>
        </button>
    </div>
</div>

<style>
    .room {
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        border-radius: var(--radius-lg);
        display: flex;
        flex-direction: column;
        overflow: hidden;          /* clip button radius at card edge */
        transition: border-color var(--t-fast), box-shadow var(--t-fast);
    }

    /* Coloured top accent bar — first flex child, clipped by border-radius */
    .room::before {
        content: '';
        display: block;
        height: 3px;
        flex-shrink: 0;
        background: var(--room-accent, var(--primary));
        opacity: 0.85;
        transition: opacity var(--t-fast);
    }
    .room:hover::before { opacity: 1; }

    .room.on {
        border-color: var(--success);
        box-shadow: inset 3px 0 0 var(--success);
    }
    @media (hover: hover) {
        .room:hover { border-color: var(--border-strong); }
        .room.on:hover { border-color: var(--success); }
    }

    /* ── Info section ── */
    .body {
        padding: var(--space-3) var(--space-3) var(--space-2);
        display: flex;
        flex-direction: column;
        gap: 2px;
        flex: 1;
    }
    .name {
        font-weight: 700;
        font-size: 15px;
        line-height: 1.2;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
    .meta {
        display: flex;
        align-items: baseline;
        gap: 5px;
        font-size: 12px;
    }
    .count {
        font-variant-numeric: tabular-nums;
        font-weight: 600;
        color: var(--success);
    }
    .count.dim { color: var(--text-faint); }
    .status-label {
        color: var(--text-muted);
        text-transform: uppercase;
        letter-spacing: 0.05em;
        font-size: 10px;
    }

    /* ── Button row ── */
    .actions {
        display: grid;
        grid-template-columns: 1fr 1fr;
        border-top: 1px solid var(--border);
    }

    .act-btn {
        all: unset;
        display: flex;
        align-items: center;
        justify-content: center;
        gap: var(--space-1);
        padding: 10px 0;
        min-height: 44px;
        font-size: 13px;
        font-weight: 600;
        cursor: pointer;
        touch-action: manipulation;
        transition: background var(--t-fast), color var(--t-fast);
        user-select: none;
    }
    .act-btn + .act-btn {
        border-left: 1px solid var(--border);
    }
    .act-btn:active { background: var(--surface-hover); }
    .act-btn:focus-visible { outline: 2px solid var(--primary); outline-offset: -2px; }

    /* On button: warm green — signals "turn on" at a glance */
    .on-btn {
        color: var(--success);
        background: var(--success-soft);
    }
    .on-btn:hover { background: color-mix(in srgb, var(--success-soft) 180%, transparent); }

    /* Off button: muted default, red on hover */
    .off-btn { color: var(--text-muted); }
    .off-btn:hover { color: var(--danger); background: var(--danger-soft); }

    @media (pointer: coarse) {
        .act-btn { min-height: 48px; font-size: 14px; }
    }
</style>
