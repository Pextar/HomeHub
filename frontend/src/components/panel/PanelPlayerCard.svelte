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
</script>

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
        {#if onOpen}
            <!-- Transport and volume stay out of the button so the player
                 still answers on the panel itself. -->
            <button
                class="p-open"
                onclick={onOpen}
                aria-label="Open music — {featured.trackTitle ??
                    (featured.playing ? 'playing' : 'nothing playing')} on {featured.title}"
            >
                <span class="p-artwrap">
                    {#if featured.art}
                        <img class="p-art" src={featured.art} alt="" loading="lazy" />
                    {:else}
                        <span class="p-art placeholder">[ art ]</span>
                    {/if}
                    {#if featured.playing}
                        <span class="p-wave"><Waveform /></span>
                    {/if}
                </span>

                <span class="p-track">
                    <span class="p-title">
                        {featured.trackTitle ?? (featured.playing ? "Playing" : "Not playing")}
                    </span>
                    <span class="p-subrow">
                        <span class="p-sub">{featured.trackSub || featured.title}</span>
                        <span class="p-go" aria-hidden="true"><Icon name="chevronRight" size={16} /></span>
                    </span>
                </span>
            </button>
        {:else}
            <div class="p-artwrap">
                {#if featured.art}
                    <img class="p-art" src={featured.art} alt="" loading="lazy" />
                {:else}
                    <span class="p-art placeholder">[ art ]</span>
                {/if}
                {#if featured.playing}
                    <span class="p-wave"><Waveform /></span>
                {/if}
            </div>

            <div class="p-track">
                <span class="p-title">
                    {featured.trackTitle ?? (featured.playing ? "Playing" : "Not playing")}
                </span>
                <span class="p-subrow">
                    <span class="p-sub">{featured.trackSub || featured.title}</span>
                </span>
            </div>
        {/if}

        {#if featured.standby}
            <!-- A speaker asleep is a speaker one tap from awake: the wall
                 wakes it rather than sending anyone to the full view. -->
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

            {#if queued}
                <!-- The one thing queueing changes that can be seen. -->
                <p class="p-queued">
                    <Icon name="check" size={14} />
                    <span>{queued.next ? "Playing next" : "Added to the queue"} — {queued.title}</span>
                </p>
            {/if}

            {#if full && onShowQueue}
                {#if music.nextInQueue}
                    <!-- The queue's door, named for what's actually next (§15.8). -->
                    <button class="p-next" onclick={onShowQueue}>
                        <span class="n-label mono">Up next</span>
                        <span class="n-title">{music.nextInQueue.title ?? "Unknown track"}</span>
                        <span class="p-go" aria-hidden="true"><Icon name="chevronRight" size={16} /></span>
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
                        <span class="p-go" aria-hidden="true"><Icon name="chevronRight" size={16} /></span>
                    </button>
                {/if}
            {/if}

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
                <!-- A fader is an imprecise aim at arm's length, so the wall
                     also gets a discrete step either side of it. -->
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

            {#if full && multi}
                <!-- One fader per speaker, under the room-wide one — the
                     balance question a group or a zone always raises. -->
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
        flex: 1;
        min-height: 0;
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
        padding: var(--space-5);
        border-radius: var(--r-lg);
        border: 1px solid var(--hairline);
        background: var(--card);
        overflow-y: auto;
        transition:
            background var(--t-med),
            border-color var(--t-med);
    }
    .p-card.playing {
        background: var(--tile-on-gradient);
        border-color: var(--tile-on-border);
    }

    /* The tap-through button: reset to a plain flex column so it reads as
       the art + meta it contains, not as a button. */
    .p-open {
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
        padding: 0;
        border: 0;
        background: none;
        color: inherit;
        font: inherit;
        text-align: left;
        cursor: pointer;
        border-radius: var(--r-md);
        min-width: 0;
        /* The art is what gives first, so a tall card's controls stay put
           instead of scrolling off the bottom of the panel. */
        min-height: 0;
        flex-shrink: 1;
        /* min-height: 0 lets this whole button — art plus track text — get
           squeezed thinner than its children's combined min size. .p-artwrap
           clips its own overflow, but .p-track doesn't, so without clipping
           here too the title/subtitle bleed straight through onto the
           transport row underneath instead of handing off to .p-card's
           scroll like the floor above is supposed to. */
        overflow: hidden;
    }
    .p-open:focus-visible {
        box-shadow: var(--focus-ring);
    }

    .p-artwrap {
        position: relative;
        display: block;
        /* The art shrinks first when the card runs short on room (see
           .p-open above) — but an <img> sized by aspect-ratio doesn't
           actually shrink with its flex box, it just overflows past it
           and bleeds onto the transport row underneath. Clipping turns
           that into a crop instead of an overlap; the floor keeps the
           crop from going all the way to nothing — past it, .p-card's
           own overflow-y:auto takes over instead. */
        overflow: hidden;
        border-radius: var(--r-md);
        flex-shrink: 1;
        min-height: 96px;
    }
    /* Cover art is square, so the frame is too — a fixed height in a
       flexible column crops the top and bottom off every record. It gives
       way before the controls do when the card runs out of room. */
    .p-art {
        width: 100%;
        aspect-ratio: 1;
        max-height: 260px;
        object-fit: cover;
        border-radius: var(--r-md);
        display: block;
        margin-inline: auto;
    }
    span.p-art {
        font-size: 11px;
        aspect-ratio: auto;
        height: 200px;
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

    /* Portrait stack: the art shrinks so the transport stays reachable. */
    @media (orientation: portrait), (max-width: 760px) {
        .p-art {
            max-height: 200px;
        }
        .p-card {
            flex: none;
        }
    }
</style>
