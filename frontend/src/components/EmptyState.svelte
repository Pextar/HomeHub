<script lang="ts">
    import type { Snippet, ComponentProps } from "svelte";
    import Icon from "./Icon.svelte";
    import { scale } from "svelte/transition";
    import { cubicOut } from "svelte/easing";
    import { dur } from "../lib/motion";

    interface Props {
        icon?: ComponentProps<typeof Icon>["name"];
        title: string;
        message?: string;
        compact?: boolean;
        /** Fill the leftover viewport height so a lone empty state doesn't
            strand itself at the top of a blank screen. */
        fill?: boolean;
        children?: Snippet;
    }
    let { icon, title, message, compact = false, fill = false, children }: Props = $props();
</script>

<!-- Settles in like a list tile would — an empty view shouldn't pop. -->
<div class="empty-state" class:empty-state-compact={compact} class:empty-fill={fill}
    in:scale={{ start: 0.96, opacity: 0, duration: dur(220), easing: cubicOut }}>
    {#if icon}
        <Icon name={icon} size={48} />
    {/if}
    <h3>{title}</h3>
    {#if message}<p>{message}</p>{/if}
    {#if children}{@render children()}{/if}
</div>
