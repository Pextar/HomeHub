<script lang="ts">
    import HomeSectionHead from "./HomeSectionHead.svelte";
    import TimerRow from "../TimerRow.svelte";
    import { data } from "../../lib/stores.svelte";
    import { fly, scale } from "svelte/transition";
    import { flip } from "svelte/animate";
    import { cubicOut } from "svelte/easing";
    import { dur, stagger } from "../../lib/motion";

    const timers = $derived(data.value.timers);
</script>

{#if timers.length > 0}
    <section class="home-section">
        <HomeSectionHead icon="timer" title="Pending timers">
            {#snippet actions()}
                <span class="header-meta">{timers.length}</span>
            {/snippet}
        </HomeSectionHead>
        <div class="timers">
            {#each timers as timer, i (timer.id)}
                <div
                    animate:flip={{ duration: dur(280), easing: cubicOut }}
                    in:fly={{ y: 10, duration: dur(220), delay: stagger(i), easing: cubicOut }}
                    out:scale={{ start: 0.97, opacity: 0, duration: dur(160) }}
                >
                    <TimerRow {timer} />
                </div>
            {/each}
        </div>
    </section>
{/if}

<style>
    .timers { display: flex; flex-direction: column; gap: var(--space-2); }
</style>
