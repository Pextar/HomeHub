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
          {#each chips as c (c.text)}
            <span class="entity-chip" data-tone={c.tone ?? ""}>{c.text}</span>
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
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    padding: var(--space-4) var(--space-5);
    display: grid;
    grid-template-columns: 1fr auto;
    gap: var(--space-3);
    align-items: start;
    transition: border-color var(--t-fast), transform var(--t-fast), box-shadow var(--t-fast);
  }
  @media (hover: hover) {
    .card:hover { border-color: var(--border-strong); transform: translateY(-2px); box-shadow: var(--shadow-md); }
  }
  .info {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }
  .name {
    font-weight: 600;
    font-size: 1rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .meta {
    color: var(--text-muted);
    font-size: 12px;
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-top: var(--space-2);
  }
  /* Renamed from .chip to .entity-chip to avoid colliding with the new
     global .chip utility class. */
  .entity-chip {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 3px 10px;
    border-radius: var(--radius-pill);
    background: var(--card-3);
    border: 1px solid var(--border);
    font-size: 12px;
    color: var(--text-muted);
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .entity-chip[data-tone="on"] {
    color: var(--on);
    background: var(--on-soft);
    border-color: transparent;
  }
  .entity-chip[data-tone="off"] {
    color: var(--danger);
    background: var(--danger-soft);
    border-color: transparent;
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    justify-content: flex-end;
  }
  @media (max-width: 600px) {
    .card {
      grid-template-columns: 1fr;
    }
    .actions {
      justify-content: flex-start;
    }
  }
</style>
