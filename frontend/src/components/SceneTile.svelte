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
    <div class="tile-icon">▶</div>
    <div class="tile-body">
        <div class="name">{scene.name}</div>
        <div class="meta">{scene.actions.length} action{scene.actions.length === 1 ? "" : "s"}</div>
    </div>
</button>

<style>
    .tile {
        background: var(--surface);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        padding: var(--space-3) var(--space-4);
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
        /* Subtle gradient wash using brand colours */
        background: linear-gradient(
            135deg,
            color-mix(in srgb, var(--primary) 8%, var(--surface)) 0%,
            var(--surface) 100%
        );
        /* Left accent stripe in the brand gradient */
        box-shadow: inset 3px 0 0 var(--primary);
    }
    .tile:hover {
        border-color: var(--primary);
        background: linear-gradient(
            135deg,
            color-mix(in srgb, var(--primary) 14%, var(--surface)) 0%,
            var(--surface) 100%
        );
    }
    @media (hover: hover) {
        .tile:hover { transform: translateY(-2px); box-shadow: inset 3px 0 0 var(--primary), var(--shadow-md); }
    }
    .tile:active { transform: scale(0.97); transition-duration: 80ms; }

    /* Play icon badge */
    .tile-icon {
        width: 30px;
        height: 30px;
        border-radius: 50%;
        background: var(--primary-soft);
        color: var(--primary);
        display: grid;
        place-items: center;
        font-size: 10px;
        flex-shrink: 0;
        transition: background var(--t-fast), transform var(--t-fast);
    }
    .tile:hover .tile-icon {
        background: var(--primary);
        color: var(--primary-fg);
        transform: scale(1.1);
    }

    .tile-body { flex: 1; min-width: 0; }
    .name { font-weight: 600; font-size: 15px; }
    .meta { color: var(--text-muted); font-size: 12px; margin-top: 2px; }

    /* Touch: taller tiles, slightly larger text */
    @media (pointer: coarse) {
        .tile { min-height: 80px; }
        .name { font-size: 16px; }
    }
</style>
