<script lang="ts">
    /**
     * The one row shape for everything the panel's catalog returns (§14).
     *
     * A song or a container plays outright, an artist opens their page, and
     * the two trailing buttons queue without interrupting — for a Sonos
     * destination only, the queue being a Sonos group's. They are buttons
     * rather than an overflow menu because this is a wall: a menu costs a
     * tap to open, a tap to choose and a tap to dismiss, and at arm's
     * length two named 44px targets beat all three.
     *
     * `big` is the search's top result: the same row at full size, saying
     * what it is and where the tap goes. `lead` is the named artist above a
     * list of their songs — the one dense row that keeps its line.
     *
     * The two layout modes arrive as props rather than being inherited from
     * an ancestor's class, because a component's styles are scoped and a
     * parent's `.kb-open .r-art` would no longer reach in here.
     */
    import Icon from "../Icon.svelte";
    import { topLine, rowSub } from "../../lib/music/catalog";
    import { fmtMs } from "../../lib/music/format";
    import type { PanelMusicStore, PanelSource } from "../../lib/panel-music.svelte";
    import type { SpotifyItem } from "../../lib/types";

    let {
        item,
        big = false,
        lead = false,
        kbOpen = false,
        full = false,
        music,
        featured,
        onOpenArtist,
        onPick,
    }: {
        item: SpotifyItem;
        /** The search's top result, at full size. */
        big?: boolean;
        /** The named artist above a list of their songs. */
        lead?: boolean;
        /** The software keyboard is up, so the rows go dense. */
        kbOpen?: boolean;
        /** The results have the whole width — the full-bleed layout. */
        full?: boolean;
        music: PanelMusicStore;
        featured: PanelSource | undefined;
        onOpenArtist: (uri: string, art?: { art_url?: string; round?: boolean }) => void;
        onPick: (item: SpotifyItem) => void;
    } = $props();
</script>

