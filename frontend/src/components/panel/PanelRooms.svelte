<script lang="ts">
    import Icon from "../Icon.svelte";
    import Waveform from "../music/Waveform.svelte";
    import { api } from "../../lib/api";
    import { data } from "../../lib/stores.svelte";
    import { runAction, roomIcon, haptic } from "../../lib/utils";

    // The panel's room tiles: tap toggles the room's lights. No detail
    // screens, no overflow menus — a wall panel does the one domestic
    // gesture and nothing else.
    //
    // `band` is the shape it takes when the panel also has music: a single
    // row along the bottom rather than a grid filling a column. The tiles
    // are read far more often than they are pressed — the status of the
    // house at a glance — so on a surface that also carries a player they
    // give it the height and keep the width (DESIGN.md §16).
    //
    // `playingRooms` is the rooms making noise, lowercased. A room that is
    // playing shows the waveform where its switch would be: on a wall the
    // one thing you want to find without reading is which room the sound
    // is coming from, and §6.8's motif says it without a word.
    let { band = false, playingRooms = [] }: { band?: boolean; playingRooms?: string[] } = $props();

    const v = $derived(data.value);

    const isPlaying = (name: string) => playingRooms.includes(name.toLowerCase());

    // Live on-counts derived from socket state (same derivation as the
    // dashboard) so tiles answer the tap immediately instead of waiting
    // for the next server refresh.
    const rooms = $derived.by(() => {
        // eslint-disable-next-line svelte/prefer-svelte-reactivity -- transient local Map, built and consumed synchronously
        const onByRoom = new Map<string, number>();
        for (const s of v.sockets) {
            if (!s.room) continue;
            onByRoom.set(s.room, (onByRoom.get(s.room) ?? 0) + (s.state ? 1 : 0));
        }
        return v.rooms
            .filter((r) => r.sockets > 0)
            .map((r) => ({ ...r, on: onByRoom.get(r.name) ?? 0 }));
    });

    // A home without rooms gets its devices instead — an empty centre is
    // never acceptable on a control surface.
    const fallbackDevices = $derived(rooms.length === 0 ? v.sockets : []);

    let busy = $state<Record<string, boolean>>({});

    function toggleRoom(name: string, on: boolean) {
        if (busy[name]) return;
        busy[name] = true;
        haptic();
        void runAction(() => (on ? api.roomOn(name) : api.roomOff(name))).finally(() => {
            busy[name] = false;
        });
    }

    function toggleDevice(id: string, on: boolean) {
        if (busy[id]) return;
        busy[id] = true;
        haptic();
        void runAction(() => (on ? api.socketOn(id) : api.socketOff(id))).finally(() => {
            busy[id] = false;
        });
    }
</script>

