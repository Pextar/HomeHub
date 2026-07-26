<script lang="ts">
    /**
     * One honest row where a grid of dead cards would otherwise go.
     *
     * Music uses it wherever a section has nothing to show but still owes the
     * user an explanation and a way onward: "Nothing playing" instead of one
     * idle card per zone, "Favorites need a Sonos room" instead of a rail of
     * disabled cards, "Nothing to group" instead of a blank Zones sheet.
     *
     * The `action` is a `.chip`, not a button — this is a pointer, not a
     * primary action, and the section it replaced wasn't one either.
     */
    import type { Snippet } from "svelte";
    import Icon from "../Icon.svelte";

    let {
        icon = "speaker",
        title,
        /** The explanation. A snippet, since most of them carry inline markup. */
        children,
        action = undefined,
    }: {
        icon?: "speaker";
        title: string;
        children: Snippet;
        action?: { label: string; onClick: () => void };
    } = $props();
</script>

<div class="quiet-card">
    <span class="quiet-ico"><Icon name={icon} size={20} /></span>
    <span class="quiet-meta">
        <span class="quiet-title">{title}</span>
        <span class="quiet-sub">{@render children()}</span>
    </span>
    {#if action}
        <button class="chip quiet-go" onclick={action.onClick}>{action.label}</button>
    {/if}
</div>

<style>
    .quiet-card {
        display: flex; align-items: center; gap: var(--space-3);
        padding: 14px;
        background: var(--card); border: 1px solid var(--hairline);
        border-radius: var(--r-lg);
    }
    .quiet-ico {
        width: 44px; height: 44px; border-radius: var(--r-md);
        display: grid; place-items: center; flex-shrink: 0;
        background: var(--card-3); color: var(--text-mute);
    }
    .quiet-meta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
    .quiet-title { font-size: 14px; font-weight: 600; }
    .quiet-sub { font-size: 12.5px; color: var(--text-mute); }
    .quiet-go { flex-shrink: 0; }
</style>
