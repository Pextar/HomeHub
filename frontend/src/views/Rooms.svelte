<script lang="ts">
    import Topbar from "../components/Topbar.svelte";
    import EmptyState from "../components/EmptyState.svelte";
    import RoomCard from "../components/RoomCard.svelte";
    import Icon from "../components/Icon.svelte";
    import { data } from "../lib/stores.svelte";
    import { openModal } from "../lib/modal.svelte";
    import RoomModal from "../modals/RoomModal.svelte";
    import { fly, scale } from "svelte/transition";
    import { flip } from "svelte/animate";
    import { cubicOut } from "svelte/easing";
    import { dur, stagger } from "../lib/motion";

    const v = $derived(data.value);

    // Live room on-counts derived from socket state so the cards stay in sync
    // with optimistic toggles rather than waiting for the next server refresh.
    const liveRooms = $derived.by(() => {
        // eslint-disable-next-line svelte/prefer-svelte-reactivity -- transient local Map, built and consumed synchronously
        const onByRoom = new Map<string, number>();
        for (const s of v.sockets) {
            const r = s.room || "Unassigned";
            onByRoom.set(r, (onByRoom.get(r) ?? 0) + (s.state ? 1 : 0));
        }
        return v.rooms
            .map(r => ({ ...r, on: onByRoom.get(r.name) ?? 0 }))
            .sort((a, b) => a.name.localeCompare(b.name));
    });

    const totalOn = $derived(liveRooms.reduce((n, r) => n + r.on, 0));

    function addRoom() { openModal(RoomModal, {}); }
</script>

<Topbar
    title="Rooms"
    subtitle={v.rooms.length === 0
        ? "Organise your devices by where they live"
        : `${v.rooms.length} room${v.rooms.length === 1 ? "" : "s"} · ${totalOn} on`}>
    {#snippet actions()}
        <button class="btn btn-primary" onclick={addRoom}>
            <Icon name="plus" size={16} /> New room
        </button>
    {/snippet}
</Topbar>

{#if v.rooms.length === 0}
    <EmptyState icon="couch" title="No rooms yet"
        message="Rooms group your devices by where they are — a living room, a bedroom, the kitchen. Create one to get started.">
        <button class="btn btn-primary" onclick={addRoom}>
            <Icon name="plus" size={16} /> New room
        </button>
    </EmptyState>
{:else}
    <div class="grid">
        {#each liveRooms as room, i (room.id)}
            <div class="cell"
                animate:flip={{ duration: dur(280), easing: cubicOut }}
                in:fly={{ y: 12, duration: dur(240), delay: stagger(i), easing: cubicOut }}
                out:scale={{ start: 0.97, opacity: 0, duration: dur(160) }}>
                <RoomCard {room} manage />
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
        .grid { grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: var(--space-3); }
    }
    .cell { display: flex; min-width: 0; }
    .cell > :global(.room) { flex: 1; min-width: 0; }
</style>
