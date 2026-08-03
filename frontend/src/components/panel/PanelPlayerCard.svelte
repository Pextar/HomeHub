<script lang="ts">
    import Icon from "../Icon.svelte";
    import Waveform from "../music/Waveform.svelte";
    import Slider from "../music/Slider.svelte";
    import TrackRail from "../music/TrackRail.svelte";
    import { kefSourceLabel, KEF_SOURCES } from "../../lib/kef";
    import { repeatLabel } from "../../lib/music/sonos.svelte";
    import { clock } from "../../lib/music/clock.svelte";
    import type { PanelMusicStore } from "../../lib/panel-music.svelte";

    // The featured source's player card, shared by both of the panel's
    // depths (DESIGN.md §16): the dashboard column shows it with its art as
    // the tap-through into music; the music depth shows it `full` — per-
    // member faders, the KEF input selector, the sleep timer and the
    // Up-next row into the queue — since that is where the wall's music
    // jobs happen.
    //
    // Every capability renders only where the room says it has one (§15):
    // the rail seeks on a Sonos track and is a read-only rail elsewhere,
    // skips appear where a skip would reach something (`canSkip`), play
    // modes are a Sonos coordinator's, and standby is a Wake button rather
    // than a dead label — waking a speaker is the wall's job, not the full
    // view's.
    let {
        music,
        onOpen = undefined,
        full = false,
        onShowQueue = undefined,
    }: {
        music: PanelMusicStore;
        /** Given, the art + meta are a button into the music depth. */
        onOpen?: () => void;
        /** The depth's richer card: member faders, KEF inputs, sleep, Up next. */
        full?: boolean;
        /** The Up-next row's destination — the queue pane. */
        onShowQueue?: () => void;
    } = $props();

    const featured = $derived(music.featured);
    const gs = $derived(featured?.groupState);
    const members = $derived(featured?.members ?? []);
    const multi = $derived(members.length > 1);

    // A rail gets nothing to say when there is no track loaded to describe.
    const railIdle = $derived(!featured?.trackTitle && !featured?.playing);
    const railLabel = $derived(
        featured?.kind === "kef"
            ? `${kefSourceLabel(featured.input) || "Input"} — no track position`
            : featured?.kind === "zone"
              ? "played together — no track position"
              : undefined, // TrackRail's default: "live stream — no track position"
    );

    const repeatText = $derived(
        gs?.repeat === "all" ? "Repeat all" : gs?.repeat === "one" ? "Repeat one" : "Repeat",
    );

    /** How a source chip names its make, for the ones that aren't obvious. */
    function chipTitle(kind: string): string {
        return kind === "zone" ? "HomeHub room" : kind === "kef" ? "KEF speaker" : "Sonos room";
    }

    // ── The queued confirmation ─────────────────────────────────────────
    // A track dropped into the queue changes nothing on screen — the wall
    // has no dock and no toast to lean on — so the card says so itself for
    // a few seconds, wherever the queueing happened (a search row, an
    // artist page, a record).
    const QUEUED_MS = 5000;
    const queued = $derived.by(() => {
        void clock.beat;
        const q = music.lastQueued;
        if (!q || Date.now() - q.at > QUEUED_MS) return null;
        return q;
    });

    // ── Sleep timer (a Sonos group's) ───────────────────────────────────
    // The wall's own setting: "stop in half an hour" is asked at the light
    // switch on the way to bed, not on a phone. Chips rather than a form —
    // there are no forms on a kiosk.
    const SLEEP_CHOICES = [15, 30, 60] as const;
    const sleepOn = $derived(music.sleepMinutes > 0);

    /** Whether anything rides below the cover in the scrolling half. The
     *  cover claims that whole half otherwise, and leaves a sliver of the
     *  first row showing when it doesn't — a kiosk has no scrollbar, so a
     *  region whose content stops exactly at its own edge gives no sign
     *  that it moves at all. */
    const hasExtras = $derived.by(() => {
        if (!featured || featured.standby) return false;
        if (gs) return true; // play modes, and the Up-next row under them
        if (!full) return false;
        // The depth's own rows: per-speaker faders, sleep, KEF inputs.
        return multi || featured.kind === "sonos" || featured.kind === "kef";
    });
