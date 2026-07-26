<script lang="ts">
    /**
     * Music's home screen: what is playing, the favorites shelf, the zones at
     * a glance, and the way through to Speakers.
     *
     * "Playing now" means playing. When nothing is, it collapses to a single
     * quiet row rather than one dead card per zone — idle zones stay one tap
     * away in the chips below, which is where "open a room" belongs.
     */
    import type { Snippet } from "svelte";
    import Icon from "../Icon.svelte";
    import Waveform from "./Waveform.svelte";
    import NowCard from "./NowCard.svelte";
    import CardTransport from "./CardTransport.svelte";
    import QuietCard from "./QuietCard.svelte";
    import NavRow from "./NavRow.svelte";
    import type { SonosBridge } from "../../lib/music/sonos.svelte";
    import type { KEFBridge } from "../../lib/music/kef.svelte";
    import type { Busy } from "../../lib/music/busy.svelte";
    import type { Destination } from "../../lib/music/destination.svelte";
    import type { SpotifyStore } from "../../lib/music/spotify.svelte";
    import type { SonosFavorite, SonosGroupView, KEFSpeakerView } from "../../lib/types";

    let {
        sonos,
        kef,
        busy,
        destination,
        spotify,
        totalSpeakers,
        readyCount,
        /** The zone the dock is holding, so its card can report visibility. */
        dockCoordinator = undefined,
        onDockVisible,
        onOpenPlayer,
        onOpenKEFPlayer,
        onOpenSearch,
        onOpenZones,
        onOpenSpeakers,
        targetRow,
        favCard,
    }: {
        sonos: SonosBridge;
        kef: KEFBridge;
        busy: Busy;
        destination: Destination;
        spotify: SpotifyStore;
        totalSpeakers: number;
        readyCount: number;
        dockCoordinator?: string;
        onDockVisible: (visible: boolean) => void;
        onOpenPlayer: (g: SonosGroupView) => void;
        onOpenKEFPlayer: (sp: KEFSpeakerView) => void;
        onOpenSearch: () => void;
        onOpenZones: () => void;
        onOpenSpeakers: () => void;
        targetRow: Snippet;
        favCard: Snippet<[SonosFavorite, string | null]>;
    } = $props();
</script>

<!-- ── Playing now ─────────────────────────────────────────────────
     Only what is actually playing. Idle zones are one tap away in the
     room chips below, so listing them here would just make the heading
     lie and bury the thing the user came for. -->
