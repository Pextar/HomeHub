<script lang="ts">
    /**
     * The browse depth's Rooms pane (DESIGN.md §16): which room the wall is
     * driving, and the tap-based Sonos grouping around it.
     *
     * It leads with the featured room's own preferences, its timers and its
     * memory — this is the "which device" surface, and those three are the
     * room's rather than the catalog's — then the list of every room the
     * panel can feature, and closes on the household's listening picture.
     *
     * Split out of PanelBrowse because it is the one pane that shares
     * nothing with the others: no layout mode reaches into it, and the
     * arming state below is its alone.
     */
    import Icon from "../Icon.svelte";
    import Waveform from "../music/Waveform.svelte";
    import PanelRoomSettings from "./PanelRoomSettings.svelte";
    import PanelTimers from "./PanelTimers.svelte";
    import PanelRoomMemory from "./PanelRoomMemory.svelte";
    import PanelInsights from "./PanelInsights.svelte";
    import { kefSourceLabel } from "../../lib/kef";
    import type { PanelMusicStore, PanelSource } from "../../lib/panel-music.svelte";

    let {
        music,
        featured,
        onArtist,
    }: {
        music: PanelMusicStore;
        featured: PanelSource | undefined;
        /** An artist named in the insights picture, opened on the search
         *  pane — which this pane has no way back to on its own. */
        onArtist: (name: string) => void;
    } = $props();

    // Split is the one destructive gesture here and there is no confirm
    // modal on a kiosk, so it arms for a few seconds instead.
    let splitArmed = $state(false);
    let splitTimer: ReturnType<typeof setTimeout> | undefined;
    function splitClick() {
        if (splitArmed) {
            splitArmed = false;
            clearTimeout(splitTimer);
            music.ungroupFeatured();
            return;
        }
        splitArmed = true;
        clearTimeout(splitTimer);
        splitTimer = setTimeout(() => (splitArmed = false), 3000);
    }

    function roomSub(s: PanelSource): string {
        if (s.kind === "kef") return ["KEF", kefSourceLabel(s.input)].filter(Boolean).join(" · ");
        if (s.kind === "zone") {
            // A zone says what it is made of and how it is being driven —
            // "buffered" is a real difference from a native group, and the
            // backend already worded it, so the wall repeats it rather than
            // inferring one (§15).
            const n = s.members?.length ?? 0;
            const how =
                s.route === "stream"
                    ? "streamed together"
                    : s.route === "airplay"
                      ? "sent together over AirPlay"
                      : "played together";
            return [n > 1 ? `${n} speakers` : "HomeHub room", how].filter(Boolean).join(" · ");
        }
        return s.members && s.members.length > 1 ? `${s.members.length} speakers` : "Sonos";
    }
</script>

