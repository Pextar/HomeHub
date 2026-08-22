<script lang="ts">
    import type { Snippet, ComponentProps } from "svelte";
    import Icon from "../Icon.svelte";

    /**
     * The head every home section wears: amber-soft icon badge, 17px title,
     * and whatever the section puts on its trailing edge (a count pill, an
     * "All" chip, a row of filter chips).
     *
     * It is a component rather than a class two files both style because the
     * sections became separate components when the screen became reorderable,
     * and scoped CSS does not reach across that boundary.
     */
    interface Props {
        icon: ComponentProps<typeof Icon>["name"];
        title: string;
        actions?: Snippet;
    }
    let { icon, title, actions }: Props = $props();
</script>

<div class="section-head">
    <h2><span class="section-ico"><Icon name={icon} size={15} /></span>{title}</h2>
    {#if actions}{@render actions()}{/if}
</div>

<style>
    .section-head {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
    }
    .section-head h2 {
        display: inline-flex;
        align-items: center;
        gap: 8px;
        font-size: 17px;
        font-weight: 600;
    }
    .section-ico {
        width: 24px; height: 24px;
        border-radius: var(--r-sm);
        display: grid; place-items: center;
        background: var(--on-soft);
        color: var(--on);
        flex-shrink: 0;
    }
</style>
