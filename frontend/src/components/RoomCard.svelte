<script lang="ts">
    import Icon from "./Icon.svelte";
    import ConfirmModal from "./ConfirmModal.svelte";
    import RoomModal from "../modals/RoomModal.svelte";
    import { route, data, toasts } from "../lib/stores.svelte";
    import { openModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { runAction } from "../lib/utils";
    import { scale } from "svelte/transition";
    import { cubicOut } from "svelte/easing";
    import type { RoomSummary } from "../lib/types";

    interface Props { room: RoomSummary; manage?: boolean; }
    let { room, manage = false }: Props = $props();

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

    // ── Management (only rendered when `manage` is set) ───────────────────
    function allOn()  { moreOpen = false; runAction(() => api.roomOn(room.name), `${room.name} on`); }
    function allOff() { moreOpen = false; runAction(() => api.roomOff(room.name), `${room.name} off`); }
    function rename() { moreOpen = false; openModal(RoomModal, { existing: { id: room.id, name: room.name } }); }
    async function confirmDelete() {
        moreOpen = false;
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Delete room?",
            message: `Remove "${room.name}"? The ${room.sockets} device${room.sockets === 1 ? "" : "s"} in this room will become unassigned.`,
            confirmLabel: "Delete",
            danger: true,
        });
        if (!ok) return;
        try {
            await api.deleteRoom(room.id);
            toasts.success("Room deleted", room.name);
            await data.refresh();
        } catch (e) {
            toasts.error("Delete failed", (e as Error).message);
        }
    }

    let moreOpen = $state(false);
    let el = $state<HTMLElement>();
    $effect(() => {
        if (!moreOpen) return;
        function onDoc(e: MouseEvent) { if (!el?.contains(e.target as Node)) moreOpen = false; }
        function onKey(e: KeyboardEvent) { if (e.key === "Escape") moreOpen = false; }
        document.addEventListener("click", onDoc, true);
        document.addEventListener("keydown", onKey, true);
        return () => {
            document.removeEventListener("click", onDoc, true);
            document.removeEventListener("keydown", onKey, true);
        };
    });
</script>