</script>

<!-- The art, identical either side of the tap-through button, so the two
     branches can't drift. `.p-artbox` is the square itself — the frame that
     gives way when the card runs short — and the waveform hangs off it
     rather than off the flexible full-width slot it sits in, or it would
     float in the margin beside a shrunk cover. -->
{#snippet art()}
    {#if featured}
        <span class="p-artwrap" class:full>
            <span class="p-artbox">
                {#if featured.art}
                    <img class="p-art" src={featured.art} alt="" loading="lazy" />
                {:else}
                    <span class="p-art placeholder">[ art ]</span>
                {/if}
                {#if featured.playing}
                    <span class="p-wave"><Waveform /></span>
                {/if}
            </span>
        </span>
    {/if}
{/snippet}

{#if music.sources.length > 1}
    <div class="p-sources" role="group" aria-label="Room">
        {#each music.sources as s (s.key)}
            <button
                class="p-chip"
                class:active={featured?.key === s.key}
                aria-pressed={featured?.key === s.key}
                title={chipTitle(s.kind)}
                onclick={() => (music.selected = s.key)}
            >
                <!-- Which room is playing, readable from across the room —
                     without having to select each one to find out. -->
                {#if s.playing}<span class="p-chipwave"><Waveform /></span>{/if}
                {s.title}
            </button>
        {/each}
    </div>
{/if}

{#if featured}
    <article class="p-card" class:playing={featured.playing}>
        <!-- The card is two regions, and which one a control is in is the
             whole layout decision (§16). This one scrolls: the cover, what
             is playing, and the room's preferences. The strip below never
             does. A wall is read from across the room and tapped in
             passing, so the tapping half has to be where it was last time,
             and the cover gets whatever height is left over rather than
             being the first thing squeezed out by a sleep timer. -->
        <div class="p-scroll" class:has-extras={hasExtras}>
            {#if onOpen}
                <!-- Transport and volume stay out of the button so the
                     player still answers on the panel itself. -->
                <button
                    class="p-head p-open"
                    onclick={onOpen}
                    aria-label="Open music — {featured.trackTitle ??
                        (featured.playing ? 'playing' : 'nothing playing')} on {featured.title}"
                >
                    {@render art()}

                    <span class="p-track">
                        <span class="p-title">
                            {featured.trackTitle ?? (featured.playing ? "Playing" : "Not playing")}
                        </span>
                        <span class="p-subrow">
                            <span class="p-sub">{featured.trackSub || featured.title}</span>
                            <span class="p-go" aria-hidden="true"
                                ><Icon name="chevronRight" size={16} /></span
                            >
                        </span>
                    </span>
                </button>
            {:else}
                <div class="p-head">
                    {@render art()}

                    <div class="p-track">
                        <span class="p-title">
                            {featured.trackTitle ?? (featured.playing ? "Playing" : "Not playing")}
                        </span>
                        <span class="p-subrow">
                            <span class="p-sub">{featured.trackSub || featured.title}</span>
                        </span>
                    </div>
                </div>
            {/if}

            {#if !featured.standby}
                {#if gs}
                    <!-- Preferences, not device states, so chips rather than
                     switches — the same shape the full player gives them. -->
                    <div class="p-modeblock">
                        <div class="p-modes">
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
                            <!-- What happens after the last queued song: carry
                             on with the queue, or keep the room going with
                             music like it (§15.5). The hub's preference,
                             not the speaker's, but it reads as one more
                             play mode. -->
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
                        {#if full}
                            <!-- The choice only shows itself once the queue runs
                             out, so the depth's card says which way it will
                             go. -->
                            <p class="p-modenote">
                                {featured.autoplay
                                    ? "When the queue ends, similar music keeps playing."
                                    : "When the queue ends, playback stops."}
                            </p>
                        {/if}
                    </div>
                {/if}

                {#if full && onShowQueue}
                    {#if music.nextInQueue}
                        <!-- The queue's door, named for what's actually next (§15.8). -->
                        <button class="p-next" onclick={onShowQueue}>
                            <span class="n-label mono">Up next</span>
                            <span class="n-title">{music.nextInQueue.title ?? "Unknown track"}</span
                            >
                            <span class="p-go" aria-hidden="true"
                                ><Icon name="chevronRight" size={16} /></span
                            >
                        </button>
                    {:else if gs && gs.queue_length > 0 && !music.queueOrderKnown}
                        <!-- Shuffle picks its own next track and repeat-one
                         plays this one again, so naming a next track here
                         would be a guess. The door stays; the claim goes. -->
                        <button class="p-next" onclick={onShowQueue}>
                            <span class="n-label mono">Queue</span>
                            <span class="n-title"
                                >{gs.queue_length} songs — {gs.repeat === "one"
                                    ? "repeating this one"
                                    : "shuffled"}</span
                            >
                            <span class="p-go" aria-hidden="true"
                                ><Icon name="chevronRight" size={16} /></span
                            >
                        </button>
                    {/if}
                {/if}

                {#if full && multi}
                    <!-- One fader per speaker. The room-wide fader is pinned in
                     the strip below rather than sitting directly above
                     these, so the block names itself instead of leaning on
                     that adjacency. -->
                    <p class="p-sublabel mono">Speakers</p>
                    <div class="p-members">
                        {#each members as m (m.id)}
                            <div class="p-member">
                                <button
                                    class="v-ico"
                                    class:mute={m.muted}
                                    aria-label="{m.muted ? 'Unmute' : 'Mute'} {m.name}"
                                    disabled={music.busy["mute:" + m.id]}
                                    onclick={() => music.toggleMute(featured, m.id)}
                                >
                                    <Icon name={m.muted ? "volumeOff" : "volume"} size={16} />
                                </button>
                                <span class="m-name">{m.name}</span>
                                <Slider
                                    value={music.memVol[m.id] ?? m.volume}
                                    label="Volume {m.name}"
                                    valueText="{music.memVol[m.id] ?? m.volume}%"
                                    onInput={(v) => music.dragMemberVolume(m.id, v)}
                                    onChange={(v) => music.setMemberVolume(m.id, v)}
                                />
                                <span class="v-val mono">{music.memVol[m.id] ?? m.volume}</span>
                            </div>
                        {/each}
                    </div>
                {/if}

                {#if full && featured.kind === "sonos"}
                    <!-- Sleep timer: group-scoped like the play modes, and the
                     one setting the wall has more claim to than the phone. -->
                    <div class="p-sleep">
                        <span class="p-sleeplabel">
                            <Icon name="moon" size={15} />
                            <span>Sleep</span>
                            {#if sleepOn}
                                <span class="p-sleepleft mono">{music.sleepMinutes} min left</span>
                            {/if}
                        </span>
                        <div class="p-sleepchips" role="group" aria-label="Sleep timer">
                            <!-- No chip is marked "on": the speaker reports the
                             minutes *left*, not the length that was set, so
                             a highlighted chip would be a guess. The label
                             carries the truth instead. -->
                            {#each SLEEP_CHOICES as mins (mins)}
                                <button
                                    class="p-chip"
                                    disabled={music.busy["sleep:" + featured.id]}
                                    onclick={() => music.setSleep(mins)}
                                >
                                    {mins}m
                                </button>
                            {/each}
                            {#if sleepOn}
                                <button
                                    class="p-chip"
                                    disabled={music.busy["sleep:" + featured.id]}
                                    onclick={() => music.setSleep(0)}
                                >
                                    Off
                                </button>
                            {/if}
                        </div>
                    </div>
                {/if}

                {#if full && featured.kind === "kef"}
                    <!-- The input selector is the "play this" control: there is
                     no queue to point somewhere, so switching to the optical
                     input *is* "play the TV" (§15). -->
                    <div class="p-inputs" role="group" aria-label="Input">
                        {#each KEF_SOURCES as src (src.value)}
                            <button
                                class="p-chip"
                                class:active={featured.input === src.value}
                                aria-pressed={featured.input === src.value}
                                disabled={music.busy["src:" + featured.id]}
                                onclick={() => music.setKefSource(featured, src.value)}
                            >
                                {src.label}
                            </button>
                        {/each}
                    </div>
                {/if}
            {/if}
        </div>

        <!-- The pinned strip: where the track is, play/pause and skip, and
             how loud. These are what a wall gets walked up to for, so they
             hold the same place whatever else the room has to show. -->
        <div class="p-controls">
            {#if featured.standby}
                <!-- A speaker asleep is a speaker one tap from awake: the
                     wall wakes it rather than sending anyone to the full
                     view. -->
                <div class="p-standby">
                    <p>In standby</p>
                    <button
                        class="p-wakebtn"
                        disabled={music.busy["power:" + featured.id]}
                        onclick={() => music.wake(featured)}
                    >
                        <Icon name="power" size={18} /><span>Wake {featured.title}</span>
                    </button>
                </div>
            {:else}
                {#if queued}
                    <!-- The one thing queueing changes that can be seen —
                         so it belongs where the eye already is, not at the
                         far end of a scroll it may never reach. -->
                    <p class="p-queued">
                        <Icon name="check" size={14} />
                        <span
                            >{queued.next ? "Playing next" : "Added to the queue"} — {queued.title}</span
                        >
                    </p>
                {/if}

                <TrackRail
                    position={music.posSec}
                    duration={music.durSec}
                    seekable={music.seekable}
                    idle={railIdle}
                    liveLabel={railLabel}
                    onSeek={(sec) => music.seek(sec)}
                />

                <div class="p-transport">
                    {#if featured.canSkip}
                        <button
                            class="t-btn"
                            aria-label="Previous track"
                            disabled={music.busy["previous:" + featured.id]}
                            onclick={() => music.skip(featured, "previous")}
                        >
                            <Icon name="skipPrev" size={24} />
                        </button>
                    {/if}
                    <button
                        class="t-btn primary"
                        class:on={featured.playing}
                        aria-label={featured.playing ? "Pause" : "Play"}
                        disabled={music.busy["play:" + featured.id]}
                        onclick={() => music.togglePlay(featured)}
                    >
                        <Icon name={featured.playing ? "pause" : "play"} size={30} />
                    </button>
                    {#if featured.canSkip}
                        <button
                            class="t-btn"
                            aria-label="Next track"
                            disabled={music.busy["next:" + featured.id]}
                            onclick={() => music.skip(featured, "next")}
                        >
                            <Icon name="skipNext" size={24} />
                        </button>
                    {/if}
                </div>

                <div class="p-volume">
                    <button
                        class="v-ico"
                        class:mute={featured.muted}
                        aria-label={featured.muted ? "Unmute" : "Mute"}
                        disabled={music.busy["mute:" + featured.id]}
                        onclick={() => music.toggleMute(featured)}
                    >
                        <Icon name={featured.muted ? "volumeOff" : "volume"} size={18} />
                    </button>
                    <!-- A fader is an imprecise aim at arm's length, so the
                         wall also gets a discrete step either side of it. -->
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
            {/if}
        </div>
    </article>
{/if}

<style>
    .p-sources {
        display: flex;
        flex-wrap: wrap;
        gap: var(--space-2);
        margin-bottom: var(--space-3);
        flex-shrink: 0;
    }
    .p-chip {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        padding: 8px 14px;
        border-radius: var(--r-pill);
        border: 1px solid var(--hairline);
        background: var(--card-2);
        color: var(--text-mute);
        font-family: inherit;
        font-size: 13px;
        font-weight: 500;
        cursor: pointer;
        transition:
            background var(--t-fast),
            color var(--t-fast),
            border-color var(--t-fast),
            transform var(--t-fast);
    }
    .p-chip:active {
        transform: scale(0.95);
        transition-duration: 80ms;
    }
    .p-chip.active {
        background: var(--text);
        color: var(--bg);
        border-color: var(--text);
    }
    .p-chip:disabled {
        opacity: 0.55;
    }
    .p-chipwave {
        display: inline-flex;
        margin-right: 1px;
    }

    .p-card {
        /* How small the cover may get before the card starts scrolling
           instead. Shared, because .p-open's own floor is built from it. */
        --art-floor: 96px;
        flex: 1;
        min-height: 0;
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
        padding: var(--space-5);
        border-radius: var(--r-lg);
        border: 1px solid var(--hairline);
        background: var(--card);
        /* The card itself never scrolls now — .p-scroll inside it does, so
           the strip under it can't be pushed off the panel. */
        overflow: hidden;
        transition:
            background var(--t-med),
            border-color var(--t-med);
    }
    .p-card.playing {
        background: var(--tile-on-gradient);
        border-color: var(--tile-on-border);
    }

    /* Everything but the transport strip. It takes the card's leftover
       height and scrolls what doesn't fit — which, past the cover and the
       track, is only ever preferences. */
    .p-scroll {
        flex: 1 1 auto;
        min-height: 0;
        overflow-y: auto;
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
    }
    /* The transport strip, pinned. The hairline is the only thing that says
       the card is in two halves — without it the strip reads as the bottom
       of a list that happens to stop there, and there is nothing else on a
       kiosk (no scrollbar, no chrome) to say the half above it moves. */
    .p-controls {
        flex: none;
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
        padding-top: var(--space-3);
        border-top: 1px solid var(--hairline);
    }

    /* Cover + track title, as one block so the cover can never grow to the
       point of pushing the name of what is playing off the fold: the two
       together are capped at the scroll area, and .p-artwrap inside gives
       way to keep them there. Above that cap the cover stops at its own
       max-height and the preferences below start showing. */
    .p-head {
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
        /* Does not shrink: the cover's size is the scroll area's, not
           whatever the preferences below leave over. They scroll; this
           doesn't, so `max-height` is the only thing that sizes it and the
           cover inside gives way to hold the pair inside that cap. */
        flex: 0 0 auto;
        max-height: 100%;
        min-width: 0;
    }
    /* Something below to reach: hold back a sliver of it, so the half that
       scrolls looks like it does. */
    .p-scroll.has-extras .p-head {
        max-height: calc(100% - var(--space-10));
        /* The floor, spelled out. `min-height: 0` would waive the automatic
           minimum size and let this block be squeezed below art-floor +
           text, which is what used to slice the subtitle through the middle
           of its glyphs; `min-height: auto` can't state it either, since it
           would be read off .p-artwrap's *ratio* — a full-width square —
           and the cover could then never shrink at all. So: the art's own
           floor, the gap, and the two lines of meta under it. */
        min-height: calc(var(--art-floor) + var(--space-4) + 56px);
        /* Belt to those braces: nothing should reach the floor now, but a
           stray overflow crops rather than bleeding onto the strip. */
        overflow: hidden;
    }

    /* The tap-through button: reset to a plain flex column so it reads as
       the art + meta it contains, not as a button. */
    .p-open {
        padding: 0;
        border: 0;
        background: none;
        color: inherit;
        font: inherit;
        text-align: left;
        cursor: pointer;
        border-radius: var(--r-md);
        min-width: 0;
    }
    .p-open:focus-visible {
        box-shadow: var(--focus-ring);
    }

    /* The frame is what gives when the card runs out of room, never the
       controls (§16) — and it has to give as a *square*, on both axes at
       once. Sizing the art `width: 100%` + `aspect-ratio` derived its
       height from the *column* instead, so flexbox shrank the box it sat
       in while the <img> kept its full height, and the crop that was there
       to catch the overflow sliced every cover down to a letterbox strip
       of its top third.
       Height is the scarce axis on a 768px wall, so height leads: this
       slot carries the ratio (a full-width square at rest, flex-shrunk
       from there) and .p-artbox below takes its height from the slot and
       its width from the ratio. */
    .p-artwrap {
        display: flex;
        justify-content: center;
        flex: 0 1 auto;
        aspect-ratio: 1;
        max-height: 340px;
        /* The floor. Past it, .p-scroll takes over. */
        min-height: var(--art-floor);
    }
    /* The music depth's own card (`full`): its column is wider still, so
       the art — the biggest thing on it — grows to match rather than
       sitting at the dashboard's cap with empty margin either side. */
    .p-artwrap.full {
        max-height: 420px;
    }
    /* The square itself — the cover's actual box, so the radius, the crop
       and the waveform badge all key off it rather than off the slot,
       which stays column-wide. */
    .p-artbox {
        position: relative;
        height: 100%;
        aspect-ratio: 1;
        max-width: 100%;
        overflow: hidden;
        border-radius: var(--r-md);
    }
    .p-art {
        display: block;
        width: 100%;
        height: 100%;
        object-fit: cover;
    }
    span.p-art {
        font-size: 11px;
    }
    .p-wave {
        position: absolute;
        left: var(--space-3);
        bottom: var(--space-3);
        padding: 6px 8px;
        border-radius: var(--r-sm);
        background: var(--bg-bar);
        display: inline-flex;
    }

    .p-track {
        display: flex;
        flex-direction: column;
        gap: 4px;
        min-width: 0;
        flex-shrink: 0;
    }
    .p-title {
        font-size: 21px;
        font-weight: 600;
        letter-spacing: -0.02em;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .p-subrow {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-2);
        min-width: 0;
    }
    .p-sub {
        font-size: 14px;
        color: var(--text-mute);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .p-go {
        color: var(--text-dim);
        flex-shrink: 0;
        display: inline-flex;
    }

    .p-standby {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: var(--space-3);
        padding: var(--space-4) 0;
        flex-shrink: 0;
    }
    .p-standby p {
        margin: 0;
        font-size: 13px;
        color: var(--text-dim);
    }
    .p-wakebtn {
        display: inline-flex;
        align-items: center;
        gap: 8px;
        min-height: 48px;
        padding: 0 var(--space-5);
        border-radius: var(--r-pill);
        border: 1px solid var(--border-strong);
        background: var(--card-2);
        color: var(--text);
        font-family: inherit;
        font-size: 14px;
        font-weight: 500;
        cursor: pointer;
        transition:
            background var(--t-fast),
            transform var(--t-fast);
    }
    .p-wakebtn:active {
        transform: scale(0.96);
        transition-duration: 80ms;
    }
    .p-wakebtn:disabled {
        opacity: 0.55;
    }

    /* Transport sized for a wall poke: 64px sides, 80px centre. */
    .p-transport {
        display: flex;
        align-items: center;
        justify-content: center;
        gap: var(--space-5);
        flex-shrink: 0;
    }
    .t-btn {
        width: 64px;
        height: 64px;
        border-radius: 50%;
        border: 1px solid var(--hairline);
        background: var(--card-2);
        color: var(--text);
        display: grid;
        place-items: center;
        cursor: pointer;
        transition:
            background var(--t-fast),
            color var(--t-fast),
            border-color var(--t-fast),
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
        width: 80px;
        height: 80px;
        background: var(--on);
        border-color: var(--on);
        color: var(--primary-fg);
    }

    .p-modeblock {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: var(--space-2);
        flex-shrink: 0;
    }
    .p-modes {
        display: flex;
        flex-wrap: wrap; /* four chips don't fit the 352px column on one line */
        justify-content: center;
        gap: var(--space-2);
    }
    .p-modenote {
        margin: 0;
        font-size: 12.5px;
        color: var(--text-dim);
        text-align: center;
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

    /* Queued: the only visible trace an untouched player leaves. */
    .p-queued {
        display: flex;
        align-items: center;
        gap: 8px;
        margin: 0;
        padding: 10px var(--space-3);
        border-radius: var(--r-md);
        background: var(--on-soft);
        color: var(--on);
        font-size: 12.5px;
        flex-shrink: 0;
    }
    .p-queued span {
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .p-next {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        min-height: 48px;
        padding: 10px var(--space-3);
        border-radius: var(--r-md);
        border: 1px solid var(--hairline);
        background: var(--card-2);
        color: inherit;
        font: inherit;
        cursor: pointer;
        text-align: left;
        flex-shrink: 0;
        transition:
            background var(--t-fast),
            transform var(--t-fast);
    }
    .p-next:active {
        transform: scale(0.98);
        transition-duration: 80ms;
    }
    .n-label {
        font-size: 10.5px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-dim);
        flex-shrink: 0;
    }
    .n-title {
        flex: 1;
        min-width: 0;
        font-size: 14px;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    .p-volume {
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
    .v-ico:disabled {
        opacity: 0.5;
    }
    /* The ± steps: same hit area as the mute button, quieter ink. */
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
        transition:
            background var(--t-fast),
            transform var(--t-fast);
    }
    .v-step:active {
        transform: scale(0.92);
        transition-duration: 80ms;
    }
    .v-step:disabled {
        opacity: 0.5;
    }
    .v-val {
        font-size: 13px;
        color: var(--text-mute);
        width: 3ch;
        text-align: right;
        flex-shrink: 0;
    }

    /* Names the block the room-wide fader used to introduce by sitting
       right above it. Mono uppercase micro-label, per §4. */
    .p-sublabel {
        margin: 0 0 calc(var(--space-2) * -1);
        font-size: 10.5px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-dim);
        flex-shrink: 0;
    }
    .p-members {
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
        flex-shrink: 0;
    }
    .p-member {
        display: flex;
        align-items: center;
        gap: var(--space-2);
    }
    .m-name {
        width: 72px;
        flex-shrink: 0;
        font-size: 12.5px;
        color: var(--text-mute);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    .p-sleep {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
        flex-wrap: wrap;
        flex-shrink: 0;
    }
    .p-sleeplabel {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        font-size: 12.5px;
        color: var(--text-mute);
    }
    .p-sleepleft {
        font-size: 11px;
        color: var(--on);
    }
    .p-sleepchips {
        display: flex;
        gap: var(--space-2);
    }

    .p-inputs {
        display: flex;
        flex-wrap: wrap;
        gap: var(--space-2);
        flex-shrink: 0;
    }

    /* Distance-scaled targets: this is a wall, so every chip on it clears
       the §2 floor rather than inheriting a phone's sizing. */
    @media (pointer: coarse) {
        .p-chip,
        .p-mode {
            min-height: 44px;
            padding-inline: 16px;
        }
    }

    /* The reference wall panel is a 1024×768 iPad Air 2 (§16), where the
       card's whole stack — cover, meta, scrubber, transport, play modes,
       volume — lands within a dozen pixels of the column it has to fit.
       Tightening the card's own spacing there is what keeps the cover off
       its floor, and keeps the depth's extra rows from sitting as far
       under the fold. Nothing that is a touch target shrinks. */
    @media (max-height: 820px) and (orientation: landscape) {
        .p-card,
        .p-scroll,
        .p-controls {
            gap: var(--space-3);
        }
        .p-card {
            padding: var(--space-4);
        }
        .p-transport {
            gap: var(--space-4);
        }
    }

    /* Portrait stack: the whole page scrolls, so the card sizes to its
       content and neither region owns a scroll of its own — a pinned strip
       inside a page that already scrolls is just a shorter card. */
    @media (orientation: portrait), (max-width: 760px) {
        .p-artwrap,
        .p-artwrap.full {
            max-height: 280px;
        }
        .p-card {
            flex: none;
            overflow: visible;
        }
        .p-scroll {
            flex: none;
            overflow: visible;
        }
        .p-head {
            max-height: none;
        }
    }
</style>
