<script lang="ts">
    import type { Snippet } from "svelte";
    import { onMount } from "svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { lockBodyScroll, unlockBodyScroll } from "../lib/scroll-lock";

    interface Props {
        title: string;
        subtitle?: string;
        body?: Snippet;
        actions?: Snippet;
        size?: "default" | "wide";
    }
    let { title, subtitle, body, actions, size = "default" }: Props = $props();

    function onKey(e: KeyboardEvent) {
        if (e.key === "Escape") closeModal();
    }

    function onBackdrop(e: MouseEvent) {
        if (e.target === e.currentTarget) closeModal();
    }

    // Lock background scroll for the lifetime of the modal. iOS won't otherwise
    // honour overflow: hidden on body, so without this the page can scroll
    // behind the sheet when a touch overscrolls.
    onMount(() => {
        lockBodyScroll();
        return () => unlockBodyScroll();
    });

    let dialog: HTMLDivElement | undefined = $state();
    let bodyEl: HTMLDivElement | undefined = $state();

    // Focus the first focusable element after mount.
    $effect(() => {
        if (!dialog) return;
        const focusables = dialog.querySelectorAll<HTMLElement>(
            "button, [href], input, select, textarea, [tabindex]:not([tabindex='-1'])"
        );
        const first = Array.from(focusables).find(el => !el.hasAttribute("disabled"));
        first?.focus();
    });

    // ── Drag-to-dismiss (mobile bottom sheet) ────────────────────────────
    // Two entry points:
    //   1. The head (pill + title row) — always drags the sheet.
    //   2. The body — drags only when scrollTop === 0 AND the gesture is a
    //      net downward pull. Otherwise the body scrolls normally.
    //
    // The intent gate on the body (`pendingBody`) is what makes nested
    // scrolling feel right: we don't claim the gesture until the user has
    // moved a few pixels down, so taps and small jitters don't accidentally
    // start a drag.
    let dragY = $state(0);
    let dragging = $state(false);
    let pendingBody = false;
    let dragStartY = 0;
    let dragStartX = 0;
    let dismissing = false;

    function isMobile() {
        return window.matchMedia("(max-width: 600px)").matches;
    }

    function startDrag(e: PointerEvent, target: HTMLElement) {
        dragging = true;
        dragStartY = e.clientY;
        dragStartX = e.clientX;
        dragY = 0;
        try { target.setPointerCapture(e.pointerId); } catch { /* not capturable */ }
    }

    // ── Head: always drags ───────────────────────────────────────────────
    function onHeadPointerDown(e: PointerEvent) {
        if (dismissing || !isMobile()) return;
        // Don't hijack pointerdown on the close button.
        if ((e.target as HTMLElement).closest("button")) return;
        startDrag(e, e.currentTarget as HTMLElement);
        e.preventDefault();
    }

    function onHeadPointerMove(e: PointerEvent) {
        if (!dragging) return;
        dragY = Math.max(0, e.clientY - dragStartY);
    }

    function onHeadPointerUp() {
        if (!dragging) return;
        finishDrag();
    }

    function onHeadPointerCancel() {
        if (!dragging) return;
        dragging = false;
        requestAnimationFrame(() => { dragY = 0; });
    }

    // ── Body: drags when scrolled to top, otherwise scrolls ──────────────
    function onBodyPointerDown(e: PointerEvent) {
        if (dismissing || !isMobile()) return;
        if (e.pointerType === "mouse") return; // mouse uses head only
        if (!bodyEl) return;
        if (bodyEl.scrollTop > 0) return; // body is scrolled — let it scroll
        // Don't drag if the touch lands on an interactive control.
        const target = e.target as HTMLElement;
        if (target.closest("input, textarea, select, button, a, [role='slider']")) return;
        pendingBody = true;
        dragStartY = e.clientY;
        dragStartX = e.clientX;
    }

    function onBodyPointerMove(e: PointerEvent) {
        if (dragging) {
            dragY = Math.max(0, e.clientY - dragStartY);
            // While dragging from body, suppress scroll.
            e.preventDefault();
            return;
        }
        if (!pendingBody) return;
        const dy = e.clientY - dragStartY;
        const dx = e.clientX - dragStartX;
        // Need a clear downward gesture to claim it as a drag.
        if (dy > 8 && dy > Math.abs(dx)) {
            pendingBody = false;
            startDrag(e, bodyEl!);
            dragY = dy;
            e.preventDefault();
        } else if (dy < -4 || Math.abs(dx) > 12) {
            // Upward scroll or horizontal swipe — release intent.
            pendingBody = false;
        }
    }

    function onBodyPointerUp() {
        pendingBody = false;
        if (!dragging) return;
        finishDrag();
    }

    function onBodyPointerCancel() {
        pendingBody = false;
        if (!dragging) return;
        dragging = false;
        requestAnimationFrame(() => { dragY = 0; });
    }

    function finishDrag() {
        dragging = false;
        if (dragY > 90) {
            dismissing = true;
            dragY = 600;
            setTimeout(() => closeModal(), 230);
        } else {
            requestAnimationFrame(() => { dragY = 0; });
        }
    }
</script>

<svelte:window onkeydown={onKey} />

<div
    class="root"
    role="presentation"
    onclick={onBackdrop}
    onkeydown={(e) => { if (e.key === "Escape") closeModal(); }}
    aria-hidden="false"
