<script lang="ts">
    import type { Snippet, ComponentProps } from "svelte";
    import Icon from "./Icon.svelte";

    interface Props {
        icon?: ComponentProps<typeof Icon>["name"];
        title: string;
        message?: string;
        compact?: boolean;
        children?: Snippet;
    }
    let { icon, title, message, compact = false, children }: Props = $props();
</script>

<div class="empty-state" class:empty-state-compact={compact}>
    {#if icon}
        <Icon name={icon} size={48} />
    {/if}
    <h3>{title}</h3>
    {#if message}<p>{message}</p>{/if}
    {#if children}{@render children()}{/if}
</div>
