<script lang="ts">
    import HomeSectionHead from "./HomeSectionHead.svelte";
    import SocketCard from "../SocketCard.svelte";
    import { data } from "../../lib/stores.svelte";
    import { scale } from "svelte/transition";
    import { flip } from "svelte/animate";
    import { cubicOut } from "svelte/easing";
    import { dur, stagger } from "../../lib/motion";

    const favorites = $derived(data.value.sockets.filter((s) => s.favorite));
</script>

{#if favorites.length > 0}
    <section class="home-section">
        <HomeSectionHead icon="star" title="Favorites">
            {#snippet actions()}
                <span class="header-meta">{favorites.length}</span>
            {/snippet}
        </HomeSectionHead>
        <div class="home-grid">
            {#each favorites as socket, i (socket.id)}
                <div
                    class="favorite-item"
                    animate:flip={{ duration: dur(280), easing: cubicOut }}
                    in:scale={{ start: 0.95, opacity: 0, duration: dur(220), delay: stagger(i), easing: cubicOut }}
                >
                    <SocketCard {socket} />
                </div>
            {/each}
        </div>
    </section>
{/if}

<style>
    .favorite-item { display: flex; min-width: 0; }
    .favorite-item > :global(.tile) { flex: 1; min-width: 0; }
</style>
