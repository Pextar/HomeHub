<script lang="ts">
    /**
     * The app's own catalog screens (DESIGN.md §15.9), riding in the wall's
     * work column (§16).
     *
     * ArtistScreen and ContextScreen were built against the Music view's
     * stores, and they are worth reusing rather than rewriting for the wall —
     * an artist's page that reads differently depending on which screen you
     * found it on is a bug, and two implementations is how that happens. So
     * what lives here is the adapter: the panel store answering the two
     * contracts those screens ask for.
     *
     * `destination` is the featured source, because on a wall there is no
     * per-row target to choose — the room chips on the header already said
     * where a tap plays, and the screens' `targetRow` says it again in
     * words. `busy` is the same map the rows disable on, so a call started
     * from a catalog page greys the same control a call started anywhere
     * else does.
     */
    import ArtistScreen from "../music/ArtistScreen.svelte";
    import ContextScreen from "../music/ContextScreen.svelte";
    import Icon from "../Icon.svelte";
    import { toasts } from "../../lib/stores.svelte";
    import { contextItem } from "../../lib/music/catalog-cache.svelte";
    import type { CatalogStack } from "../../lib/music/catalog-stack.svelte";
    import type { Busy } from "../../lib/music/busy.svelte";
    import type { PanelMusicStore, PanelSource } from "../../lib/panel-music/types";
    import type { SpotifyItem } from "../../lib/types";

    let {
        catalog,
        music,
        featured,
        onPick,
    }: {
        catalog: CatalogStack;
        music: PanelMusicStore;
        featured: PanelSource | undefined;
        onPick: (item: SpotifyItem) => void;
    } = $props();

    const dest = {
        get current() {
            return featured ? { kind: featured.kind, id: featured.id } : null;
        },
        get sonosTarget() {
            return featured?.kind === "sonos" ? featured.id : null;
        },
    };

    const busy: Busy = {
        is: (k) => !!music.busy[k],
        async claim(k, fn) {
            if (music.busy[k]) return undefined;
            music.busy[k] = true;
            try {
                return await fn();
            } finally {
                music.busy[k] = false;
            }
        },
        async run(k, fn, errTitle, after) {
            await busy.claim(k, async () => {
                try {
                    await fn();
                    await after?.();
                } catch (e) {
                    toasts.error(errTitle, (e as Error).message);
                }
            });
        },
    };

    let artistScr = $state<ArtistScreen | null>(null);
    let contextScr = $state<ContextScreen | null>(null);

    /**
     * Escape closes an open row menu before it climbs the stack, so the depth
     * asks here first. Answers whether it consumed the key. Only one screen
     * is ever mounted, so at most one of these answers.
     */
    export function closeMenu(): boolean {
        return !!(artistScr?.closeMenu() || contextScr?.closeMenu());
    }
</script>

<!-- The catalog screens name where they'll sound; on the wall that's always
     the featured source — its chips ride on the header. -->
{#snippet playOnRow()}
    <span class="play-on">
        <Icon name="speaker" size={14} />
        <span>{featured ? `Plays on ${featured.title}` : "No speaker reachable"}</span>
    </span>
{/snippet}

<div class="b-stack">
    {#if catalog.top?.kind === "artist"}
        <ArtistScreen
            artist={catalog.artistDetail}
            loading={catalog.artistLoading}
            destination={dest}
            {busy}
            targetRow={playOnRow}
            onBack={() => void catalog.pop()}
            onPick={onPick}
            onEnqueue={(item, next) => music.enqueue(item, next)}
            onOpenArtist={(uri) => void catalog.openArtist(uri)}
            onOpenContext={(uri) => void catalog.openContext(uri)}
            bind:this={artistScr}
        />
    {:else}
        <ContextScreen
            context={catalog.contextDetail}
            loading={catalog.contextLoading}
            destination={dest}
            {busy}
            targetRow={playOnRow}
            onBack={() => void catalog.pop()}
            onPlayAll={() => catalog.contextDetail && onPick(contextItem(catalog.contextDetail))}
            onPick={onPick}
            onEnqueue={(item, next) => music.enqueue(item, next)}
            onOpenArtist={(uri) => void catalog.openArtist(uri)}
            bind:this={contextScr}
        />
    {/if}
</div>

<style>
    .b-stack {
        flex: 1;
        min-height: 0;
        overflow-y: auto;
        overflow-x: hidden;
        padding-bottom: var(--space-2);
    }
    .play-on {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        font-size: 12.5px;
        color: var(--text-mute);
    }

    /* Portrait / narrow fallback: the page scrolls as one rather than the
       column owning its own overflow. */
    @media (orientation: portrait), (max-width: 760px) {
        .b-stack {
            overflow: visible;
            flex: none;
        }
    }
</style>
