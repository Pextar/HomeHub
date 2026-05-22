<script lang="ts">
    import Topbar from "../components/Topbar.svelte";
    import EmptyState from "../components/EmptyState.svelte";
    import ScheduleRow from "../components/ScheduleRow.svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores.svelte";
    import { openModal } from "../lib/modal.svelte";
    import ScheduleModal from "../modals/ScheduleModal.svelte";
    import { fly, scale } from "svelte/transition";
    import { flip } from "svelte/animate";
    import { cubicOut } from "svelte/easing";
    import { dur, stagger } from "../lib/motion";

    const v = $derived(data.value);
    const anyEnabled = $derived(v.schedules.some(s => s.enabled));
    let pausing = $state(false);

    // "Vacation mode": flip every schedule off (or back on) in one call.
    async function toggleAll() {
        if (pausing) return;
        pausing = true;
        const enable = !anyEnabled;
        try {
            const r = await api.setAllSchedules(enable);
            toasts.success(enable ? "Schedules resumed" : "Schedules paused",
                `${r.changed} schedule${r.changed === 1 ? "" : "s"} ${enable ? "enabled" : "disabled"}.`);
            await data.refresh();
        } catch (e) {
            toasts.error("Failed", (e as Error).message);
        } finally {
            pausing = false;
        }
    }
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
    <!-- Controls + list share a wrapper so the gap between them stays tight
         while the view's normal gap separates this block from the topbar. -->
    <div class="schedule-section">
        <div class="section-controls">
            <button class="btn btn-ghost" onclick={toggleAll} disabled={pausing}>
                {anyEnabled ? "Pause all" : "Resume all"}
            </button>
        </div>
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
    </div>
{/if}

<style>
    .schedule-section { display: flex; flex-direction: column; gap: var(--space-2); }
    .section-controls { display: flex; justify-content: flex-end; }
    .list { display: flex; flex-direction: column; gap: var(--space-2); }
</style>