<div class="row" class:big class:lead class:kb-open={kbOpen} class:full>
    <button
        class="r-open"
        disabled={item.kind !== "artist" && music.busy["item:" + item.uri]}
        onclick={() =>
            item.kind === "artist"
                ? onOpenArtist(
                      item.uri,
                      item.art_url ? { art_url: item.art_url, round: true } : undefined,
                  )
                : onPick(item)}
    >
        {#if item.art_url}
            <img
                class="r-art"
                class:round={item.kind === "artist"}
                src={item.art_url}
                alt=""
                loading="lazy"
            />
        {:else}
            <span class="r-art placeholder" class:round={item.kind === "artist"}>[ art ]</span>
        {/if}
        <span class="r-meta">
            <span class="r-name">{item.name}</span>
            {#if big}
                <span class="r-line">{topLine(item)}</span>
                {#if item.kind === "artist"}
                    <span class="r-cta"
                        >See top tracks &amp; albums <Icon
                            name="chevronRight"
                            size={13}
                        /></span
                    >
                {/if}
            {:else if lead}
                <!-- The one row that keeps its line in type mode, and
                     leads it with the kind: dense rows are a name and a
                     chevron, and "Adele" over a list of Adele songs has
                     to say it is the artist or it reads as one more
                     song. -->
                <span class="r-sub">{topLine(item)}</span>
            {:else if rowSub(item)}
                <span class="r-sub">{rowSub(item)}</span>
            {/if}
        </span>
        <span class="r-tail">
            {#if !big && item.duration_ms}
                <span class="r-dur mono">{fmtMs(item.duration_ms)}</span>
            {/if}
            {#if !(big && item.kind === "artist")}
                <!-- A song plays; an artist opens — the tail says which. -->
                <Icon name={item.kind === "artist" ? "chevronRight" : "play"} size={16} />
            {/if}
        </span>
    </button>
    {#if featured?.kind === "sonos" && item.kind !== "artist"}
        <button
            class="r-q"
            aria-label="Play {item.name} next"
            disabled={music.busy["q:" + item.uri]}
            onclick={() => music.enqueue(item, true)}
        >
            <Icon name="skipNext" size={16} />
        </button>
        <button
            class="r-q"
            aria-label="Add {item.name} to the queue"
            disabled={music.busy["q:" + item.uri]}
            onclick={() => music.enqueue(item, false)}
        >
            <Icon name="plus" size={16} />
        </button>
    {/if}
</div>

<style>
    /* The row is a container, not a control: a song plays outright, an
       artist opens their page, and the trailing overflow queues without
       interrupting. */
    .row {
        position: relative;
        display: flex;
        align-items: center;
        gap: var(--space-1);
        min-width: 0;
        border-radius: var(--r-md);
        transition: background var(--t-fast);
    }
    @media (hover: hover) {
        .row:hover {
            background: var(--card-2);
        }
    }
    .r-open {
        flex: 1;
        min-width: 0;
        display: flex;
        align-items: center;
        gap: var(--space-3);
        min-height: 64px;
        padding: var(--space-2);
        border: 1px solid transparent;
        border-radius: var(--r-md);
        background: none;
        color: inherit;
        font: inherit;
        text-align: left;
        cursor: pointer;
        transition:
            background var(--t-fast),
            transform var(--t-fast);
    }
    .r-open:active {
        transform: scale(0.98);
        background: var(--card-2);
        transition-duration: 80ms;
    }
    .r-open:disabled {
        opacity: 0.55;
    }
    .r-art {
        width: 48px;
        height: 48px;
        border-radius: var(--r-sm);
        object-fit: cover;
        flex-shrink: 0;
        display: block;
    }
    /* An artist's art is a portrait — round, the way the app reads them,
       and a quiet tell for the one kind that opens rather than plays. */
    .r-art.round {
        border-radius: 50%;
    }
    span.r-art {
        font-size: 10px;
    }
    .r-meta {
        flex: 1;
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 2px;
    }
    .r-name {
        font-size: 16px;
        font-weight: 500;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .r-sub {
        font-size: 13px;
        color: var(--text-mute);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .r-tail {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        flex-shrink: 0;
        color: var(--text-dim);
        padding-right: var(--space-2);
    }
    .r-dur {
        font-size: 13px;
        color: var(--text-mute);
    }

    /* The top result: the same row, card-sized — the biggest tappable
       thing in the results, because it is the answer most searches were
       after (§15.9). */
    .row.big {
        background: var(--card-2);
        border: 1px solid var(--hairline);
        border-radius: var(--r-lg);
        padding: var(--space-2);
    }
    @media (hover: hover) {
        .row.big:hover {
            background: var(--card-2);
            border-color: var(--border-strong);
        }
    }
    .row.big .r-open {
        gap: var(--space-4);
    }
    .row.big .r-art {
        width: 76px;
        height: 76px;
        border-radius: var(--r-md);
    }
    .row.big .r-art.round {
        border-radius: 50%;
    }
    .row.big .r-name {
        font-size: 19px;
        font-weight: 600;
        letter-spacing: -0.02em;
    }
    .row.big .r-meta {
        gap: 4px;
    }
    .r-line {
        font-family: var(--font-mono);
        font-size: 11px;
        letter-spacing: 0.05em;
        text-transform: uppercase;
        color: var(--text-mute);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    /* Says where the tap goes, so "open" never has to be guessed from a
       chevron alone. */
    .r-cta {
        display: inline-flex;
        align-items: center;
        gap: 4px;
        margin-top: 2px;
        font-size: 13px;
        color: var(--on);
    }

    /* Queue without interrupting: two named targets on the row, no menu to
       open or dismiss. */
    .r-q {
        width: 44px;
        height: 44px;
        display: grid;
        place-items: center;
        flex-shrink: 0;
        border: 1px solid var(--hairline);
        border-radius: 50%;
        background: var(--card-2);
        color: var(--text-mute);
        cursor: pointer;
        transition:
            background var(--t-fast),
            color var(--t-fast),
            transform var(--t-fast);
    }
    .r-q:last-child {
        margin-right: 4px;
    }
    .r-q:active {
        transform: scale(0.92);
        transition-duration: 80ms;
    }
    .r-q:disabled {
        opacity: 0.4;
    }
    @media (hover: hover) {
        .r-q:hover {
            color: var(--text);
            background: var(--card-3);
        }
    }

    /* ── Dense mode ──────────────────────────────────────────────────────
       The software keyboard is up and the rows give back what it took.
       Scoped to the row itself now that it is a component: what used to be
       `.kb-open .r-art` in the depth's stylesheet is the same rule reached
       through this row's own class. */
    .row.kb-open .r-open {
        min-height: 48px;
        padding: var(--space-1) var(--space-2);
    }
    .row.kb-open .r-art {
        width: 36px;
        height: 36px;
    }
    .row.kb-open .r-sub {
        display: none;
    }
    /* Except the named artist's. It is the row that isn't a song, sitting
       above a list of them, and one line is what says so. */
    .row.kb-open.lead .r-sub {
        display: block;
    }

    /* Typing on the wall, with the results across the whole of it: the two
       modes compose, and where they meet the width buys back what the
       keyboard took.

       The artist comes back, on the title's own line. Dense rows drop it
       in a 556px column because there is nowhere to put it; at ~500px a
       column there is, and a list of ten songs with no artists on it is a
       list you cannot choose from — which is worse than one row fewer. */
    .row.full.kb-open .r-meta {
        flex-direction: row;
        align-items: baseline;
        gap: var(--space-2);
        overflow: hidden;
    }
    .row.full.kb-open .r-sub {
        display: block;
        flex: 0 1 auto;
        min-width: 0;
    }
    .row.full.kb-open .r-name {
        flex: 0 1 auto;
    }
</style>
