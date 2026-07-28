<script lang="ts">
    import Icon from "../Icon.svelte";
    import Waveform from "../music/Waveform.svelte";
    import Slider from "../music/Slider.svelte";
    import TrackRail from "../music/TrackRail.svelte";
    import { kefSourceLabel, KEF_SOURCES } from "../../lib/kef";
    import { repeatLabel } from "../../lib/music/sonos.svelte";
    import type { PanelMusicStore } from "../../lib/panel-music.svelte";

    // The featured source's player card, shared by both of the panel's
    // depths (DESIGN.md §16): the dashboard column shows it with its art as
    // the tap-through into music; the music depth shows it `full` — per-
    // member faders, the KEF input selector and the Up-next row into the
    // queue — since that is where the wall's music jobs happen.
    //
    // Every capability renders only where the room says it has one (§15):
    // the rail seeks on a Sonos track and is a read-only rail elsewhere,
    // play modes are a Sonos coordinator's, skips don't exist on KEF, and
    // standby renders as a label, never a dead control.
    let {
        music,
        onOpen = undefined,
        full = false,
        onShowQueue = undefined,
    }: {
        music: PanelMusicStore;
        /** Given, the art + meta are a button into the music depth. */
        onOpen?: () => void;
        /** The depth's richer card: member faders, KEF inputs, Up next. */
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
            : undefined, // TrackRail's default: "live stream — no track position"
    );

    const repeatText = $derived(
        gs?.repeat === "all" ? "Repeat all" : gs?.repeat === "one" ? "Repeat one" : "Repeat",
    );
</script>

{#if music.sources.length > 1}
    <div class="p-sources">
        {#each music.sources as s (s.key)}
            <button
                class="p-chip"
                class:active={featured?.key === s.key}
                onclick={() => (music.selected = s.key)}
            >
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
            <button class="p-open" onclick={onOpen} aria-label="Search music">
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
                        <span class="p-go" aria-hidden="true"><Icon name="chevronLeft" size={16} /></span>
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
            <!-- A refused control renders as a label, never dead. -->
            <div class="p-standby">In standby — wake it from the Music view</div>
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
                {#if featured.kind === "sonos"}
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
                {#if featured.kind === "sonos"}
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
                        <Icon name={gs.repeat === "one" ? "repeatOne" : "repeat"} size={16} /><span
                            >{repeatText}</span
                        >
                    </button>
                    <button
                        class="p-mode"
                        class:on={gs.crossfade}
                        aria-pressed={gs.crossfade}
                        disabled={music.busy["xfade:" + featured.id]}
                        onclick={() => music.toggleCrossfade()}
                    >
                        <span>Crossfade</span>
                    </button>
                </div>
            {/if}

            {#if full && music.nextInQueue && onShowQueue}
                <!-- The queue's door, named for what's actually next (§15.8). -->
                <button class="p-next" onclick={onShowQueue}>
                    <span class="n-label mono">Up next</span>
                    <span class="n-title">{music.nextInQueue.title ?? "Unknown track"}</span>
                    <span class="p-go" aria-hidden="true"><Icon name="chevronLeft" size={16} /></span>
                </button>
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
                <Slider
                    value={music.vol}
                    label="Volume"
                    valueText="{music.vol}%"
                    onInput={(v) => {
                        music.dragging = true;
                        music.vol = v;
                    }}
                    onChange={(v) => {
                        music.dragging = false;
                        music.setVolume(featured, v);
                    }}
                />
                <span class="v-val mono">{music.vol}</span>
            </div>

            {#if full && multi}
                <!-- One fader per speaker, under the room-wide one — the
                     balance question a group always raises. -->
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
                                onInput={(v) => {
                                    music.memDragging(m.id, true);
                                    music.memVol[m.id] = v;
                                }}
                                onChange={(v) => {
                                    music.memDragging(m.id, false);
                                    music.setMemberVolume(m.id, v);
                                }}
                            />
                            <span class="v-val mono">{music.memVol[m.id] ?? m.volume}</span>
                        </div>
                    {/each}
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
        flex-shrink: 0;
    }
    .p-open:focus-visible {
        box-shadow: var(--focus-ring);
    }

    .p-artwrap {
        position: relative;
        display: block;
        flex-shrink: 0;
    }
    .p-art {
        width: 100%;
        height: 200px;
        object-fit: cover;
        border-radius: var(--r-md);
        display: block;
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
        transform: rotate(180deg);
        display: inline-flex;
    }

    .p-standby {
        font-size: 13px;
        color: var(--text-dim);
        text-align: center;
        padding: var(--space-4) 0;
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

    .p-modes {
        display: flex;
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

    .p-next {
        display: flex;
        align-items: center;
        gap: var(--space-3);
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
        gap: var(--space-3);
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

    .p-inputs {
        display: flex;
        flex-wrap: wrap;
        gap: var(--space-2);
        flex-shrink: 0;
    }

    /* Portrait stack: the art shrinks so the transport stays reachable. */
    @media (orientation: portrait), (max-width: 760px) {
        .p-art {
            height: 160px;
        }
        .p-card {
            flex: none;
        }
    }
</style>
