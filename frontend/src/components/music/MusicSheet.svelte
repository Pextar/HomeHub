<script lang="ts">
    /**
     * The bottom sheet Music's remaining sheet surfaces wear: Zones, the zone
     * editor, and all three players. Search left this group when its grouped
     * overview outgrew a sheet's 82vh and became a screen instead
     * (`SearchScreen.svelte`) — a browsing surface earns the same real
     * estate Speakers and the catalog drill-ins get, not a quick lookup's.
     *
     * It exists because those five had a copy each of the same forty lines of
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
    import { dur, grow, type Origin } from "../../lib/motion";

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
        /** Desktop-only: a wide stage window instead of the phone-width dialog.
         *  Below 901px it is the same sheet it always was. */
        wide = false,
        /** The playing track's art — blurred into the sheet's ambient backdrop:
         *  a wash behind the head on a phone, the whole stage's light on
         *  desktop. Absent leaves the sheet on its own flat surface. */
        backdropUri = undefined,
        /** What the sheet was opened from, measured at the tap: the sheet
         *  unfolds out of that frame and collapses back into it. Null (a back
         *  gesture, the keyboard, reduced motion) gets the plain slide. */
        origin = null,
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
        wide?: boolean;
        backdropUri?: string;
        origin?: Origin | null;
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

    // The head and the ambient backdrop are handed to `grow` along with the
    // scroll body: while the window is moving, the content is faded with it,
    // the frosted blur stands down and the blurred art waits. All three are
    // driven from the transition rather than from a class here, because a
    // removed subtree's effects are already paused by the time the sheet
    // closes — and because they are the three things a moving clip cannot
    // afford to redraw (see `lib/motion.ts`).
    let topEl = $state<HTMLElement | null>(null);
    let bgEl = $state<HTMLElement | null>(null);
</script>

<div
    class="scrim"
    in:fade={{ duration: dur(280) }}
    out:fade={{ duration: dur(200) }}
    onclick={onDismiss}
    aria-hidden="true"
></div>
<div
    class="sheet"
    class:dragging
    class:stage={wide}
    class:ambient={!!backdropUri}
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
    in:grow={{ origin, content: () => scrollEl, bar: () => topEl, ambient: () => bgEl }}
    out:grow={{
        origin,
        out: true,
        instant: dismissing,
        content: () => scrollEl,
        bar: () => topEl,
        ambient: () => bgEl,
    }}
