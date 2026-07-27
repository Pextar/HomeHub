<script lang="ts">
    /**
     * The hero: one room, big, at the top of Music.
     *
     * The screen used to open on a grid of small equal cards — one per thing
     * playing, one per zone, one per speaker — which meant the answer to "what
     * is playing, and can I change it" was spread across a dozen tiles none of
     * which was bigger than a chip. There is one now. It shows the focused
     * room, it carries the whole transport, and everything else on the screen
     * exists to point at it.
     *
     * The room grid below is what changes it. When more than one room is
     * playing at once the pager dots switch between them without scrolling,
     * because "which of the two things playing am I looking at" is the one
     * question the grid can't answer at a glance.
     *
     * Idle is a first-class state, not an empty one: the hero still names the
     * room and offers the way to start something in it.
     */
    import Icon from "../Icon.svelte";
    import Waveform from "./Waveform.svelte";
    import Slider from "./Slider.svelte";
    import TrackRail from "./TrackRail.svelte";
    import PlayerTransport from "./PlayerTransport.svelte";
    import { NEXT_REPEAT, repeatLabel } from "../../lib/music/sonos.svelte";
    import type { Room, RoomsModel } from "../../lib/music/rooms.svelte";
    import type { SonosBridge } from "../../lib/music/sonos.svelte";

    let {
        room,
        rooms,
        sonos,
        /** Rooms the pager steps between — everything currently playing. */
        pager = [],
        onFocus,
        onOpen,
        onBrowse,
    }: {
        room: Room | null;
        rooms: RoomsModel;
        sonos: SonosBridge;
        pager?: Room[];
        onFocus: (r: Room) => void;
        onOpen: () => void;
        onBrowse: () => void;
    } = $props();

    const playing = $derived(!!room && rooms.isPlaying(room));
    const art = $derived(room ? rooms.art(room) : undefined);
    const title = $derived(room ? rooms.title(room) : "");
    const duration = $derived(room ? rooms.durationSec(room) : 0);
    /** Sonos can seek, but only into something with a length to seek into. */
    const seekable = $derived(!!room && room.canSeek && duration > 0);

    const gs = $derived(room ? rooms.groupState(room) : undefined);
    const group = $derived(room?.group);

    /** The headline, and the line under it, in every state a room can be in. */
    const meta = $derived.by(() => {
        if (!room) return { head: "No speakers yet", sub: "Add one to start playing." };
        if (title) return { head: title, sub: rooms.subLine(room) || rooms.memberLine(room) };
        if (!room.reachable) return { head: "Not answering", sub: rooms.memberLine(room) };
        return { head: rooms.nowLine(room), sub: rooms.memberLine(room) };
    });
</script>

