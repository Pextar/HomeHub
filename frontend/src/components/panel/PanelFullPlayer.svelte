<script lang="ts">
    /**
     * The panel's third depth: the player with the whole screen
     * (`#/panel?music=1&player=1`, DESIGN.md §16).
     *
     * The dashboard band answers "what's on" while you walk past. The music
     * depth answers "put something on". This one answers **"I am
     * listening"** — you have already started a record and the wall has
     * nothing left to browse for, so the cover goes to the size a record
     * deserves and the space the search results were using goes to what is
     * coming next.
     *
     * It is a screen, not a sheet: a kiosk has no sheets (§16), so it
     * arrives on its own route with a back chip, Escape climbs one level to
     * the depth, and falling asleep drops the whole ladder to the ambient
     * face the same way the depth does.
     */
    import { onMount } from "svelte";
    import { fade } from "svelte/transition";
    import Icon from "../Icon.svelte";
    import Waveform from "../music/Waveform.svelte";
    import Slider from "../music/Slider.svelte";
    import TrackRail from "../music/TrackRail.svelte";
    import QueuePane from "../music/QueuePane.svelte";
    import PanelRoomChips from "./PanelRoomChips.svelte";
    import { kefSourceLabel } from "../../lib/kef";
    import { repeatLabel } from "../../lib/music/sonos.svelte";
    import { dur } from "../../lib/motion";
    import type { PanelMusicStore } from "../../lib/panel-music.svelte";

    let { music, onBack }: { music: PanelMusicStore; onBack: () => void } = $props();

    const featured = $derived(music.featured);
    const gs = $derived(featured?.groupState);
    const queueCount = $derived(gs?.queue_length ?? 0);
    /** Only a Sonos group has a queue to give the leftover height to. */
    const hasQueue = $derived(featured?.kind === "sonos");

    const railIdle = $derived(!featured?.trackTitle && !featured?.playing);
    const railLabel = $derived(
        featured?.kind === "kef"
            ? `${kefSourceLabel(featured.input) || "Input"} — no track position`
            : featured?.kind === "zone"
              ? "played together — no track position"
              : undefined,
    );
    const repeatText = $derived(
        gs?.repeat === "all" ? "Repeat all" : gs?.repeat === "one" ? "Repeat one" : "Repeat",
    );

    // Art that 404s (an expired service URL, a proxy that can't reach the
    // speaker) used to leave an empty box on the biggest square on the
    // panel — indistinguishable from a cover still loading. Fall back to
    // §6.7's placeholder, keyed to the URL so the next track gets its own
    // try. Same rule the band's card follows (§16).
    let artFailed = $state<string | null>(null);
    const artSrc = $derived(featured?.art && featured.art !== artFailed ? featured.art : null);

    // ── The cover's size, stated in pixels ──────────────────────────────
    // Height is the scarce axis on a wall, so the square is sized from the
    // height it is allowed — and the reading is a measurement, never a
    // chain of ratios (§16). Saying it in CSS meant an `aspect-ratio` on a
    // box whose width was itself shrink-to-fit, so the width came back from
    // the *caption* under the cover and clamped a 700px-tall square to the
    // length of an artist's name: every record on a wide panel rendered as
    // a vertical strip of its own middle. The screen whose entire point is
    // the cover is not the layout to be clever in front of.
    //
    // So: measure the boxes the cover cannot itself size — the body's
    // stretched height and width, and the two lines of meta that ride under
    // it — then write both axes onto the square outright.
    const ART_FLOOR = 160;
    const ART_CAP = 720;
    /** The gap between the cover and its caption, and between the columns. */
    const HEAD_GAP = 16;
    const COL_GAP = 20;
    /** What the controls column may never be squeezed below. */
    const SIDE_MIN = 400;

    // Landscape is the designed-for shape and the one that measures; the
    // portrait fallback lets CSS size the cover from its width, the one
    // direction every engine agrees on. Mirrors the media query below.
    let landscape = $state(true);
    $effect(() => {
        const mq = window.matchMedia("(orientation: portrait), (max-width: 900px)");
        const apply = () => (landscape = !mq.matches);
        apply();
        mq.addEventListener("change", apply);
        return () => mq.removeEventListener("change", apply);
    });

    let bodyW = $state(0);
    let bodyH = $state(0);
    let metaH = $state(0);

    const coverPx = $derived.by(() => {
        if (!landscape) return 0; // portrait: CSS sizes it from the width
        const height = bodyH - metaH - HEAD_GAP;
        const width = bodyW - SIDE_MIN - COL_GAP;
        if (height <= 0 || width <= 0) return 0; // pre-measure, one frame
        return Math.max(ART_FLOOR, Math.min(height, width, ART_CAP));
    });
    const coverStyle = $derived(coverPx ? `width:${coverPx}px` : "");
    const artStyle = $derived(coverPx ? `width:${coverPx}px;height:${coverPx}px` : "");

    // ── The queue arrives already scrolled to what is playing ───────────
    // "The queue, in full, with the row playing marked" is what this screen
    // is for beyond size — and a room forty tracks into a playlist opened
    // on track one, with the mark somewhere below the fold. Nudge only when
    // the row is actually out of view, and only when it changes: this runs
    // beside a five-second poll on an A8X.
    let queueEl = $state<HTMLElement | undefined>();
    let scrolledTo = "";
    $effect(() => {
        const el = queueEl;
        const key = `${featured?.id ?? ""}:${featured?.queueTrack ?? 0}:${music.queue.length}`;
        if (!el || !featured?.queueTrack || key === scrolledTo) return;
        scrolledTo = key;
        requestAnimationFrame(() => {
            const row = el.querySelector<HTMLElement>(".q-row.current");
            if (!row) return;
            const r = row.getBoundingClientRect();
            const box = el.getBoundingClientRect();
            if (r.top >= box.top && r.bottom <= box.bottom) return;
            el.scrollTop += r.top - box.top - 12;
        });
    });

    // Escape climbs one level, the same ladder the back chip walks (§15.6).
    onMount(() => {
        const onKey = (e: KeyboardEvent) => {
            if (e.key === "Escape") onBack();
        };
        window.addEventListener("keydown", onKey);
        return () => window.removeEventListener("keydown", onKey);
    });
