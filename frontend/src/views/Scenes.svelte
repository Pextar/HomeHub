<script lang="ts">
    import Topbar from "../components/Topbar.svelte";
    import EmptyState from "../components/EmptyState.svelte";
    import SceneTile from "../components/SceneTile.svelte";
    import Icon from "../components/Icon.svelte";
    import { data } from "../lib/stores.svelte";
    import { openModal } from "../lib/modal.svelte";
    import SceneModal from "../modals/SceneModal.svelte";
    import { fly, scale } from "svelte/transition";
    import { flip } from "svelte/animate";
    import { cubicOut } from "svelte/easing";
    import { dur, stagger } from "../lib/motion";

    const v = $derived(data.value);
</script>

<Topbar title="Scenes" subtitle="{v.scenes.length} configured">
    {#snippet actions()}
        <button class="chip" onclick={() => openModal(SceneModal, {})}><Icon name="plus" size={14} /> New scene</button>
    {/snippet}
</Topbar>

{#if v.scenes.length === 0}
    <EmptyState icon="scenes" title="No scenes yet"
        message="Save the perfect lighting combo as a scene and recall it anytime.">
        <button class="btn btn-primary" onclick={() => openModal(SceneModal, {})}>Add scene</button>
    </EmptyState>
{:else}
    <div class="grid">
        {#each v.scenes as sc, i (sc.id)}
            <div class="cell"
                animate:flip={{ duration: dur(280), easing: cubicOut }}
                in:fly={{ y: 12, duration: dur(240), delay: stagger(i), easing: cubicOut }}
                out:scale={{ start: 0.97, opacity: 0, duration: dur(160) }}>
                <SceneTile scene={sc} manage />
            </div>
        {/each}
    </div>
{/if}

<style>
    .grid {
        display: grid;
        grid-template-columns: repeat(2, minmax(0, 1fr));
        gap: 10px;
    }
    @media (min-width: 600px) {
        .grid { grid-template-columns: repeat(auto-fill, minmax(220px, 1fr)); gap: var(--space-3); }
    }
    .cell { display: flex; min-width: 0; }
    .cell > :global(.scene) { flex: 1; min-width: 0; }
</style>