>
    <div
        class="dialog"
        class:wide={size === "wide"}
        class:dragging
        role="dialog"
        aria-modal="true"
        aria-labelledby="modal-title"
        bind:this={dialog}
        tabindex="-1"
        style:transform={dragY > 0 ? `translateY(${dragY}px)` : ''}
        style:opacity={dragY > 0 ? Math.max(0.4, 1 - dragY / 300) : undefined}
        style:transition={dragging ? 'none' : dragY > 0 ? 'transform 0.22s ease-in, opacity 0.22s ease-in' : 'transform 0.3s cubic-bezier(0.34, 1.56, 0.64, 1)'}
    >
        <!-- Head doubles as the drag affordance on mobile: pill + title row.
             Touch-action: none on mobile prevents iOS from claiming the
             gesture for scroll before we get pointermove. -->
        <div
            class="head"
            role="none"
            onpointerdown={onHeadPointerDown}
            onpointermove={onHeadPointerMove}
            onpointerup={onHeadPointerUp}
            onpointercancel={onHeadPointerCancel}
        >
            <div class="drag-handle" aria-hidden="true"></div>
            <div class="head-row">
                <div>
                    <h2 id="modal-title">{title}</h2>
                    {#if subtitle}<p class="subtitle">{subtitle}</p>{/if}
                </div>
                <button class="icon-btn" aria-label="Close" onclick={() => closeModal()}>×</button>
            </div>
        </div>
        {#if body}
            <div
                class="body"
                role="none"
                bind:this={bodyEl}
                onpointerdown={onBodyPointerDown}
                onpointermove={onBodyPointerMove}
                onpointerup={onBodyPointerUp}
                onpointercancel={onBodyPointerCancel}
            >{@render body()}</div>
        {/if}
        {#if actions}
            <div class="actions">{@render actions()}</div>
        {/if}
    </div>
</div>

<style>
    .root {
        position: fixed; inset: 0;
        background: rgba(8, 11, 22, 0.65);
        backdrop-filter: blur(4px);
        display: grid; place-items: center;
        z-index: 150;
        padding: var(--space-4);
        animation: fade 0.15s ease;
        /* Prevent any overscroll from leaking to the underlying document. */
        overscroll-behavior: contain;
    }
    :global([data-theme="light"]) .root { background: rgba(20, 24, 38, 0.40); }
    @keyframes fade { from { opacity: 0 } to { opacity: 1 } }

    .dialog {
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        border-radius: var(--radius-xl);
        width: 100%;
        max-width: 520px;
        max-height: calc(100vh - 32px);
        box-shadow: var(--shadow-lg);
        display: flex;
        flex-direction: column;
        overflow: hidden;
        animation: pop 0.18s ease both;
    }
    .dialog.wide { max-width: 720px; }
    @keyframes pop {
        from { opacity: 0; transform: translateY(8px) scale(0.99); }
        to { opacity: 1; transform: none; }
    }

    .drag-handle { display: none; }

    .head {
        display: flex;
        flex-direction: column;
        padding: var(--space-6) var(--space-6) 0;
        flex-shrink: 0;
    }
    .head-row {
        display: flex; align-items: flex-start; justify-content: space-between;
        gap: var(--space-3);
    }
    .head .icon-btn { font-size: 18px; }
    .subtitle { color: var(--text-muted); font-size: 13px; margin-top: 4px; }

    .body {
        display: flex; flex-direction: column; gap: var(--space-4);
        padding: var(--space-4) var(--space-6);
        overflow-y: auto;
        flex: 1 1 auto;
        min-height: 0;
        -webkit-overflow-scrolling: touch;
        /* Don't let body scroll bleed into the page underneath. */
        overscroll-behavior: contain;
    }

    .actions {
        display: flex;
        justify-content: flex-end;
        gap: var(--space-2);
        border-top: 1px solid var(--border);
        padding: var(--space-4) var(--space-6);
        flex-shrink: 0;
    }

    /* ── Mobile: bottom sheet ── */
    @media (max-width: 600px) {
        .root { align-items: flex-end; padding: 0; }
        .dialog {
            max-width: 100% !important;
            border-bottom-left-radius: 0;
            border-bottom-right-radius: 0;
            max-height: 92vh;
            animation-name: slide-up;
        }
        @keyframes slide-up {
            from { opacity: 0; transform: translateY(40px); }
            to   { opacity: 1; transform: none; }
        }

        /* The whole head bar acts as the drag handle. touch-action: none
           prevents iOS from claiming the gesture for scroll. */
        .head {
            touch-action: none;
            cursor: grab;
            padding: 0 var(--space-4);
            /* Make the head feel like a "grippable" area without changing
               visual padding too much. */
            padding-top: var(--space-1);
        }
        .dialog.dragging .head { cursor: grabbing; }

        /* Pill — purely visual on mobile, the head wrapper handles input. */
        .drag-handle {
            display: block;
            width: 40px;
            height: 5px;
            border-radius: 999px;
            background: var(--border-strong);
            margin: var(--space-2) auto var(--space-2);
            flex-shrink: 0;
            pointer-events: none;
        }

        .head-row { padding: var(--space-1) 0 0; }
        .body {
            padding: var(--space-3) var(--space-4);
            /* `pan-y` lets vertical scrolling work but blocks the browser
               from interpreting touches as horizontal swipes / back-gesture. */
            touch-action: pan-y;
        }
        .actions {
            flex-direction: column;
            padding: var(--space-3) var(--space-4) var(--space-4);
        }
        .actions :global(.btn) { width: 100%; justify-content: center; }
    }
</style>
