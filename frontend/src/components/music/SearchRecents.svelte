<script lang="ts">
    /**
     * The room's recent searches, as a chip cloud under the box.
     *
     * They are reachable while results are up, not only from an empty box:
     * the moment they are most useful is when *this* search didn't pan out.
     * So they ride under the box while it has the caret — the shape every
     * search box uses for its suggestions — and the parent decides when.
     *
     * The chips answer `pointerdown` rather than `click`, and that is the
     * whole reason this is fiddly: the cloud is shown because the box has the
     * caret, and a click arrives *after* the blur that hides it. Pointerdown
     * lands while the chip is still there, and the default is prevented so
     * the box never loses the caret in the first place.
     *
     * A query is filed with the picture of whatever it led to, so the cloud
     * reads as things you played rather than as strings you typed.
     */
    import Icon from "../Icon.svelte";
    import type { SearchHistory } from "../../lib/music/history.svelte";

    let {
        recents,
        /** Named when the house has more than one room to search for. */
        roomLabel = "",
        onRun,
    }: {
        recents: SearchHistory;
        roomLabel?: string;
        onRun: (q: string) => void;
    } = $props();
</script>

<div class="sp-history">
    <div class="sp-history-head">
        <span class="eylabel">
            Recent searches{#if roomLabel} · {roomLabel}{/if}
        </span>
        <button type="button" class="chip sp-hist-clear" onclick={() => recents.clear()}>Clear</button>
    </div>
    <div class="sp-history-list">
        {#each recents.list as h (h.q)}
            <div class="sp-hist-chip">
                <button
                    type="button"
                    class="sp-hist-run"
                    onpointerdown={(e) => {
                        e.preventDefault();
                        onRun(h.q);
                    }}
                >
                    {#if h.art_url}
                        <img class="sp-hist-art" class:round={h.round} src={h.art_url} alt="" />
                    {:else}
                        <Icon name="search" size={12} />
                    {/if}
                    <span>{h.q}</span>
                </button>
                <button
                    type="button"
                    class="icon-btn sp-hist-x"
                    aria-label={`Remove "${h.q}" from recent searches`}
                    onpointerdown={(e) => {
                        e.preventDefault();
                        recents.remove(h.q);
                    }}
                >
                    <Icon name="close" size={10} />
                </button>
            </div>
        {/each}
    </div>
</div>

<style>
    .sp-history { display: flex; flex-direction: column; gap: var(--space-2); }
    .sp-history-head { display: flex; align-items: center; justify-content: space-between; gap: var(--space-2); }
    .sp-hist-clear { padding: 3px 10px; font-size: 11px; }
    .sp-history-list { display: flex; flex-wrap: wrap; gap: var(--space-2); }
    .sp-hist-chip {
        display: inline-flex; align-items: center;
        background: var(--card-2); border: 1px solid var(--hairline);
        border-radius: var(--r-pill);
    }
    .sp-hist-run {
        display: inline-flex; align-items: center; gap: 6px;
        padding: 7px 4px 7px 12px;
        background: transparent; border: 0; border-radius: var(--r-pill) 0 0 var(--r-pill);
        font: inherit; font-size: 12.5px; color: var(--text-mute); cursor: pointer;
    }
    @media (hover: hover) { .sp-hist-run:hover { color: var(--text); } }
    .sp-hist-chip .sp-hist-x { width: 26px; height: 26px; margin-right: 3px; color: var(--text-dim); }
    /* The query's own top result, once the search behind it has answered —
       round for an artist's photo, square for everything else's cover art. */
    .sp-hist-art { width: 20px; height: 20px; border-radius: var(--r-sm); object-fit: cover; flex-shrink: 0; background: var(--card-3); }
    .sp-hist-art.round { border-radius: 50%; }
</style>