>
    {#if backdropUri}
        <!-- The sheet's ambient light: the track's own art, blown up and
             blurred into the room the sheet is standing in. A wash behind the
             head on a phone; the whole window on the desktop stage. -->
        <div
            class="sheet-bg"
            aria-hidden="true"
            bind:this={bgEl}
            style:background-image={`url("${backdropUri}")`}
        ></div>
        <div class="sheet-wash" aria-hidden="true"></div>
    {/if}
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
            bind:this={topEl}
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
        position: fixed;
        inset: 0;
        z-index: var(--z-scrim);
        background: rgba(0, 0, 0, 0.5);
    }
    .sheet {
        position: fixed;
        z-index: var(--z-sheet);
        left: 0;
        right: 0;
        bottom: 0;
        max-height: 92vh;
        background: var(--bg);
        border-radius: var(--r-xl) var(--r-xl) 0 0;
        border: 1px solid var(--hairline);
        border-bottom: 0;
        box-shadow: var(--shadow-lg);
        outline: none;
        /* Keep scrolled content inside the top radius, and GPU-promote the
           sheet so the drag transform stays smooth. */
        overflow: hidden;
        will-change: transform;
    }
    /* ── The ambient backdrop ─────────────────────────────────────────
       The player used to open on a flat near-black band: the head, the
       grabber and the gap above the art were the first thing on screen and
       the only part of the sheet with nothing in it. It carries the track's
       own art now — blown up, blurred and washed down until it is light
       rather than picture. Mobile takes it across the top, where the head
       is; the stage below takes the whole window. */
    .sheet-bg,
    .sheet-wash {
        position: absolute;
        top: 0;
        left: 0;
        right: 0;
        pointer-events: none;
    }
    .sheet-bg {
        height: 320px;
        background-size: cover;
        background-position: center;
        filter: blur(44px) saturate(1.45);
        opacity: 0.62;
        transform: scale(1.35);
        transform-origin: top center;
    }
    /* Full height, opaque from the stop down: a 44px blur throws light well
       past the layer that casts it, and a wash that stopped where the art
       stops would leave a seam across the sheet where it ended. */
    .sheet-wash {
        bottom: 0;
        background: linear-gradient(
            to bottom,
            rgba(20, 19, 15, 0.34) 0px,
            rgba(20, 19, 15, 0.72) 210px,
            var(--bg) 470px
        );
    }
    :global([data-theme="light"]) .sheet-wash {
        background: linear-gradient(
            to bottom,
            rgba(245, 241, 234, 0.5) 0px,
            rgba(245, 241, 234, 0.78) 210px,
            var(--bg) 470px
        );
    }
    .grabber {
        width: 38px;
        height: 4px;
        border-radius: 2px;
        background: var(--border-strong);
        margin: 8px auto 0;
        pointer-events: none;
    }
    .sheet-scroll {
        /* Above the ambient layers. */
        position: relative;
        z-index: 1;
        max-height: 92vh;
        overflow-y: auto;
        overscroll-behavior: contain;
        -webkit-overflow-scrolling: touch;
        padding: 0 var(--space-5) calc(var(--space-8) + env(safe-area-inset-bottom));
        display: flex;
        flex-direction: column;
        gap: var(--space-5);
    }
    /* The dock floats over Zones and Search, so the last row of either has to
       clear it rather than spending its life underneath. */
    .sheet-scroll.docked {
        padding-bottom: calc(var(--space-8) + 72px + env(safe-area-inset-bottom));
    }
    /* On desktop the sheet becomes a centered dialog. */
    @media (min-width: 601px) {
        .sheet {
            left: 50%;
            bottom: auto;
            top: 50%;
            transform: translate(-50%, -50%);
            width: min(440px, calc(100vw - 48px));
            max-height: 88vh;
            border-radius: var(--r-xl);
            border-bottom: 1px solid var(--hairline);
        }
        .sheet-scroll {
            max-height: 88vh;
        }
    }

    /* From the desktop shell's width up, a `wide` sheet stops being a phone
       dialog and becomes a stage: closer to the size of the room it's playing
       in, with the track's own art blurred into the walls. The content grid
       lives in the player — here it only gets the bigger frame. */
    @media (min-width: 901px) {
        .sheet.stage {
            width: min(1120px, calc(100vw - 96px));
            height: min(740px, calc(100vh - 64px));
            max-height: none;
        }
        .sheet.stage .sheet-scroll {
            max-height: none;
            height: 100%;
            padding: 0 44px 40px;
        }
        /* The stage is lit end to end rather than washed from the top — and
           it is painted at half size and scaled up, which is the same 64px of
           blur over a quarter of the pixels. At the stage's size the full-res
           version is a genuinely expensive first paint, and it lands on the
           frame the sheet is trying to open on. */
        .sheet.stage .sheet-bg {
            inset: -12% auto auto -12%;
            width: 62%;
            height: 62%;
            opacity: 1;
            transform: scale(2);
            transform-origin: top left;
            filter: blur(32px) saturate(1.35);
        }
        .sheet.stage .sheet-wash {
            background: linear-gradient(
                158deg,
                rgba(20, 19, 15, 0.42) 0%,
                rgba(20, 19, 15, 0.66) 55%,
                rgba(20, 19, 15, 0.85) 100%
            );
        }
        /* The stage's head takes the scrim and drops the frost. A
           `backdrop-filter` over an already-blurred wash adds nothing you can
           see, and it is not free: it repaints the whole band on every frame
           of a scroll *and* on every frame of the opening clip, which is why
           the transition has to switch it off and hand it back — a hand-back
           that reads as a pop against a lit backdrop. On a phone the head sits
           over real content and the frost is doing work, so it stays there. */
        .sheet.stage .sheet-top {
            backdrop-filter: none;
            -webkit-backdrop-filter: none;
        }
        :global([data-theme="light"]) .sheet.stage .sheet-wash {
            background: linear-gradient(
                158deg,
                rgba(245, 241, 234, 0.48) 0%,
                rgba(245, 241, 234, 0.7) 55%,
                rgba(245, 241, 234, 0.86) 100%
            );
        }
        /* The hairline vanishes against a bright backdrop — the stage's edge
           borrows the accent instead. */
        .sheet.stage {
            border-color: rgba(245, 189, 110, 0.16);
        }
        :global([data-theme="light"]) .sheet.stage {
            border-color: rgba(201, 122, 31, 0.24);
        }
    }
    /* The bar is the drag handle on phones, so the browser must not claim
       the gesture for scrolling first. */
    @media (max-width: 600px) {
        .sheet-top {
            touch-action: none;
            cursor: grab;
        }
        .sheet.dragging .sheet-top {
            cursor: grabbing;
        }
        .sheet-scroll {
            touch-action: pan-y;
        }
    }

    /* The band is translucent and blurred, and its bottom edge fades out — art
       and rows dissolve as they pass underneath instead of being cut off
       against an opaque slab. An opaque bar was tried first and read as a
       floating block cutting the content in half. */
    .sheet-top {
        --fade: 22px;
        position: sticky;
        top: 0;
        z-index: var(--z-raised);
        margin: 0 calc(var(--space-5) * -1) calc(var(--fade) * -1);
        padding: 0 var(--space-5) var(--fade);
        background: var(--bg-bar);
        backdrop-filter: blur(18px) saturate(1.3);
        -webkit-backdrop-filter: blur(18px) saturate(1.3);
        -webkit-mask-image: linear-gradient(to bottom, #000 calc(100% - var(--fade)), transparent);
        mask-image: linear-gradient(to bottom, #000 calc(100% - var(--fade)), transparent);
    }
    /* Over an ambient backdrop the bar stops being a slab and becomes a
       scrim: enough to keep the title legible, thin enough that the light
       behind it is the thing you notice. */
    .sheet.ambient .sheet-top {
        background: linear-gradient(
            to bottom,
            rgba(20, 19, 15, 0.6) 0%,
            rgba(20, 19, 15, 0) 100%
        );
    }
    :global([data-theme="light"]) .sheet.ambient .sheet-top {
        background: linear-gradient(
            to bottom,
            rgba(245, 241, 234, 0.68) 0%,
            rgba(245, 241, 234, 0) 100%
        );
    }
    .player-head {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
        padding: var(--space-2) 0 var(--space-3);
    }
    .p-icon {
        width: 38px;
        height: 38px;
        border-radius: 50%;
        background: var(--card-2);
        border: 1px solid var(--hairline);
    }
    /* Keeps the title centered on sheets whose head carries no action chip. */
    .p-icon-gap {
        width: 38px;
        flex-shrink: 0;
    }
    .p-onair {
        text-align: center;
        min-width: 0;
    }
    .p-onair-name {
        font-size: 13px;
        font-weight: 600;
        margin-top: 2px;
    }
    .p-onair-sub {
        font-size: 11.5px;
        color: var(--text-mute);
        margin-top: 2px;
        line-height: 1.35;
    }

    @media (prefers-reduced-motion: reduce) {
        /* The drag snap-back is an inline style, so it needs its own override. */
        .sheet {
            transition: none !important;
        }
    }
</style>