<section class="block">
    <div class="eyrow">Playing now</div>
    {#if sonos.playingGroups.length === 0 && kef.playing.length === 0}
        <QuietCard
            title="Nothing playing"
            action={spotify.status
                ? {
                      // Not gated on `connected`: the people who most need
                      // a pointer at Spotify are the ones who haven't set
                      // it up, and with the subnav gone this card and the
                      // header icon are the only things that say the
                      // module searches at all (DESIGN.md §15).
                      label: spotify.connected ? "Search" : "Set up Spotify",
                      onClick: onOpenSearch,
                  }
                : undefined}
        >
            <span class="mono">{readyCount}</span>
            speaker{readyCount === 1 ? "" : "s"} ready —
            {sonos.favorites.length > 0 && !destination.kefSpeaker
                ? "start a favorite below"
                : "pick a room to open it"}
        </QuietCard>
    {:else}
        <div class="now-grid">
            {#each sonos.playingGroups as g (g.coordinator_id)}
                {@const c = sonos.coordinatorOf(g)}
                {@const st = c?.state}
                <NowCard
                    name={sonos.groupTitle(g)}
                    line={[st?.track?.title, st?.track?.artist].filter(Boolean).join(" · ") ||
                        "Live audio"}
                    artUri={st?.track?.art_uri}
                    playing
                    progress={sonos.progressOf(g)}
                    onOpen={() => onOpenPlayer(g)}
                    isDock={g.coordinator_id === dockCoordinator}
                    {onDockVisible}
                >
                    {#snippet transport()}
                        <CardTransport
                            playing={sonos.isPlaying(g)}
                            onToggle={() => sonos.togglePlay(g)}
                            toggleBusy={!c || busy.is("play:" + c?.id)}
                            onPrev={() => sonos.skip(g, "previous")}
                            prevBusy={!c || busy.is("previous:" + c?.id)}
                            onNext={() => sonos.skip(g, "next")}
                            nextBusy={!c || busy.is("next:" + c?.id)}
                        />
                    {/snippet}
                </NowCard>
            {/each}

            <!-- KEF speakers that are playing, in the same grid and with
                 the same card. It is a way in to a player like every
                 other card here — the sheet it opens drops the queue and
                 the group, which KEF hasn't got, and keeps the rest. -->
            {#each kef.playing as sp (sp.id)}
                <NowCard
                    name={sp.name}
                    line={[kef.nowLine(sp), kef.subLine(sp)].filter(Boolean).join(" · ")}
                    artUri={sp.state?.track?.art_uri}
                    playing
                    progress={kef.progress(sp)}
                    onOpen={() => onOpenKEFPlayer(sp)}
                >
                    {#snippet transport()}
                        <!-- Play/pause only, like the Sonos card below
                             430px: the sheet is where the skips live. -->
                        <CardTransport
                            playing={kef.isPlaying(sp)}
                            onToggle={() => kef.togglePlay(sp)}
                            toggleBusy={busy.is("kefplay:" + sp.id)}
                        />
                    {/snippet}
                </NowCard>
            {/each}
        </div>
    {/if}
</section>

<!-- ── Favorites ───────────────────────────────────────────────── -->
{#if sonos.favorites.length > 0}
    <section class="block">
        <div class="block-head">
            <div class="eyrow">Favorites</div>
            {@render targetRow()}
        </div>
        {#if destination.kefSpeaker}
            <!-- "My Sonos" is a household list, and a KEF speaker has no
                 way to play an entry from it. A rail of disabled cards
                 would be a row of dead controls (§15), so the section
                 says what it needs instead — and the fix is one tap on
                 the destination row directly above. -->
            <QuietCard
                title="Favorites need a Sonos room"
                action={spotify.status
                    ? {
                          label: spotify.connected ? "Search" : "Set up Spotify",
                          onClick: onOpenSearch,
                      }
                    : undefined}
            >
                They come out of your Sonos household, so {destination.kefSpeaker.name} can't
                play one — pick a Sonos room above{#if spotify.connected}, or search to play
                    there{/if}.
            </QuietCard>
        {:else}
            <div class="favs h-scroll">
                {#each sonos.favorites as f (f.id)}
                    {@render favCard(f, destination.sonosTarget)}
                {/each}
            </div>
        {/if}
    </section>
{/if}

<!-- ── Zones at a glance (Home) ─────────────────────────────────
     "Zones", not "Rooms": the app-level nav already owns that word for
     the whole house, and reusing it here for speaker grouping was the
     confusing part (DESIGN.md §15). -->
<section class="block">
    <div class="block-head">
        <div class="eyrow">Zones</div>
        <button class="link-btn" onclick={onOpenZones}>Manage</button>
    </div>
    <div class="room-chips">
        {#each sonos.reachable as sp (sp.id)}
            {@const g = sonos.groupOfSpeaker(sp.id)}
            <button
                class="room-chip"
                class:on={sonos.speakerPlaying(sp.id)}
                disabled={!g}
                onclick={() => g && onOpenPlayer(g)}
            >
                {#if sonos.speakerPlaying(sp.id)}
                    <Waveform />
                {:else}
                    <Icon name="speaker" size={14} />
                {/if}
                <span>{sp.name}</span>
            </button>
        {/each}
        <!-- KEF speakers are rooms that play too, so they belong in this
             row — and they open a player, like every chip beside them.
             They are absent from Zones instead, which is honest: Zones
             answers what plays together, and a KEF speaker never does. -->
        {#each kef.reachable as sp (sp.id)}
            <button
                class="room-chip"
                class:on={kef.isPlaying(sp)}
                onclick={() => onOpenKEFPlayer(sp)}
            >
                {#if kef.isPlaying(sp)}
                    <Waveform />
                {:else}
                    <Icon name="speaker" size={14} />
                {/if}
                <span>{sp.name}</span>
            </button>
        {/each}
    </div>
</section>

<!-- The way through to the device inventory. A plain row rather than a
     header icon, because what it opens is a screen — Speakers pushes,
     Search and Zones lift. -->
<NavRow icon="speaker" title="Speakers" count={totalSpeakers} onClick={onOpenSpeakers}>
    {#snippet sub()}
        {#if sonos.offline.length > 0}
            <span class="mono">{sonos.offline.length}</span>
            unreachable — fix an address, or set one up
        {:else}
            Names, addresses, tone and the status light
        {/if}
    {/snippet}
</NavRow>

<style>
    /* ── Zones at a glance ── */
    .room-chips {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
        gap: var(--space-2);
    }
    .room-chip {
        display: flex; align-items: center; justify-content: center; gap: 6px;
        min-height: 44px; padding: 10px var(--space-3);
        background: var(--card-2); border: 1px solid var(--hairline);
        border-radius: var(--r-pill);
        color: var(--text-mute); font-size: 12.5px; cursor: pointer;
        transition: border-color var(--t-fast), color var(--t-fast);
    }
    .room-chip span {
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .room-chip.on { background: var(--on-soft); color: var(--on); border-color: transparent; }
    .room-chip:disabled { opacity: 0.5; cursor: default; }
    @media (hover: hover) {
        .room-chip:not(:disabled):hover { border-color: var(--border-strong); color: var(--text); }
        .room-chip.on:not(:disabled):hover { color: var(--on); }
    }

    .now-grid {
        display: grid;
        /* Wide enough that the track title still has room next to the
           three-button transport — narrower columns crushed it to an ellipsis
           on desktop. */
        grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
        gap: var(--space-3);
    }
    .favs { display: flex; gap: var(--space-3); padding-bottom: var(--space-1); }
    .link-btn {
        background: none; border: 0; padding: 0;
        color: var(--text-mute); font-size: 12.5px; cursor: pointer;
    }
    .link-btn:hover { color: var(--text); }
</style>
