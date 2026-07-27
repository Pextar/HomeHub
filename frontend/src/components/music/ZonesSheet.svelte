<script lang="ts">
    /**
     * Zones: what plays together, in the two shapes that actually exist.
     *
     * "Zones", not "Rooms": the app-level nav already owns that word for the
     * whole house, and reusing it here for speaker grouping was the confusing
     * part (DESIGN.md §15). It opens over Home the same way the player does,
     * and swaps to a player or the zone editor when one is tapped rather than
     * stacking a second sheet on top of itself.
     *
     * Two sections, and the split is the honest one:
     *
     *   **Zones** are HomeHub's own — a named set of speakers of any mix of
     *   makes, built by ticking members, played by whatever route can serve
     *   them all. This is the noun the media protocol added, and it is the only
     *   way a KEF and a Sonos ever play together.
     *
     *   **Sonos grouping** is the puck grid, and it stays exactly what it was:
     *   a gesture that drives Sonos' own grouping, which a KEF speaker cannot
     *   join. Dragging one puck onto another must never look like it could
     *   build a cross-vendor zone, so the two are not conflated in one gesture
     *   and not merged into one grid.
     */
    import Icon from "../Icon.svelte";
    import MusicSheet from "./MusicSheet.svelte";
    import RoomPuck from "./RoomPuck.svelte";
    import QuietCard from "./QuietCard.svelte";
    import NavRow from "./NavRow.svelte";
    import ZoneCard from "./ZoneCard.svelte";
    import CardTransport from "./CardTransport.svelte";
    import type { SonosBridge } from "../../lib/music/sonos.svelte";
    import type { KEFBridge } from "../../lib/music/kef.svelte";
    import type { ZonesBridge } from "../../lib/music/zones.svelte";
    import type { Busy } from "../../lib/music/busy.svelte";
    import type { createPuckDrag } from "../../lib/music/puck-drag.svelte";
    import type { MediaZone, SonosGroupView } from "../../lib/types";

    let {
        sonos,
        kef,
        zones,
        busy,
        drag,
        docked = false,
        onDismiss,
        onOpenRoom,
        onOpenZone,
        onNewZone,
        onEditZone,
        onOpenSpeakers,
        scrollEl = $bindable<HTMLElement | null>(null),
    }: {
        sonos: SonosBridge;
        kef: KEFBridge;
        zones: ZonesBridge;
        busy: Busy;
        drag: ReturnType<typeof createPuckDrag>;
        docked?: boolean;
        onDismiss: () => void;
        onOpenRoom: (g: SonosGroupView) => void;
        onOpenZone: (z: MediaZone) => void;
        onNewZone: () => void;
        onEditZone: (z: MediaZone) => void;
        onOpenSpeakers: () => void;
        scrollEl?: HTMLElement | null;
    } = $props();

    /** A zone worth offering a transport: something is on it to resume, or
     *  actively playing with no track metadata to name (radio, line-in) —
     *  `nowLine` calls that case "Live audio", and it still needs a pause. */
    function hasTrack(z: MediaZone): boolean {
        return !!zones.leadOf(z)?.state?.track?.title || zones.isPlaying(z);
    }

    const nameOf = (id: string) => sonos.speakerById.get(id)?.name;
    const grabbedName = $derived(drag.grabbedName(nameOf));
</script>

<MusicSheet
    label="Zones"
    title="Zones"
    sub="Speakers that play together, of any make"
    backLabel="Close Zones"
    onBack={onDismiss}
    {onDismiss}
    {docked}
    bind:scrollEl
