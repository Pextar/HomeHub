<script lang="ts">
    import Icon from "./Icon.svelte";
    import { untrack, onMount, onDestroy } from "svelte";
    import { api } from "../lib/api";
    import { socketAction, automationsUsingSocket, plural } from "../lib/utils";
    import { openModal } from "../lib/modal.svelte";
    import { toasts, data } from "../lib/stores.svelte";
    import type { Socket } from "../lib/types";
    import SocketModal from "../modals/SocketModal.svelte";
    import TimerModal from "../modals/TimerModal.svelte";
    import TasmotaLightModal from "../modals/TasmotaLightModal.svelte";
    import MatterLightModal from "../modals/MatterLightModal.svelte";
    import ConfirmModal from "./ConfirmModal.svelte";
    import { scale } from "svelte/transition";
    import { cubicOut } from "svelte/easing";

    interface Props { socket: Socket; }
    let { socket }: Props = $props();

    const isTasmota  = $derived(socket.protocol === "tasmota");
    const isMatter   = $derived(socket.protocol === "matter" || socket.protocol === "matter-thread");
    const isThread   = $derived(socket.protocol === "matter-thread");
    const isReadOnly = $derived(!!socket.readonly);
    const isSmartLight = $derived((isTasmota || isMatter) && !isReadOnly);

    const proto = $derived(isTasmota ? "tasmota" : isMatter ? "matter" : "rf");
    const protoLabel = $derived(isTasmota ? "Wi-Fi" : isThread ? "Thread" : isMatter ? "Matter" : "RF");
    const protoIcon = $derived(isTasmota ? "wifi" : isMatter ? "devices" : "radio");

    // One-shot "pulse" ring whenever the socket's state flips. A state flip
    // also invalidates the cached brightness — scenes/automations often pair
    // "on" with a level, so the cached value is likely stale after a flip.
    let prevState = untrack(() => socket.state);
    let pulsing = $state(false);
    $effect(() => {
        const s = socket.state;
        if (s !== prevState) {
            prevState = s;
            pulsing = true;
            if (isSmartLight) brightness = null; // lazy-fetch effect refetches
            const t = setTimeout(() => { pulsing = false; }, 550);
            return () => clearTimeout(t);
        }
    });

    // Brightness drives the inline slider + "On · NN%" label. Lazy-loaded from
    // the bridge once the tile scrolls into view (a wall of smart lights would
    // otherwise fire a burst of requests on mount). Full control (colour, CT,
    // presets) lives in the light modal opened by tapping the card body.
    let brightness = $state<number | null>(null);
    let cardEl = $state<HTMLElement>();
    let visible = $state(false);
    onMount(() => {
        if (!cardEl) return;
        const io = new IntersectionObserver((entries) => {
            if (entries.some(e => e.isIntersecting)) { visible = true; io.disconnect(); }
        }, { rootMargin: "100px" });
        io.observe(cardEl);
        return () => io.disconnect();
    });
    $effect(() => {
        if (!visible || !isSmartLight || brightness !== null) return;
        if (isTasmota) {
            api.tasmotaGetState(socket.id).then(s => { if (s.dimmer != null) brightness = s.dimmer; }).catch(() => {});
        } else if (isMatter) {
            api.matterGetState(socket.id).then(s => { if (s.level != null) brightness = s.level; }).catch(() => {});
        }
    });

    let brightnessTimer: ReturnType<typeof setTimeout> | undefined;
    onDestroy(() => clearTimeout(brightnessTimer));
    function setBrightness(v: number) {
        brightness = v;
        clearTimeout(brightnessTimer);
        brightnessTimer = setTimeout(async () => {
            try {
                if (isTasmota) await api.tasmotaSetState(socket.id, { dimmer: v });
                else if (isMatter) await api.matterSetState(socket.id, { level: v });
            } catch (e) { toasts.error("Brightness", (e as Error).message); }
        }, 120);
    }

    const statusText = $derived(
        socket.state ? (isSmartLight && brightness != null ? `On · ${brightness}%` : "On") : "Off"
    );
    const showRail = $derived(isSmartLight && socket.state && brightness != null);

    async function toggleFavorite() {
        moreOpen = false;
        try { await api.socketToggleFavorite(socket.id); await data.refresh(); }
        catch (e) { toasts.error("Failed", (e as Error).message); }
    }

    async function confirmDelete() {
        moreOpen = false;
        const autoN = automationsUsingSocket(data.value.automations, socket.id);
        const extra = autoN > 0 ? ` ${plural(autoN, "automation")} that use it will also be updated or removed.` : "";
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Delete device?",
            message: `"${socket.name}" and any schedules pointing to it will be removed.${extra}`,
            confirmLabel: "Delete",
            danger: true,
        });
        if (!ok) return;
        try {
            await api.deleteSocket(socket.id);
            toasts.success("Device deleted", socket.name);
            await data.refresh();
        } catch (e) { toasts.error("Failed", (e as Error).message); }
    }

    async function openControls() {
        moreOpen = false;
        if (isTasmota) await openModal(TasmotaLightModal, { socket });
        else if (isMatter) await openModal(MatterLightModal, { socket });
        else return;
        // The modal can change brightness/colour — refetch so the
        // "On · NN%" label and inline slider don't go stale.
        brightness = null;
    }
    function openTimer() { moreOpen = false; openModal(TimerModal, { socket }); }
    function openEdit()  { moreOpen = false; openModal(SocketModal, { existing: socket }); }

    // Actions popover — opened by tapping the tile body. Replaces the old
    // On/Off/Toggle button row; the switch is the primary control now.
    let moreOpen = $state(false);
    let moreBtnEl = $state<HTMLElement>();
    // Fixed position for the menu — computed when it opens so the popup never
    // goes off-screen (cards can be as narrow as half the viewport in the
    // 2-column Favorites grid).
    let menuPos = $state<{ bottom: number; right?: number; left?: number } | null>(null);

    $effect(() => {
        if (!moreOpen) { menuPos = null; return; }

        // Compute where the menu should appear relative to the viewport.
        if (moreBtnEl) {
            const rect = moreBtnEl.getBoundingClientRect();
            const menuWidth = 210;
            const vw = window.innerWidth;
            const gap = 6;
            const bottom = window.innerHeight - rect.top + gap;
            const rightDist = vw - rect.right; // px from right edge of viewport

            if (vw - rightDist - menuWidth < 8) {
                // Would overflow the left edge — anchor from the left instead.
                menuPos = { bottom, left: Math.max(8, rect.left) };
            } else {
                menuPos = { bottom, right: rightDist };
            }
        }

        function onDocClick(e: MouseEvent) {
            if (!cardEl?.contains(e.target as Node)) moreOpen = false;
        }
        function onKey(e: KeyboardEvent) { if (e.key === "Escape") moreOpen = false; }
        document.addEventListener("click", onDocClick, true);
        document.addEventListener("keydown", onKey, true);
        return () => {
            document.removeEventListener("click", onDocClick, true);
            document.removeEventListener("keydown", onKey, true);
        };
    });

    function onBodyClick() {
        // Sensors open only the actions menu (edit/delete), never controls.
        // Smart lights open their control modal. RF opens the actions menu.
        if (isReadOnly) { moreOpen = !moreOpen; return; }
        if (isSmartLight) openControls();
        else moreOpen = !moreOpen;
    }
