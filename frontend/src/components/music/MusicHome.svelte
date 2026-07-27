<script lang="ts">
    /**
     * Music's home screen: one hero, one grid of rooms, one way through to the
     * devices. That is the whole screen.
     *
     * What it replaced was four stacked sections that each described the same
     * speakers differently — a "Playing now" grid, a favorites rail with its
     * own destination picker, a row of small chips mixing zones with rooms with
     * lone speakers, and a "Zones" sheet one level down holding the grouping
     * gesture. The same Sonos One could appear three times on this screen under
     * three names, and the thing you most wanted to do to it (group it with
     * another) was somewhere else entirely.
     *
     * So: the hero says what is playing and controls it. The grid says where
     * else there is sound, focuses the hero when tapped, and is where grouping
     * happens — you drag one room onto another, right here, on the rooms
     * themselves. Everything to *start* something lives on Browse, because
     * choosing music and choosing a room are different jobs and only one of
     * them belongs on the screen you land on.
     */
    import Icon from "../Icon.svelte";
    import NowHero from "./NowHero.svelte";
    import RoomCard from "./RoomCard.svelte";
    import NavRow from "./NavRow.svelte";
    import QuietCard from "./QuietCard.svelte";
    import type { Room, RoomsModel } from "../../lib/music/rooms.svelte";
    import type { createRoomDrag } from "../../lib/music/room-drag.svelte";
    import type { SonosBridge } from "../../lib/music/sonos.svelte";
    import type { Destination } from "../../lib/music/destination.svelte";

    let {
        rooms,
        sonos,
        destination,
        drag,
        totalSpeakers,
        readyCount,
        /** What the dock is holding, so the hero can report its own visibility. */
        dockKey = undefined,
        onDockVisible,
        onOpenPlayer,
        onBrowse,
        onOpenSpeakers,
        onNewRoom,
    }: {
        rooms: RoomsModel;
        sonos: SonosBridge;
        destination: Destination;
        drag: ReturnType<typeof createRoomDrag>;
        totalSpeakers: number;
        readyCount: number;
        dockKey?: string;
        onDockVisible: (visible: boolean) => void;
        onOpenPlayer: (r: Room) => void;
        onBrowse: () => void;
        onOpenSpeakers: () => void;
        onNewRoom: () => void;
    } = $props();

    const focused = $derived(destination.room);

    /**
     * Tap once to focus, tap the focused one again to open it. Two gestures on
     * one target, and the second is only reachable once the first has told you
     * what you are about to open — which is what makes it safe on a grid where
     * a stray tap used to launch a whole sheet.
     */
    function select(r: Room) {
        if (destination.is(r)) onOpenPlayer(r);
        else destination.focus(r);
    }

    /**
     * The dock is a fallback, never a duplicate: it appears only once the hero
     * it repeats has left the screen. The bottom inset discounts the band the
     * dock and the tab bar occupy — a hero sitting behind them counts as gone.
     */
    function heroAnchor(node: HTMLElement, on: boolean) {
        let obs: IntersectionObserver | undefined;
        let active = false;
        function attach(next: boolean) {
            obs?.disconnect();
            obs = undefined;
            if (active && !next) onDockVisible(false);
            active = next;
            if (!next) return;
            obs = new IntersectionObserver(([entry]) => onDockVisible(entry.isIntersecting), {
                threshold: 0.4,
                rootMargin: "0px 0px -96px 0px",
            });
            obs.observe(node);
        }
        attach(on);
        return {
            update: attach,
            destroy() {
                obs?.disconnect();
                if (active) onDockVisible(false);
            },
        };
    }
</script>

<div use:heroAnchor={!!focused && dockKey === focused.key}>
    <NowHero
        room={focused}
        {rooms}
        {sonos}
        pager={rooms.playing}
        onFocus={(r) => destination.focus(r)}
        onOpen={() => focused && onOpenPlayer(focused)}
        {onBrowse}
    />
</div>

<section class="block">
    <div class="block-head">
        <div class="eyrow">Rooms</div>
        <div class="rooms-actions">
            <!-- The gesture builds most of these; this is for the times you
                 want to name one first, or pick speakers that aren't beside
                 each other in the grid. -->
            <span class="rooms-hint">
                <Icon name="grip" size={12} />
                drag one onto another to group
            </span>
            <button class="chip" onclick={onNewRoom}>
                <Icon name="plus" size={13} /> New room
            </button>
        </div>
    </div>

    {#if rooms.list.length === 0}
        <QuietCard title="No rooms answering">
            <span class="mono">{totalSpeakers}</span>
            speaker{totalSpeakers === 1 ? "" : "s"} registered, none reachable right now — check
            their addresses under Speakers.
        </QuietCard>
    {:else}
        <div class="room-grid">
            {#each rooms.list as r (r.key)}
                <RoomCard
                    room={r}
                    {rooms}
                    focused={destination.is(r)}
                    lifted={drag.drag?.key === r.key}
                    held={drag.grabKey === r.key}
                    dropping={drag.dropKey === r.key}
                    aiming={drag.aiming(r)}
                    grabbedName={drag.grabbedName}
                    onSelect={() => select(r)}
                    onToggle={() => rooms.togglePlay(r)}
                    onPointerDown={(e) => drag.onPointerDown(e, r)}
                    onPointerMove={drag.onPointerMove}
                    onPointerUp={drag.onPointerUp}
                    onPointerCancel={drag.end}
                    onClickCapture={drag.onClickCapture}
                    onKeyDown={(e) => drag.onKeyDown(e, r)}
                />
            {/each}
        </div>
        <p class="hint keys-note">
            On a keyboard: press <kbd>G</kbd> on a room to pick it up, <kbd>Tab</kbd> to another
            and <kbd>Enter</kbd> to play them together.
        </p>
    {/if}
</section>

<!-- Speakers the topology never mentioned can't be a room, so Home only points
     at them; fixing an address is device work, and device work is one screen
     over. -->
{#if rooms.offline.length > 0}
    <NavRow icon="speaker" title={offlineTitle} onClick={onOpenSpeakers}>
        {#snippet sub()}
            Not answering — fix an address, or set one up
        {/snippet}
    </NavRow>
{/if}

<NavRow icon="sliders" title="Speakers" count={totalSpeakers} onClick={onOpenSpeakers}>
    {#snippet sub()}
        <span class="mono">{readyCount}</span>
        ready · names, addresses, tone and the status light
    {/snippet}
</NavRow>

{#snippet offlineTitle()}
    <span class="mono">{rooms.offline.length}</span>
    unreachable
{/snippet}

<style>
    .room-grid {
        display: grid;
        /* Wide enough for a track title beside the play button; narrower
           columns crushed it to an ellipsis on desktop. */
        grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
        gap: var(--space-3);
    }
    .rooms-actions { display: flex; align-items: center; gap: var(--space-3); }
    .rooms-hint {
        display: flex; align-items: center; gap: 5px;
        font-size: 11.5px; color: var(--text-dim);
    }
    /* On a phone the press-and-hold is the discovery, and the line steals a
       whole row from the grid. */
    @media (max-width: 520px) {
        .rooms-hint { display: none; }
    }
    .keys-note { display: none; }
    @media (hover: hover) and (pointer: fine) {
        .keys-note { display: block; }
    }
    kbd {
        font-family: var(--font-mono); font-size: 10.5px;
        padding: 1px 5px; border-radius: 4px;
        background: var(--card-2); border: 1px solid var(--hairline);
        color: var(--text-mute);
    }
</style>
