<script lang="ts">
    import type { Snippet } from "svelte";
    import Icon from "./Icon.svelte";

    interface Props {
        icon?: keyof typeof iconNames;
        title: string;
        message?: string;
        compact?: boolean;
        children?: Snippet;
    }
    const iconNames = {
        socket: "socket", clock: "clock", groups: "groups",
        scenes: "scenes", search: "search", sensor: "sensor", user: "user",
    } as const;
    let { icon, title, message, compact = false, children }: Props = $props();
</script>

<div class="empty-state" class:empty-state-compact={compact}>
    {#if icon}
        <Icon name={icon as any} size={48} />
    {/if}
    <h3>{title}</h3>
    {#if message}<p>{message}</p>{/if}
    {#if children}{@render children()}{/if}
</div>
