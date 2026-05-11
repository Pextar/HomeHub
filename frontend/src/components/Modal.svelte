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
    >
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

    .dialog {
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        border-radius: var(--radius-xl);
        padding: var(--space-6);
        width: 100%;
        max-width: 520px;
        max-height: calc(100vh - 32px);
        overflow: auto;
        box-shadow: var(--shadow-lg);
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
        animation: pop 0.18s ease both;
    }
    .dialog.wide { max-width: 720px; }
    @keyframes pop {
        from { opacity: 0; transform: translateY(8px) scale(0.99); }
        to { opacity: 1; transform: none; }
    }
    .head {
        display: flex; align-items: flex-start; justify-content: space-between;
        gap: var(--space-3);
    }
    .head .icon-btn { font-size: 18px; }
    .subtitle { color: var(--text-muted); font-size: 13px; margin-top: 4px; }
    .body { display: flex; flex-direction: column; gap: var(--space-4); }
    .actions {
        display: flex;
        justify-content: flex-end;
        gap: var(--space-2);
        border-top: 1px solid var(--border);
        padding-top: var(--space-4);
        margin-top: var(--space-2);
    }

    @media (max-width: 600px) {
        .root { align-items: flex-end; padding: 0; }
        .dialog {
            max-width: 100% !important;
            border-bottom-left-radius: 0;
            border-bottom-right-radius: 0;
            max-height: 92vh;
            padding: var(--space-4);
            animation-name: slide-up;
        }
        @keyframes slide-up {
            from { opacity: 0; transform: translateY(40px); }
            to   { opacity: 1; transform: none; }
        }
        .actions { flex-direction: column; }
        .actions :global(.btn) { width: 100%; justify-content: center; }
    }
</style>
