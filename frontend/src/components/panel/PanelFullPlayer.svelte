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
        <div class="fp-body">
            <!-- The record. It takes the height and gives the width back;
                 the meta rides under it rather than beside it, so the cover
                 is square at the screen's full height. -->
            <!-- The record: the cover at the size a wall can read from the
                 sofa, with the name of what is on it directly underneath —
                 the two together are one object, and putting the name in the
                 controls column left this side of the screen half empty. -->
            <section class="fp-record" aria-label="Now playing">
                <span class="fp-art">
                    {#if featured.art}
                        <img class="fp-cover" src={featured.art} alt="" />
                    {:else}
                        <span class="fp-cover placeholder">[ art ]</span>
                    {/if}
                    {#if featured.playing}
                        <span class="fp-wave"><Waveform /></span>
                    {/if}
                </span>
                <div class="fp-meta">
                    <h2 class="fp-title">
                        {featured.trackTitle ?? (featured.playing ? "Playing" : "Not playing")}
                    </h2>
                    <p class="fp-sub">{featured.trackSub || featured.title}</p>
                </div>
            </section>

            <section class="fp-side" aria-label="Player controls">
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
                    {/if}

                    <!-- What this screen is for beyond size: the queue, in
                         full, with the row playing marked — the one thing
                         the depth's column never had the height to show. A
                         room with no queue behind it says so rather than
                         leaving a hole. -->
                    <div class="fp-queue">
                        {#if featured.kind === "sonos"}
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
                        {:else}
                            <p class="fp-noqueue">
                                {featured.kind === "kef"
                                    ? "A KEF speaker plays its input — there is no queue to show."
                                    : "This room is played together — its queue lives with whatever is streaming to it."}
                            </p>
                        {/if}
                    </div>
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

    .fp-body {
        flex: 1;
        min-height: 0;
        display: flex;
        gap: var(--space-5);
    }

    /* ── The record ──────────────────────────────────────────────────── */
    /* Square, and sized by the screen's own height: the row stretches it, so
       the height is definite and the ratio gives the width — the same trick
       the dashboard band's cover uses (§16), with nothing left to compete
       for it. Nothing else shares this column: anything that did would take
       height off the cover, and the cover is the point of the screen. */
    .fp-record {
        flex: 0 0 auto;
        align-self: center;
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
        min-width: 0;
        max-width: 41vw;
    }
    .fp-art {
        position: relative;
        /* Sized from *one* definite axis, then squared by the ratio. Capping
           the width of a row-stretched box does not work and was a bug twice
           over: stretch makes the height definite, so a `max-width` clamps
           the width without the height following and the record comes out
           cropped. So the height is stated outright — what the screen has
           left after its chrome and this block's own caption, or the share of
           the width the controls can spare, whichever is smaller — and the
           width follows from the ratio. */
        height: min(calc(100dvh - 214px), 41vw);
        aspect-ratio: 1;
        max-width: 100%;
        overflow: hidden;
        border-radius: var(--r-lg);
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
        min-width: 400px;
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

    /* The queue takes whatever the controls leave and owns its own scroll —
       the page never does (§16). */
    .fp-queue {
        flex: 1 1 auto;
        min-height: 0;
        display: flex;
        flex-direction: column;
        overflow-y: auto;
        padding-top: var(--space-3);
        border-top: 1px solid var(--hairline);
    }
    .fp-noqueue {
        margin: 0;
        font-size: 13px;
        line-height: 1.5;
        color: var(--text-dim);
    }

    .fp-standby {
        flex: 1;
        display: flex;
        flex-direction: column;
        align-items: center;
        justify-content: center;
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

    /* Portrait: the record over the controls, and the page scrolls. */
    @media (orientation: portrait), (max-width: 900px) {
        .fp-body {
            flex-direction: column;
        }
        .fp-record {
            max-width: 100%;
            align-self: stretch;
        }
        .fp-art {
            height: auto;
            width: min(100%, 340px);
            align-self: center;
        }
        .fp-side {
            min-width: 0;
        }
    }
</style>
