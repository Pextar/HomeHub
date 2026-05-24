<script lang="ts">
    import Topbar from "../components/Topbar.svelte";
    import EmptyState from "../components/EmptyState.svelte";
    import EntityCard from "../components/EntityCard.svelte";
    import Icon from "../components/Icon.svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores.svelte";
    import { runAction } from "../lib/utils";
    import { openModal } from "../lib/modal.svelte";
    import GroupModal from "../modals/GroupModal.svelte";
    import ConfirmModal from "../components/ConfirmModal.svelte";
    import { fly, scale } from "svelte/transition";
    import { flip } from "svelte/animate";
    import { cubicOut } from "svelte/easing";
    import { dur, stagger } from "../lib/motion";

    const v = $derived(data.value);

    function groupStats(g: typeof v.groups[number]) {
        const sockets = g.socket_ids.map(id => data.socketById(id));
        const onCount = sockets.filter(s => s?.state).length;
        const chips = sockets.map((s) => ({
            text: s ? s.name : "(removed device)",
            tone: s ? (s.state ? "on" as const : "off" as const) : "" as const,
        }));
        return { onCount, chips };
    }

    async function confirmDelete(g: typeof v.groups[number]) {
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Delete group?",
            message: `“${g.name}” and any schedules pointing at it will be removed. The sockets themselves are not affected.`,
            confirmLabel: "Delete",
            danger: true,
        });
        if (!ok) return;
        try {
            await api.deleteGroup(g.id);
            toasts.success("Group deleted", g.name);
            await data.refresh();
        } catch (e) { toasts.error("Failed", (e as Error).message); }
    }
</script>

<Topbar title="Groups" subtitle="{v.groups.length} configured">
    {#snippet actions()}
        <button class="chip" onclick={() => openModal(GroupModal, {})}><Icon name="plus" size={14} /> New group</button>
    {/snippet}
</Topbar>

{#if v.groups.length === 0}
    <EmptyState icon="groups" title="No groups yet"
        message="Group sockets together to control them in one click.">
        <button class="btn btn-primary" onclick={() => openModal(GroupModal, {})}>Add group</button>
    </EmptyState>
{:else}
    <div class="list">
        {#each v.groups as g, i (g.id)}
            {@const stats = groupStats(g)}
            <div
                animate:flip={{ duration: dur(280), easing: cubicOut }}
                in:fly={{ y: 12, duration: dur(240), delay: stagger(i), easing: cubicOut }}
                out:scale={{ start: 0.97, opacity: 0, duration: dur(160) }}>
            <EntityCard
                name={g.name}
                meta="{g.socket_ids.length} socket{g.socket_ids.length === 1 ? '' : 's'}{stats.onCount > 0 ? ` · ${stats.onCount} on` : ''}"
                chips={stats.chips}
            >
                {#snippet actions()}
                    <button class="btn btn-success"
                        disabled={stats.onCount === g.socket_ids.length}
                        onclick={() => runAction(() => api.groupAction(g.id, 'on'), `${g.name} on`)}>On</button>
                    <button class="btn btn-danger"
                        disabled={stats.onCount === 0}
                        onclick={() => runAction(() => api.groupAction(g.id, 'off'), `${g.name} off`)}>Off</button>
                    <button class="btn"
                        onclick={() => runAction(() => api.groupAction(g.id, 'toggle'), `${g.name} toggled`)}>Toggle</button>
                    <button class="icon-btn" aria-label="Edit"
                        onclick={() => openModal(GroupModal, { existing: g })}>
                        <Icon name="edit" size={16} />
                    </button>
                    <button class="icon-btn danger" aria-label="Delete"
                        onclick={() => confirmDelete(g)}>
                        <Icon name="trash" size={16} />
                    </button>
                {/snippet}
            </EntityCard>
            </div>
        {/each}
    </div>
{/if}

<style>
    .list { display: flex; flex-direction: column; gap: var(--space-3); }
</style>
