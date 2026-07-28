<script lang="ts">
    /**
     * The catalog's track list — one row per song, as informative as Spotify's
     * own: index number where order means something, art, title with its
     * explicit mark, artists with the record it sits on, and its length.
     *
     * Tapping a row plays it now; queueing without interrupting lives behind
     * the row's overflow, and only when the destination has a queue — the
     * queue is a Sonos group's, so a control that would be refused is not
     * rendered at all.
     *
     * Shared by the search results, the artist page's "Popular" and the
     * album/playlist pages rather than grown three times — the row shape and
     * the menu are identical everywhere.
     */
    import { scale } from "svelte/transition";
    import { cubicOut } from "svelte/easing";
    import Icon from "../Icon.svelte";
    import { dur } from "../../lib/motion";
    import { fmtMs } from "../../lib/music/format";
    import type { Busy } from "../../lib/music/busy.svelte";
    import type { SpotifyItem } from "../../lib/types";

    let {
        items,
        /** Numbered rows, where the order is the point (an album's sides,
         *  an artist's most-played). */
        numbered = false,
        /** Row art. An album's own tracks all carry the record's cover, so
         *  the album page leaves it off; a playlist's differ per row. */
        showArt = true,
        /** Append the album name to the sub line. Off on the album page,
         *  where every row would repeat the page's own title. */
        showAlbum = true,
        /** A sub line equal to this is dropped — on an album page the record's
         *  own artist is already in the header, so repeating it down twelve
         *  rows is noise. A featured artist still differs, so it survives. */
        omitSub = "",
        busy,
        /** False when no room is picked — every row's tap needs one. */
        canPlay = true,
        /** The Sonos coordinator the queue belongs to; null hides the
         *  overflow entirely (KEF rooms and zones have no queue). */
        queueTarget = null,
        onPick,
        onEnqueue,
    }: {
        items: SpotifyItem[];
        numbered?: boolean;
        showArt?: boolean;
        showAlbum?: boolean;
        omitSub?: string;
        busy: Busy;
        canPlay?: boolean;
        queueTarget?: string | null;
        onPick: (item: SpotifyItem) => void;
        onEnqueue?: (item: SpotifyItem, next: boolean) => void;
    } = $props();

    /** The line under a title: who plays it, and what it's on — minus
     *  whatever the page around it has already said. */
    function subLine(item: SpotifyItem): string {
        const artists = item.sub && item.sub !== omitSub ? item.sub : "";
        const album = showAlbum && item.album ? item.album : "";
        return [artists, album].filter(Boolean).join(" · ");
    }

    // ── Row overflow menus ───────────────────────────────────────────────
    // Keyed by item URI: at most one menu is open at a time.
    let menuFor = $state<string | null>(null);
    $effect(() => {
        if (!menuFor) return;
        const close = () => (menuFor = null);
        // The opening click calls stopPropagation, so it never reaches here.
        document.addEventListener("click", close);
        return () => document.removeEventListener("click", close);
    });
    function toggleMenu(e: MouseEvent, uri: string) {
        e.stopPropagation();
        menuFor = menuFor === uri ? null : uri;
    }
    /** An open menu takes focus and answers the arrow keys, so queueing a
     *  result never means tabbing back through the whole list. */
    function menuNav(node: HTMLElement) {
        const items = () =>
            Array.from(node.querySelectorAll<HTMLButtonElement>("[role='menuitem']"));
        items()[0]?.focus();
        function onKey(e: KeyboardEvent) {
            if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
            e.preventDefault();
            const list = items();
            const i = list.indexOf(document.activeElement as HTMLButtonElement);
            const next = e.key === "ArrowDown" ? i + 1 : i - 1;
            list[(next + list.length) % list.length]?.focus();
        }
        node.addEventListener("keydown", onKey);
        return { destroy: () => node.removeEventListener("keydown", onKey) };
    }

    /**
     * Escape closes an open row menu before it closes the screen, so the
     * shell asks here first. Answers whether it consumed the key.
     */
    export function closeMenu(): boolean {
        if (!menuFor) return false;
        menuFor = null;
        return true;
    }
</script>

