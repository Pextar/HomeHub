<script lang="ts">
    // Renders the topmost modal from the imperative stack.
    import { modalStack } from "../lib/modal.svelte";
    const stack = $derived(modalStack());
    const top = $derived(stack[stack.length - 1]);
    const TopComp = $derived(top?.component);
</script>

{#if top && TopComp}
    {#key top.id}
        <TopComp {...top.props} />
    {/key}
{/if}
