<script lang="ts">
    import Topbar from "../components/Topbar.svelte";
    import EmptyState from "../components/EmptyState.svelte";
    import EntityCard from "../components/EntityCard.svelte";
    import Icon from "../components/Icon.svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores.svelte";
    import { runAction } from "../lib/utils";
    import { openModal } from "../lib/modal.svelte";
    import SceneModal from "../modals/SceneModal.svelte";
    import ConfirmModal from "../components/ConfirmModal.svelte";

    const v = $derived(data.value);

    function chipsFor(sc: typeof v.scenes[number]) {
        return sc.actions.map(a => {
            const s = data.socketById(a.socket_id);
            return {
                text: `${s ? s.name : "(missing)"} → ${a.action.toUpperCase()}`,
                tone: a.action as "on" | "off",
            };
        });
    }

    async function confirmDelete(sc: typeof v.scenes[number]) {
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Delete scene?",
            message: `“${sc.name}” and any schedules pointing at it will be removed.`,
            confirmLabel: "Delete",
            danger: true,
        });
        if (!ok) return;
        try {
            await api.deleteScene(sc.id);
            toasts.success("Scene deleted", sc.name);
            await data.refresh();
        } catch (e) { toasts.error("Failed", (e as Error).message); }
    }
</script>

<Topbar title="Scenes" subtitle="{v.scenes.length} configured">
    {#snippet actions()}
        <button class="btn btn-primary" onclick={() => openModal(SceneModal, {})}>Add scene</button>
    {/snippet}
</Topbar>

{#if v.scenes.length === 0}
    <EmptyState icon="scenes" title="No scenes yet"
        message="Save the perfect lighting combo as a scene and recall it anytime.">
        <button class="btn btn-primary" onclick={() => openModal(SceneModal, {})}>Add scene</button>
    </EmptyState>
{:else}
    <div class="list">
        {#each v.scenes as sc (sc.id)}
            <EntityCard
                name={sc.name}
                meta="{sc.actions.length} action{sc.actions.length === 1 ? '' : 's'}"
                chips={chipsFor(sc)}
            >
                {#snippet actions()}
                    <button class="btn btn-primary"
                        onclick={() => runAction(() => api.activateScene(sc.id), `Scene activated: ${sc.name}`)}>
                        Activate
                    </button>
                    <button class="icon-btn" aria-label="Edit"
                        onclick={() => openModal(SceneModal, { existing: sc })}>
                        <Icon name="edit" size={16} />
                    </button>
                    <button class="icon-btn danger" aria-label="Delete"
                        onclick={() => confirmDelete(sc)}>
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
