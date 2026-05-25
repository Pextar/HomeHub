<script lang="ts">
    import Icon from "./Icon.svelte";
    import { route } from "../lib/stores.svelte";
    import type { RoomSummary } from "../lib/types";

    interface Props { room: RoomSummary; }
    let { room }: Props = $props();

    const anyOn = $derived(room.on > 0);

    // Warm / cool / neutral surface category, picked by a stable djb2 hash of
    // the room name so each room keeps a consistent tone across reorders.
    const TONES = ["warm", "cool", "neutral"] as const;
    function nameHash(s: string): number {
        let h = 5381;
        for (let i = 0; i < s.length; i++) h = ((h << 5) + h) ^ s.charCodeAt(i);
        return Math.abs(h);
    }
    const tone = $derived(TONES[nameHash(room.name) % TONES.length]);

    // Pick a room icon based on common name keywords (supports English + Swedish).
    type IconName = "bed" | "utensils" | "couch" | "monitor" | "sun" | "sensor" | "home";
    function roomIcon(name: string): IconName {
        const n = name.toLowerCase();
        if (/bed|sov(rum)?/.test(n)) return "bed";
        if (/kitchen|kök|kok|mat(sal)?|dining/.test(n)) return "utensils";
        if (/living|vardags|lounge|sal(ong)?/.test(n)) return "couch";
        if (/office|kontor|studio|study|work/.test(n)) return "monitor";
        if (/outdoor|utom(hus)?|garden|yard|patio|altan|balkong|terr/.test(n)) return "sun";
        if (/bath|wc|toilet|toalett|shower/.test(n)) return "sensor";
        return "home";
    }
    const icon = $derived(roomIcon(room.name));

    function open() {
        route.go("sockets", { room: room.name });
    }
</script>

<button class="room" class:on={anyOn} data-tone={tone} onclick={open}
    aria-label="Open {room.name}">
    <div class="top">
        <span class="ico" class:on={anyOn}><Icon name={icon} size={22} /></span>
        {#if anyOn}
            <span class="on-badge">{room.on}</span>
        {/if}
    </div>
    <div class="body">
        <div class="name" title={room.name}>{room.name}</div>
        <div class="meta">
            <span class="count" class:lit={anyOn}>{room.on}</span><span class="slash"> / {room.sockets}</span> on
        </div>
    </div>
</button>

<style>
    .room {
        all: unset;
        box-sizing: border-box;
        border: 1px solid var(--hairline);
        border-radius: var(--r-lg);
        padding: 14px;
        height: 150px;
        display: flex;
        flex-direction: column;
        justify-content: space-between;
        cursor: pointer;
        touch-action: manipulation;
        transition: border-color var(--t-fast), background var(--t-med), transform var(--t-fast), box-shadow var(--t-fast);
        /* Subtle tone tint even when off */
        background: var(--card);
    }
    .room[data-tone="warm"]    { background: linear-gradient(155deg, #201d15 0%, #1a1811 100%); }
    .room[data-tone="cool"]    { background: linear-gradient(155deg, #161d23 0%, #11171c 100%); }
    .room[data-tone="neutral"] { background: linear-gradient(155deg, #1d1d17 0%, #161611 100%); }

    :global([data-theme="light"]) .room[data-tone="warm"]    { background: linear-gradient(155deg, #fdf6ea 0%, #f8eed8 100%); }
    :global([data-theme="light"]) .room[data-tone="cool"]    { background: linear-gradient(155deg, #eef3f7 0%, #e5edf3 100%); }
    :global([data-theme="light"]) .room[data-tone="neutral"] { background: linear-gradient(155deg, #f5f3ee 0%, #edeae2 100%); }

    .room.on { border-color: transparent; }
    .room.on[data-tone="warm"]    { background: linear-gradient(155deg, #3a2f1f 0%, #271f14 100%); }
    .room.on[data-tone="cool"]    { background: linear-gradient(155deg, #1f2a30 0%, #161c20 100%); }
    .room.on[data-tone="neutral"] { background: linear-gradient(155deg, #2a2620 0%, #1d1a15 100%); }
    :global([data-theme="light"]) .room.on[data-tone="warm"]    { background: linear-gradient(155deg, #fff2dc 0%, #ffe9c6 100%); }
    :global([data-theme="light"]) .room.on[data-tone="cool"]    { background: linear-gradient(155deg, #e6eef2 0%, #dbe7ed 100%); }
    :global([data-theme="light"]) .room.on[data-tone="neutral"] { background: linear-gradient(155deg, #f3efe6 0%, #ebe4d6 100%); }

    @media (hover: hover) {
        .room:hover { border-color: var(--border-strong); transform: translateY(-2px); box-shadow: var(--shadow-md); }
        .room.on:hover { border-color: rgba(245, 189, 110, 0.28); }
    }
    .room:active { transform: scale(0.98); transition-duration: 80ms; }
    .room:focus-visible { box-shadow: var(--focus-ring); }

    .top {
        display: flex;
        justify-content: space-between;
        align-items: flex-start;
    }

    /* Room icon badge */
    .ico {
        width: 40px; height: 40px;
        border-radius: 12px;
        background: rgba(255,255,255,0.05);
        display: grid; place-items: center;
        color: var(--text-dim);
        transition: background var(--t-med), color var(--t-med);
        flex-shrink: 0;
    }
    .ico.on {
        background: var(--on-soft);
        color: var(--on);
    }
    :global([data-theme="light"]) .ico {
        background: rgba(0,0,0,0.06);
    }

    /* Small on-count pill, top-right corner */
    .on-badge {
        font-family: var(--font-mono);
        font-size: 11px;
        font-weight: 600;
        color: var(--on);
        background: var(--on-soft);
        padding: 2px 7px;
        border-radius: var(--r-pill);
        line-height: 1.4;
        flex-shrink: 0;
    }

    .name { font-weight: 600; font-size: 16px; margin-bottom: 2px; line-height: 1.2;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .meta { color: var(--text-mute); font-size: 12.5px; }
    .count { font-family: var(--font-mono); font-variant-numeric: tabular-nums; color: var(--text-mute); }
    .count.lit { color: var(--on); }
    .slash { color: var(--text-dim); }
</style>