</script>

<div class="fp" in:fade={{ duration: dur(160) }}>
    <header class="fp-head">
        <button class="back" onclick={onBack} aria-label="Back to music">
            <Icon name="chevronLeft" size={16} /><span>Music</span>
        </button>
        <PanelRoomChips {music} />
    </header>

    {#if featured}
        <div class="fp-body" bind:clientWidth={bodyW} bind:clientHeight={bodyH}>
            <!-- The record: the cover at the size a wall can read from the
                 sofa, with the name of what is on it directly underneath —
                 the two together are one object, and putting the name in the
                 controls column left this side of the screen half empty.
                 The column is exactly as wide as the square, so the caption
                 truncates against the cover rather than the cover against
                 the caption. -->
            <section class="fp-record" style={coverStyle} aria-label="Now playing">
                <span class="fp-art" style={artStyle}>
                    {#if artSrc}
                        <img
                            class="fp-cover"
                            src={artSrc}
                            alt=""
                            onerror={() => (artFailed = artSrc)}
                        />
                    {:else}
                        <span class="fp-cover placeholder">[ art ]</span>
                    {/if}
                    {#if featured.playing}
                        <span class="fp-wave"><Waveform /></span>
                    {/if}
                </span>
                <!-- Both lines stay on one line each: the cover is measured
                     against this block's height, so a caption that wrapped
                     would resize the square that decides its width. -->
                <div class="fp-meta" bind:clientHeight={metaH}>
                    <h2 class="fp-title">
                        {featured.trackTitle ?? (featured.playing ? "Playing" : "Not playing")}
                    </h2>
                    <p class="fp-sub">{featured.trackSub || featured.title}</p>
                </div>
            </section>

            <!-- The controls, capped: a fader stretched across a 1500px
                 desk monitor is a worse aim than one at arm's length on the
                 iPad this is drawn for, and a queue row that wide parts its
                 title from its duration by half a screen. The pair centres
                 in whatever is left. -->
            <section
                class="fp-side"
                class:hollow={!hasQueue || featured.standby}
                aria-label="Player controls"
            >
                {#if featured.standby}
                    <div class="fp-standby">
                        <p>In standby</p>
                        <button
                            class="fp-wake"
                            disabled={music.busy["power:" + featured.id]}
                            onclick={() => music.wake(featured)}
                        >
                            <Icon name="power" size={18} /><span>Wake {featured.title}</span>
                        </button>
                    </div>
                {:else}
                    <div class="fp-controls">
                        <TrackRail
                            position={music.posSec}
                            duration={music.durSec}
                            seekable={music.seekable}
                            idle={railIdle}
                            liveLabel={railLabel}
                            onSeek={(sec) => music.seek(sec)}
                        />

                        <div class="fp-transport">
                            {#if featured.canSkip}
                                <button
                                    class="t-btn"
                                    aria-label="Previous track"
                                    disabled={music.busy["previous:" + featured.id]}
                                    onclick={() => music.skip(featured, "previous")}
                                >
                                    <Icon name="skipPrev" size={26} />
                                </button>
                            {/if}
                            <button
                                class="t-btn primary"
                                aria-label={featured.playing ? "Pause" : "Play"}
                                disabled={music.busy["play:" + featured.id]}
                                onclick={() => music.togglePlay(featured)}
                            >
                                <Icon name={featured.playing ? "pause" : "play"} size={34} />
                            </button>
                            {#if featured.canSkip}
                                <button
                                    class="t-btn"
                                    aria-label="Next track"
                                    disabled={music.busy["next:" + featured.id]}
                                    onclick={() => music.skip(featured, "next")}
                                >
                                    <Icon name="skipNext" size={26} />
                                </button>
                            {/if}
                        </div>

                        <div class="fp-volume">
                            <button
                                class="v-ico"
                                class:mute={featured.muted}
                                aria-label={featured.muted ? "Unmute" : "Mute"}
                                disabled={music.busy["mute:" + featured.id]}
                                onclick={() => music.toggleMute(featured)}
                            >
                                <Icon name={featured.muted ? "volumeOff" : "volume"} size={18} />
                            </button>
                            <button
                                class="v-step"
                                aria-label="Volume down"
                                disabled={music.busy["vol:" + featured.id]}
                                onclick={() => music.nudgeVolume(featured, -5)}
                            >
                                <Icon name="minus" size={18} />
                            </button>
                            <Slider
                                value={music.vol}
                                label="Volume"
                                valueText="{music.vol}%"
                                onInput={(v) => music.dragVolume(featured, v)}
                                onChange={(v) => music.setVolume(featured, v)}
                            />
                            <button
                                class="v-step"
                                aria-label="Volume up"
                                disabled={music.busy["vol:" + featured.id]}
                                onclick={() => music.nudgeVolume(featured, 5)}
                            >
                                <Icon name="plus" size={18} />
                            </button>
                            <span class="v-val mono">{music.vol}</span>
                        </div>

                        {#if gs}
                            <div class="fp-modes">
                                <button
                                    class="p-mode"
                                    class:on={gs.shuffle}
                                    aria-pressed={gs.shuffle}
                                    disabled={music.busy["mode:" + featured.id]}
                                    onclick={() => music.toggleShuffle()}
                                >
                                    <Icon name="shuffle" size={16} /><span>Shuffle</span>
                                </button>
                                <button
                                    class="p-mode"
                                    class:on={gs.repeat !== "off"}
                                    aria-pressed={gs.repeat !== "off"}
                                    aria-label={repeatLabel(gs.repeat)}
                                    disabled={music.busy["mode:" + featured.id]}
                                    onclick={() => music.cycleRepeat()}
                                >
                                    <Icon
                                        name={gs.repeat === "one" ? "repeatOne" : "repeat"}
                                        size={16}
                                    /><span>{repeatText}</span>
                                </button>
                                <button
                                    class="p-mode"
                                    class:on={gs.crossfade}
                                    aria-pressed={gs.crossfade}
                                    disabled={music.busy["xfade:" + featured.id]}
                                    onclick={() => music.toggleCrossfade()}
                                >
                                    <Icon name="activity" size={16} /><span>Crossfade</span>
                                </button>
                                <button
                                    class="p-mode"
                                    class:on={!!featured.autoplay}
                                    aria-pressed={!!featured.autoplay}
                                    disabled={music.busy["autoplay:" + featured.id]}
                                    onclick={() => music.toggleAutoplay()}
                                >
                                    <Icon name="assistant" size={16} /><span>Play similar</span>
                                </button>
                            </div>
                            <!-- The choice shows itself only once the queue
                                 has run out, so the wall says which way it
                                 will go (§16). -->
                            <p class="fp-note">
                                {featured.autoplay
                                    ? "When the queue ends, similar music keeps playing."
                                    : "When the queue ends, playback stops."}
                            </p>
                        {/if}
                    </div>

                    <!-- What this screen is for beyond size: the queue, in
                         full, with the row playing marked — the one thing
                         the depth's column never had the height to show. A
                         room with no queue behind it says so in one quiet
                         line and gives the height back to the transport
                         rather than holding open a scroll region with a
                         sentence at the top of it. -->
                    {#if hasQueue}
                        <div class="fp-queue" bind:this={queueEl}>
                            <QueuePane
                                items={music.queue}
                                loading={music.queueLoading}
                                total={queueCount || music.queue.length}
                                currentTrack={featured.queueTrack}
                                playing={featured.playing}
                                confirmClear
                                clearBusy={!!music.busy["qclear:" + featured.id]}
                                isBusy={(k) => !!music.busy[k]}
                                onJump={(t) => music.jumpTo(t)}
                                onRemove={(t) => music.removeQueued(t)}
                                onClear={() => music.clearQueue()}
                            />
                        </div>
                    {:else}
                        <p class="fp-noqueue">
                            {featured.kind === "kef"
                                ? "A KEF speaker plays its input — there is no queue to show."
                                : "This room is played together — its queue lives with whatever is streaming to it."}
                        </p>
                    {/if}
                {/if}
            </section>
        </div>
    {:else}
        <div class="fp-nosrc">
            <Icon name="speaker" size={28} />
            <p>No speakers reachable</p>
        </div>
    {/if}
</div>

<style>
    .fp {
        grid-row: 1 / -1;
        display: flex;
        flex-direction: column;
        gap: var(--space-5);
        min-height: 0;
        min-width: 0;
    }

    .fp-head {
        display: flex;
        align-items: center;
        gap: var(--space-4);
        flex-shrink: 0;
    }
    .fp-head :global(.p-sources) {
        flex: 1 1 auto;
    }
    .back {
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
        flex-shrink: 0;
        transition:
            background var(--t-fast),
            transform var(--t-fast);
    }
    .back:active {
        transform: scale(0.95);
        transition-duration: 80ms;
    }

    /* The record and the controls, centred as a pair: past a certain width
       neither of them grows, so the slack belongs to the margins rather
       than to a fader nobody can aim along. */
    .fp-body {
        flex: 1;
        min-height: 0;
        display: flex;
        justify-content: center;
        gap: var(--space-5); /* COL_GAP */
    }

    /* ── The record ──────────────────────────────────────────────────── */
    /* Both boxes are written in pixels once the body has been measured
       (see `coverPx`). The rules here are the portrait fallback and the one
       frame before that measurement lands: a width-led square inside a
       column that already has a width, so there is never a frame where the
       caption decides how wide a record is. */
    .fp-record {
        flex: 0 0 auto;
        align-self: center;
        display: flex;
        flex-direction: column;
        gap: var(--space-4); /* HEAD_GAP */
        width: 340px;
        max-width: 100%;
        min-width: 0;
    }
    .fp-art {
        position: relative;
        flex: none;
        width: 100%;
        aspect-ratio: 1;
        min-height: 160px;
        overflow: hidden;
        border-radius: var(--r-lg);
        background: var(--card-2);
    }
    .fp-cover {
        display: block;
        width: 100%;
        height: 100%;
        object-fit: cover;
    }
    span.fp-cover {
        font-size: 12px;
    }
    .fp-wave {
        position: absolute;
        left: var(--space-4);
        bottom: var(--space-4);
        padding: 8px 10px;
        border-radius: var(--r-sm);
        background: var(--bg-bar);
        display: inline-flex;
    }

    .fp-meta {
        flex-shrink: 0;
        min-width: 0;
        text-align: center;
    }
    .fp-title {
        margin: 0;
        font-size: 26px;
        font-weight: 600;
        letter-spacing: -0.02em;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .fp-sub {
        margin: 4px 0 0;
        font-size: 15px;
        color: var(--text-mute);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    /* ── The controls, and what's next ───────────────────────────────── */
    .fp-side {
        flex: 1 1 auto;
        min-width: 400px; /* SIDE_MIN */
        max-width: 720px;
        overflow: hidden;
        min-height: 0;
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
        padding: var(--space-5);
        border-radius: var(--r-lg);
        border: 1px solid var(--hairline);
        background: var(--card);
    }
    /* The card stays a plain surface even while the room plays. §15.2's
       fill is what a hero, a room card or a mini-player wears to say "this
       one is making noise" among others that aren't; this screen is one
       room and nothing else, the record beside it carries the waveform, and
       the one place the ON gradient still has to read here is the queue row
       that is playing — which it can't, against a card wearing the same
       gradient. The app's own full player (`Player.svelte`) is flat for the
       same reason. */

    /* Nothing below the transport to grow into — a room without a queue, or
       one asleep. Then the card is as tall as what it has to say and rides
       beside the record rather than stretching to the screen: a KEF's
       controls held open to a full-height panel were four rows at the top
       of an empty rectangle, which reads as something that failed to load
       rather than as a speaker with no queue. */
    .fp-side.hollow {
        align-self: center;
    }

    .fp-controls {
        flex: none;
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
    }

    /* Transport, one size up from the card's: this is the screen you are
       across the room from. */
    .fp-transport {
        display: flex;
        align-items: center;
        justify-content: center;
        gap: var(--space-5);
        flex-shrink: 0;
    }
    .t-btn {
        width: 72px;
        height: 72px;
        border-radius: 50%;
        border: 1px solid var(--hairline);
        background: var(--card-2);
        color: var(--text);
        display: grid;
        place-items: center;
        cursor: pointer;
        transition:
            background var(--t-fast),
            transform var(--t-fast);
    }
    .t-btn:active {
        transform: scale(0.94);
        transition-duration: 80ms;
    }
    .t-btn:disabled {
        opacity: 0.5;
    }
    .t-btn.primary {
        width: 92px;
        height: 92px;
        background: var(--on);
        border-color: var(--on);
        color: var(--primary-fg);
    }

    .fp-volume {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        flex-shrink: 0;
    }
    .v-ico {
        width: 44px;
        height: 44px;
        display: grid;
        place-items: center;
        border: 0;
        background: none;
        color: var(--text-mute);
        cursor: pointer;
        border-radius: var(--r-sm);
        flex-shrink: 0;
        margin-left: -10px;
    }
    .v-ico.mute {
        color: var(--bad);
    }
    .v-step {
        width: 44px;
        height: 44px;
        display: grid;
        place-items: center;
        flex-shrink: 0;
        border: 1px solid var(--hairline);
        border-radius: 50%;
        background: var(--card-2);
        color: var(--text-mute);
        cursor: pointer;
        transition: transform var(--t-fast);
    }
    .v-step:active {
        transform: scale(0.92);
        transition-duration: 80ms;
    }
    .v-ico:disabled,
    .v-step:disabled {
        opacity: 0.5;
    }
    .v-val {
        font-size: 14px;
        color: var(--text-mute);
        width: 3ch;
        text-align: right;
        flex-shrink: 0;
    }

    .fp-modes {
        display: flex;
        flex-wrap: wrap;
        justify-content: center;
        gap: var(--space-2);
        flex-shrink: 0;
    }
    .p-mode {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        padding: 8px 12px;
        border-radius: var(--r-pill);
        border: 1px solid var(--hairline);
        background: var(--card-2);
        color: var(--text-mute);
        font-family: inherit;
        font-size: 12.5px;
        font-weight: 500;
        cursor: pointer;
        transition:
            background var(--t-fast),
            color var(--t-fast),
            border-color var(--t-fast),
            transform var(--t-fast);
    }
    .p-mode:active {
        transform: scale(0.95);
        transition-duration: 80ms;
    }
    .p-mode.on {
        background: var(--on-soft);
        border-color: var(--tile-on-border);
        color: var(--on);
    }
    .p-mode:disabled {
        opacity: 0.55;
    }
    .fp-note {
        margin: 0;
        text-align: center;
        font-size: 12px;
        line-height: 1.5;
        color: var(--text-dim);
        flex-shrink: 0;
    }

    /* Every control on the wall answers a keyboard too — the panel is
       reached from a desk browser as often as from the iPad it is drawn
       for, and a focus ring is the only thing that says where a tap would
       land there. */
    .back:focus-visible,
    .t-btn:focus-visible,
    .v-ico:focus-visible,
    .v-step:focus-visible,
    .p-mode:focus-visible,
    .fp-wake:focus-visible {
        outline: none;
        box-shadow: var(--focus-ring);
    }

    /* The queue takes whatever the controls leave and owns its own scroll —
       the page never does (§16). */
    .fp-queue {
        flex: 1 1 auto;
        min-height: 0;
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
        overflow-y: auto;
        border-top: 1px solid var(--hairline);
    }
    /* How long the queue is, and Clear, stay put while it scrolls: the pane
       opens at the track playing (see the effect above), so a bar that
       scrolled with the list would start out of sight — and Clear is the
       one control here you must never have to hunt for. The top padding
       rides on the bar rather than on the scrollport, or rows would show
       through the gap above it. */
    .fp-queue :global(.q-bar) {
        position: sticky;
        top: 0;
        z-index: 1;
        padding-top: var(--space-3);
        padding-bottom: var(--space-1);
        background: var(--card);
    }
    .fp-noqueue {
        margin: 0;
        flex: none;
        font-size: 13px;
        line-height: 1.5;
        text-align: center;
        color: var(--text-dim);
    }

    .fp-standby {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: var(--space-4);
    }
    .fp-standby p {
        margin: 0;
        font-size: 14px;
        color: var(--text-dim);
    }
    .fp-wake {
        display: inline-flex;
        align-items: center;
        gap: 8px;
        min-height: 52px;
        padding: 0 var(--space-5);
        border-radius: var(--r-pill);
        border: 1px solid var(--border-strong);
        background: var(--card-2);
        color: var(--text);
        font-family: inherit;
        font-size: 15px;
        font-weight: 500;
        cursor: pointer;
    }

    .fp-nosrc {
        flex: 1;
        display: flex;
        flex-direction: column;
        align-items: center;
        justify-content: center;
        gap: var(--space-3);
        border: 1px dashed var(--border);
        border-radius: var(--r-lg);
        color: var(--text-dim);
        font-size: 14px;
    }
    .fp-nosrc p {
        margin: 0;
    }

    @media (pointer: coarse) {
        .p-mode {
            min-height: 44px;
            padding-inline: 16px;
        }
    }

    /* The reference wall is a 1024×768 iPad (§16), where the controls
       column lands within a few pixels of the height it has. Tightening the
       spacing there is what keeps the cover off its floor; nothing that is
       a touch target shrinks. */
    @media (max-height: 820px) and (orientation: landscape) {
        .fp {
            gap: var(--space-4);
        }
        .fp-side,
        .fp-controls {
            gap: var(--space-3);
        }
        .fp-side {
            padding: var(--space-4);
        }
        .fp-transport {
            gap: var(--space-4);
        }
    }

    /* Portrait: the record over the controls, and the page scrolls. */
    @media (orientation: portrait), (max-width: 900px) {
        .fp-body {
            flex-direction: column;
            align-items: center;
        }
        .fp-record {
            width: min(100%, 340px);
        }
        .fp-side {
            min-width: 0;
            width: 100%;
            max-width: 560px;
            overflow: visible;
        }
        .fp-queue {
            flex: none;
            overflow: visible;
        }
    }
</style>
