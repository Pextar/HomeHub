<script lang="ts">
    import Icon from "../Icon.svelte";
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
    let { band = false }: { band?: boolean } = $props();

    const v = $derived(data.value);

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
    <header class="rooms-head">
        <h2>Rooms</h2>
    </header>

    {#if rooms.length > 0}
        <div class="rooms-grid" class:band>
            {#each rooms as r (r.id)}
                {@const anyOn = r.on > 0}
                <button
                    class="rtile"
                    class:on={anyOn}
                    disabled={busy[r.name]}
                    aria-pressed={anyOn}
                    onclick={() => toggleRoom(r.name, !anyOn)}
                >
                    <span class="r-top">
                        <span class="r-ico" class:on={anyOn}>
                            <Icon name={roomIcon(r.name)} size={28} />
                        </span>
                        <span class="r-count mono" class:lit={anyOn}
                            >{r.on}<span class="dim"> / {r.sockets}</span></span
                        >
                    </span>
                    <span class="r-name" title={r.name}>{r.name}</span>
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
                            <Icon name="socket" size={28} />
                        </span>
                        <span class="r-count">{s.room || "Unassigned"}</span>
                    </span>
                    <span class="r-name" title={s.name}>{s.name}</span>
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
    /* Band: one row across the foot of the panel. Tiles share the width
       down to a legible floor and the row scrolls sideways past that —
       the same bargain the grid makes vertically. */
    /* One row across the foot of the panel, at the height a tile needs and
       not a pixel more — the band is `auto` in the panel grid, so every row
       this doesn't claim goes to the music band above it. Tiles share the
       width down to a legible floor and the row scrolls sideways past
       that — the same bargain the grid makes vertically. */
    .rooms-grid.band {
        flex: none;
        grid-template-columns: none;
        grid-auto-flow: column;
        grid-auto-columns: minmax(150px, 1fr);
        grid-auto-rows: 96px;
        overflow-x: auto;
        overflow-y: hidden;
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
       generous badge and name for ones that fit. Laying them out as a row
       instead was tried and was worse: the icon and the count took the
       width from the middle and every room read as "Li…". The name is what
       the tile is for. */
    .rooms.band .rtile {
        padding: var(--space-3);
        gap: var(--space-2);
    }
    .rooms.band .r-ico {
        width: 40px;
        height: 40px;
    }
    .rooms.band .r-name {
        font-size: 17px;
    }

    /* Portrait / narrow: the band idea does not survive one column. */
    @media (orientation: portrait), (max-width: 900px) {
        .rooms-grid.band {
            grid-template-columns: repeat(2, minmax(0, 1fr));
            grid-auto-flow: row;
            grid-auto-rows: minmax(120px, 1fr);
            overflow-x: hidden;
        }
    }
</style>
