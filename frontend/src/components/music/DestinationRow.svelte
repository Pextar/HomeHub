<script lang="ts">
    /**
     * "Play on" — the one visible destination, shared by favorites and search.
     *
     * Always visible: with a single room it shows that room's name rather than
     * hiding the answer entirely, because "where will this play" is a question
     * the user should never have to guess at.
     *
     * The row spans all three ways to start music, and each group of chips is
     * introduced by a single marker rather than a badge per chip — the only
     * thing a marker has to solve is telling apart a name that exists in more
     * than one of them ("Kitchen" the Sonos room, "Kitchen" the KEF speaker,
     * "Kitchen" the zone that holds both).
     *
     * The markers are derived from where the *kind* changes, not from an index
     * the caller passes in: an index has to be kept in step with the order
     * `destination.list` happens to build, and a third kind is exactly the
     * kind of change that silently mislabels one.
     */
    import type { Destination, Dest } from "../../lib/music/destination.svelte";

    let { destination }: { destination: Destination } = $props();

    const list = $derived(destination.list);

    /** The marker above the first chip of each group. Rooms lead and need no
     *  label — they are what "Play on" means by default. */
    const MARKERS: Record<Dest["kind"], string> = {
        sonos: "",
        kef: "KEF",
        zone: "Zones",
    };

    /** True on the first chip of a group that carries a marker. */
    function marks(i: number): string {
        const kind = list[i].kind;
        if (i > 0 && list[i - 1].kind === kind) return "";
        return MARKERS[kind];
    }
</script>

{#if list.length > 1}
    <div class="fav-targets" role="radiogroup" aria-label="Play on">
        <span class="t-label">Play on</span>
        {#each list as d, i (d.kind + d.id)}
            {@const mark = marks(i)}
            {#if mark}
                <span class="t-label">{mark}</span>
            {/if}
            {@const on = destination.is(d)}
            <button
                class="chip"
                class:on
                role="radio"
                aria-checked={on}
                aria-label={`Play on ${destination.name(d)}${MARKERS[d.kind] ? ` (${MARKERS[d.kind]})` : ""}`}
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
