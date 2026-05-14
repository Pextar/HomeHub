<script lang="ts">
    import Topbar from "../components/Topbar.svelte";
    import EmptyState from "../components/EmptyState.svelte";
    import ScheduleRow from "../components/ScheduleRow.svelte";
    import { data } from "../lib/stores.svelte";
    import { openModal } from "../lib/modal.svelte";
    import ScheduleModal from "../modals/ScheduleModal.svelte";
    import { fly, scale } from "svelte/transition";
    import { flip } from "svelte/animate";
    import { cubicOut } from "svelte/easing";
    import { dur, stagger } from "../lib/motion";

    const v = $derived(data.value);
</script>

<Topbar title="Schedules" subtitle="{v.schedules.length} configured">
    {#snippet actions()}
        <button class="btn btn-primary" onclick={() => openModal(ScheduleModal, {})}>Add schedule</button>
    {/snippet}
</Topbar>

{#if v.schedules.length === 0}
    <EmptyState icon="clock" title="No schedules yet"
        message="Schedule your sockets, groups or scenes to fire automatically.">
        <button class="btn btn-primary" onclick={() => openModal(ScheduleModal, {})}>Add schedule</button>
    </EmptyState>
{:else}
    <div class="list">
        {#each v.schedules as s, i (s.id)}
            <div
                animate:flip={{ duration: dur(280), easing: cubicOut }}
                in:fly={{ y: 12, duration: dur(240), delay: stagger(i), easing: cubicOut }}
                out:scale={{ start: 0.97, opacity: 0, duration: dur(160) }}>
                <ScheduleRow schedule={s} />
            </div>
        {/each}
    </div>
{/if}

<style>
    .list { display: flex; flex-direction: column; gap: var(--space-2); }
</style>