<div class="tl">
    {#each items as item, i (item.uri)}
        <div class="tl-row">
            <button
                class="tl-open"
                disabled={busy.is("item:" + item.uri) || !canPlay}
                onclick={() => onPick(item)}
            >
                {#if numbered}<span class="tl-idx mono">{i + 1}</span>{/if}
                {#if showArt}
                    {#if item.art_url}
                        <img class="tl-art" src={item.art_url} alt="" loading="lazy" />
                    {:else}
                        <div class="tl-art placeholder">[ art ]</div>
                    {/if}
                {/if}
                <span class="tl-meta">
                    <span class="tl-name">
                        {item.name}{#if item.explicit}<span class="tl-e" title="Explicit">E</span>{/if}
                    </span>
                    {#if subLine(item)}
                        <span class="tl-sub">{subLine(item)}</span>
                    {/if}
                </span>
                {#if item.duration_ms}
                    <span class="tl-dur mono">{fmtMs(item.duration_ms)}</span>
                {/if}
                <span class="tl-play" aria-hidden="true"><Icon name="play" size={15} /></span>
            </button>
            {#if queueTarget && onEnqueue}
                <button
                    class="icon-btn tl-more"
                    aria-label="More for {item.name}"
                    aria-haspopup="menu"
                    aria-expanded={menuFor === item.uri}
                    disabled={busy.is("q:" + item.uri)}
                    onclick={(e) => toggleMenu(e, item.uri)}
                >
                    <Icon name="more" size={16} />
                </button>
                {#if menuFor === item.uri}
                    <div
                        class="tl-menu"
                        role="menu"
                        use:menuNav
                        in:scale={{ start: 0.95, duration: dur(140), easing: cubicOut, opacity: 0 }}
                        out:scale={{ start: 0.95, duration: dur(100), easing: cubicOut, opacity: 0 }}
                    >
                        <button class="tl-menu-item" role="menuitem" onclick={() => onEnqueue(item, true)}>
                            <Icon name="skipNext" size={16} /><span>Play next</span>
                        </button>
                        <button class="tl-menu-item" role="menuitem" onclick={() => onEnqueue(item, false)}>
                            <Icon name="queue" size={16} /><span>Add to queue</span>
                        </button>
                    </div>
                {/if}
            {/if}
        </div>
    {/each}
</div>

<style>
    .tl { display: flex; flex-direction: column; gap: 2px; }

    /* The row is a container, not a control: tapping the body plays now,
       the trailing overflow queues without interrupting. */
    .tl-row {
        position: relative;
        display: flex; align-items: center; gap: var(--space-1);
        border-radius: var(--r-md);
        transition: background 150ms ease;
    }
    @media (hover: hover) { .tl-row:hover { background: var(--card-2); } }
    .tl-open {
        flex: 1; min-width: 0;
        display: flex; align-items: center; gap: var(--space-3);
        min-height: 52px; padding: 6px var(--space-2);
        background: transparent; border: 0; border-radius: var(--r-md);
        color: var(--text); cursor: pointer; text-align: left; font: inherit;
    }
    .tl-open:active:not(:disabled) { background: var(--card-3); }
    .tl-open:disabled { opacity: 0.5; cursor: default; }

    .tl-idx {
        width: 2ch; flex-shrink: 0; text-align: right;
        font-size: 12px; color: var(--text-dim);
        font-feature-settings: "tnum" 1;
    }
    .tl-art {
        width: 44px; height: 44px; border-radius: var(--r-sm); flex-shrink: 0;
        object-fit: cover; background: var(--card-2); border: 1px solid var(--hairline);
    }
    div.tl-art { display: grid; place-items: center; font-size: 8px; color: var(--text-dim); }

    .tl-meta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
    .tl-name {
        font-size: 13.5px; font-weight: 500;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    /* The explicit mark: small, square, quiet — a badge, not a word. */
    .tl-e {
        display: inline-grid; place-items: center;
        width: 15px; height: 15px; margin-left: 6px;
        border-radius: 3px; vertical-align: 1px;
        background: var(--card-3); border: 1px solid var(--hairline);
        font-family: var(--font-mono); font-size: 8.5px; font-weight: 500;
        color: var(--text-mute);
    }
    .tl-sub {
        font-size: 11.5px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .tl-dur {
        flex-shrink: 0;
        font-size: 11.5px; color: var(--text-dim);
        font-feature-settings: "tnum" 1;
    }
    .tl-play {
        width: 36px; height: 36px; display: grid; place-items: center;
        border-radius: 50%; color: var(--text-mute); flex-shrink: 0;
        transition: color 150ms ease, background 150ms ease;
    }
    @media (hover: hover) { .tl-row:hover .tl-play { background: var(--on-soft); color: var(--on); } }

    .tl-more { width: 36px; height: 36px; flex-shrink: 0; margin-right: 4px; }
    .tl-more:disabled { opacity: 0.4; }
    .tl-menu {
        position: absolute; right: 8px; top: 46px; z-index: var(--z-menu);
        min-width: 180px;
        display: flex; flex-direction: column;
        background: var(--card-2);
        border: 1px solid var(--border-strong);
        border-radius: var(--r-md);
        overflow: hidden;
        box-shadow: var(--shadow-md);
    }
    .tl-menu-item {
        display: flex; align-items: center; gap: var(--space-3);
        padding: 12px var(--space-4);
        background: transparent; border: 0;
        border-bottom: 1px solid var(--hairline);
        cursor: pointer; font: inherit; font-size: 14px;
        color: var(--text); text-align: left;
    }
    .tl-menu-item:last-child { border-bottom: 0; }
    @media (hover: hover) { .tl-menu-item:hover { background: var(--card-3); } }

    /* ── Touch: hit areas grow to the 44px floor ── */
    @media (pointer: coarse) {
        .tl-more { width: 44px; height: 44px; }
    }
    /* The trailing play mark is a hover affordance — it only ever lights up
       under a cursor, and tapping the row is the play in any case. On a phone
       it was 36px of the row spent saying nothing, paid for by truncating the
       title and the artist, which are the things you choose a song by. */
    @media (max-width: 560px) {
        .tl-play { display: none; }
    }
    @media (prefers-reduced-motion: reduce) {
        .tl-row, .tl-play { transition-duration: 0.001ms; }
    }
</style>
