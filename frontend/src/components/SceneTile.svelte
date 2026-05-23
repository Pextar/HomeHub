<script lang="ts">
    import { api } from "../lib/api";
    import { runAction } from "../lib/utils";
    import type { Scene } from "../lib/types";

    interface Props { scene: Scene; }
    let { scene }: Props = $props();
</script>

<button
    class="scene-tile"
    type="button"
    onclick={() => runAction(() => api.activateScene(scene.id), `Scene activated: ${scene.name}`)}
>
    <div class="scene-icon" aria-hidden="true">
        <span class="hue"></span>
    </div>
    <div class="scene-body">
        <div class="name">{scene.name}</div>
        <div class="meta num-display">{scene.actions.length} action{scene.actions.length === 1 ? "" : "s"}</div>
    </div>
</button>

<style>
    /* Warm-dark scene card (renamed from .tile to avoid the global .tile
       collision). Matches ScenesScreen "All scenes" cards: rounded card,
       rounded icon chip holding a coloured hue dot, name + mono count. */
    .scene-tile {
        background: var(--bg-raised);
        border: 1px solid var(--border);
        border-radius: var(--radius-lg);
        padding: var(--space-4);
        display: flex;
        align-items: center;
        gap: var(--space-3);
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
    .scene-tile:hover { border-color: var(--primary); }
    @media (hover: hover) {
        .scene-tile:hover { transform: translateY(-2px); box-shadow: var(--shadow-md); }
    }
    .scene-tile:active { transform: scale(0.97); transition-duration: 80ms; }

    /* Rounded icon chip holding the scene hue dot */
    .scene-icon {
        width: 32px;
        height: 32px;
        border-radius: var(--radius-sm);
        background: var(--card-3);
        display: grid;
        place-items: center;
        flex-shrink: 0;
        transition: background var(--t-fast), transform var(--t-fast);
    }
    .scene-icon .hue {
        width: 14px;
        height: 14px;
        border-radius: 50%;
        background: var(--primary);
        transition: box-shadow var(--t-fast);
    }
    .scene-tile:hover .scene-icon { background: var(--primary-soft); }
    .scene-tile:hover .scene-icon .hue { box-shadow: 0 0 12px var(--on-glow); }

    .scene-body { flex: 1; min-width: 0; }
    .name { font-weight: 600; font-size: 15px; }
    .meta { color: var(--text-faint); font-size: 11.5px; margin-top: 4px; font-variant-numeric: tabular-nums; }

    /* Touch: taller tiles, slightly larger text */
    @media (pointer: coarse) {
        .scene-tile { min-height: 80px; }
        .name { font-size: 16px; }
    }
</style>