</script>

<article class="tile" class:on={socket.state} class:pulsing class:readonly={isReadOnly} bind:this={cardEl}>
    {#if !isReadOnly}
        <button class="sw" class:on={socket.state}
            role="switch" aria-checked={socket.state}
            aria-label="Toggle {socket.name}"
            onclick={(e) => { e.stopPropagation(); socketAction(socket, "toggle"); }}></button>
    {:else}
        <span class="sw-placeholder" aria-hidden="true"></span>
    {/if}

    <button class="tile-hit" onclick={onBodyClick}
        aria-haspopup="menu"
        aria-expanded={moreOpen}>
        <span class="tile-bulb"><Icon name={isReadOnly ? "sensor" : "light"} size={18} /></span>
        <span class="tile-info">
            <span class="name" title={socket.name}>{socket.name}</span>
            <span class="meta-row">
                <span class="meta">{isReadOnly ? "Sensor" : statusText}{socket.room ? ` · ${socket.room}` : ""}</span>
                <span class="protocol-badge" data-proto={proto} title={`${socket.protocol || "rf"} · ${socket.code}`}>
                    <Icon name={protoIcon} size={11} />{protoLabel}
                </span>
            </span>
        </span>
    </button>

    {#if showRail}
        <input
            type="range"
            class="brightness-slider"
            min="1" max="100"
            value={brightness ?? 100}
            style="--pct:{brightness ?? 100}%"
            aria-label="Brightness {brightness}%"
            oninput={(e) => setBrightness((e.currentTarget as HTMLInputElement).valueAsNumber)}
        />
    {/if}

    <button class="more-corner" aria-label="Device actions" bind:this={moreBtnEl}
        onclick={(e) => { e.stopPropagation(); moreOpen = !moreOpen; }}>
        <Icon name="more" size={16} />
    </button>

    {#if moreOpen && menuPos}
        <div class="overflow-menu" role="menu"
            style:position="fixed"
            style:bottom="{menuPos.bottom}px"
            style:right={menuPos.right != null ? `${menuPos.right}px` : undefined}
            style:left={menuPos.left != null ? `${menuPos.left}px` : undefined}
            in:scale={{ start: 0.95, duration: 140, easing: cubicOut, opacity: 0 }}
            out:scale={{ start: 0.95, duration: 100, easing: cubicOut, opacity: 0 }}>
            {#if isSmartLight && !isReadOnly}
                <button class="overflow-item" role="menuitem" onclick={openControls}>
                    <Icon name="sun" size={16} /><span>Brightness &amp; colour</span>
                </button>
            {/if}
            {#if !isReadOnly}
                <button class="overflow-item" role="menuitem" onclick={openTimer}>
                    <Icon name="timer" size={16} /><span>Set timer</span>
                </button>
            {/if}
            <button class="overflow-item" role="menuitem" onclick={toggleFavorite}>
                <Icon name={socket.favorite ? "star" : "starOutline"} size={16} />
                <span>{socket.favorite ? "Remove favourite" : "Add to favourites"}</span>
            </button>
            <button class="overflow-item" role="menuitem" onclick={openEdit}>
                <Icon name="edit" size={16} /><span>Edit device</span>
            </button>
            <button class="overflow-item danger" role="menuitem" onclick={confirmDelete}>
                <Icon name="trash" size={16} /><span>Delete</span>
            </button>
        </div>
    {/if}
</article>

<style>
    .tile {
        position: relative;
        border-radius: var(--r-lg);
        padding: 16px;
        background: var(--card);
        border: 1px solid var(--hairline);
        display: flex;
        flex-direction: column;
        overflow: visible;
        transition: background var(--t-med), border-color var(--t-med), box-shadow var(--t-fast);
    }
    .tile.on {
        background: var(--tile-on-gradient);
        border-color: var(--tile-on-border);
    }
    @media (hover: hover) {
        .tile:hover { border-color: var(--border-strong); }
        .tile.on:hover { border-color: rgba(245, 189, 110, 0.32); }
    }

    .tile.pulsing.on { animation: pulse-on 0.55s ease-out; }
    @keyframes pulse-on {
        0%   { box-shadow: 0 0 0 0 var(--on-glow); }
        100% { box-shadow: 0 0 0 16px rgba(245, 189, 110, 0); }
    }

    /* Primary control: the switch, pinned top-right per the mockup. */
    .sw {
        position: absolute;
        top: 16px; right: 16px;
        z-index: 2;
        width: 44px; height: 26px;
        background: var(--card-3);
        border: 0; border-radius: 13px;
        cursor: pointer;
        flex-shrink: 0;
        touch-action: manipulation;
        transition: background 150ms ease;
    }
    .sw::after {
        content: "";
        position: absolute;
        top: 3px; left: 3px;
        width: 20px; height: 20px;
        border-radius: 50%;
        background: #b5b1a8;
        transition: transform 220ms var(--spring), background 150ms ease;
    }
    .sw.on { background: var(--on); }
    .sw.on::after { transform: translateX(18px); background: #fff; }

    /* Placeholder occupies the same slot as .sw so the layout is unchanged. */
    .sw-placeholder {
        position: absolute;
        top: 16px; right: 16px;
        width: 44px; height: 26px;
    }
    .tile.readonly { opacity: 0.85; }
    .tile.readonly .tile-bulb { color: var(--text-muted); }

    /* Tap target for the tile body — opens controls (smart) or actions menu. */
    .tile-hit {
        all: unset;
        display: flex;
        flex-direction: column;
        gap: 12px;
        cursor: pointer;
        /* leave room for the absolute switch on the first row */
        padding-right: 52px;
        min-height: 36px;
    }
    .tile-hit:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: var(--r-md); }

    .tile-bulb {
        width: 36px; height: 36px;
        border-radius: 50%;
        background: var(--card-3);
        display: grid; place-items: center;
        position: relative;
        flex-shrink: 0;
        color: var(--text-mute);
        transition: background var(--t-med), color var(--t-med);
    }
    .tile.on .tile-bulb {
        background: var(--on);
        color: var(--primary-fg);
        box-shadow: 0 0 0 1px var(--on), 0 0 24px 4px var(--on-glow);
    }
    .tile.on .tile-bulb::after {
        content: "";
        position: absolute;
        inset: -22px;
        border-radius: 50%;
        background: radial-gradient(closest-side, var(--on-glow), transparent 70%);
        pointer-events: none;
        z-index: -1;
    }

    .tile-info { display: flex; flex-direction: column; gap: 2px; margin-top: 2px; min-width: 0; padding-right: 0; }
    .name {
        font-weight: 600; font-size: 15px;
        display: -webkit-box;
        -webkit-line-clamp: 2;
        line-clamp: 2;
        -webkit-box-orient: vertical;
        overflow: hidden;
        line-height: 1.25;
    }
    .meta-row {
        display: flex; align-items: center; justify-content: space-between; gap: 8px;
    }
    .meta {
        color: var(--text-mute); font-size: 12px;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0;
    }
    .protocol-badge { flex-shrink: 0; }
    .tile.on .meta { color: var(--on); }

    /* Interactive brightness slider — replaces the old read-only .rail.
       Positioned as a flex sibling to .tile-hit so tapping the slider
       never triggers openControls(). margin-right clears the .more-corner. */
    .brightness-slider {
        -webkit-appearance: none;
        appearance: none;
        display: block;
        width: 100%;
        height: 6px;
        border-radius: 3px;
        /* Fallback for browsers where track pseudo-element doesn't inherit --pct */
        background: linear-gradient(to right, var(--on) var(--pct, 100%), var(--card-3) var(--pct, 100%));
        cursor: pointer;
        padding: 0;
        margin-top: 4px;
        margin-right: 32px;
        touch-action: pan-y;
    }
    /* WebKit / Blink track */
    .brightness-slider::-webkit-slider-runnable-track {
        height: 6px;
        border-radius: 3px;
        background: linear-gradient(to right, var(--on) var(--pct, 100%), var(--card-3) var(--pct, 100%));
    }
    /* WebKit thumb */
    .brightness-slider::-webkit-slider-thumb {
        -webkit-appearance: none;
        width: 16px;
        height: 16px;
        margin-top: -5px;
        border-radius: 50%;
        background: var(--on);
        box-shadow: 0 1px 6px rgba(0, 0, 0, 0.5);
        cursor: grab;
        transition: transform 80ms ease;
    }
    .brightness-slider:active::-webkit-slider-thumb {
        transform: scale(1.2);
        cursor: grabbing;
    }
    .brightness-slider:focus-visible { outline: none; }
    .brightness-slider:focus-visible::-webkit-slider-thumb {
        box-shadow: 0 0 0 3px var(--on-soft), 0 1px 6px rgba(0, 0, 0, 0.5);
    }
    /* Firefox track + progress */
    .brightness-slider::-moz-range-track {
        height: 6px;
        border-radius: 3px;
        background: var(--card-3);
    }
    .brightness-slider::-moz-range-progress {
        height: 6px;
        border-radius: 3px;
        background: var(--on);
    }
    .brightness-slider::-moz-range-thumb {
        width: 16px;
        height: 16px;
        border-radius: 50%;
        border: none;
        background: var(--on);
        box-shadow: 0 1px 6px rgba(0, 0, 0, 0.5);
        cursor: grab;
        transition: transform 80ms ease;
    }
    .brightness-slider:active::-moz-range-thumb {
        transform: scale(1.2);
        cursor: grabbing;
    }
    .brightness-slider:focus-visible::-moz-range-thumb {
        box-shadow: 0 0 0 3px var(--on-soft), 0 1px 6px rgba(0, 0, 0, 0.5);
    }
    /* Larger thumb on touch for a ≥ 44 px effective hit area */
    @media (pointer: coarse) {
        .brightness-slider::-webkit-slider-thumb {
            width: 22px;
            height: 22px;
            margin-top: -8px;
        }
        .brightness-slider::-moz-range-thumb {
            width: 22px;
            height: 22px;
        }
    }
    @media (prefers-reduced-motion: reduce) {
        .brightness-slider::-webkit-slider-thumb,
        .brightness-slider::-moz-range-thumb { transition-duration: 0.001ms; }
    }

    /* Subtle ⋯ affordance, bottom-right. Hover-revealed on desktop, always on
       touch so management actions stay discoverable on the cleaner tile. */
    .more-corner {
        position: absolute;
        bottom: 10px; right: 10px;
        width: 28px; height: 28px;
        display: grid; place-items: center;
        border: 0; background: transparent;
        color: var(--text-mute);
        border-radius: var(--r-sm);
        cursor: pointer;
        opacity: 0;
        transition: opacity var(--t-fast), background var(--t-fast), color var(--t-fast);
    }
    .more-corner:hover { background: var(--surface-hover); color: var(--text); }
    @media (hover: hover) { .tile:hover .more-corner { opacity: 1; } }
    @media (pointer: coarse) { .more-corner { opacity: 0.6; bottom: 8px; right: 8px; } }

    /* ── Actions popover ── */
    /* position/bottom/right|left are set inline (position:fixed + computed
       coords) so the menu never clips off-screen on narrow grid cards. */
    .overflow-menu {
        z-index: 200;
        min-width: 210px;
        display: flex;
        flex-direction: column;
        background: var(--bg-raised);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        overflow: hidden;
        box-shadow: var(--shadow-md);
    }
    .overflow-item {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: 12px var(--space-4);
        background: transparent;
        border: none;
        border-bottom: 1px solid var(--border);
        cursor: pointer;
        font: inherit;
        font-size: 14px;
        color: var(--text);
        text-align: left;
        touch-action: manipulation;
        transition: background var(--t-fast);
    }
    .overflow-item:last-child { border-bottom: none; }
    .overflow-item :global(svg) { color: var(--text-muted); flex-shrink: 0; }
    .overflow-item:hover { background: var(--surface-hover); }
    .overflow-item.danger { color: var(--danger); }
    .overflow-item.danger :global(svg) { color: var(--danger); }

    @media (pointer: coarse) {
        .name { font-size: 15.5px; }
        .overflow-item { padding: 14px var(--space-4); font-size: 15px; min-height: 52px; }
    }
</style>
