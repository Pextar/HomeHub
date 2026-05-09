<script lang="ts">
    import Topbar from "../components/Topbar.svelte";
    import EmptyState from "../components/EmptyState.svelte";
    import ScheduleRow from "../components/ScheduleRow.svelte";
    import { data } from "../lib/stores";
    import { openModal } from "../lib/modal.svelte";
    import ScheduleModal from "../modals/ScheduleModal.svelte";

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
        {#each v.schedules as s (s.id)}
            <ScheduleRow schedule={s} />
        {/each}
    </div>
{/if}

<style>
    .list { display: flex; flex-direction: column; gap: var(--space-2); }
</style>
