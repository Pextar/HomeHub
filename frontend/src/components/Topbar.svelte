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
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
        /* Never stack — title left, button right, always */
        flex-wrap: nowrap;
    }
    .title { display: flex; flex-direction: column; gap: 2px; flex: 1; min-width: 0; }
    .subtitle { color: var(--text-muted); font-size: 13px; }
    .actions { display: flex; gap: var(--space-2); flex-shrink: 0; }

    /* iOS large-title feel on mobile: heavier weight, bigger size */
    @media (max-width: 900px) {
        h1 {
            font-size: 1.75rem;
            font-weight: 800;
            letter-spacing: -0.025em;
        }
        .subtitle { font-size: 12px; }
    }
</style>
