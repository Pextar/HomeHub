<script lang="ts">
    /**
     * The bottom sheet all of Music's surfaces wear: Zones, Search, and both
     * players.
     *
     * It exists because those four had a copy each of the same forty lines of
     * scrim, sheet, sticky head and drag wiring, plus one shared copy of the
     * gesture state in the view. That state is per-sheet by nature — a swap
     * must not inherit the previous sheet's half-finished drag — so keeping it
     * here, where it dies with the instance, is what removes the reset the
     * view used to have to remember to call.
     *
     * The dismiss kit is DESIGN.md §15's, in full: grabber, a collapse button,
     * Escape (the view's, since it also has to leave screens), backdrop click,
     * and drag-down. The player covers the nav, so a user must never feel
     * stuck in one.
     */
    import type { Snippet } from "svelte";
    import Icon from "../Icon.svelte";
    import type { ComponentProps } from "svelte";
    import { fade } from "svelte/transition";
    import { dur, sheet } from "../../lib/motion";

    /** Icon names come from the shared set, so a typo is a build error. */
    type IconName = ComponentProps<typeof Icon>["name"];

    interface Action {
        icon: IconName;
        label: string;
        onClick: () => void;
    }

    let {
        /** Names the dialog for assistive tech. */
        label,
        /** Mono uppercase line above the title — the players use it. */
        eyebrow = "",
        title,
        sub = "",
        /** `chevronLeft` when the button steps back a pane rather than out. */
        backIcon = "chevronDown" as IconName,
        backLabel,
        onBack,
        /** Trailing control. Absent leaves a gap, so the title stays centred. */
        action = undefined,
        /** True while the dock floats over this sheet and the last row must clear it. */
        docked = false,
        /** Backdrop click and the drag-down throw both land here. */
        onDismiss,
        /** Bound so the view can note the scroll offset before a swap, and so
         *  Zones can drive its edge-scroll while a puck is held. */
        scrollEl = $bindable<HTMLElement | null>(null),
        /** Bound by the players, which take focus when they open. */
        sheetEl = $bindable<HTMLElement | null>(null),
        /** True once a drag-down has committed and the sheet is riding out.
         *  Bound because the player's art swipe has to stand down while it
         *  happens — two gestures on one sheet, only one of them still valid. */
        dismissing = $bindable(false),
        children,
    }: {
        label: string;
        eyebrow?: string;
        title: string;
        sub?: string;
        backIcon?: IconName;
        backLabel: string;
        onBack: () => void;
        action?: Action;
        docked?: boolean;
        onDismiss: () => void;
        scrollEl?: HTMLElement | null;
        sheetEl?: HTMLElement | null;
        dismissing?: boolean;
        children: Snippet;
    } = $props();

    // ── Drag-to-dismiss ──────────────────────────────────────────────────
    // The same gesture the shared Modal sheet carries, and it must stay that
    // way: the top bar always drags, and the scroll body drags only from the
    // top and only on a clear downward pull, so a long queue still scrolls.
    let dragY = $state(0);
    let dragging = $state(false);
    let pendingBody = false;
    let dragStartY = 0;
    let dragStartX = 0;

    // Mobile only: from 601px the sheet is a centered dialog whose transform
    // carries its centering, so a drag offset would knock it off-centre.
    function draggable(): boolean {
        return window.matchMedia("(max-width: 600px)").matches;
    }
    // Pointer events from the top bar bubble into the scroll container (and,
    // once captured, keep reporting it as their target) — the body handlers
    // ignore them so only one drag path is ever live.
    function fromTop(e: PointerEvent): boolean {
        return !!(e.target as HTMLElement | null)?.closest?.(".sheet-top");
    }

    function startDrag(e: PointerEvent, target: HTMLElement) {
        dragging = true;
        dragStartY = e.clientY;
        dragStartX = e.clientX;
        dragY = 0;
        try {
            target.setPointerCapture(e.pointerId);
        } catch {
            /* not capturable */
        }
    }
    function cancelDrag() {
        if (!dragging) return;
        dragging = false;
        requestAnimationFrame(() => {
            dragY = 0;
        });
    }
    function finishDrag() {
        dragging = false;
        if (dragY > 90) {
            // Ride the throw out instead of snapping back and then playing
            // the sheet's own exit — the finger already did that animation.
            dismissing = true;
            dragY = 600;
            setTimeout(onDismiss, 220);
        } else {
            requestAnimationFrame(() => {
                dragY = 0;
            });
        }
    }

    // Top bar — always drags.
    function onTopPointerDown(e: PointerEvent) {
        if (dismissing || !draggable()) return;
        if ((e.target as HTMLElement).closest("button")) return; // close / back
        startDrag(e, e.currentTarget as HTMLElement);
        e.preventDefault();
    }
    function onTopPointerMove(e: PointerEvent) {
        if (!dragging) return;
        dragY = Math.max(0, e.clientY - dragStartY);
    }
    function onTopPointerUp() {
        if (dragging) finishDrag();
    }

    // Body — drags when the scroll is at the top, otherwise scrolls.
    function onBodyPointerDown(e: PointerEvent) {
        if (dismissing || !draggable() || fromTop(e)) return;
        if (e.pointerType === "mouse") return; // pointer devices use the bar
        if (!scrollEl || scrollEl.scrollTop > 0) return;
        if ((e.target as HTMLElement).closest("input, button, a, [role='slider']")) return;
        pendingBody = true;
        dragStartY = e.clientY;
        dragStartX = e.clientX;
    }
    function onBodyPointerMove(e: PointerEvent) {
        if (fromTop(e)) return;
        if (dragging) {
            dragY = Math.max(0, e.clientY - dragStartY);
            e.preventDefault(); // claimed: don't scroll as well
            return;
        }
        if (!pendingBody) return;
        const dy = e.clientY - dragStartY;
        const dx = e.clientX - dragStartX;
        if (dy > 8 && dy > Math.abs(dx)) {
            pendingBody = false;
            const from = dragStartY;
            startDrag(e, scrollEl!);
            dragStartY = from; // keep the origin so the sheet doesn't jump back
            dragY = dy;
            e.preventDefault();
        } else if (dy < -4 || Math.abs(dx) > 12) {
            pendingBody = false; // scrolling up or swiping sideways
        }
    }
    function onBodyPointerUp(e: PointerEvent) {
        if (fromTop(e)) return;
        pendingBody = false;
        if (dragging) finishDrag();
    }
    function onBodyPointerCancel(e: PointerEvent) {
        if (fromTop(e)) return;
        pendingBody = false;
        cancelDrag();
    }
