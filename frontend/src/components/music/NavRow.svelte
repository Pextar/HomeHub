<script lang="ts">
    /**
     * The §11 list row, as Music uses it: 36px icon left, title and subline
     * middle, chevron right, the whole thing one target.
     *
     * Four of them across the module — the way through to Speakers, the Live
     * updates row, Zones' pointer at unreachable speakers, and the count on
     * Home — which is three too many copies of the same forty lines.
     *
     * `on` takes the sanctioned "ON" treatment rather than a status colour of
     * its own: push being up is the same kind of fact as a lit lamp.
     */
    import type { Snippet } from "svelte";
    import Icon from "../Icon.svelte";
    import type { ComponentProps } from "svelte";

    type IconName = ComponentProps<typeof Icon>["name"];

    let {
        icon,
        /** Lights the icon amber. */
        on = false,
        title,
        sub,
        /** Trailing count, before the chevron. Mono, per §2. */
        count = undefined,
        onClick,
    }: {
        icon: IconName;
        on?: boolean;
        title: string | Snippet;
        sub: Snippet;
        count?: number;
        onClick: () => void;
    } = $props();
</script>

<button class="lu-row" onclick={onClick}>
    <span class="lu-ico" class:on><Icon name={icon} size={18} /></span>
    <span class="lu-meta">
        <span class="lu-title">
            {#if typeof title === "string"}{title}{:else}{@render title()}{/if}
        </span>
        <span class="lu-sub">{@render sub()}</span>
    </span>
    {#if count !== undefined}
        <span class="lu-count mono">{count}</span>
    {/if}
    <span class="lu-chev" aria-hidden="true"><Icon name="chevronDown" size={18} /></span>
</button>

<style>
    .lu-row {
        width: 100%;
        margin-top: var(--space-4);
        display: flex;
        align-items: center;
        gap: var(--space-3);
        min-height: 60px;
        padding: var(--space-3) var(--space-4);
        text-align: left;
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        border-radius: var(--radius-lg);
        color: inherit;
        font: inherit;
        cursor: pointer;
        transition: background var(--t-fast), border-color var(--t-fast);
    }
    .lu-ico {
        flex-shrink: 0;
        width: 36px;
        height: 36px;
        display: grid;
        place-items: center;
        border-radius: var(--radius-md);
        background: var(--surface);
        color: var(--text-mute);
    }
    .lu-ico.on {
        background: var(--primary-soft);
        color: var(--primary);
    }
    .lu-meta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
    .lu-title { font-size: 14px; font-weight: 600; letter-spacing: -0.01em; }
    .lu-sub { font-size: 12px; color: var(--text-mute); line-height: 1.4; }
    .lu-chev { flex-shrink: 0; display: flex; color: var(--text-dim); transform: rotate(-90deg); }
    @media (hover: hover) {
        .lu-row:hover { background: var(--bg-raised); border-color: var(--border-strong); }
    }
</style>
