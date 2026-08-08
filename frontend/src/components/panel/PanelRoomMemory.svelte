<script lang="ts">
    import Icon from "../Icon.svelte";
    import { fmtHour, playCount } from "../../lib/music/format";
    import type { PanelMusicStore } from "../../lib/panel-music.svelte";
    import type { MediaPlay } from "../../lib/types";

    /**
     * What this room remembers, and the one control that lets it forget
     * (DESIGN.md §16). Rides on the depth's Rooms pane, under the room's own
     * settings and timers, because a room's memory is a room's preference —
     * the same argument that moved the play modes off the player.
     *
     * The counterweight to a ranked shelf. `topPlays` is what puts the right
     * record in front of you at eight in the morning, and it works by
     * counting: what this room keeps coming back to leads the band, the
     * depth, and the wake-up's suggestion. That is the right answer right up
     * until the thing the room keeps coming back to is a mistake — and a
     * mistake is precisely what gets replayed, because it is the tile in the
     * first slot. Until this row existed the cures were to out-play it thirty
     * times or to delete the speaker.
     *
     * Rows, not the shelf's tiles, and that is the whole reason this is a
     * list rather than an × on a cover. A 132px tile has no room for a second
     * target that clears §2's 44px floor without swallowing taps meant for
     * the art, and the art's tap is the one this whole surface exists for. A
     * full-width row has room for both.
     *
     * Forgetting arms for a second tap, like every destructive tap on a
     * kiosk. One arm at a time: a wall gets poked in passing, and two armed
     * rows would be two things one stray tap could remove.
     *
     * Absent, never dead (§15.1): the list is this room's own plays or it is
     * not shown. The household fallback the shelves lean on is other rooms'
     * memory, and one room is not the place to edit it.
     */
    let { music }: { music: PanelMusicStore } = $props();

    const featured = $derived(music.featured);
    /** This room's own plays, ranked where there is a ranking. The same two
     *  reads the shelves use, so the list is literally what the wall offers
     *  — forgetting a row you cannot see would be a strange thing to ask
     *  someone to trust. */
    const ranked = $derived(music.topPlays);
    const plays = $derived(ranked.length > 0 ? ranked : music.history);
    const own = $derived(!music.historyHousehold && plays.length > 0);

    /** The label carries exactly the claim the list supports — the shelf
     *  rule (§16), repeated here because this is the same evidence. */
    const label = $derived(
        ranked.length > 0
            ? music.topPlaysByHour
                ? `Played here around ${fmtHour(music.topPlaysHour)}`
                : "Played here most"
            : "Played here",
    );

    /** Which row is armed, by URI. Cleared by a second tap, by arming
     *  another, and by the list changing under it — a row that renumbered
     *  while armed would be a target that moved. */
    let armed = $state<string | null>(null);
    $effect(() => {
        void plays;
        armed = null;
    });

    function forgetClick(p: MediaPlay) {
        if (armed !== p.uri) {
            armed = p.uri;
            return;
        }
        armed = null;
        music.forgetPlay(p);
    }
</script>

