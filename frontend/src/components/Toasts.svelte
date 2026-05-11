<script lang="ts">
    import { toasts } from "../lib/stores.svelte";
</script>

<div class="toasts" aria-live="polite" aria-atomic="false">
    {#each toasts.items as t (t.id)}
        <div class="toast" data-tone={t.tone} role="status">
            <div class="body">
                <div class="title">{t.title}</div>
                {#if t.message}<div class="msg">{t.message}</div>{/if}
            </div>
            <button class="close" aria-label="Dismiss" onclick={() => toasts.dismiss(t.id)}>×</button>
        </div>
    {/each}
</div>

<style>
    .toasts {
        position: fixed;
        bottom: var(--space-5);
        right: var(--space-5);
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
        z-index: 200;
        max-width: 360px;
        pointer-events: none;
    }
    .toast {
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        border-left: 3px solid var(--info);
        border-radius: var(--radius-md);
        padding: var(--space-3) var(--space-4);
        box-shadow: var(--shadow-md);
        display: flex;
        gap: var(--space-3);
        align-items: flex-start;
        animation: in 0.18s ease both;
        pointer-events: auto;
    }
    .toast[data-tone="success"] { border-left-color: var(--success); }
    .toast[data-tone="error"] { border-left-color: var(--danger); }
    .toast[data-tone="warn"] { border-left-color: var(--warn); }
    .body { flex: 1; min-width: 0; }
    .title { font-weight: 600; }
    .msg { color: var(--text-muted); font-size: 13px; margin-top: 2px; }
    .close {
        background: transparent; border: 0;
        color: var(--text-faint); cursor: pointer;
        padding: 2px 4px;
        font-size: 18px;
        line-height: 1;
    }
    @keyframes in {
        from { opacity: 0; transform: translateY(8px); }
        to { opacity: 1; transform: translateY(0); }
    }

    @media (max-width: 900px) {
        .toasts {
            bottom: calc(60px + env(safe-area-inset-bottom) + var(--space-3));
            right: var(--space-3);
            left: var(--space-3);
            max-width: none;
        }
    }
</style>
