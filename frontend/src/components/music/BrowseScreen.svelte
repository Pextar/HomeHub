<script lang="ts">
    /**
     * A catalog drill-in screen: an artist's top tracks and albums, or a
     * favorite's own track listing. Not a sheet — both are reached from a
     * sheet's worth of navigation (Search, or the Home favorites shelf) or
     * from the player, and a sheet must never open another sheet
     * (DESIGN.md §15) — so this takes the §11 detail shape instead: back
     * chip, centered title, hero art, then one row list per section.
     *
     * Shared by ArtistScreen's two sections (top tracks, albums) and the
     * favorite browse screen's one (tracks, plus a "Play all" hero action)
     * rather than duplicated between them — the row shape and the "playing
     * on" row are identical either way.
     */
    import type { Snippet } from "svelte";
    import { fly } from "svelte/transition";
    import { cubicOut } from "svelte/easing";
    import Icon from "../Icon.svelte";
    import { dur } from "../../lib/motion";
    import type { Destination } from "../../lib/music/destination.svelte";
    import type { Busy } from "../../lib/music/busy.svelte";
    import type { SpotifyItem } from "../../lib/types";

    let {
        loading,
        art,
        artRound = false,
        title,
        sub,
        backLabel,
        onBack,
        onPlayAll,
        playAllBusy = false,
        /** Favorites are Sonos-only, so "Play all" needs a Sonos target
         *  specifically — not just any destination — where every other row
         *  here plays a plain Spotify track and takes whatever is selected. */
        playAllDisabled = false,
        destination,
        busy,
        targetRow,
        sections,
        onPick,
        empty,
    }: {
        loading: boolean;
        art?: string;
        /** An artist's picture reads as a portrait; a playlist/album's as a cover. */
        artRound?: boolean;
        title: string;
        sub?: string;
        backLabel: string;
        onBack: () => void;
        /** Present only for a favorite: play the whole list on the destination below. */
        onPlayAll?: () => void;
        playAllBusy?: boolean;
        playAllDisabled?: boolean;
        destination: Destination;
        busy: Busy;
        targetRow: Snippet;
        sections: { label: string; items: SpotifyItem[] }[];
        onPick: (item: SpotifyItem) => void;
        /** Shown once loading has finished and every section came back empty. */
        empty: string;
    } = $props();

    const anyItems = $derived(sections.some((s) => s.items.length > 0));
</script>

<div class="screen-head">
    <button class="icon-btn" aria-label={backLabel} onclick={onBack}>
        <Icon name="chevronLeft" size={18} />
    </button>
    <div class="screen-title">
        <h1>{title}</h1>
        {#if sub}<span class="screen-sub">{sub}</span>{/if}
    </div>
    <span class="head-spacer" aria-hidden="true"></span>
</div>

<div class="browse" in:fly={{ y: 10, duration: dur(240), easing: cubicOut }}>
    {#if loading}
        <div class="skeleton br-hero-sk"></div>
        <div class="skeleton br-row-sk"></div>
        <div class="skeleton br-row-sk"></div>
        <div class="skeleton br-row-sk"></div>
    {:else}
        <section class="card br-hero">
            {#if art}
                <img class="br-art" class:br-art-round={artRound} src={art} alt="" />
            {:else}
                <div class="br-art placeholder" class:br-art-round={artRound}>[ art ]</div>
            {/if}
            <div class="br-hero-body">
                <div class="br-target">{@render targetRow()}</div>
                {#if onPlayAll}
                    <button class="btn btn-primary br-playall" disabled={playAllBusy || playAllDisabled} onclick={onPlayAll}>
                        <Icon name="play" size={15} />
                        Play all
                    </button>
                {/if}
            </div>
        </section>

        {#if !anyItems}
            <p class="br-empty">{empty}</p>
        {:else}
            {#each sections as section (section.label)}
                {#if section.items.length > 0}
                    <section class="br-section">
                        <div class="eyrow">{section.label}</div>
                        <div class="br-list">
                            {#each section.items as item (item.uri)}
                                <button class="br-row" disabled={busy.is("item:" + item.uri) || !destination.current}
                                    onclick={() => onPick(item)}>
                                    {#if item.art_url}
                                        <img class="br-thumb" src={item.art_url} alt="" loading="lazy" />
                                    {:else}
                                        <div class="br-thumb placeholder">[ art ]</div>
                                    {/if}
                                    <span class="br-meta">
                                        <span class="br-name">{item.name}</span>
                                        {#if item.sub}<span class="br-sub">{item.sub}</span>{/if}
                                    </span>
                                    <span class="br-play"><Icon name="play" size={15} /></span>
                                </button>
                            {/each}
                        </div>
                    </section>
                {/if}
            {/each}
        {/if}
    {/if}
</div>

<style>
    /* ── Screen head — the §11 shape, matching Speakers ── */
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
        font-size: 12px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    /* Balances the back chip so the title stays centred with nothing on the right. */
    .head-spacer { width: 32px; height: 32px; flex-shrink: 0; }

    .browse { display: flex; flex-direction: column; gap: var(--space-4); margin-top: var(--space-4); }
    .br-hero-sk { height: 140px; border-radius: var(--r-lg); }
    .br-row-sk { height: 52px; border-radius: var(--r-md); }

    .br-hero { display: flex; align-items: center; gap: var(--space-4); }
    .br-art {
        width: 88px; height: 88px; border-radius: var(--r-md); flex-shrink: 0;
        object-fit: cover; background: var(--card-2); border: 1px solid var(--hairline);
    }
    .br-art-round { border-radius: 50%; }
    div.br-art { display: grid; place-items: center; font-size: 9px; color: var(--text-dim); }
    .br-hero-body { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: var(--space-3); }
    .br-playall { align-self: flex-start; padding: 8px 16px; }

    .br-empty { font-size: 12.5px; color: var(--text-mute); }

    .br-section { display: flex; flex-direction: column; gap: var(--space-2); }
    .br-list { display: flex; flex-direction: column; gap: 2px; }
    .br-row {
        display: flex; align-items: center; gap: var(--space-3);
        min-height: 52px; padding: 6px var(--space-2);
        background: transparent; border: 0; border-radius: var(--r-md);
        color: var(--text); cursor: pointer; text-align: left; font: inherit;
        transition: background 150ms ease;
    }
    @media (hover: hover) { .br-row:hover { background: var(--card-2); } }
    .br-row:active:not(:disabled) { background: var(--card-3); }
    .br-row:disabled { opacity: 0.5; cursor: default; }
    .br-thumb {
        width: 40px; height: 40px; border-radius: var(--r-sm); flex-shrink: 0;
        object-fit: cover; background: var(--card-2); border: 1px solid var(--hairline);
    }
    div.br-thumb { display: grid; place-items: center; font-size: 8px; color: var(--text-dim); }
    .br-meta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
    .br-name {
        font-size: 13.5px; font-weight: 500;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .br-sub {
        font-size: 11.5px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .br-play {
        width: 32px; height: 32px; display: grid; place-items: center;
        border-radius: 50%; color: var(--text-mute); flex-shrink: 0;
    }

    @media (pointer: coarse) {
        .br-play { width: 44px; height: 44px; }
    }
    @media (prefers-reduced-motion: reduce) {
        .br-row { transition-duration: 0.001ms; }
    }
</style>