{#if featured && own && music.canForget}
    <section class="mem" aria-label={label}>
        <h3 class="s-label">{label}</h3>
        <div class="mem-list">
            {#each plays as p (p.uri)}
                <div class="mem-row">
                    <button
                        class="mem-main"
                        aria-label="Play {p.title} again on {featured.title}"
                        disabled={!!music.busy["hist:" + p.uri] || !!music.busy["forget:" + p.uri]}
                        onclick={() => music.playFromHistory(p)}
                    >
                        {#if p.art_uri}
                            <img class="mem-art" src={p.art_uri} alt="" loading="lazy" />
                        {:else}
                            <span class="mem-art placeholder">[ art ]</span>
                        {/if}
                        <span class="mem-meta">
                            <span class="mem-title">{p.title}</span>
                            {#if p.sub}<span class="mem-sub">{p.sub}</span>{/if}
                        </span>
                        <!-- What separates the record this room lives on
                             from the one somebody tried once, in mono and
                             only past one play (§16's shelf rule). -->
                        {#if playCount(p) > 1}
                            <span class="mem-count mono">×{playCount(p)}</span>
                        {/if}
                    </button>
                    <button
                        class="mem-x"
                        class:armed={armed === p.uri}
                        aria-label={armed === p.uri
                            ? `Confirm — stop offering ${p.title} in ${featured.title}`
                            : `Stop offering ${p.title} in ${featured.title}`}
                        disabled={!!music.busy["forget:" + p.uri]}
                        onclick={() => forgetClick(p)}
                    >
                        {#if armed === p.uri}
                            <span class="mem-arm">Forget?</span>
                        {:else}
                            <Icon name="close" size={14} />
                        {/if}
                    </button>
                </div>
            {/each}
        </div>
    </section>
{/if}

<style>
    .mem {
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
        padding-bottom: var(--space-4);
        margin-bottom: var(--space-2);
        border-bottom: 1px solid var(--hairline);
        flex-shrink: 0;
    }
    .s-label {
        margin: 0;
        font-size: 11px;
        font-weight: 500;
        font-family: var(--font-mono);
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-dim);
    }
    .mem-list {
        display: flex;
        flex-direction: column;
        gap: 2px;
    }
    .mem-row {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        min-width: 0;
    }
    /* The row is the play button — the same bargain the shelf's tile makes,
       so a list that is read for editing still starts music on a tap. */
    .mem-main {
        flex: 1 1 auto;
        min-width: 0;
        display: flex;
        align-items: center;
        gap: var(--space-3);
        min-height: 44px;
        padding: 4px var(--space-2);
        border: 0;
        border-radius: var(--r-md);
        background: none;
        color: inherit;
        font-family: inherit;
        text-align: left;
        cursor: pointer;
        transition: background var(--t-fast);
    }
    .mem-main:active {
        background: var(--card-2);
    }
    .mem-main:disabled {
        opacity: 0.55;
    }
    .mem-main:focus-visible {
        box-shadow: var(--focus-ring);
    }
    .mem-art {
        flex: none;
        width: 32px;
        height: 32px;
        object-fit: cover;
        border-radius: var(--r-sm);
        border: 1px solid var(--hairline);
        background: var(--card-2);
    }
    span.mem-art {
        display: grid;
        place-items: center;
        font-size: 9px;
        color: var(--text-dim);
    }
    .mem-meta {
        min-width: 0;
        flex: 1 1 auto;
        display: flex;
        flex-direction: column;
    }
    .mem-title {
        font-size: 13.5px;
        font-weight: 500;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .mem-sub {
        font-size: 11.5px;
        color: var(--text-mute);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .mem-count {
        flex: none;
        font-size: 11.5px;
        color: var(--text-dim);
    }

    /* Quiet until it is armed, then it says what the next tap does in words
       — an × that has already been tapped once looks exactly like one that
       hasn't, and this is a wall being poked from a step away. */
    .mem-x {
        flex: none;
        min-width: 44px;
        height: 44px;
        display: grid;
        place-items: center;
        padding: 0 var(--space-2);
        border: 1px solid transparent;
        border-radius: var(--r-pill);
        background: none;
        color: var(--text-dim);
        font-family: inherit;
        font-size: 12px;
        font-weight: 500;
        cursor: pointer;
        transition:
            color var(--t-fast),
            border-color var(--t-fast),
            transform var(--t-fast);
    }
    .mem-x:active {
        transform: scale(0.95);
        transition-duration: 80ms;
    }
    .mem-x.armed {
        border-color: var(--bad);
        color: var(--bad);
    }
    .mem-x:disabled {
        opacity: 0.55;
    }
    .mem-arm {
        white-space: nowrap;
    }
</style>
