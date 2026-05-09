<script lang="ts">
    import Topbar from "../components/Topbar.svelte";
    import Icon from "../components/Icon.svelte";
    import SocketCard from "../components/SocketCard.svelte";
    import EmptyState from "../components/EmptyState.svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores";
    import { groupSocketsByRoom, runAction } from "../lib/utils";
    import { openModal } from "../lib/modal.svelte";
    import SocketModal from "../modals/SocketModal.svelte";

    const v = $derived(data.value);

    let search = $state("");
    let roomFilter = $state("");

    const allRooms = $derived(
        [...new Set(v.sockets.map(s => s.room || "Unassigned"))].sort()
    );

    const filtered = $derived.by(() => {
        let list = v.sockets;
        if (search) {
            const q = search.toLowerCase();
            list = list.filter(s =>
                s.name.toLowerCase().includes(q) ||
                (s.room || "").toLowerCase().includes(q) ||
                s.code.toLowerCase().includes(q)
            );
        }
        if (roomFilter) {
            list = list.filter(s => (s.room || "Unassigned") === roomFilter);
        }
        return list;
    });

    const groups = $derived(groupSocketsByRoom(filtered));
    const sortedRooms = $derived([...groups.keys()].sort((a, b) => a.localeCompare(b)));
</script>

<Topbar title="Sockets" subtitle="{v.sockets.length} configured">
    {#snippet actions()}
        <button class="btn btn-primary" onclick={() => openModal(SocketModal, {})}>Add socket</button>
    {/snippet}
</Topbar>

{#if v.sockets.length === 0}
    <EmptyState icon="socket" title="No sockets yet" message="Add your first 433MHz socket to get started.">
        <button class="btn btn-primary" onclick={() => openModal(SocketModal, {})}>Add socket</button>
    </EmptyState>
{:else}
    <div class="toolbar">
        <div class="search">
            <Icon name="search" size={16} />
            <input type="search" placeholder="Search sockets…" bind:value={search} aria-label="Search sockets" />
        </div>
        <select bind:value={roomFilter} aria-label="Filter by room" style="max-width:240px">
            <option value="">All rooms</option>
            {#each allRooms as r}
                <option value={r}>{r}</option>
            {/each}
        </select>
    </div>

    {#if filtered.length === 0}
        <EmptyState compact title="No matches" message="Try a different search or clear the filters." />
    {:else}
        {#each sortedRooms as room (room)}
            {@const items = groups.get(room) ?? []}
            {@const onCount = items.filter(s => s.state).length}
            <section class="room-section">
                <div class="room-header">
                    <h3>{room} · {items.length} sockets · {onCount} on</h3>
                    <div class="room-actions">
                        <button class="btn btn-ghost"
                            onclick={() => runAction(() => api.roomOn(room), `${room} on`)}>All on</button>
                        <button class="btn btn-ghost"
                            onclick={() => runAction(() => api.roomOff(room), `${room} off`)}>All off</button>
                    </div>
                </div>
                <div class="grid">
                    {#each items as s (s.id)}
                        <SocketCard socket={s} />
                    {/each}
                </div>
            </section>
        {/each}
    {/if}
{/if}

<style>
    .toolbar {
        display: flex;
        gap: var(--space-3);
        flex-wrap: wrap;
        padding: var(--space-3);
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        align-items: center;
    }
    .search {
        flex: 1;
        min-width: 220px;
        display: flex;
        align-items: center;
        gap: var(--space-2);
        padding: 0 var(--space-3);
        background: var(--surface);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        color: var(--text-muted);
    }
    .search input {
        flex: 1;
        border: 0;
        background: transparent;
        padding: 9px 0;
        color: var(--text);
    }
    .search input::placeholder { color: var(--text-faint); }

    .room-section { display: flex; flex-direction: column; gap: var(--space-3); }
    .room-section + .room-section { margin-top: var(--space-6); }
    .room-header {
        display: flex; align-items: center; justify-content: space-between;
        gap: var(--space-3);
    }
    .room-header h3 {
        color: var(--text-muted);
        font-size: 12px;
        text-transform: uppercase;
        letter-spacing: 0.06em;
    }
    .room-actions { display: flex; gap: var(--space-2); }
    .grid {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
        gap: var(--space-4);
    }
</style>
