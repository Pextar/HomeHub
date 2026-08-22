<script lang="ts">
    import HomeSectionHead from "./HomeSectionHead.svelte";
    import SocketCard from "../SocketCard.svelte";
    import { data } from "../../lib/stores.svelte";

    const v = $derived(data.value);

    // Desktop device grid — filterable by room on the dashboard.
    let deviceRoom = $state("");
    const allDeviceRooms = $derived(
        [...new Set(v.sockets.map((s) => s.room || "Unassigned"))].sort(),
    );
    const filtered = $derived.by(() => {
        if (deviceRoom === "on") return v.sockets.filter((s) => s.state);
        if (deviceRoom) return v.sockets.filter((s) => (s.room || "Unassigned") === deviceRoom);
        return v.sockets;
    });
</script>

<!-- Desktop only: it is the wide-screen replacement for the room cards, and a
     four-column grid of every device is not a phone's home screen. -->
{#if v.sockets.length > 0}
    <section class="home-section desktop-devices">
        <HomeSectionHead icon="devices" title="Devices">
            {#snippet actions()}
                <div class="device-chips">
                    <button class="chip" class:active={deviceRoom === ""} onclick={() => (deviceRoom = "")}>
                        All
                    </button>
                    <button class="chip" class:active={deviceRoom === "on"} onclick={() => (deviceRoom = "on")}>
                        On
                    </button>
                    {#each allDeviceRooms as r (r)}
                        <button class="chip" class:active={deviceRoom === r} onclick={() => (deviceRoom = r)}>
                            {r}
                        </button>
                    {/each}
                </div>
            {/snippet}
        </HomeSectionHead>
        {#if filtered.length === 0}
            <p class="field-help">No devices match this filter.</p>
        {:else}
            <div class="device-grid">
                {#each filtered as socket (socket.id)}
                    <SocketCard {socket} />
                {/each}
            </div>
        {/if}
    </section>
{/if}

<style>
    .desktop-devices { display: none; }
    @media (min-width: 1024px) {
        .desktop-devices { display: flex; }
    }

    .device-chips { display: flex; gap: 4px; flex-wrap: wrap; align-items: center; }

    .device-grid {
        display: grid;
        grid-template-columns: repeat(3, 1fr);
        gap: 12px;
    }
    @media (min-width: 1280px) {
        .device-grid { grid-template-columns: repeat(4, 1fr); }
    }
</style>
