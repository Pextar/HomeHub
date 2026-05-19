<script lang="ts">
    import type { Snippet } from "svelte";
    import { closeModal } from "../lib/modal.svelte";

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

    // After mount, focus the first focusable element inside the dialog so
    // keyboard users land where they expect.
    let dialog: HTMLDivElement | undefined = $state();
    $effect(() => {
        if (!dialog) return;
        const focusables = dialog.querySelectorAll<HTMLElement>(
            "button, [href], input, select, textarea, [tabindex]:not([tabindex='-1'])"
        );
        const first = Array.from(focusables).find(el => !el.hasAttribute("disabled"));
        first?.focus();
    });

    // Drag-to-dismiss (mobile bottom sheet only).
    // The handle must be outside the scrollable .body element — otherwise iOS
    // intercepts the touch for scroll before setPointerCapture can claim it.
    let dragY = $state(0);
    let dragging = $state(false);
    let dragStartY = 0;
    let dismissing = false;

    function onHandlePointerDown(e: PointerEvent) {
        if (dismissing) return;
        dragging = true;
        dragStartY = e.clientY;
        dragY = 0;
        (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
        e.preventDefault();
    }
    function onHandlePointerMove(e: PointerEvent) {
        if (!dragging) return;
        dragY = Math.max(0, e.clientY - dragStartY);
    }
    function onHandlePointerUp() {
        if (!dragging) return;
        dragging = false;
        if (dragY > 80) {
            dismissing = true;
            dragY = 600;
            setTimeout(() => closeModal(), 230);
        } else {
            requestAnimationFrame(() => { dragY = 0; });
        }
    }
    function onHandlePointerCancel() {
        if (!dragging) return;
        dragging = false;
        requestAnimationFrame(() => { dragY = 0; });
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
        role="dialog"
        aria-modal="true"
        aria-labelledby="modal-title"
        bind:this={dialog}
        tabindex="-1"
        style:transform={dragY > 0 ? `translateY(${dragY}px)` : ''}
        style:opacity={dragY > 0 ? Math.max(0.4, 1 - dragY / 300) : undefined}
        style:transition={dragging ? 'none' : dragY > 0 ? 'transform 0.22s ease-in, opacity 0.22s ease-in' : 'transform 0.3s cubic-bezier(0.34, 1.56, 0.64, 1)'}
    >
        <!-- Drag handle: rendered in DOM always but only visible + interactive
             on mobile (≤600 px). Sits outside .body so iOS scroll capture
             cannot intercept the pointer before setPointerCapture fires. -->
        <div class="drag-handle" aria-hidden="true"
            onpointerdown={onHandlePointerDown}
            onpointermove={onHandlePointerMove}
            onpointerup={onHandlePointerUp}
            onpointercancel={onHandlePointerCancel}></div>

        <div class="head">
            <div>
                <h2 id="modal-title">{title}</h2>
                {#if subtitle}<p class="subtitle">{subtitle}</p>{/if}
            </div>
            <button class="icon-btn" aria-label="Close" onclick={() => closeModal()}>×</button>
        </div>
        {#if body}
            <div class="body">{@render body()}</div>
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
    }
    :global([data-theme="light"]) .root { background: rgba(20, 24, 38, 0.40); }
    @keyframes fade { from { opacity: 0 } to { opacity: 1 } }

    /* ── Dialog shell — overflow: hidden so only .body scrolls.
       The drag handle and head/actions live outside the scroll zone. ── */
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
        display: flex; align-items: flex-start; justify-content: space-between;
        gap: var(--space-3);
        padding: var(--space-6) var(--space-6) 0;
        flex-shrink: 0;
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

        /* Pill handle — touch-action: none keeps iOS from hijacking the
           pointer for scroll before setPointerCapture fires. */
        .drag-handle {
            display: block;
            width: 36px;
            height: 5px;
            border-radius: 999px;
            background: var(--border-strong);
            margin: var(--space-3) auto var(--space-1);
            align-self: center;
            flex-shrink: 0;
            touch-action: none;
            cursor: grab;
            /* Larger invisible tap target */
            padding: 12px 32px;
            box-sizing: content-box;
        }
        .drag-handle:active { cursor: grabbing; }

        .head { padding: var(--space-2) var(--space-4) 0; }
        .body { padding: var(--space-3) var(--space-4); }
        .actions {
            flex-direction: column;
            padding: var(--space-3) var(--space-4) var(--space-4);
        }
        .actions :global(.btn) { width: 100%; justify-content: center; }
    }
</style>