<div class="room" class:on={anyOn} class:manage data-tone={tone} bind:this={el}>
    <button class="room-hit" onclick={open} aria-label="Open {room.name}">
        <span class="top">
            <span class="ico" class:on={anyOn}><Icon name={icon} size={22} /></span>
            {#if anyOn}
                <span class="on-badge">{room.on}</span>
            {/if}
        </span>
        <span class="body">
            <span class="name" title={room.name}>{room.name}</span>
            <span class="meta">
                <span class="count" class:lit={anyOn}>{room.on}</span><span class="slash"> / {room.sockets}</span> on
            </span>
        </span>
    </button>

    {#if manage}
        <button class="more-corner" aria-label="{room.name} actions"
            onclick={(e) => { e.stopPropagation(); moreOpen = !moreOpen; }}>
            <Icon name="more" size={16} />
        </button>
        {#if moreOpen}
            <div class="overflow-menu" role="menu"
                in:scale={{ start: 0.95, duration: 140, easing: cubicOut, opacity: 0 }}
                out:scale={{ start: 0.95, duration: 100, easing: cubicOut, opacity: 0 }}>
                <button class="overflow-item" role="menuitem" onclick={allOn}>
                    <Icon name="power" size={16} /><span>All on</span>
                </button>
                <button class="overflow-item" role="menuitem" onclick={allOff}>
                    <Icon name="power" size={16} /><span>All off</span>
                </button>
                <button class="overflow-item" role="menuitem" onclick={rename}>
                    <Icon name="edit" size={16} /><span>Rename</span>
                </button>
                <button class="overflow-item danger" role="menuitem" onclick={confirmDelete}>
                    <Icon name="trash" size={16} /><span>Delete</span>
                </button>
            </div>
        {/if}
    {/if}
</div>

<style>
    .room {
        position: relative;
        box-sizing: border-box;
        border: 1px solid var(--hairline);
        border-radius: var(--r-lg);
        height: 150px;
        display: flex;
        transition: border-color var(--t-fast), background var(--t-med), transform var(--t-fast), box-shadow var(--t-fast);
        /* Subtle tone tint even when off */
        background: var(--card);
    }
    /* ON uses the one sanctioned surface (DESIGN.md §6.1) — the per-room
       tone gradients that used to live here were off-spec ("no gradients
       except .tile.on and the timeline"); room identity now sits on the
       icon tint below instead. */
    .room.on {
        background: var(--tile-on-gradient);
        border-color: var(--tile-on-border);
    }

    @media (hover: hover) {
        .room:hover { border-color: var(--border-strong); transform: translateY(-2px); box-shadow: var(--shadow-md); }
        .room.on:hover { border-color: rgba(245, 189, 110, 0.28); }
    }

    .room-hit {
        all: unset;
        box-sizing: border-box;
        flex: 1;
        min-width: 0;
        padding: 14px;
        display: flex;
        flex-direction: column;
        justify-content: space-between;
        cursor: pointer;
        touch-action: manipulation;
    }
    .room-hit:active { transform: scale(0.98); transition: transform 80ms; }
    .room-hit:focus-visible { box-shadow: var(--focus-ring); border-radius: var(--r-lg); }

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
    /* Per-room identity, token-tinted (replaces the old tone gradients). */
    .room[data-tone="warm"]:not(.on) .ico { color: var(--p-rf); }
    .room[data-tone="cool"]:not(.on) .ico { color: var(--cool); }
    .ico.on {
        background: var(--on-soft);
        color: var(--on);
    }
    :global([data-theme="light"]) .ico {
        background: rgba(0,0,0,0.06);
    }

    /* Small on-count pill, top-right corner. Nudged left in manage mode so it
       never collides with the overflow trigger. */
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
    .room.manage .on-badge { margin-right: 28px; }

    .body { display: flex; flex-direction: column; min-width: 0; }
    .name { font-weight: 600; font-size: 16px; margin-bottom: 2px; line-height: 1.2;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .meta { color: var(--text-mute); font-size: 12.5px; }
    .count { font-family: var(--font-mono); font-variant-numeric: tabular-nums; color: var(--text-mute); }
    .count.lit { color: var(--on); }
    .slash { color: var(--text-dim); }

    /* ── Manage affordances ─────────────────────────────────────────────── */
    .more-corner {
        position: absolute;
        top: 10px; right: 10px;
        width: 28px; height: 28px;
        display: grid; place-items: center;
        border: 0; background: transparent;
        color: var(--text-mute);
        border-radius: var(--r-sm);
        cursor: pointer;
        transition: background var(--t-fast), color var(--t-fast);
    }
    .more-corner:hover { background: var(--surface-hover); color: var(--text); }

    .overflow-menu {
        position: absolute;
        right: 10px; top: 42px;
        z-index: 10;
        min-width: 170px;
        display: flex; flex-direction: column;
        background: var(--bg-raised);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        overflow: hidden;
        box-shadow: var(--shadow-md);
    }
    .overflow-item {
        display: flex; align-items: center; gap: var(--space-3);
        padding: 12px var(--space-4);
        background: transparent; border: 0;
        border-bottom: 1px solid var(--border);
        cursor: pointer; font: inherit; font-size: 14px;
        color: var(--text); text-align: left;
    }
    .overflow-item:last-child { border-bottom: none; }
    .overflow-item :global(svg) { color: var(--text-muted); flex-shrink: 0; }
    .overflow-item:hover { background: var(--surface-hover); }
    .overflow-item.danger { color: var(--danger); }
    .overflow-item.danger :global(svg) { color: var(--danger); }

    @media (pointer: coarse) {
        .more-corner { width: 36px; height: 36px; }
        .overflow-item { padding: 14px var(--space-4); font-size: 15px; min-height: 52px; }
    }
</style>