<div class="b-pane">
    <!-- The featured room's own preferences lead the pane:
         this is the "which device" surface, and they used to
         be stacked under the cover in the player column
         where they cost it two thirds of its height. -->
    <PanelRoomSettings {music} />
    <!-- And what the room is going to do on its own: the
         sleep timer someone sets on the way to bed, and the
         wake-up that is the same mechanism run the other
         way. Both are the room's, like the settings above
         them, and both reach every kind of room the panel
         can feature (§16). -->
    <PanelTimers {music} />
    <!-- And what the room remembers, which is a room's
         preference like the two above it: the ranked shelves
         put what this room keeps coming back to at the front
         of the wall, and this is the only place that ranking
         can be corrected. A row rather than an × on a cover
         — a 132px tile has no room for a second target that
         clears the 44px floor without swallowing the tap the
         shelf exists for (§16). -->
    <PanelRoomMemory {music} />
    <h3 class="s-label">Rooms</h3>
    <div class="rm-list">
        {#each music.sources as s (s.key)}
            {@const isFeatured = featured?.key === s.key}
            <!-- Chosen is an edge, not a glow: the ON
                 gradient means "playing" everywhere else in
                 the app (§6.1), and a silent room that
                 merely has the focus must not wear it. -->
            <div class="rm-row" class:active={isFeatured} class:live={s.playing}>
                <button
                    class="rm-main"
                    aria-label="Feature {s.title}"
                    aria-pressed={isFeatured}
                    onclick={() => (music.selected = s.key)}
                >
                    <span class="rm-meta">
                        <span class="rm-name">{s.title}</span>
                        <span class="rm-sub">{roomSub(s)}</span>
                    </span>
                    {#if s.playing}
                        <span class="rm-wave"><Waveform /></span>
                    {/if}
                </button>
                {#if s.kind === "sonos" && featured?.kind === "sonos" && !isFeatured}
                    <!-- The pair the wall gets asked for, in
                         the order they are wanted: play it
                         here as well, or take it here and
                         leave. Move is the quieter of the
                         two — it is the rarer ask, and it
                         stops sound in the room you are
                         standing in. -->
                    <button
                        class="k-chip rm-join"
                        disabled={!!music.busy["join:" + s.id] ||
                            !!music.busy["move:" + s.id]}
                        onclick={() => music.joinSource(s)}
                    >
                        Join {featured.title}
                    </button>
                    <button
                        class="k-chip rm-move"
                        aria-label="Move the music to {s.title}"
                        disabled={!!music.busy["join:" + s.id] ||
                            !!music.busy["move:" + s.id]}
                        onclick={() => music.moveTo(s)}
                    >
                        Move
                    </button>
                {:else if isFeatured && s.kind === "sonos" && (s.members?.length ?? 0) > 1}
                    <button
                        class="k-chip"
                        class:on={splitArmed}
                        disabled={!!music.busy["ungroup:" + s.id]}
                        onclick={splitClick}
                    >
                        {splitArmed ? "Split?" : "Split"}
                    </button>
                {/if}
            </div>
            {#if isFeatured && s.kind === "sonos" && (s.members?.length ?? 0) > 1}
                <div class="rm-members">
                    {#each s.members ?? [] as m (m.id)}
                        <span class="rm-mchip">
                            {m.name}{#if m.coordinator}
                                <span class="rm-lead mono">lead</span>
                            {:else}
                                <button
                                    class="rm-x"
                                    aria-label="Remove {m.name} from the group"
                                    disabled={!!music.busy["leave:" + m.id]}
                                    onclick={() => music.leaveMember(m.id)}
                                >
                                    <Icon name="close" size={12} />
                                </button>
                            {/if}
                        </span>
                    {/each}
                </div>
            {/if}
        {/each}
    </div>
    <p class="rm-note">
        Sonos rooms group natively — join, move or split them here. A HomeHub room
        plays any mix of speakers together and can be started from this panel, but
        arranging one is done in the Music view.
    </p>
    <!-- The one picture no single room can give: which rooms
         do the listening, what the house keeps coming back
         to, and when in the day it is loud. It sits at the
         foot of the pane that is about rooms, because that
         is what it is about — a kiosk gets no screen for
         something nobody taps. -->
    <PanelInsights {music} onArtist={(name) => onArtist(name)} />
</div>

<style>
    /* The pane's own scroll, and the shelf label above the list — both the
       same rules the depth's other panes use, kept here rather than made
       global because this is all either of them is: two shapes, not a
       vocabulary worth spreading across app.css. */
    .b-pane {
        flex: 1;
        min-height: 0;
        overflow-y: auto;
        /* Without this, `overflow-y: auto` computes the x axis to `auto`
           too and a row that measures 432.03px in a 432px pane pans
           sideways by the pixel the rounding invents (see `.fp-queue`). */
        overflow-x: hidden;
        padding-bottom: var(--space-2);
    }
    .s-label {
        margin: var(--space-4) 0 var(--space-2);
        font-family: var(--font-mono);
        font-size: 11px;
        font-weight: 500;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-dim);
    }

    .rm-list {
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
    }
    .rm-row {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        padding: var(--space-2);
        border: 1px solid var(--hairline);
        border-radius: var(--r-md);
        background: var(--card);
        transition:
            border-color var(--t-fast),
            background var(--t-fast);
    }
    /* Playing and chosen are independent states, so they are drawn in two
       different registers: audio gets the §6.1 ON gradient, the chosen
       room gets a ring. A silent room that merely holds the focus used to
       wear "on", and a playing room that wasn't chosen looked identical to
       the one that was. */
    .rm-row.live {
        border-color: var(--tile-on-border);
        background: var(--tile-on-gradient);
    }
    .rm-row.active {
        border-color: var(--border-strong);
        box-shadow: inset 0 0 0 2px var(--border-strong);
    }
    .rm-main {
        flex: 1;
        min-width: 0;
        display: flex;
        align-items: center;
        gap: var(--space-3);
        min-height: 56px;
        padding: var(--space-2);
        border: 0;
        background: none;
        color: inherit;
        font: inherit;
        text-align: left;
        cursor: pointer;
        border-radius: var(--r-sm);
    }
    .rm-meta {
        flex: 1;
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 2px;
    }
    .rm-name {
        font-size: 16px;
        font-weight: 500;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .rm-sub {
        font-size: 12.5px;
        color: var(--text-mute);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .rm-wave {
        display: inline-flex;
        flex-shrink: 0;
        padding: 6px 8px;
        border-radius: var(--r-sm);
        background: var(--bg-bar);
    }
    .rm-join {
        flex-shrink: 0;
        max-width: 40%;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    /* The quieter half of the pair: same chip, no border, so the two read
       as one control with a primary and a secondary rather than as two
       equal offers. */
    .rm-move {
        flex-shrink: 0;
        border-color: transparent;
        color: var(--text-dim);
    }
    .rm-move:active {
        color: var(--on);
    }
    .rm-members {
        display: flex;
        flex-wrap: wrap;
        gap: var(--space-2);
        padding: var(--space-1) var(--space-2) var(--space-2);
    }
    .rm-mchip {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        padding: 7px 12px;
        border-radius: var(--r-pill);
        border: 1px solid var(--hairline);
        background: var(--card-2);
        color: var(--text-mute);
        font-size: 12.5px;
        font-weight: 500;
    }
    .rm-lead {
        font-size: 10px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-dim);
    }
    .rm-x {
        position: relative;
        width: 24px;
        height: 24px;
        display: grid;
        place-items: center;
        border: 0;
        border-radius: 50%;
        background: var(--card-3);
        color: var(--text-mute);
        cursor: pointer;
    }
    .rm-x:disabled {
        opacity: 0.4;
    }
    .rm-note {
        margin: var(--space-4) 0 0;
        font-size: 12.5px;
        line-height: 1.5;
        color: var(--text-dim);
    }

    @media (pointer: coarse) {
        .rm-x::after {
            content: "";
            position: absolute;
            inset: -10px;
        }
    }
</style>
