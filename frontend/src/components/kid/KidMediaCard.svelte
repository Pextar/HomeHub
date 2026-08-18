<script lang="ts">
    /**
     * The one cover card for the kid module (DESIGN.md §17): albums,
     * playlists and artists are chosen by their picture as much as by
     * their name, so the picture is the target.
     */
    import { rowSub } from "../../lib/music/catalog";
    import type { SpotifyItem } from "../../lib/types";

    const KIND_EMOJI: Record<string, string> = {
        track: "🎵",
        album: "💿",
        playlist: "📃",
        artist: "🎤",
    };

    let {
        item,
        onPick,
    }: {
        item: SpotifyItem;
        onPick: (item: SpotifyItem) => void;
    } = $props();
</script>

<button class="kms-card" onclick={() => onPick(item)}>
    {#if item.art_url}
        <img class="kms-card-art" class:round={item.kind === "artist"} src={item.art_url} alt="" loading="lazy" />
    {:else}
        <span class="kms-card-art kms-card-none" class:round={item.kind === "artist"} aria-hidden="true">
            {KIND_EMOJI[item.kind]}
        </span>
    {/if}
    <span class="kms-card-name">{item.name}</span>
    {#if rowSub(item)}
        <span class="kms-card-sub">{rowSub(item)}</span>
    {/if}
</button>

<style>
    .kms-card {
        display: flex;
        flex-direction: column;
        align-items: stretch;
        gap: var(--space-2);
        padding: var(--space-3);
        border-radius: var(--radius-lg);
        border: 2px solid var(--border);
        background: var(--bg-elevated);
        cursor: pointer;
        text-align: left;
        transition: transform 0.12s ease, border-color 0.15s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .kms-card:active { transform: scale(0.96); border-color: var(--kid-accent); }
    .kms-card-art {
        width: 100%;
        aspect-ratio: 1;
        border-radius: var(--radius-md);
        object-fit: cover;
    }
    .kms-card-art.round { border-radius: 50%; }
    .kms-card-none {
        background: var(--surface-hover);
        display: grid;
        place-items: center;
        font-size: 2.6rem;
    }
    .kms-card-name {
        font-size: 0.95rem;
        font-weight: 800;
        color: var(--text);
        line-height: 1.2;
        display: -webkit-box;
        -webkit-line-clamp: 2;
        line-clamp: 2;
        -webkit-box-orient: vertical;
        overflow: hidden;
    }
    .kms-card-sub {
        font-size: 0.78rem;
        font-weight: 600;
        color: var(--text-muted);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
</style>
