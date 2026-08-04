<script lang="ts">
    import Icon from "../Icon.svelte";
    import Slider from "../music/Slider.svelte";
    import { KEF_SOURCES } from "../../lib/kef";
    import { repeatLabel } from "../../lib/music/sonos.svelte";
    import type { PanelMusicStore } from "../../lib/panel-music.svelte";

    // The featured room's own settings, on the music depth's Rooms pane.
    //
    // These used to be stacked under the cover in the player column, and
    // they were the reason nothing there fitted: play modes, a sleep timer,
    // one fader per speaker and a KEF input selector came to more height
    // than the column had, so the cover was squeezed to a third of its size
    // and the rest went behind a scroll with a half-chip peeking out of it.
    //
    // They belong here. The depth already names its panes Search / Queue /
    // Rooms, and Rooms is the "which device" surface (DESIGN.md §16.3) —
    // the room's preferences sit with the room. What is left in the player
    // column is a player: cover, track, scrubber, transport, volume.
    let { music }: { music: PanelMusicStore } = $props();

    const featured = $derived(music.featured);
    const gs = $derived(featured?.groupState);
    const members = $derived(featured?.members ?? []);
    const multi = $derived(members.length > 1);

    const repeatText = $derived(
        gs?.repeat === "all" ? "Repeat all" : gs?.repeat === "one" ? "Repeat one" : "Repeat",
    );

    // The wall's own setting: "stop in half an hour" is asked at the light
    // switch on the way to bed, not on a phone. Chips rather than a form —
    // there are no forms on a kiosk.
    const SLEEP_CHOICES = [15, 30, 60] as const;
    const sleepOn = $derived(music.sleepMinutes > 0);

    /** A HomeHub zone of one speaker has no play modes, no balance and no
     *  input to pick — nothing to put here, so nothing goes here. A block
     *  with only a heading in it is worse than no block. */
    const hasAny = $derived(
        !!gs || multi || featured?.kind === "sonos" || featured?.kind === "kef",
    );
</script>

{#if featured && !featured.standby && hasAny}
    <div class="rs">
        <h3 class="s-label">{featured.title}</h3>

        {#if gs}
            <!-- Preferences, not device states, so chips rather than
                 switches — the same shape the full player gives them. -->
            <div class="rs-modes">
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
                    <Icon name="activity" size={16} /><span>Crossfade</span>
                </button>
                <!-- What happens after the last queued song: carry on with
                     the queue, or keep the room going with music like it
                     (§15.5). The hub's preference, not the speaker's, but it
                     reads as one more play mode. -->
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
            <!-- The choice only shows itself once the queue has run out, so
                 the panel says which way it will go. -->
            <p class="rs-note">
                {featured.autoplay
                    ? "When the queue ends, similar music keeps playing."
                    : "When the queue ends, playback stops."}
            </p>
        {/if}

        {#if multi}
            <!-- One fader per speaker. The room-wide fader lives on the
                 player, so this block names itself rather than leaning on
                 sitting directly under it. -->
            <p class="rs-sub mono">Speakers</p>
            <div class="rs-members">
                {#each members as m (m.id)}
                    <div class="rs-member">
                        <button
                            class="v-ico"
                            class:mute={m.muted}
                            aria-label="{m.muted ? 'Unmute' : 'Mute'} {m.name}"
                            disabled={music.busy["mute:" + m.id]}
                            onclick={() => music.toggleMute(featured, m.id)}
                        >
                            <Icon name={m.muted ? "volumeOff" : "volume"} size={16} />
                        </button>
                        <span class="rs-mname">{m.name}</span>
                        <Slider
                            value={music.memVol[m.id] ?? m.volume}
                            label="Volume {m.name}"
                            valueText="{music.memVol[m.id] ?? m.volume}%"
                            onInput={(v) => music.dragMemberVolume(m.id, v)}
                            onChange={(v) => music.setMemberVolume(m.id, v)}
                        />
                        <span class="rs-val mono">{music.memVol[m.id] ?? m.volume}</span>
                    </div>
                {/each}
            </div>
        {/if}

        {#if featured.kind === "sonos"}
            <!-- Sleep timer: group-scoped like the play modes, and the one
                 setting the wall has more claim to than the phone. -->
            <div class="rs-row">
                <span class="rs-rowlabel">
                    <Icon name="moon" size={15} />
                    <span>Sleep</span>
                    {#if sleepOn}
                        <span class="rs-left mono">{music.sleepMinutes} min left</span>
                    {/if}
                </span>
                <div class="rs-chips" role="group" aria-label="Sleep timer">
                    <!-- No chip is marked "on": the speaker reports the
                         minutes *left*, not the length that was set, so a
                         highlighted chip would be a guess. The label carries
                         the truth instead. -->
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

        {#if featured.kind === "kef"}
            <!-- The input selector is the "play this" control: there is no
                 queue to point somewhere, so switching to the optical input
                 *is* "play the TV" (§15). -->
            <div class="rs-row">
                <span class="rs-rowlabel">
                    <Icon name="speaker" size={15} />
                    <span>Input</span>
                </span>
                <div class="rs-chips" role="group" aria-label="Input">
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
            </div>
        {/if}
    </div>
{/if}

<style>
    .rs {
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
        padding-bottom: var(--space-4);
        margin-bottom: var(--space-2);
        border-bottom: 1px solid var(--hairline);
        flex-shrink: 0;
    }
    .s-label {
        margin: 0;
        font-size: 11px;
        font-weight: 500;
        font-family: var(--font-mono);
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-dim);
    }

    .rs-modes {
        display: flex;
        flex-wrap: wrap;
        gap: var(--space-2);
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
    .rs-note {
        margin: 0;
        font-size: 12.5px;
        color: var(--text-dim);
    }

    .rs-sub {
        margin: var(--space-2) 0 0;
        font-size: 10.5px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-dim);
    }
    .rs-members {
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
    }
    .rs-member {
        display: flex;
        align-items: center;
        gap: var(--space-2);
    }
    .rs-mname {
        width: 96px;
        flex-shrink: 0;
        font-size: 12.5px;
        color: var(--text-mute);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
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
    .rs-val {
        font-size: 13px;
        color: var(--text-mute);
        width: 3ch;
        text-align: right;
        flex-shrink: 0;
    }

    .rs-row {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
        flex-wrap: wrap;
    }
    .rs-rowlabel {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        font-size: 12.5px;
        color: var(--text-mute);
    }
    .rs-left {
        font-size: 11px;
        color: var(--on);
    }
    .rs-chips {
        display: flex;
        flex-wrap: wrap;
        gap: var(--space-2);
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

    /* Distance-scaled targets: this is a wall, so every chip on it clears
       the §2 floor rather than inheriting a phone's sizing. */
    @media (pointer: coarse) {
        .p-chip,
        .p-mode {
            min-height: 44px;
            padding-inline: 16px;
        }
    }
</style>
