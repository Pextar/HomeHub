<script lang="ts">
    /**
     * The kid player's fold-away half (DESIGN.md §17): how the music behaves
     * once it is playing, and the balance between the speakers playing it.
     *
     * These are set-and-forget, and together they are half a phone screen, so
     * they live behind the "More controls" disclosure — which keeps the fold
     * for what a finger actually returns to: the track, the transport, the
     * room's volume.
     *
     * Play modes are a Sonos group's, so they only appear where there is a
     * group state to read; the per-speaker faders only where there is more
     * than one speaker to balance. When neither holds, the disclosure that
     * opens this isn't drawn at all rather than opening on nothing.
     */
    import KidVolumeRow from "./KidVolumeRow.svelte";
    import { repeatLabel } from "../../lib/music/sonos.svelte";
    import type { PanelRooms, PanelSource, PanelTransport, PanelVolume } from "../../lib/panel-music/types";

    let {
        music,
        featured,
        /** The kid's ceiling — see KidVolumeRow. */
        volMax,
    }: {
        music: PanelRooms & PanelTransport & PanelVolume;
        featured: PanelSource;
        volMax: number;
    } = $props();

    const gs = $derived(featured.groupState);
    const members = $derived(featured.members ?? []);
    const multi = $derived(members.length > 1);

    const repeatText = $derived(
        gs?.repeat === "all" ? "Repeat all" : gs?.repeat === "one" ? "Repeat one" : "Repeat",
    );
</script>

<div class="km-extra">
    {#if gs}
        <div class="km-modes" role="group" aria-label="Play modes">
            <button
                class="km-mode"
                class:on={gs.shuffle}
                aria-pressed={gs.shuffle}
                disabled={music.busy["mode:" + featured.id]}
                onclick={() => music.toggleShuffle()}
            >
                🔀 Shuffle
            </button>
            <button
                class="km-mode"
                class:on={gs.repeat !== "off"}
                aria-pressed={gs.repeat !== "off"}
                aria-label={repeatLabel(gs.repeat)}
                disabled={music.busy["mode:" + featured.id]}
                onclick={() => music.cycleRepeat()}
            >
                {gs.repeat === "one" ? "🔂" : "🔁"} {repeatText}
            </button>
            <button
                class="km-mode"
                class:on={gs.crossfade}
                aria-pressed={gs.crossfade}
                disabled={music.busy["xfade:" + featured.id]}
                onclick={() => music.toggleCrossfade()}
            >
                ✨ Crossfade
            </button>
            <button
                class="km-mode"
                class:on={!!featured.autoplay}
                aria-pressed={!!featured.autoplay}
                disabled={music.busy["autoplay:" + featured.id]}
                onclick={() => music.toggleAutoplay()}
            >
                🎈 Play similar
            </button>
        </div>
        <p class="km-modenote">
            {featured.autoplay
                ? "When the songs run out, more like them keep playing 🎶"
                : "When the songs run out, the music stops."}
        </p>
    {/if}

    {#if multi}
        <!-- One fader per speaker under the room-wide one — the balance
             question a group always raises. -->
        <div class="km-members">
            {#each members as m (m.id)}
                <KidVolumeRow
                    value={Math.min(volMax, music.memberVol(m))}
                    max={volMax}
                    readout={music.memberVol(m)}
                    label="Volume {m.name}"
                    name={m.name}
                    muted={m.muted}
                    muteBusy={music.busy["mute:" + m.id]}
                    muteLabel="{m.muted ? 'Unmute' : 'Mute'} {m.name}"
                    onMute={() => music.toggleMute(featured, m.id)}
                    onInput={(v) => music.dragMemberVolume(m.id, v)}
                    onChange={(v) => music.setMemberVolume(m.id, v)}
                />
            {/each}
        </div>
    {/if}
</div>

<style>
    .km-extra {
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
    }

    .km-modes {
        display: flex;
        flex-wrap: wrap;
        gap: var(--space-2);
        justify-content: center;
    }
    .km-mode {
        font-size: 0.95rem;
        font-weight: 800;
        padding: 10px 16px;
        min-height: 48px;
        border-radius: 999px;
        border: 2px solid var(--border);
        background: var(--surface);
        color: var(--text-muted);
        cursor: pointer;
        transition: transform 0.12s ease, border-color 0.15s ease, background 0.15s ease, color 0.15s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .km-mode:active { transform: scale(0.94); }
    .km-mode.on {
        background: var(--kid-accent-soft);
        border-color: var(--kid-accent);
        color: var(--kid-accent);
    }
    .km-mode:disabled { opacity: 0.5; }
    .km-modenote {
        font-size: 0.85rem;
        font-weight: 600;
        color: var(--text-faint);
        text-align: center;
    }

    .km-members {
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
        border-top: 2px dashed var(--border);
        padding-top: var(--space-3);
    }
</style>
