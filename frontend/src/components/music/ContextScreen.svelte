<script lang="ts">
    /**
     * An album's or a playlist's own page: the cover, who made it, when, how
     * long it runs, and the full track listing — numbered for an album,
     * because the running order is part of the record.
     *
     * It exists so a tap on a container is "let me see what's on this" rather
     * than "fire an unknown hour of audio into the kitchen". Playing it whole
     * is still one tap, on the button that says so.
     *
     * A screen, not a sheet: it is pushed onto Browse's stack, so `back`
     * climbs one level to whatever opened it — the search shelf, or an
     * artist's discography (DESIGN.md §15.6).
     */
    import type { Snippet } from "svelte";
    import { fly } from "svelte/transition";
    import { cubicOut } from "svelte/easing";
    import Icon from "../Icon.svelte";
    import TrackList from "./TrackList.svelte";
    import { dur } from "../../lib/motion";
    import { fmtCount, fmtTotalMs } from "../../lib/music/format";
    import type { Destination } from "../../lib/music/destination.svelte";
    import type { Busy } from "../../lib/music/busy.svelte";
    import type { SpotifyContextDetail, SpotifyItem } from "../../lib/types";

    let {
        context,
        loading,
        destination,
        busy,
        targetRow,
        onBack,
        onPlayAll,
        /** Overridden by the favorites path, whose whole-list play is a Sonos
         *  household call under a different busy key. */
        playAllBusy = undefined,
        playAllDisabled = false,
        /** Why the whole-list play can't run right now, when it can't. */
        playAllNote = "",
        onPick,
        onEnqueue,
        onOpenArtist,
    }: {
        context: SpotifyContextDetail | null;
        loading: boolean;
        destination: Destination;
        busy: Busy;
        targetRow: Snippet;
        onBack: () => void;
        onPlayAll: () => void;
        playAllBusy?: boolean;
        playAllDisabled?: boolean;
        playAllNote?: string;
        onPick: (item: SpotifyItem) => void;
        onEnqueue: (item: SpotifyItem, next: boolean) => void;
        onOpenArtist: (uri: string) => void;
    } = $props();

    const isAlbum = $derived(context?.kind === "album");
    const tracks = $derived(context?.tracks ?? []);

    /** How long the whole thing runs — summed from what came back, so it is
     *  absent rather than wrong when the service didn't send durations. */
    const totalMs = $derived(tracks.reduce((sum, t) => sum + (t.duration_ms ?? 0), 0));

    /** The line under the title: everything that identifies the record. */
    const facts = $derived.by(() => {
        const c = context;
        if (!c) return [];
        const out: string[] = [];
        if (c.year) out.push(c.year);
        const n = c.total_tracks ?? tracks.length;
        if (n) out.push(`${n} song${n === 1 ? "" : "s"}`);
        const dur = fmtTotalMs(totalMs);
        if (dur) out.push(dur);
        if (c.followers) out.push(`${fmtCount(c.followers)} saves`);
        return out;
    });

    /**
     * Spotify writes playlist descriptions as HTML fragments (they carry
     * links). Rendering that would be an injection; the text alone is what
     * the line is for, so the tags come out.
     */
    const description = $derived(
        (context?.description ?? "")
            .replace(/<[^>]*>/g, "")
            .replace(/&amp;/g, "&")
            .replace(/&quot;/g, '"')
            .replace(/&#x27;|&#39;/g, "'")
            .trim(),
    );

    let list = $state<TrackList | null>(null);

    /** Escape closes an open row menu before it leaves the screen. */
    export function closeMenu(): boolean {
        return !!list?.closeMenu();
    }
</script>

<div class="screen-head">
    <button class="icon-btn" aria-label="Back" onclick={onBack}>
        <Icon name="chevronLeft" size={18} />
    </button>
    <div class="screen-title">
        <h1>{context?.name ?? (isAlbum ? "Album" : "Playlist")}</h1>
        <span class="screen-sub">{context ? (isAlbum ? "Album" : "Playlist") : ""}</span>
    </div>
    <span class="head-spacer" aria-hidden="true"></span>
</div>

<div class="cx" in:fly={{ y: 10, duration: dur(240), easing: cubicOut }}>
    {#if loading || !context}
        <div class="skeleton sk-hero"></div>
        <div class="skeleton sk-row"></div>
        <div class="skeleton sk-row"></div>
        <div class="skeleton sk-row"></div>
    {:else}
        <section class="card cx-hero">
            <div class="cx-top">
                {#if context.art_url}
                    <img class="cx-art" src={context.art_url} alt="" />
                {:else}
                    <div class="cx-art placeholder">[ cover ]</div>
                {/if}
                <div class="cx-id">
                    <h2 class="cx-name">{context.name}</h2>
                    {#if context.sub}
                        {#if isAlbum && context.artist_uri}
                            <!-- The artist is a place to go, so it is a link,
                                 not a caption. -->
                            <button class="cx-by" onclick={() => onOpenArtist(context.artist_uri!)}>
                                {context.sub}
                                <Icon name="chevronLeft" size={12} />
                            </button>
                        {:else}
                            <span class="cx-owner">{context.sub}</span>
                        {/if}
                    {/if}
                    {#if facts.length}
                        <span class="cx-facts mono">{facts.join(" · ")}</span>
                    {/if}
                </div>
            </div>

            {#if description}
                <p class="cx-desc">{description}</p>
            {/if}

            <div class="cx-where">{@render targetRow()}</div>
            <button
                class="btn btn-primary cx-playall"
                disabled={(playAllBusy ?? busy.is("item:" + context.uri)) ||
                    playAllDisabled ||
                    !destination.current}
                onclick={onPlayAll}
            >
                <Icon name="play" size={15} />
                Play {isAlbum ? "album" : "playlist"}
            </button>
            {#if playAllNote}
                <p class="hint cx-note">{playAllNote}</p>
            {/if}
        </section>

        {#if tracks.length === 0}
            <p class="cx-empty">
                This {isAlbum ? "album" : "playlist"} didn't come back with any tracks.
            </p>
        {:else}
            <section class="block">
                <div class="eyrow">{isAlbum ? "Tracks" : "Songs"}</div>
                <!-- An album's rows all share the page's cover and its title,
                     so they carry neither; a playlist's differ per row and
                     both are worth showing. -->
                <TrackList
                    items={tracks}
                    numbered={isAlbum}
                    showArt={!isAlbum}
                    showAlbum={!isAlbum}
                    omitSub={isAlbum ? (context.sub ?? "") : ""}
                    {busy}
                    canPlay={!!destination.current}
                    queueTarget={destination.sonosTarget}
                    onPick={onPick}
                    {onEnqueue}
                    bind:this={list}
                />
            </section>
        {/if}
    {/if}
</div>

<style>
    /* ── Screen head — the §11 shape ── */
    .screen-head { display: flex; align-items: center; gap: var(--space-3); }
    .screen-title {
        flex: 1; min-width: 0;
        display: flex; flex-direction: column; gap: 2px;
        text-align: center;
    }
    .screen-title h1 {
        font-family: var(--font-sans);
        font-size: 20px; font-weight: 600; letter-spacing: -0.02em;
        color: var(--text);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .screen-sub {
        font-family: var(--font-mono);
        font-size: 10px; letter-spacing: 0.1em; text-transform: uppercase;
        color: var(--text-dim);
    }
    .head-spacer { width: 32px; height: 32px; flex-shrink: 0; }

    .cx { display: flex; flex-direction: column; gap: var(--space-5); margin-top: var(--space-4); }
    .sk-hero { height: 180px; border-radius: var(--r-lg); }
    .sk-row { height: 52px; border-radius: var(--r-md); }

    .cx-hero { display: flex; flex-direction: column; gap: var(--space-4); }
    .cx-top { display: flex; align-items: flex-end; gap: var(--space-4); }
    .cx-art {
        width: 128px; height: 128px; flex-shrink: 0;
        border-radius: var(--r-md); object-fit: cover;
        background: var(--card-2); border: 1px solid var(--hairline);
        box-shadow: var(--shadow-md);
    }
    div.cx-art { display: grid; place-items: center; font-size: 9px; color: var(--text-dim); }
    .cx-id { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 5px; }
    .cx-name {
        font-size: 22px; font-weight: 600; letter-spacing: -0.03em; line-height: 1.15;
        color: var(--text);
    }
    .cx-by {
        align-self: flex-start;
        display: inline-flex; align-items: center; gap: 3px;
        min-height: 28px; padding: 0;
        background: none; border: 0;
        font: inherit; font-size: 13px; font-weight: 500;
        color: var(--on); cursor: pointer; text-align: left;
    }
    .cx-by :global(svg) { transform: rotate(180deg); }
    .cx-owner { font-size: 13px; color: var(--text-mute); }
    .cx-facts {
        font-size: 11px; letter-spacing: 0.04em; text-transform: uppercase;
        color: var(--text-dim);
        font-feature-settings: "tnum" 1;
    }
    .cx-desc {
        font-size: 12.5px; color: var(--text-mute); line-height: 1.5;
    }
    .cx-where { display: flex; }
    .cx-playall { align-self: flex-start; padding: 9px 18px; }
    .cx-note { margin-top: -8px; }

    .cx-empty { font-size: 12.5px; color: var(--text-mute); }

    /* On a phone the cover stops competing with the title for the row. */
    @media (max-width: 460px) {
        .cx-top { flex-direction: column; align-items: flex-start; gap: var(--space-3); }
        .cx-art { width: 108px; height: 108px; }
    }
    @media (pointer: coarse) {
        .cx-by { min-height: 44px; }
    }
</style>
