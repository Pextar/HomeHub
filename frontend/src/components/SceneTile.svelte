<script lang="ts">
    import { api } from "../lib/api";
    import { runAction } from "../lib/utils";
    import type { Scene } from "../lib/types";

    interface Props { scene: Scene; }
    let { scene }: Props = $props();
</script>

<button
    class="tile"
    type="button"
    onclick={() => runAction(() => api.activateScene(scene.id), `Scene activated: ${scene.name}`)}
>
    <div class="name">{scene.name}</div>
    <div class="meta">{scene.actions.length} action{scene.actions.length === 1 ? "" : "s"}</div>
</button>

<style>
    .tile {
        background: var(--surface);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        padding: var(--space-4);
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
        cursor: pointer;
        touch-action: manipulation;
        transition: border-color var(--t-fast), background var(--t-fast),
            transform var(--t-fast), box-shadow var(--t-fast);
        text-align: left;
        font: inherit;
        color: inherit;
        width: 100%;
        min-height: 72px;
    }
    .tile:hover { border-color: var(--border-strong); background: var(--surface-hover); }
    @media (hover: hover) {
        .tile:hover { transform: translateY(-2px); box-shadow: var(--shadow-md); }
    }
    .tile:active { transform: scale(0.97); background: var(--surface-hover); transition-duration: 80ms; }
    .name { font-weight: 600; font-size: 15px; }
    .meta { color: var(--text-muted); font-size: 12px; }

    /* Touch: taller tiles, slightly larger text */
    @media (pointer: coarse) {
        .tile { min-height: 80px; padding: var(--space-4) var(--space-4); }
        .name { font-size: 16px; }
    }
</style>