<section class="hero" class:on={playing}>
    <div class="hero-head">
        <span class="hero-eyebrow">
            {#if playing}<Waveform />{:else}<Icon name="speaker" size={13} />{/if}
            <span>{playing ? "Playing on" : "Focused"}</span>
        </span>
        {#if room}
            <button class="hero-room" onclick={onOpen}>
                <span>{room.name}</span>
                <Icon name="chevronDown" size={15} />
            </button>
        {/if}
    </div>

    <div class="hero-body">
        <!-- The art is the way into the full player: the biggest, most
             obviously tappable thing on the screen opens the biggest surface. -->
        <button
            class="hero-art-btn"
            aria-label={room ? `Open the player for ${room.name}` : "Open the player"}
            disabled={!room}
            onclick={onOpen}
        >
            {#if art}
                <img class="hero-art" src={art} alt="" />
            {:else}
                <span class="hero-art placeholder"><Icon name="musicNotes" size={26} /></span>
            {/if}
        </button>

        <div class="hero-meta">
            <button class="hero-title-btn" disabled={!room} onclick={onOpen}>
                <span class="hero-title" class:idle={!title}>{meta.head}</span>
                <span class="hero-sub">{meta.sub}</span>
            </button>

            {#if room}
                <TrackRail
                    position={rooms.livePosition(room)}
                    {duration}
                    {seekable}
                    idle={!title}
                    onSeek={(sec) => rooms.seek(room, sec)}
                />
            {/if}
        </div>
    </div>

    {#if room}
        <PlayerTransport
            {playing}
            onToggle={() => rooms.togglePlay(room)}
            toggleBusy={rooms.playBusy(room) || !room.reachable}
            onPrev={room.canSkip ? () => rooms.skip(room, "previous") : undefined}
            prevBusy={rooms.prevBusy(room)}
            onNext={room.canSkip ? () => rooms.skip(room, "next") : undefined}
            nextBusy={rooms.nextBusy(room)}
            {seekable}
            modes={gs && group
                ? {
                      shuffle: gs.shuffle,
                      repeat: gs.repeat,
                      repeatLabel: repeatLabel(gs.repeat),
                      busy: false,
                      onShuffle: () => sonos.setPlayMode(group, { shuffle: !gs.shuffle }),
                      onRepeat: () => sonos.setPlayMode(group, { repeat: NEXT_REPEAT[gs.repeat] }),
                  }
                : undefined}
        />

        <div class="hero-foot">
            <button
                class="hero-mute icon-btn"
                aria-label={rooms.muted(room) ? `Unmute ${room.name}` : `Mute ${room.name}`}
                aria-pressed={rooms.muted(room)}
                onclick={() => rooms.toggleMute(room)}
            >
                <Icon name={rooms.muted(room) ? "volumeOff" : "volume"} size={17} />
            </button>
            <Slider
                value={rooms.volume(room)}
                label="{room.name} volume"
                onInput={(v) => rooms.dragVolume(room, v)}
                onChange={(v) => rooms.setVolume(room, v)}
            />
            <span class="hero-vol mono">{rooms.volume(room)}</span>
        </div>

        {#if !title && room.reachable}
            <button class="btn btn-primary hero-browse" onclick={onBrowse}>
                <Icon name="search" size={15} />
                Find something to play
            </button>
        {/if}
    {/if}

    <!-- Only when there is genuinely more than one thing playing; otherwise the
         grid below is already the whole answer. -->
    {#if pager.length > 1}
        <div class="hero-pager" role="tablist" aria-label="Rooms playing">
            {#each pager as p (p.key)}
                <button
                    class="dot"
                    class:on={p.key === room?.key}
                    role="tab"
                    aria-selected={p.key === room?.key}
                    aria-label={p.name}
                    onclick={() => onFocus(p)}
                ></button>
            {/each}
        </div>
    {/if}
</section>

<style>
    .hero {
        position: relative;
        display: flex; flex-direction: column; gap: var(--space-4);
        padding: var(--space-4);
        background: var(--card); border: 1px solid var(--hairline);
        border-radius: var(--r-lg);
        transition: background var(--t-med), border-color var(--t-med);
    }
    .hero.on { background: var(--tile-on-gradient); border-color: var(--tile-on-border); }

    .hero-head { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); }
    .hero-eyebrow {
        display: flex; align-items: center; gap: 6px;
        font-family: var(--font-mono);
        font-size: 10px; letter-spacing: 0.12em; text-transform: uppercase;
        color: var(--text-dim);
    }
    .hero-room {
        display: flex; align-items: center; gap: 4px;
        min-height: 32px; padding: 4px 4px 4px 10px;
        max-width: 60%;
        background: none; border: 0;
        color: var(--text); font: inherit; font-size: 13px; font-weight: 600;
        cursor: pointer;
    }
    .hero-room span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    @media (pointer: coarse) { .hero-room { min-height: 44px; } }

    .hero-body { display: flex; align-items: center; gap: var(--space-4); }
    .hero-art-btn { background: none; border: 0; padding: 0; cursor: pointer; flex-shrink: 0; }
    .hero-art-btn:disabled { cursor: default; }
    .hero-art {
        display: block;
        width: 104px; height: 104px; border-radius: var(--r-md);
        object-fit: cover; background: var(--card-3);
        box-shadow: var(--shadow-md);
    }
    span.hero-art {
        display: grid; place-items: center;
        color: var(--text-dim); box-shadow: none;
        border: 1px solid var(--hairline);
    }
    .hero.on .hero-art { box-shadow: 0 10px 30px -8px var(--on-glow); }

    .hero-meta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: var(--space-3); }
    .hero-title-btn {
        display: flex; flex-direction: column; gap: 3px; min-width: 0;
        background: none; border: 0; padding: 0;
        text-align: left; color: var(--text); cursor: pointer; font: inherit;
    }
    .hero-title-btn:disabled { cursor: default; }
    .hero-title {
        font-size: 19px; font-weight: 600; letter-spacing: -0.02em;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .hero-title.idle { color: var(--text-mute); font-weight: 500; }
    .hero-sub {
        font-size: 12.5px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }

    .hero-foot { display: flex; align-items: center; gap: var(--space-3); }
    .hero-mute { width: 36px; height: 36px; flex-shrink: 0; color: var(--text-mute); }
    .hero-mute[aria-pressed="true"] { color: var(--on); }
    @media (pointer: coarse) { .hero-mute { width: 44px; height: 44px; } }
    .hero-vol { font-size: 12px; color: var(--text-dim); width: 24px; text-align: right; }

    .hero-browse { align-self: center; }

    .hero-pager { display: flex; justify-content: center; gap: 7px; }
    .dot {
        width: 7px; height: 7px; padding: 0;
        border: 0; border-radius: 50%;
        background: var(--card-3); cursor: pointer;
        transition: background var(--t-fast), transform var(--t-fast);
    }
    .dot.on { background: var(--on); transform: scale(1.25); }
    @media (pointer: coarse) {
        /* Keeps the 44px hit area without growing the mark itself. */
        .dot { position: relative; }
        .dot::after { content: ""; position: absolute; inset: -18px; }
    }

    @media (max-width: 460px) {
        .hero-body { gap: var(--space-3); }
        .hero-art { width: 84px; height: 84px; }
        .hero-title { font-size: 17px; }
    }
    @media (prefers-reduced-motion: reduce) {
        .hero, .dot { transition-duration: 0.001ms; }
    }
</style>