>
    <!-- ── HomeHub zones ──────────────────────────────────────────────
         The cross-vendor half. A zone is built by membership, so the way in is
         a button, not a gesture — and the way to change one is the card's own
         Edit, which swaps this sheet for the editor and gets it back after. -->
    <div class="zone-list">
        <div class="zl-head">
            <div class="eyrow">Zones</div>
            <button class="chip" onclick={onNewZone}>
                <Icon name="plus" size={13} /> New zone
            </button>
        </div>

        <!-- Nothing at all until the first read answers: "No zones yet" is a
             claim, and claiming it before we know is how a sheet opened on a
             slow network tells the user their zones are gone. -->
        {#if zones.zones.length === 0}
            {#if zones.loaded}
                <QuietCard title="No zones yet">
                    A zone is a set of speakers that play together — a Sonos and a KEF in one, if
                    you like. HomeHub streams to whichever mix can't group natively.
                </QuietCard>
            {/if}
        {:else}
            {#each zones.zones as z (z.id)}
                <ZoneCard
                    zone={z}
                    {zones}
                    onOpen={() => onOpenZone(z)}
                    onEdit={() => onEditZone(z)}
                    transport={hasTrack(z) ? zoneTransport : undefined}
                />
            {/each}
        {/if}
    </div>

    <!-- ── Sonos grouping ─────────────────────────────────────────────
         Unchanged, and deliberately separate: this drives Sonos' own grouping,
         which a KEF speaker cannot join. -->
    <div class="rooms">
        <div class="eyrow">Sonos grouping</div>
        <p class="hint">Tap a room to open it · drag one onto another to group</p>
        {#each sonos.multiGroups as g (g.coordinator_id)}
            <!-- The enclosure is a drop target in its own right: "drag a third
                 onto an existing group" reads as dropping on the group, so the
                 gap between its pucks must not be a miss. -->
            <div
                class="group-wrap"
                class:drop={drag.dropZone === g.coordinator_id}
                data-zone={g.coordinator_id}
            >
                <div class="glabel">
                    <Icon name="check" size={11} />
                    <span>{sonos.groupTitle(g)}</span>
                    <button
                        class="ungroup"
                        disabled={busy.is("ungroup:" + g.coordinator_id)}
                        onclick={() => sonos.ungroup(g)}>Ungroup</button
                    >
                </div>
                <div class="puck-grid">
                    {#each g.member_ids as id (id)}
                        {@const sp = sonos.speakerById.get(id)}
                        {#if sp}
                            {@render puck(sp)}
                        {/if}
                    {/each}
                </div>
            </div>
        {/each}
        {#if sonos.soloSpeakers.length}
            <div class="puck-grid">
                {#each sonos.soloSpeakers as sp (sp.id)}
                    {@render puck(sp)}
                {/each}
            </div>
        {/if}

        <!-- Grouping is a Sonos capability. A house with only KEF speakers
             would otherwise open a blank sheet with no explanation, which
             reads as broken rather than as "this doesn't apply to your
             speakers". -->
        {#if sonos.multiGroups.length === 0 && sonos.soloSpeakers.length === 0}
            <QuietCard title="Nothing to group" action={{ label: "Speakers", onClick: onOpenSpeakers }}>
                {#if kef.speakers.length > 0}
                    KEF speakers stand alone — they have no zones to group. Their controls are
                    on Speakers.
                {:else}
                    No Sonos speaker is answering right now — check them under Speakers.
                {/if}
            </QuietCard>
        {/if}

        <!-- Speakers the live topology never mentioned can't be pucks and
             can't be grouped, so Zones only points at them. -->
        {#if sonos.offline.length > 0}
            <NavRow icon="speaker" onClick={onOpenSpeakers} title={offlineTitle}>
                {#snippet sub()}
                    Not in the current Sonos topology — check them under Speakers
                {/snippet}
            </NavRow>
        {/if}

        <p class="hint zones-keys">
            On a keyboard: press <kbd>G</kbd> on a room to pick it up,
            <kbd>Tab</kbd> to another and <kbd>Enter</kbd> to group them.
        </p>
    </div>
</MusicSheet>

{#snippet offlineTitle()}
    <span class="mono">{sonos.offline.length}</span>
    speaker{sonos.offline.length === 1 ? "" : "s"} unreachable
{/snippet}

<!-- One transport for every zone card: play/pause only. Skips belong to the
     zone's player, the same division the Sonos and KEF cards keep. -->
{#snippet zoneTransport(z: MediaZone)}
    <CardTransport
        playing={zones.isPlaying(z)}
        onToggle={() => zones.togglePlay(z)}
        toggleBusy={busy.is("zplay:" + z.id)}
    />
{/snippet}

{#snippet puck(sp: import("../../lib/types").SonosSpeakerView)}
    {@const g = sonos.groupOfSpeaker(sp.id)}
    <RoomPuck
        speaker={sp}
        playing={sonos.speakerPlaying(sp.id)}
        sub={sonos.speakerNowLine(sp.id)}
        openable={!!g}
        lifted={drag.drag?.id === sp.id}
        held={drag.grabId === sp.id}
        dropping={drag.dropId === sp.id}
        aiming={drag.grabId !== null &&
            drag.grabId !== sp.id &&
            sonos.groupOfSpeaker(drag.grabId)?.coordinator_id !== g?.coordinator_id}
        {grabbedName}
        onOpen={() => g && onOpenRoom(g)}
        onPointerDown={(e) => drag.onPointerDown(e, sp)}
        onPointerMove={(e) => drag.onPointerMove(e)}
        onPointerUp={() => drag.onPointerUp()}
        onPointerCancel={() => drag.end()}
        onClickCapture={(e) => drag.onClickCapture(e)}
        onKeyDown={(e) => drag.onKeyDown(e, sp, nameOf)}
    />
{/snippet}

<style>
    .zone-list { display: flex; flex-direction: column; gap: var(--space-3); }
    .zl-head {
        display: flex; align-items: center; justify-content: space-between;
        gap: var(--space-3);
    }
    .zl-head .chip { flex-shrink: 0; }

    .rooms { display: flex; flex-direction: column; gap: var(--space-3); }
    /* The grouping section's own caption, under its section head. */
    .rooms > .hint { margin-top: calc(var(--space-2) * -1); }
    .puck-grid {
        display: grid;
        /* 140, not 160: inside the dashed group enclosure the extra padding
           tipped a phone's two-up grid into a single stacked column. */
        grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
        gap: var(--space-3);
    }
    .group-wrap {
        border: 1px dashed var(--tile-on-border);
        border-radius: var(--r-lg);
        padding: var(--space-2);
        display: flex; flex-direction: column; gap: var(--space-2);
        transition: border-color var(--t-fast), box-shadow var(--t-fast);
    }
    /* Aimed at as a whole — the dashed edge goes solid amber, the same
       statement a puck's drop ring makes. */
    .group-wrap.drop {
        border-style: solid;
        border-color: var(--on);
        box-shadow: 0 0 0 1px var(--on), 0 0 22px -6px var(--on-glow);
    }
    .glabel {
        display: flex; align-items: center; gap: 6px;
        padding: 2px 6px;
        font-family: var(--font-mono);
        font-size: 10px; letter-spacing: 0.08em; text-transform: uppercase;
        color: var(--on);
    }
    .glabel span { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .ungroup {
        background: none; border: 0; padding: 2px 4px;
        color: var(--text-mute); font-family: var(--font-sans);
        font-size: 11px; letter-spacing: 0; text-transform: none;
        cursor: pointer;
    }
    .ungroup:hover { color: var(--text); }
    .ungroup:disabled { opacity: 0.5; }
    @media (pointer: coarse) {
        .ungroup { min-width: 44px; min-height: 44px; }
    }

    /* Said once, at the foot of the sheet — the gesture is the affordance,
       this is the footnote for the keyboard. */
    .zones-keys { margin-top: var(--space-2); }
    .zones-keys kbd {
        font-family: var(--font-mono);
        font-size: 11px;
        padding: 1px 5px;
        border-radius: 5px;
        background: var(--card-3);
        border: 1px solid var(--hairline);
        color: var(--text);
    }
</style>
