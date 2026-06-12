<script lang="ts">
    import { toasts } from "../lib/stores.svelte";
    import { fly, scale } from "svelte/transition";
    import { flip } from "svelte/animate";
    import { cubicOut } from "svelte/easing";
    import { dur } from "../lib/motion";
</script>

<div class="toasts" aria-live="polite" aria-atomic="false">
    {#each toasts.items as t (t.id)}
        <div class="toast" data-tone={t.tone}
            animate:flip={{ duration: dur(220), easing: cubicOut }}
            in:fly={{ y: 16, duration: dur(240), easing: cubicOut }}
            out:scale={{ start: 0.9, opacity: 0, duration: dur(160) }}>
            <div class="body">
                <div class="title">{t.title}</div>
                {#if t.message}<div class="msg">{t.message}</div>{/if}
            </div>
            {#if t.action}
                <button class="action" onclick={() => { t.action!.onClick(); toasts.dismiss(t.id); }}>
                    {t.action.label}
                </button>
            {/if}
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
        --tone: var(--info);
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        border-left: 3px solid var(--tone);
        border-radius: var(--radius-md);
        padding: var(--space-3) var(--space-4);
        box-shadow: var(--shadow-md);
        display: flex;
        gap: var(--space-3);
        align-items: flex-start;
        pointer-events: auto;
    }
    .toast[data-tone="success"] { --tone: var(--success); }
    .toast[data-tone="error"]   { --tone: var(--danger); }
    .toast[data-tone="warn"]    { --tone: var(--warn); }
    .body { flex: 1; min-width: 0; }
    .title {
        font-weight: 600;
        display: flex;
        align-items: center;
        gap: var(--space-2);
    }
    /* Colored tone marker beside the title — keeps the warm accent visible
       even when the thin left border is easy to miss. */
    .title::before {
        content: "";
        width: 7px;
        height: 7px;
        border-radius: 50%;
        background: var(--tone);
        flex-shrink: 0;
    }
    .msg { color: var(--text-muted); font-size: 13px; margin-top: 2px; }
    .close {
        background: transparent; border: 0;
        color: var(--text-faint); cursor: pointer;
        padding: 2px 4px;
        font-size: 18px;
        line-height: 1;
    }
    .action {
        background: transparent;
        border: 1px solid var(--border-strong);
        border-radius: var(--radius-sm);
        color: var(--text);
        font-weight: 600;
        font-size: 12px;
        padding: 4px 10px;
        cursor: pointer;
    }
    .action:hover { background: var(--surface-hover); }

    @media (max-width: 900px) {
        .toasts {
            bottom: calc(60px + env(safe-area-inset-bottom) + var(--space-3));
            right: var(--space-3);
            left: var(--space-3);
            max-width: none;
        }
    }

    /* Touch screens: give the dismiss/action controls a real tap target. */
    @media (pointer: coarse) {
        .close {
            min-width: 44px;
            min-height: 44px;
            padding: 0;
            display: grid;
            place-items: center;
            font-size: 22px;
            margin: -8px -8px -8px 0;
        }
        .action { min-height: 40px; padding: 8px 14px; font-size: 14px; }
    }
</style>
