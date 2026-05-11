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

    const v = $derived(data.value);

    function chipsFor(g: typeof v.groups[number]) {
        return g.socket_ids.map(id => {
            const s = data.socketById(id);
            return { text: s ? s.name : `(missing: ${id})`, tone: "" as "" };
        });
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
        <button class="btn btn-primary" onclick={() => openModal(GroupModal, {})}>Add group</button>
    {/snippet}
</Topbar>

{#if v.groups.length === 0}
    <EmptyState icon="groups" title="No groups yet"
        message="Group sockets together to control them in one click.">
        <button class="btn btn-primary" onclick={() => openModal(GroupModal, {})}>Add group</button>
    </EmptyState>
{:else}
    <div class="list">
        {#each v.groups as g (g.id)}
            <EntityCard
                name={g.name}
                meta="{g.socket_ids.length} socket{g.socket_ids.length === 1 ? '' : 's'}"
                chips={chipsFor(g)}
            >
                {#snippet actions()}
                    <button class="btn btn-success"
                        onclick={() => runAction(() => api.groupAction(g.id, 'on'), `${g.name} on`)}>On</button>
                    <button class="btn btn-danger"
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
        {/each}
    </div>
{/if}

<style>
    .list { display: flex; flex-direction: column; gap: var(--space-3); }
</style>
