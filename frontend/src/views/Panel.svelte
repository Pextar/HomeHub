<script lang="ts">
    import { onMount } from "svelte";
    import PanelClock from "../components/panel/PanelClock.svelte";
    import PanelRooms from "../components/panel/PanelRooms.svelte";
    import PanelMusic from "../components/panel/PanelMusic.svelte";
    import Icon from "../components/Icon.svelte";
    import { route, data, uiPrefs } from "../lib/stores.svelte";
    import { isPanelNight, panelIdleMs, type PanelNowPlaying } from "../lib/panel";
    import { fade } from "svelte/transition";
    import { dur } from "../lib/motion";

    // The panel is a kiosk surface (DESIGN.md §16): one landscape screen,
    // no navigation, no chrome. Three zones — clock/status, room lights,
    // music — plus an ambient face it falls back to when untouched.

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

    function wake() {
        lastTouch = Date.now();
        idle = false;
    }
    $effect(() => {
        const id = setInterval(() => {
            if (Date.now() - lastTouch > panelIdleMs(now)) idle = true;
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

    // PanelMusic reports whether it found any speakers — the third column
    // only exists then; a home without speakers gets a wider rooms grid.
    // Seeded from the same "speakers-seen" memory Home's card keeps, so a
    // home with speakers doesn't watch the column pop in after the poll.
    let hasSpeakers = $state(seenSpeakers());
    function seenSpeakers(): boolean {
        try {
            return localStorage.getItem("speakers-seen") === "true";
        } catch {
            return false;
        } // private browsing
    }

    // And what's playing — the ambient face carries it (§16).
    let playing = $state<PanelNowPlaying | null>(null);

    // Entering the panel marks this device as panel-homed (the dashboard
    // route renders the panel, idle time walks back here); Exit lifts the
    // mark (§16).
    onMount(() => uiPrefs.setPanelHome(true));
    function exit() {
        uiPrefs.setPanelHome(false);
        route.go("dashboard");
    }
</script>

<!-- The pointerdown handler is the wake layer for the ambient face, not an
     interactive control. -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="panel" class:has-music={hasSpeakers} onpointerdown={wake}>
    <PanelClock {timeLabel} {dateLabel} {lightsOn} {lightsTotal} {insideTemp} />
    <PanelRooms />
    <PanelMusic bind:hasSpeakers bind:playing />

    <button class="exit" onclick={exit}>
        <Icon name="close" size={14} /><span>Exit</span>
    </button>

    {#if idle}
        <div class="ambient" class:night={isNight} transition:fade={{ duration: dur(600) }}>
            <div class="face" style:transform={drift}>
                <div class="a-clock mono">{timeLabel}</div>
                <div class="a-date">{dateLabel}</div>
                <div class="a-status mono">{statusLine}</div>
                {#if playing}
                    <div class="a-music">
                        {#if playing.art}
                            <img class="a-art" src={playing.art} alt="" />
                        {/if}
                        <span class="a-track">{playing.title}</span>
                        {#if playing.sub}
                            <span class="a-sub mono">{playing.sub}</span>
                        {/if}
                    </div>
                {/if}
            </div>
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
        /* Columns sized so the centre stays wide enough for un-truncated
           room names even with the music column present (~320px). */
        grid-template-columns: 280px minmax(0, 1fr);
        /* The row is capped at the panel's fixed height so each zone owns
           its overflow — without it an `auto` row sizes to content and the
           page itself spills past the viewport. */
        grid-template-rows: minmax(0, 1fr);
        gap: var(--space-5);
        padding: var(--space-6);
        background: var(--bg);
        color: var(--text);
        overflow: hidden;
        user-select: none;
        -webkit-user-select: none;
        -webkit-touch-callout: none;
        touch-action: manipulation;
    }
    .panel.has-music {
        grid-template-columns: 280px minmax(0, 1fr) 336px;
    }

    /* Portrait / narrow fallback: stacked single column that scrolls. The
       panel is designed landscape-first but must not break when rotated. */
    @media (orientation: portrait), (max-width: 760px) {
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

    /* The one way out — quiet, top-right, big enough for a wall poke. */
    .exit {
        position: absolute;
        top: var(--space-4);
        right: var(--space-4);
        z-index: var(--z-raised);
        display: inline-flex;
        align-items: center;
        gap: 6px;
        height: 44px;
        padding: 0 var(--space-4);
        border-radius: var(--r-pill);
        border: 1px solid var(--hairline);
        background: var(--card);
        color: var(--text-mute);
        font-size: 13px;
        font-weight: 500;
        font-family: inherit;
        cursor: pointer;
        transition:
            color var(--t-fast),
            border-color var(--t-fast),
            transform var(--t-fast);
    }
    .exit:active {
        transform: scale(0.95);
        transition-duration: 80ms;
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

    /* Now-playing on the ambient face — present only while something
       actually plays; the clock stays the subject, this is the footnote. */
    .a-music {
        margin-top: var(--space-5);
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: 6px;
    }
    .a-art {
        width: 72px;
        height: 72px;
        object-fit: cover;
        border-radius: var(--r-md);
        margin-bottom: 4px;
    }
    .a-track {
        font-size: 19px;
        font-weight: 600;
        letter-spacing: -0.02em;
        max-width: 70vw;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .a-sub {
        font-size: 13px;
        color: var(--text-dim);
        max-width: 70vw;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
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
