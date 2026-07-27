<script lang="ts">
    /**
     * A favorite that turns out to be a list — a Spotify playlist or album —
     * rather than one song: its own tracks, so tapping the favorite reads as
     * "look inside" and playing it whole is still one tap away via "Play
     * all". A screen, not a sheet, for the same reason as the artist page:
     * it is reached from the Home shelf or the idle player, and a sheet must
     * never open another one.
     */
    import type { Snippet } from "svelte";
    import BrowseScreen from "./BrowseScreen.svelte";
    import type { Destination } from "../../lib/music/destination.svelte";
    import type { Busy } from "../../lib/music/busy.svelte";
    import type { SonosFavorite, SpotifyContextDetail, SpotifyItem } from "../../lib/types";

    let {
        favorite,
        context,
        loading,
        destination,
        busy,
        targetRow,
        onBack,
        onPlayAll,
        playAllBusy,
        onPick,
    }: {
        favorite: SonosFavorite;
        context: SpotifyContextDetail | null;
        loading: boolean;
        destination: Destination;
        busy: Busy;
        targetRow: Snippet;
        onBack: () => void;
        onPlayAll: () => void;
        playAllBusy: boolean;
        onPick: (item: SpotifyItem) => void;
    } = $props();

    const sections = $derived([{ label: "Tracks", items: context?.tracks ?? [] }]);
</script>

<BrowseScreen
    loading={loading || !context}
    art={context?.art_url ?? favorite.art_uri}
    title={context?.name ?? favorite.title}
    sub={context?.sub}
    backLabel="Back to Music"
    onBack={onBack}
    {onPlayAll}
    {playAllBusy}
    playAllDisabled={!destination.sonosTarget}
    {destination}
    {busy}
    {targetRow}
    {sections}
    {onPick}
    empty="This list didn't come back with any tracks."
/>
