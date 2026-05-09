<script lang="ts">
    import type { Snippet } from "svelte";

    interface Props {
        name: string;
        meta: string;
        chips?: { text: string; tone?: "on" | "off" | "" }[];
        actions?: Snippet;
    }
    let { name, meta, chips = [], actions }: Props = $props();
</script>

<article class="card">
    <div class="head">
        <div class="info">
            <div class="name">{name}</div>
            <div class="meta">{meta}</div>
            {#if chips.length}
                <div class="chips">
                    {#each chips as c}
                        <span class="chip" data-tone={c.tone ?? ""}>{c.text}</span>
                    {/each}
                </div>
            {/if}
        </div>
    </div>
    {#if actions}
        <div class="actions">{@render actions()}</div>
    {/if}
</article>

<style>
    .card {
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        border-radius: var(--radius-lg);
        padding: var(--space-4) var(--space-5);
        display: grid;
        grid-template-columns: 1fr auto;
        gap: var(--space-3);
        align-items: start;
    }
    .info { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
    .name { font-weight: 600; font-size: 1rem; }
    .meta { color: var(--text-muted); font-size: 12px; }
    .chips {
        display: flex;
        flex-wrap: wrap;
        gap: 6px;
        margin-top: var(--space-2);
    }
    .chip {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        padding: 3px 10px;
        border-radius: 999px;
        background: var(--surface);
        border: 1px solid var(--border);
        font-size: 12px;
        color: var(--text-muted);
    }
    .chip[data-tone="on"]  { color: var(--success); border-color: rgba(52, 211, 153, 0.3); }
    .chip[data-tone="off"] { color: var(--danger);  border-color: rgba(248, 113, 113, 0.3); }
    .actions {
        display: flex;
        flex-wrap: wrap;
        gap: var(--space-2);
        justify-content: flex-end;
    }
    @media (max-width: 600px) {
        .card { grid-template-columns: 1fr; }
        .actions { justify-content: flex-start; }
    }
</style>
