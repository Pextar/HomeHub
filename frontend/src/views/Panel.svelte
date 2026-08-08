<script lang="ts">
    import { onMount } from "svelte";
    import PanelClock from "../components/panel/PanelClock.svelte";
    import PanelRooms from "../components/panel/PanelRooms.svelte";
    import PanelMusic from "../components/panel/PanelMusic.svelte";
    import PanelBrowse from "../components/panel/PanelBrowse.svelte";
    import PanelFullPlayer from "../components/panel/PanelFullPlayer.svelte";
    import { route, data, uiPrefs } from "../lib/stores.svelte";
    import { isPanelNight, panelIdleMs } from "../lib/panel";
    import { createPanelMusic } from "../lib/panel-music.svelte";
    import { createSpotify } from "../lib/music/spotify.svelte";
    import { createSearchHistory } from "../lib/music/history.svelte";
    import { fade } from "svelte/transition";
    import { dur } from "../lib/motion";

    // The panel is a kiosk surface (DESIGN.md §16): no chrome, no app
    // shell. The dashboard depth is three stacked bands — a status strip,
    // the music band, and a row of room tiles — with a music depth one tap
    // in, the full-screen player one further, and an ambient face they all
    // fall back to when untouched.

    // ── Clock ────────────────────────────────────────────────────────────
    // One tick for the whole panel; children take the labels as props.
    // Displayed as HH:MM everywhere — seconds are phone detail, not
    // wall-panel detail.
    let now = $state(new Date());
    $effect(() => {
        const id = setInterval(() => {
            now = new Date();
        }, 1000);
        return () => clearInterval(id);
    });
    const timeLabel = $derived(now.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }));
    const dateLabel = $derived(
        now.toLocaleDateString([], { weekday: "long", day: "numeric", month: "long" }),
    );

    // ── Glance stats (shared by the hero column and the ambient face) ────
    const v = $derived(data.value);
    const lightsOn = $derived(v.sockets.filter((s) => s.state).length);
    const lightsTotal = $derived(v.sockets.length);
    const tempSensors = $derived(
        v.sensors.filter((s) => s.kind === "temperature" && s.last_value != null),
    );
    const insideTemp = $derived(
        tempSensors.length
            ? Math.round(
                  tempSensors.reduce((sum, s) => sum + (s.last_value ?? 0), 0) / tempSensors.length,
              )
            : null,
    );
    const statusLine = $derived(
        [
            `${lightsOn} of ${lightsTotal} lights on`,
            insideTemp != null ? `${insideTemp}° inside` : "",
        ]
            .filter(Boolean)
            .join(" · "),
    );

    // ── Idle → ambient face ──────────────────────────────────────────────
    // An always-on display's resting state is the glance face: clock, date,
    // one status line — and what's playing, when something is. Any touch
    // wakes it. At night it sleeps sooner and shows dimmer — kinder to the
    // room and to the backlight. #/panel?idle=1 arrives already asleep;
    // that's how the app-level auto-return lands (DESIGN.md §16).
    const isNight = $derived(isPanelNight(now));
    let idle = $state(route.query.idle === "1");
    let lastTouch = Date.now();

    // The music depth is the same route one level in (#/panel?music=1), and
    // the full-screen player one further (&player=1), so the kiosk coherence
    // around the panel — sticky home, the app-level idle auto-return —
    // covers both untouched, and back is one hash away. The ladder is
    // dashboard → depth → player, and every way back climbs one rung.
    const musicOpen = $derived(route.query.music === "1");
    const playerOpen = $derived(musicOpen && route.query.player === "1");
    const deep = $derived(musicOpen || playerOpen);

    // Touches feed the activity clock on pointerdown; waking is the face's
    // own click (below), so the tap that wakes can never act on the panel.
    function poke() {
        lastTouch = Date.now();
    }
    function wake() {
        lastTouch = Date.now();
        idle = false;
    }
    $effect(() => {
        const id = setInterval(() => {
            if (Date.now() - lastTouch > panelIdleMs(now)) {
                idle = true;
                // Sleep means home: idling anywhere deeper walks back to the
                // dashboard depth's ambient face (§16).
                if (deep) route.go("panel", { idle: "1" });
            }
        }, 1000);
        return () => clearInterval(id);
    });

    // Slow positional drift on the ambient face so no pixel holds the same
    // value for hours on an always-on LCD. Transform-only, minute cadence.
    const drift = $derived.by(() => {
        const mins = now.getMinutes() + now.getHours() * 60;
        const dx = Math.round(Math.sin(mins / 60) * 14);
        const dy = Math.round(Math.cos(mins / 45) * 10);
        return `translate(${dx}px, ${dy}px)`;
    });

    // The speaker brain, shared by both depths: the dashboard's music
    // column and the music depth's player/search read the same poll, the
    // same featured source and the same now-playing line, and it stays
    // alive across depth swaps (§16). It also reports whether any speakers
    // exist — the music band only exists then; a home without speakers
    // gives the room tiles the whole surface as a grid again.
    const music = createPanelMusic();
    // And what's playing — the ambient face carries it.
    const playing = $derived(music.nowPlaying);
    /** Which face the panel rests on. A room with music on gets the record;
     *  a quiet house gets the clock. Both are the same fade, the same drift
     *  and the same tap to wake — this is one face with two subjects, not a
     *  second screen (§16). */
    const listening = $derived(!!playing);

    // Which rooms are making noise, by name. The room band marks the
    // playing room with the waveform instead of a switch (§16), and a
    // source's title is the room it names — a Sonos group, a KEF speaker
    // or a zone all carry the room's own name.
    const playingRooms = $derived(
        music.sources.filter((s) => s.playing).map((s) => s.title.toLowerCase()),
    );

    // The catalog lives up here for the same reason the speakers do: the
    // music depth is a route away, and a route away and back used to throw
    // the search out with the component. Typing on a wall is the most
    // expensive thing this surface asks for, so walking off to fetch
    // something and coming back must not cost it twice. Recents are keyed
    // by the featured room with the app's own key format, so the wall and
    // the phone share one per-room history.
    const recents = createSearchHistory(() => {
        const f = music.featured;
        return f ? `${f.kind}:${f.id}` : null;
    });
    const spotify = createSpotify((q, art) => recents.add(q, art));
    // `status` is null both while loading and when the endpoint refuses
    // (the Spotify routes are admin-only); `booted` separates the two so a
    // refusal doesn't hang the depth on skeletons.
    let booted = $state(false);
    onMount(() => {
        void spotify
            .load()
            .then(() => {
                // The shelves the depth idles on beyond this account's
                // listening: what it saved, what it keeps, who it plays, and
                // what came out this week. Asked for here rather than in
                // `load()` because the app's Music view has a search box and
                // a dock and never idles on shelves — this is the surface
                // that browses, so it is the one that pays for them.
                void spotify.loadBrowse();
            })
            .finally(() => {
                booted = true;
            });
    });

    $effect(() => {
        // Asleep, the panel is a clock: the speakers only need catching up
        // with often enough that waking is current, and a pushed change
        // still lands immediately. An always-on tablet polls for years —
        // this is the one place that cost can be given back for free.
        music.setIdle(idle);
        // Sleeping also ends the search. Keeping it across a walk to the
        // kitchen is the point of hoisting it; keeping it until the next
        // person walks up to the wall is somebody else's half-typed
        // question.
        if (idle) spotify.clearQuery();
    });

    // Entering the panel marks this device as panel-homed (the dashboard
    // route renders the panel, idle time walks back here); Exit lifts the
    // mark (§16).
    onMount(() => uiPrefs.setPanelHome(true));
    function exit() {
        uiPrefs.setPanelHome(false);
        route.go("dashboard");
    }
