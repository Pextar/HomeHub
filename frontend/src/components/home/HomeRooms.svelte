<script lang="ts">
    import HomeSectionHead from "./HomeSectionHead.svelte";
    import RoomCard from "../RoomCard.svelte";
    import { data } from "../../lib/stores.svelte";
    import { scale } from "svelte/transition";
    import { flip } from "svelte/animate";
    import { cubicOut } from "svelte/easing";
    import { dur, stagger } from "../../lib/motion";

    const v = $derived(data.value);

    // Live room on-counts derived from socket state so RoomCards stay in sync
    // with optimistic toggles rather than waiting for the next server refresh.
    const rooms = $derived.by(() => {
        // eslint-disable-next-line svelte/prefer-svelte-reactivity -- transient local Map, built and consumed synchronously
        const onByRoom = new Map<string, number>();
        for (const s of v.sockets) {
            const r = s.room || "Unassigned";
            onByRoom.set(r, (onByRoom.get(r) ?? 0) + (s.state ? 1 : 0));
        }
        return v.rooms.map((r) => ({ ...r, on: onByRoom.get(r.name) ?? 0 }));
    });
</script>

<!-- Phones only: on desktop the room cards are replaced by the filterable
     device grid (DESIGN.md §5), which is a different section entirely. -->
<section class="home-section mobile-rooms">
    <HomeSectionHead icon="couch" title="Rooms">
        {#snippet actions()}
            {#if rooms.length > 0}<span class="header-meta">{rooms.length}</span>{/if}
        {/snippet}
    </HomeSectionHead>
    {#if !v.loaded}
        <div class="home-grid" aria-hidden="true">
            {#each Array.from({ length: 4 }) as _, i (i)}
                <div class="skeleton skel-room"></div>
            {/each}
        </div>
    {:else if rooms.length === 0}
        <p class="field-help">No rooms yet. Create devices and assign rooms to them.</p>
    {:else}
        <div class="home-grid rooms">
            {#each rooms as room, i (room.name)}
                <div
                    class="room-item"
                    animate:flip={{ duration: dur(280), easing: cubicOut }}
                    in:scale={{ start: 0.95, opacity: 0, duration: dur(220), delay: stagger(i), easing: cubicOut }}
                >
                    <RoomCard {room} />
                </div>
            {/each}
        </div>
    {/if}
</section>

<style>
    .skel-room { height: 72px; border-radius: var(--r-lg); }
    /* Rooms sit closer together than the card grids do — they are the densest
       thing on the phone's home screen. */
    .rooms { gap: var(--space-2); }
    @media (min-width: 560px) {
        .rooms { gap: var(--space-3); }
    }
    .room-item { display: flex; min-width: 0; }
    .room-item > :global(.room) { flex: 1; min-width: 0; }

    @media (min-width: 1024px) {
        .mobile-rooms { display: none; }
    }
</style>
