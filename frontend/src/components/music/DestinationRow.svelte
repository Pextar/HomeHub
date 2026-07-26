<script lang="ts">
    /**
     * "Play on" — the one visible destination, shared by favorites and search.
     *
     * Always visible: with a single room it shows that room's name rather than
     * hiding the answer entirely, because "where will this play" is a question
     * the user should never have to guess at.
     *
     * The row spans both bridges, and the KEF speakers follow the Sonos zones
     * behind a single `KEF` marker — one marker, not a badge per chip, because
     * the only thing it has to solve is telling apart a name that exists on
     * both sides.
     */
    import type { Destination, Dest } from "../../lib/music/destination.svelte";

    let {
        destination,
        /** Where the KEF speakers begin, so the marker goes in once. */
        kefStart,
    }: {
        destination: Destination;
        kefStart: number;
    } = $props();

    const list = $derived(destination.list);
</script>

{#if list.length > 1}
    <div class="fav-targets" role="radiogroup" aria-label="Play on">
        <span class="t-label">Play on</span>
        {#each list as d, i (d.kind + d.id)}
            {#if i === kefStart && kefStart > 0}
                <span class="t-label">KEF</span>
            {/if}
            {@const on = destination.is(d)}
            <button
                class="chip"
                class:on
                role="radio"
                aria-checked={on}
                aria-label={`Play on ${destination.name(d)}${d.kind === "kef" ? " (KEF)" : ""}`}
                onclick={() => (destination.current = d as Dest)}
            >
                {destination.name(d)}
            </button>
        {/each}
    </div>
{:else if list.length === 1}
    <div class="fav-targets">
        <span class="t-label">Play on</span>
        <span class="t-one">{destination.name(list[0])}</span>
    </div>
{/if}

<style>
    .fav-targets { display: flex; align-items: center; gap: var(--space-2); flex-wrap: wrap; }
    .t-label {
        font-family: var(--font-mono);
        font-size: 10px; letter-spacing: 0.1em; text-transform: uppercase;
        color: var(--text-dim);
    }
    .t-one { font-size: 12.5px; color: var(--text-mute); }
</style>