</script>

<div
    class="scrim"
    transition:fade={{ duration: dur(200) }}
    onclick={onDismiss}
    aria-hidden="true"
></div>
<div
    class="sheet"
    class:dragging
    role="dialog"
    aria-modal="true"
    aria-label={label}
    tabindex="-1"
    bind:this={sheetEl}
    style:transform={dragY > 0 ? `translateY(${dragY}px)` : ""}
    style:opacity={dragY > 0 ? Math.max(0.4, 1 - dragY / 300) : undefined}
    style:transition={dragging
        ? "none"
        : dragY > 0
          ? "transform 0.22s ease-in, opacity 0.22s ease-in"
          : "transform 0.3s cubic-bezier(0.34, 1.56, 0.64, 1)"}
    in:sheet={{}}
    out:sheet={{ instant: dismissing }}
>
    <div
        class="sheet-scroll"
        class:docked
        role="none"
        bind:this={scrollEl}
        onpointerdown={onBodyPointerDown}
        onpointermove={onBodyPointerMove}
        onpointerup={onBodyPointerUp}
        onpointercancel={onBodyPointerCancel}
    >
        <!-- Grabber + header travel together and stick, so a long queue never
             scrolls the way out off the screen. The bar is also the drag
             handle. -->
        <div
            class="sheet-top"
            role="none"
            onpointerdown={onTopPointerDown}
            onpointermove={onTopPointerMove}
            onpointerup={onTopPointerUp}
            onpointercancel={cancelDrag}
        >
            <div class="grabber" aria-hidden="true"></div>
            <header class="player-head">
                <button class="icon-btn p-icon" aria-label={backLabel} onclick={onBack}>
                    <Icon name={backIcon} size={18} />
                </button>
                <div class="p-onair">
                    {#if eyebrow}<div class="eyrow">{eyebrow}</div>{/if}
                    <div class="p-onair-name">{title}</div>
                    {#if sub}<div class="p-onair-sub">{sub}</div>{/if}
                </div>
                {#if action}
                    <button
                        class="icon-btn p-icon"
                        aria-label={action.label}
                        onclick={action.onClick}
                    >
                        <Icon name={action.icon} size={17} />
                    </button>
                {:else}
                    <!-- Balances the back button so the title stays centered. -->
                    <span class="p-icon-gap" aria-hidden="true"></span>
                {/if}
            </header>
        </div>

        {@render children()}
    </div>
</div>

<style>
    /* Above the mobile nav bar (z 100) and the nav drawer (120), below the
       modal stack (150) — DESIGN.md §15 has the player covering the nav, and
       a "Clear queue" confirm still has to land on top of the player. */
    .scrim {
        position: fixed; inset: 0; z-index: 125;
        background: rgba(0, 0, 0, 0.5);
    }
    .sheet {
        position: fixed; z-index: 126;
        left: 0; right: 0; bottom: 0;
        max-height: 92vh;
        background: var(--bg);
        border-radius: var(--r-xl) var(--r-xl) 0 0;
        border: 1px solid var(--hairline); border-bottom: 0;
        box-shadow: var(--shadow-lg);
        outline: none;
        /* Keep scrolled content inside the top radius, and GPU-promote the
           sheet so the drag transform stays smooth. */
        overflow: hidden;
        will-change: transform;
    }
    .grabber {
        width: 38px; height: 4px; border-radius: 2px;
        background: var(--border-strong);
        margin: 8px auto 0;
        pointer-events: none;
    }
    .sheet-scroll {
        max-height: 92vh; overflow-y: auto;
        overscroll-behavior: contain;
        -webkit-overflow-scrolling: touch;
        padding: 0 var(--space-5)
            calc(var(--space-8) + env(safe-area-inset-bottom));
        display: flex; flex-direction: column; gap: var(--space-5);
    }
    /* The dock floats over Zones and Search, so the last row of either has to
       clear it rather than spending its life underneath. */
    .sheet-scroll.docked {
        padding-bottom: calc(var(--space-8) + 72px + env(safe-area-inset-bottom));
    }
    /* On desktop the sheet becomes a centered dialog. */
    @media (min-width: 601px) {
        .sheet {
            left: 50%; bottom: auto; top: 50%;
            transform: translate(-50%, -50%);
            width: min(440px, calc(100vw - 48px));
            max-height: 88vh;
            border-radius: var(--r-xl); border-bottom: 1px solid var(--hairline);
        }
        .sheet-scroll { max-height: 88vh; }
    }
    /* The bar is the drag handle on phones, so the browser must not claim
       the gesture for scrolling first. */
    @media (max-width: 600px) {
        .sheet-top { touch-action: none; cursor: grab; }
        .sheet.dragging .sheet-top { cursor: grabbing; }
        .sheet-scroll { touch-action: pan-y; }
    }

    /* The band is translucent and blurred, and its bottom edge fades out — art
       and rows dissolve as they pass underneath instead of being cut off
       against an opaque slab. An opaque bar was tried first and read as a
       floating block cutting the content in half. */
    .sheet-top {
        --fade: 22px;
        position: sticky; top: 0; z-index: 3;
        margin: 0 calc(var(--space-5) * -1) calc(var(--fade) * -1);
        padding: 0 var(--space-5) var(--fade);
        background: var(--bg-bar);
        backdrop-filter: blur(18px) saturate(1.3);
        -webkit-backdrop-filter: blur(18px) saturate(1.3);
        -webkit-mask-image: linear-gradient(to bottom, #000 calc(100% - var(--fade)), transparent);
        mask-image: linear-gradient(to bottom, #000 calc(100% - var(--fade)), transparent);
    }
    .player-head {
        display: flex; align-items: center; justify-content: space-between;
        gap: var(--space-3);
        padding: var(--space-2) 0 var(--space-3);
    }
    .p-icon {
        width: 38px; height: 38px; border-radius: 50%;
        background: var(--card-2); border: 1px solid var(--hairline);
    }
    /* Keeps the title centered on sheets whose head carries no action chip. */
    .p-icon-gap { width: 38px; flex-shrink: 0; }
    .p-onair { text-align: center; min-width: 0; }
    .p-onair-name { font-size: 13px; font-weight: 600; margin-top: 2px; }
    .p-onair-sub {
        font-size: 11.5px; color: var(--text-mute); margin-top: 2px;
        line-height: 1.35;
    }

    @media (prefers-reduced-motion: reduce) {
        /* The drag snap-back is an inline style, so it needs its own override. */
        .sheet { transition: none !important; }
    }
</style>
