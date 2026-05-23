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

    // Warm / cool / neutral surface category, picked by a stable djb2 hash of
    // the room name so each room keeps a consistent tone across reorders.
    const TONES = ["warm", "cool", "neutral"] as const;

    function nameHash(s: string): number {
        let h = 5381;
        for (let i = 0; i < s.length; i++) h = ((h << 5) + h) ^ s.charCodeAt(i);
        return Math.abs(h);
    }

    const tone = $derived(TONES[nameHash(room.name) % TONES.length]);
</script>

<div class="room" class:on={anyOn} data-tone={tone}>
    <div class="body">
        <div class="name" title={room.name}>{room.name}</div>
        <div class="meta">
            <span class="count" class:dim={!anyOn}>{room.on}/{room.sockets}</span>
            <span class="status-label">{allOn ? "all on" : anyOn ? "on" : "off"}</span>
        </div>
    </div>
    <div class="actions">
        <button class="act-btn on-btn" aria-label="Turn {room.name} on"
            disabled={allOn}
            onclick={() => runAction(() => api.roomOn(room.name), `${room.name} on`)}>
            <Icon name="sun" size={14} />
            <span>On</span>
        </button>
        <button class="act-btn off-btn" aria-label="Turn {room.name} off"
            disabled={allOff}
            onclick={() => runAction(() => api.roomOff(room.name), `${room.name} off`)}>
            <Icon name="moon" size={14} />
            <span>Off</span>
        </button>
    </div>
</div>

<style>
    .room {
        background: var(--card);
        border: 1px solid var(--hairline);
        border-radius: var(--radius-lg);
        display: flex;
        flex-direction: column;
        overflow: hidden;          /* clip button radius at card edge */
        transition: border-color var(--t-fast), background var(--t-med), box-shadow var(--t-fast);
    }

    /* Active rooms light up with a warm / cool / neutral gradient surface. */
    .room.on { border-color: transparent; }
    .room.on[data-tone="warm"]    { background: linear-gradient(155deg, #3a2f1f 0%, #271f14 100%); }
    .room.on[data-tone="cool"]    { background: linear-gradient(155deg, #1f2a30 0%, #161c20 100%); }
    .room.on[data-tone="neutral"] { background: linear-gradient(155deg, #2a2620 0%, #1d1a15 100%); }
    :global([data-theme="light"]) .room.on[data-tone="warm"]    { background: linear-gradient(155deg, #fff2dc 0%, #ffe9c6 100%); }
    :global([data-theme="light"]) .room.on[data-tone="cool"]    { background: linear-gradient(155deg, #e6eef2 0%, #dbe7ed 100%); }
    :global([data-theme="light"]) .room.on[data-tone="neutral"] { background: linear-gradient(155deg, #f3efe6 0%, #ebe4d6 100%); }

    @media (hover: hover) {
        .room:hover { border-color: var(--border-strong); }
        .room.on:hover { border-color: rgba(245, 189, 110, 0.25); }
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
        font-family: var(--font-mono);
        font-variant-numeric: tabular-nums;
        font-weight: 600;
        color: var(--on);
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
    .act-btn:disabled { opacity: 0.4; cursor: not-allowed; }

    /* Both buttons neutral by default — no false "active" impression */
    .on-btn  { color: var(--text-muted); }
    .off-btn { color: var(--text-muted); }

    /* Hover feedback: amber for On, cool for Off */
    .on-btn:hover  { color: var(--on);   background: var(--on-soft);   }
    .off-btn:hover { color: var(--cool); background: var(--cool-soft); }

    /* When the room has devices on, give the On button a subtle tint
       so users can see it's the "current" state without it screaming */
    .room.on .on-btn { color: var(--on); }
    .room.on .actions { border-top-color: rgba(245, 189, 110, 0.15); }
    .room.on .act-btn + .act-btn { border-left-color: rgba(245, 189, 110, 0.15); }

    @media (pointer: coarse) {
        .act-btn { min-height: 48px; font-size: 14px; }
    }
</style>
