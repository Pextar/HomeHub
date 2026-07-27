<script lang="ts">
    /**
     * An artist's page, reached by tapping an artist in Search — top tracks
     * to pick from, and their albums. A screen, not a sheet: Search is
     * itself a sheet, and a sheet must never open another one (DESIGN.md
     * §15), so opening this one stands Search down first the same way a KEF
     * speaker's settings chip stands its player down before pushing.
     *
     * Tapping a track or an album plays it immediately on the destination
     * below, the same tap-to-play pattern as every other catalog row in the
     * module — there is nothing more to configure once you've picked one.
     */
    import type { Snippet } from "svelte";
    import BrowseScreen from "./BrowseScreen.svelte";
    import type { Destination } from "../../lib/music/destination.svelte";
    import type { Busy } from "../../lib/music/busy.svelte";
    import type { SpotifyArtistDetail, SpotifyItem } from "../../lib/types";

    let {
        artist,
        loading,
        destination,
        busy,
        targetRow,
        onBack,
        onPick,
    }: {
        artist: SpotifyArtistDetail | null;
        loading: boolean;
        destination: Destination;
        busy: Busy;
        targetRow: Snippet;
        onBack: () => void;
        onPick: (item: SpotifyItem) => void;
    } = $props();

    const sections = $derived([
        { label: "Top tracks", items: artist?.top_tracks ?? [] },
        { label: "Albums", items: artist?.albums ?? [] },
    ]);
</script>

<BrowseScreen
    loading={loading || !artist}
    art={artist?.art_url}
    artRound
    title={artist?.name ?? "Artist"}
    backLabel="Back to Music"
    onBack={onBack}
    {destination}
    {busy}
    {targetRow}
    {sections}
    {onPick}
    empty="Nothing came back for this artist yet."
/>