</script>

<!-- The pointerdown handler only feeds the activity clock, not an
     interactive control. -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="panel" class:has-music={music.hasSpeakers} onpointerdown={poke}>
    {#if playerOpen}
        <!-- Back climbs one rung to the depth; the close chip drops the whole
             ladder home, which is the other thing a wall gets asked for when
             the record is on and nobody is browsing (§16). -->
        <PanelFullPlayer
            {music}
            onBack={() => route.go("panel", { music: "1" })}
            onClose={() => route.go("panel")}
        />
    {:else if musicOpen}
        <PanelBrowse
            {music}
            {spotify}
            {recents}
            {booted}
            openArtistNamed={route.query.artist ?? ""}
            openPane={route.query.pane ?? ""}
        />
    {:else}
        <PanelClock {timeLabel} {dateLabel} {lightsOn} {lightsTotal} {insideTemp} onExit={exit} />
        <PanelMusic {music} />
        <PanelRooms band={music.hasSpeakers} {playingRooms} />
    {/if}

    {#if idle}
        <!-- Wake on click, claimed by the face itself while it still covers
             the panel: a pointerdown wake lets the tap's click fall through
             to a tile once the face is gone — instantly under reduced motion,
             or when a long press outlives the fade. -->
        <!-- svelte-ignore a11y_no_static_element_interactions, a11y_click_events_have_key_events -->
        <div
            class="ambient"
            class:night={isNight}
            transition:fade={{ duration: dur(600) }}
            onclick={wake}
        >
            {#if listening}
                <!-- The listening face. While a record is on, the resting
                     state of a wall in a room where music is playing should
                     be the record — it is the most-looked-at surface the
                     panel has and it was spending it on a 40px thumbnail
                     under a clock nobody is reading at that moment. The
                     clock stays, demoted to a corner: it is still a wall
                     panel, and the time is still the thing you glance up
                     for. Same drift, same fade, same tap to wake. -->
                <div class="face listening" style:transform={drift}>
                    <div class="l-art">
                        {#if playing?.art}
                            <img src={playing.art} alt="" />
                        {:else}
                            <span class="placeholder">[ art ]</span>
                        {/if}
                    </div>
                    <div class="l-meta">
                        <div class="l-clock mono">{timeLabel}</div>
                        <div class="l-track">{playing?.title}</div>
                        {#if playing?.sub}
                            <div class="l-sub">{playing.sub}</div>
                        {/if}
                        <div class="l-status mono">{statusLine}</div>
                    </div>
                </div>
            {:else}
                <div class="face" style:transform={drift}>
                    <div class="a-clock mono">{timeLabel}</div>
                    <div class="a-date">{dateLabel}</div>
                    <div class="a-status mono">{statusLine}</div>
                </div>
            {/if}
        </div>
    {/if}
</div>

<style>
    /* Landscape kiosk grid. Everything is sized to fit 1024×768 without
       scrolling — a wall panel has no scroll gesture, so each zone owns
       its overflow internally instead. */
    .panel {
        position: relative;
        height: 100vh;
        height: 100dvh;
        box-sizing: border-box;
        display: grid;
        /* Rows, not columns. The panel used to be three columns — clock,
           rooms, music — which gave the player a ~336px slot and the two
           surfaces you only *read* two thirds of a landscape screen. It is
           the other way round: the status strip and the room tiles are
           glance surfaces and run the full width at the height they need,
           and everything left over is the music band, which is what a wall
           panel is actually used to drive (DESIGN.md §16).
           The rows are capped at the panel's fixed height so each zone owns
           its overflow — without it an `auto` row sizes to content and the
           page itself spills past the viewport.

           The bands run edge to edge and are divided by hairlines rather
           than floated apart as cards on a padded page: a wall panel has no
           page around it to show, so a margin is just screen the bands
           aren't using. Each band pads itself and draws its own rule — the
           strip a border-bottom, the room row a border-top. */
        grid-template-columns: minmax(0, 1fr);
        grid-template-rows: auto minmax(0, 1fr);
        gap: 0;
        padding: 0;
        background: var(--bg);
        color: var(--text);
        overflow: hidden;
        user-select: none;
        -webkit-user-select: none;
        -webkit-touch-callout: none;
        touch-action: manipulation;
    }
    /* With speakers: the status strip and the room row take exactly the
       height their content asks for, and the music band takes everything
       that is left. That is the whole allocation, stated once — the two
       glance surfaces are sized by what they have to say, and the surface
       that gets driven gets the slack. */
    .panel.has-music {
        grid-template-rows: auto minmax(0, 1fr) auto;
    }

    /* Portrait / narrow fallback: stacked single column that scrolls. The
       panel is designed landscape-first but must not break when rotated. */
    @media (orientation: portrait), (max-width: 900px) {
        .panel,
        .panel.has-music {
            grid-template-columns: minmax(0, 1fr);
            grid-template-rows: none;
            height: auto;
            min-height: 100vh;
            min-height: 100dvh;
            overflow-y: auto;
        }
    }

    /* ── Ambient face ────────────────────────────────────────────────── */
    .ambient {
        position: fixed;
        inset: 0;
        z-index: var(--z-overlay);
        display: grid;
        place-items: center;
        background: var(--bg);
    }
    .face {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: var(--space-3);
        text-align: center;
        transition: opacity 600ms ease;
    }
    .a-clock {
        font-size: clamp(104px, 20vw, 168px);
        font-weight: 500;
        letter-spacing: -0.03em;
        line-height: 1;
    }
    .a-date {
        font-size: 18px;
        color: var(--text-mute);
    }
    .a-status {
        font-size: 14px;
        color: var(--text-dim);
    }

    /* ── The listening face ──────────────────────────────────────────── */
    /* The record at the size a record deserves, with the clock demoted
       beside it. Two axes rather than the clock face's one: a square wants
       the height, and stacking a big cover under a big clock would leave
       both small on a 768px panel.

       The cover is sized in viewport units and capped, not measured: this
       face has nothing else competing for the space, so there is no
       reading that could chase its own tail (contrast the band and the
       full player, which both measure — §16). */
    .face.listening {
        flex-direction: row;
        align-items: center;
        gap: clamp(var(--space-7), 7vw, 88px);
        text-align: left;
        max-width: 96vw;
    }
    .l-art {
        flex: none;
        width: clamp(220px, 58vh, 620px);
        height: clamp(220px, 58vh, 620px);
        border-radius: var(--r-lg);
        overflow: hidden;
        background: var(--card-2);
        border: 1px solid var(--hairline);
        display: grid;
        place-items: center;
    }
    .l-art img {
        width: 100%;
        height: 100%;
        object-fit: cover;
        display: block;
    }
    .l-art .placeholder {
        width: 100%;
        height: 100%;
        display: grid;
        place-items: center;
        font-size: 12px;
        color: var(--text-dim);
    }
    .l-meta {
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
    }
    /* Still a wall panel: the time is what you glance up for, so it keeps
       its place at the top of the column — one size down from the clock
       face, where it is no longer the subject. */
    .l-clock {
        font-size: clamp(56px, 12vw, 140px);
        font-weight: 500;
        letter-spacing: -0.03em;
        line-height: 1;
    }
    .l-track {
        font-size: clamp(24px, 3.6vw, 48px);
        font-weight: 600;
        letter-spacing: -0.02em;
        max-width: 46vw;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .l-sub {
        font-size: clamp(15px, 2vw, 24px);
        color: var(--text-mute);
        max-width: 46vw;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .l-status {
        margin-top: var(--space-3);
        font-size: 15px;
        color: var(--text-dim);
    }

    /* Portrait: the same two parts, stacked, with the cover giving way
       first — the fallback the rest of the panel already takes (§16). */
    @media (orientation: portrait), (max-width: 900px) {
        .face.listening {
            flex-direction: column;
            text-align: center;
            gap: var(--space-5);
        }
        .l-art {
            width: min(68vw, 440px);
            height: min(68vw, 440px);
        }
        .l-meta {
            align-items: center;
        }
        .l-track,
        .l-sub {
            max-width: 84vw;
        }
    }
    /* Night: the face drops to a murmur — readable across a dark room,
       not a lamp in itself. */
    .ambient.night .face {
        opacity: 0.45;
    }

    @media (prefers-reduced-motion: reduce) {
        .face {
            transition: none;
        }
    }
</style>
