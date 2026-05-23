<script lang="ts">
    import type { Snippet } from "svelte";

    interface Props {
        title: string;
        subtitle?: string;
        actions?: Snippet;
    }
    let { title, subtitle, actions }: Props = $props();
</script>

<header class="topbar">
    <div class="title">
        <h1>{title}</h1>
        {#if subtitle}<p class="subtitle">{subtitle}</p>{/if}
    </div>
    {#if actions}
        <div class="actions">{@render actions()}</div>
    {/if}
</header>

<style>
    .topbar {
        display: flex;
        align-items: flex-start;
        justify-content: space-between;
        gap: var(--space-3);
        /* Never stack — title left, button right, always */
        flex-wrap: nowrap;
    }
    .title { display: flex; flex-direction: column; gap: 4px; flex: 1; min-width: 0; }
    .title h1 {
        font-family: var(--font-sans);
        font-size: 30px;
        font-weight: 600;
        letter-spacing: -0.03em;
        color: var(--text);
        line-height: 1.1;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .subtitle {
        color: var(--text-mute);
        font-size: 12.5px;
        font-weight: 500;
        letter-spacing: -0.01em;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .actions { display: flex; align-items: center; gap: var(--space-2); flex-shrink: 0; }

    /* Large-title feel on mobile: bigger size, still the 600 redesign weight */
    @media (max-width: 900px) {
        h1 {
            font-size: 1.75rem;
            letter-spacing: -0.03em;
        }
        .subtitle { font-size: 12px; }
    }

    /* On phones, align title top and button bottom so they don't fight */
    @media (pointer: coarse) {
        .topbar { align-items: flex-end; padding-bottom: var(--space-1); }
    }
</style>
