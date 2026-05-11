<script lang="ts">
  import Icon from "./Icon.svelte";
  import { api } from "../lib/api";
  import { describeTarget, formatCountdown } from "../lib/utils";
  import { toasts, data } from "../lib/stores.svelte";
  import type { Timer } from "../lib/types";
  import { onMount, untrack } from "svelte";

  interface Props {
    timer: Timer;
  }
  let { timer }: Props = $props();

  const target = $derived(describeTarget(timer.target_type, timer.target_id));
  const firesAt = $derived(new Date(timer.fires_at));
  let countdown = $state(untrack(() => formatCountdown(timer.fires_at)));

  // Tick the live countdown once per second.
  onMount(() => {
    const id = setInterval(() => {
      countdown = formatCountdown(firesAt);
    }, 1000);
    return () => clearInterval(id);
  });

  async function cancel() {
    try {
      await api.deleteTimer(timer.id);
      toasts.success("Timer cancelled");
      await data.refresh();
    } catch (e) {
      toasts.error("Failed", (e as Error).message);
    }
  }
</script>

<div class="row">
  <span class="action" data-action={timer.action === "on" ? "on" : "off"}
    >{timer.action}</span
  >
  <div class="info">
    <div>{target.kind}: {target.label}</div>
    <div class="when">{firesAt.toLocaleString()}</div>
  </div>
  <span class="countdown">{countdown}</span>
  <button class="icon-btn danger" aria-label="Cancel timer" onclick={cancel}>
    <Icon name="trash" size={16} />
  </button>
</div>

<style>
  .row {
    display: grid;
    grid-template-columns: auto 1fr auto auto;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
  }
  .action {
    padding: 2px 10px;
    border-radius: 999px;
    font-size: 12px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .action[data-action="on"] {
    background: var(--success-soft);
    color: var(--success);
  }
  .action[data-action="off"] {
    background: var(--danger-soft);
    color: var(--danger);
  }

  .info {
    min-width: 0;
  }
  .when {
    color: var(--text-faint);
    font-size: 12px;
  }

  .countdown {
    font-family: var(--font-mono);
    font-weight: 600;
    color: var(--primary);
  }
</style>