<section class="rooms" class:band aria-label="Room lights">
    {#if !band}
        <header class="rooms-head">
            <h2>Rooms</h2>
        </header>
    {/if}

    {#if rooms.length > 0}
        <div class="rooms-grid" class:band>
            {#each rooms as r (r.id)}
                {@const anyOn = r.on > 0}
                {@const playing = isPlaying(r.name)}
                <button
                    class="rtile"
                    class:on={anyOn}
                    disabled={busy[r.name]}
                    aria-pressed={anyOn}
                    onclick={() => toggleRoom(r.name, !anyOn)}
                >
                    <span class="r-top">
                        <span class="r-ico" class:on={anyOn}>
                            <Icon name={roomIcon(r.name)} size={band ? 16 : 28} />
                        </span>
                        {#if band}
                            <!-- The waveform takes the switch's place for the
                                 room that is playing: the tile still toggles
                                 the lights, but what it *says* is that the
                                 sound is here (§6.8). -->
                            {#if playing}
                                <Waveform />
                            {:else}
                                <span class="sw" class:on={anyOn} aria-hidden="true"></span>
                            {/if}
                        {:else}
                            <span class="r-count mono" class:lit={anyOn}
                                >{r.on}<span class="dim"> / {r.sockets}</span></span
                            >
                        {/if}
                    </span>
                    <span class="r-foot">
                        <span class="r-name" title={r.name}>{r.name}</span>
                        {#if band}
                            <!-- What the room is doing, in words. The lights
                                 are what the tile toggles, so they are what
                                 it reports; playing leads when both are
                                 true, because the waveform above has just
                                 said so. -->
                            <span class="r-sub">
                                {#if playing && anyOn}
                                    Playing · <span class="mono">{r.on}</span> on
                                {:else if playing}
                                    Playing
                                {:else if anyOn}
                                    <span class="mono">{r.on}</span>
                                    light{r.on === 1 ? "" : "s"} on
                                {:else}
                                    Off
                                {/if}
                            </span>
                        {/if}
                    </span>
                </button>
            {/each}
        </div>
    {:else if fallbackDevices.length > 0}
        <div class="rooms-grid" class:band>
            {#each fallbackDevices as s (s.id)}
                <button
                    class="rtile"
                    class:on={s.state}
                    disabled={busy[s.id]}
                    aria-pressed={s.state}
                    onclick={() => toggleDevice(s.id, !s.state)}
                >
                    <span class="r-top">
                        <span class="r-ico" class:on={s.state}>
                            <Icon name="socket" size={band ? 16 : 28} />
                        </span>
                        {#if band}
                            <span class="sw" class:on={s.state} aria-hidden="true"></span>
                        {:else}
                            <span class="r-count">{s.room || "Unassigned"}</span>
                        {/if}
                    </span>
                    <span class="r-foot">
                        <span class="r-name" title={s.name}>{s.name}</span>
                        {#if band}
                            <span class="r-sub">{s.room || "Unassigned"}</span>
                        {/if}
                    </span>
                </button>
            {/each}
        </div>
    {:else}
        <div class="rooms-empty">
            <Icon name="light" size={28} />
            <p>No devices yet.</p>
        </div>
    {/if}
</section>

<style>
    .rooms {
        display: flex;
        flex-direction: column;
        min-height: 0;
        min-width: 0;
        padding: var(--space-5) var(--space-8);
    }
    /* The foot band: a fixed height, and a hairline that separates it from
       the music band rather than a gap. 156px is what one tile needs to
       state an icon, a switch, a name and a line about what the room is
       doing — every row above it is the music band's (§16). */
    .rooms.band {
        height: 156px;
        flex-shrink: 0;
        padding: 18px var(--space-8);
        border-top: 1px solid var(--hairline);
    }
    .rooms-head {
        display: flex;
        align-items: center;
        justify-content: space-between;
        margin-bottom: var(--space-3);
        flex-shrink: 0;
    }
    h2 {
        margin: 0;
        font-size: 17px;
        font-weight: 600;
        letter-spacing: -0.02em;
    }

    /* Tiles share the height equally and fill the zone; with more rooms
       than fit, the grid scrolls internally — the page never does. */
    .rooms-grid {
        flex: 1;
        min-height: 0;
        display: grid;
        grid-template-columns: repeat(2, minmax(0, 1fr));
        /* Rows share the height equally down to a legible floor; past that
           the grid scrolls internally rather than shrinking tiles to
           nothing — the page itself never scrolls. */
        grid-auto-rows: minmax(120px, 1fr);
        gap: var(--space-3);
        overflow-y: auto;
        scrollbar-width: none;
    }
    /* One row across the foot of the panel, filling the band's height.
       Tiles share the width equally down to a legible floor and the row
       scrolls sideways past that — the same bargain the grid makes
       vertically. A flex row rather than a grid because equal *shares* of
       whatever width is left is the whole rule here, and six rooms or four
       should each read the same. */
    .rooms-grid.band {
        flex: 1;
        display: flex;
        gap: 14px;
        grid-template-columns: none;
        grid-auto-rows: auto;
        overflow-x: auto;
        overflow-y: hidden;
    }
    .rooms-grid.band .rtile {
        flex: 1 1 0;
        min-width: 132px;
    }
    .rooms-grid::-webkit-scrollbar {
        display: none;
    }

    .rtile {
        display: flex;
        flex-direction: column;
        justify-content: space-between;
        gap: var(--space-3);
        min-height: 0;
        padding: var(--space-4);
        border-radius: var(--r-lg);
        border: 1px solid var(--hairline);
        background: var(--card);
        color: var(--text);
        font-family: inherit;
        text-align: left;
        cursor: pointer;
        transition:
            background var(--t-med),
            border-color var(--t-med),
            transform var(--t-fast);
    }
    .rtile:active {
        transform: scale(0.98);
        transition-duration: 80ms;
    }
    .rtile:disabled {
        opacity: 0.7;
    }
    .rtile.on {
        background: var(--tile-on-gradient);
        border-color: var(--tile-on-border);
    }

    .r-ico {
        width: 56px;
        height: 56px;
        flex-shrink: 0;
        border-radius: var(--r-md);
        display: grid;
        place-items: center;
        background: var(--surface);
        color: var(--text-dim);
        transition:
            color var(--t-med),
            background var(--t-med),
            box-shadow var(--t-med);
    }
    .r-ico.on {
        background: var(--on-soft);
        color: var(--on);
        /* The bulb glow, kept on the small badge — cheap for the old GPU. */
        box-shadow: 0 0 24px 2px var(--on-glow);
    }
    :global([data-theme="light"]) .r-ico {
        background: rgba(0, 0, 0, 0.06);
    }

    .r-top {
        display: flex;
        align-items: flex-start;
        justify-content: space-between;
        gap: var(--space-2);
        min-width: 0;
    }
    .r-name {
        font-size: 19px;
        font-weight: 600;
        letter-spacing: -0.02em;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    /* The count as a corner pill (RoomCard's on-badge pattern) so the name
       gets the tile's full width — truncation to four characters is not a
       room name. Amber-soft when anything's lit, quiet when not. */
    .r-count {
        font-size: 12.5px;
        flex-shrink: 0;
        color: var(--text-mute);
        background: var(--surface);
        padding: 3px 8px;
        border-radius: var(--r-pill);
    }
    .r-count.lit {
        color: var(--on);
        background: var(--on-soft);
    }
    .r-count .dim {
        color: var(--text-dim);
    }

    .rooms-empty {
        flex: 1;
        display: grid;
        place-items: center;
        align-content: center;
        gap: var(--space-3);
        color: var(--text-dim);
        border: 1px dashed var(--border);
        border-radius: var(--r-lg);
    }
    .rooms-empty p {
        margin: 0;
        font-size: 14px;
    }

    /* The band is short and its tiles are narrow, so they trade the grid's
       generous badge and count for a small icon chip, a switch and two
       lines of words. Laying them out as a row instead was tried and was
       worse: the icon and the count took the width from the middle and
       every room read as "Li…". The name is what the tile is for. */
    .rooms.band .rtile {
        padding: 14px;
        gap: var(--space-2);
        border-radius: var(--r-lg);
    }
    .rooms.band .r-ico {
        width: 30px;
        height: 30px;
        border-radius: var(--r-sm);
        background: var(--card-3);
    }
    /* Filled rather than tinted at this size: 30px of amber-soft behind a
       16px glyph reads as nothing from across a room. The glow stays on
       the badge and nowhere else (§16). */
    .rooms.band .r-ico.on {
        background: var(--on);
        color: var(--primary-fg);
        box-shadow: 0 0 16px 2px var(--on-glow);
    }
    .rooms.band .r-name {
        font-size: 14px;
    }
    .r-foot {
        display: flex;
        flex-direction: column;
        gap: 2px;
        min-width: 0;
    }
    .r-sub {
        font-size: 11.5px;
        color: var(--text-mute);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    /* The switch, drawn and not built: the tile is the button, so a real
       Switch inside it would be a control inside a control. This is the
       §6.2 shape as a read-only indicator. */
    .sw {
        width: 38px;
        height: 22px;
        border-radius: var(--r-pill);
        background: var(--card-3);
        position: relative;
        flex-shrink: 0;
        transition: background var(--t-med);
    }
    .sw::after {
        content: "";
        position: absolute;
        top: 2px;
        left: 2px;
        width: 18px;
        height: 18px;
        border-radius: 50%;
        background: var(--knob-off);
        box-shadow: 0 1px 3px rgba(0, 0, 0, 0.25);
        transition:
            transform var(--t-med),
            background var(--t-med);
    }
    .sw.on {
        background: var(--on);
    }
    .sw.on::after {
        transform: translateX(16px);
        background: var(--knob);
    }
    @media (prefers-reduced-motion: reduce) {
        .sw,
        .sw::after {
            transition-duration: 0.001ms;
        }
    }

    /* Portrait / narrow: the band idea does not survive one column. */
    @media (orientation: portrait), (max-width: 900px) {
        .rooms.band {
            height: auto;
            padding: var(--space-5);
        }
        .rooms {
            padding: var(--space-5);
        }
        /* Stacked, the row is a grid again and the page owns the scroll, so
           the band's fixed height and internal overflow both stand down —
           a row that keeps them here just clips its own tiles. */
        .rooms-grid.band {
            display: grid;
            flex: none;
            grid-template-columns: repeat(2, minmax(0, 1fr));
            grid-auto-flow: row;
            grid-auto-rows: minmax(120px, auto);
            gap: var(--space-3);
            overflow: visible;
        }
        .rooms-grid.band .rtile {
            min-width: 0;
        }
    }
</style>
