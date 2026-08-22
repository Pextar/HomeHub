<script lang="ts">
    import HomeSectionHead from "./HomeSectionHead.svelte";
    import SensorCard from "../SensorCard.svelte";
    import Icon from "../Icon.svelte";
    import HomeSensorsModal from "../../modals/HomeSensorsModal.svelte";
    import { data, route } from "../../lib/stores.svelte";
    import { homeLayout } from "../../lib/home-layout.svelte";
    import { homeSensors } from "../../lib/home-layout";
    import { openModal } from "../../lib/modal.svelte";
    import { scale } from "svelte/transition";
    import { flip } from "svelte/animate";
    import { cubicOut } from "svelte/easing";
    import { dur, stagger } from "../../lib/motion";

    const v = $derived(data.value);
    const shown = $derived(homeSensors(v.sensors, homeLayout.layout));
    /** True only when the user has deliberately picked nothing. */
    const emptied = $derived(shown.length === 0 && homeLayout.sensors !== null);
</script>

{#if v.sensors.length > 0}
    <section class="home-section">
        <HomeSectionHead icon="sensor" title="Sensors">
            {#snippet actions()}
                <div class="head-chips">
                    <button class="chip" onclick={() => openModal(HomeSensorsModal, {})}>
                        <Icon name="sliders" size={13} /> Choose
                    </button>
                    <button class="chip" onclick={() => route.go("sensors")}>All</button>
                </div>
            {/snippet}
        </HomeSectionHead>

        {#if emptied}
            <!-- Picked none, rather than having none: say so, since a section
                 that quietly disappeared would read as a bug. -->
            <p class="field-help">
                No sensors on the home screen yet — choose the readings you want here.
            </p>
        {:else}
            <div class="home-grid">
                {#each shown as sensor, i (sensor.id)}
                    <div
                        class="sensor-item"
                        animate:flip={{ duration: dur(280), easing: cubicOut }}
                        in:scale={{ start: 0.95, opacity: 0, duration: dur(220), delay: stagger(i), easing: cubicOut }}
                    >
                        <SensorCard {sensor} compact />
                    </div>
                {/each}
            </div>
        {/if}
    </section>
{/if}

<style>
    .head-chips { display: flex; gap: var(--space-2); align-items: center; }
    .sensor-item { display: flex; min-width: 0; }
    .sensor-item > :global(.sensor) { flex: 1; min-width: 0; }
</style>
