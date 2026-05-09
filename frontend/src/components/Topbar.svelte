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
        gap: var(--space-4);
        flex-wrap: wrap;
    }
    .title { display: flex; flex-direction: column; gap: 2px; }
    .subtitle { color: var(--text-muted); font-size: 13px; }
    .actions { display: flex; gap: var(--space-2); flex-wrap: wrap; }
    @media (max-width: 480px) {
        .topbar { flex-direction: column; align-items: stretch; }
    }
</style>
