<script lang="ts">
    /**
     * A favorite that turns out to be a list — a Spotify playlist or album —
     * rather than one song. It is the same page as any other album or
     * playlist (`ContextScreen`), because it *is* one: the only difference is
     * how you got here and that "play it whole" goes out through the Sonos
     * household's favorite rather than the Spotify URI.
     *
     * That distinction is the reason this file still exists. Favorites are a
     * Sonos household list, so only a Sonos room can be handed one — and the
     * whole-list play has to take that road, while an individual track inside
     * it is a plain Spotify item any room can take.
     */
    import type { Snippet } from "svelte";
    import ContextScreen from "./ContextScreen.svelte";
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
        onEnqueue,
        onOpenArtist,
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
        onEnqueue: (item: SpotifyItem, next: boolean) => void;
        onOpenArtist: (uri: string) => void;
    } = $props();

    /** Until the lookup lands, the favorite's own title and art stand in — a
     *  page that already knows its name shouldn't render as a blank one. */
    const shown = $derived<SpotifyContextDetail | null>(
        context ??
            (loading
                ? null
                : {
                      kind: "playlist",
                      uri: favorite.spotify_uri ?? favorite.uri,
                      name: favorite.title,
                      art_url: favorite.art_uri,
                      tracks: [],
                  }),
    );

    let screen = $state<ContextScreen | null>(null);
    export function closeMenu(): boolean {
        return !!screen?.closeMenu();
    }
</script>

<ContextScreen
    context={shown}
    {loading}
    {destination}
    {busy}
    {targetRow}
    {onBack}
    {onPlayAll}
    {playAllBusy}
    playAllDisabled={!destination.sonosTarget}
    playAllNote={destination.sonosTarget
        ? ""
        : `Favorites come out of your Sonos household, so ${destination.label || "this room"} can't play the whole list — pick a Sonos room above, or start a single track below.`}
    {onPick}
    {onEnqueue}
    {onOpenArtist}
    bind:this={screen}
/>
