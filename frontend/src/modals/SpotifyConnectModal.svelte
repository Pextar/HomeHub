<script lang="ts">
    /**
     * Spotify Connect — where this account is playing, and moving it.
     *
     * The thing to hold on to, because it is what makes this sheet different
     * from every other music surface in HomeHub: **the subject here is the
     * account's session, not the speakers.** A Spotify account has exactly one
     * active playback session, wherever in the world it is, and this is a
     * remote control for that one thing. Everywhere else in the Music view the
     * speakers are the subject and HomeHub decides how content reaches them.
     *
     * Three honesty rules follow, and they are most of the markup:
     *
     *   - These devices are Spotify's. A phone in somebody's pocket is on the
     *     list; a Sonos HomeHub drives over SOAP is not, because it plays
     *     without a Connect session at all. So the list never claims to be
     *     "your speakers" — the rows that *are* a HomeHub speaker say so, and
     *     the rest are named as what they are.
     *   - Moving the session takes it away from wherever it was. When that is
     *     a room HomeHub is decoding for, the backend names the room and this
     *     says so *before* the tap, not as the music stopping.
     *   - HomeHub's own decoder appears on the list while it is feeding a
     *     room. It is marked, and it is not offered as a destination:
     *     transferring to it by hand does nothing a user would want, because
     *     HomeHub starts that session itself when a zone plays.
     */
    import { onMount } from "svelte";
    import Modal from "../components/Modal.svelte";
    import Icon from "../components/Icon.svelte";
    import Slider from "../components/music/Slider.svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { toasts } from "../lib/stores.svelte";
    import type { SpotifyConnectDevice, SpotifyConnectView } from "../lib/types";

    /** Re-read while the sheet is open: a transfer lands a second or two
     *  later, and other people's phones join and leave the list on their own. */
    const POLL_MS = 5000;

    let view = $state<SpotifyConnectView | null>(null);
    let loaded = $state(false);
    let error = $state("");
    /** The device a transfer is in flight to, so its row can say so. */
    let moving = $state<string | null>(null);
    /** Local volume while a finger is on the slider — same rule as the
     *  speaker sliders: a control that fights the finger reads as broken. */
    let localVolume = $state<number | null>(null);
    let volumeAt = 0;

    async function refresh() {
        try {
            view = await api.spotifyConnect();
            error = "";
        } catch (e) {
            // The read carries the one actionable failure — a login without
            // the player scope — so it is shown in place rather than as a
            // toast that disappears.
            if (!loaded) error = (e as Error).message;
        } finally {
            loaded = true;
        }
    }

    onMount(() => {
        void refresh();
        const poll = setInterval(refresh, POLL_MS);
        return () => clearInterval(poll);
    });

    const playing = $derived(view?.playing ?? null);
    /** Everything that can be moved to: not the restricted ones, which accept
     *  no commands at all, and not HomeHub's own decoder. */
    const targets = $derived(
        (view?.devices ?? []).filter((d) => !d.restricted && !d.homehub),
    );
    const others = $derived(
        (view?.devices ?? []).filter((d) => d.restricted || d.homehub),
    );

    /** The active device's volume, unless a finger is on the slider. */
    const shownVolume = $derived(
        localVolume !== null && Date.now() - volumeAt < 2000
            ? localVolume
            : (playing?.volume ?? -1),
    );

    async function transfer(d: SpotifyConnectDevice) {
        if (moving || d.id === playing?.device_id) return;
        moving = d.id;
        try {
            await api.spotifyConnectTransfer(d.id, true);
            // Spotify treats `play` as a hint and the device takes a moment
            // to claim the session, so the truth comes from the next read
            // rather than from assuming it worked.
            await new Promise((r) => setTimeout(r, 900));
            await refresh();
        } catch (e) {
            toasts.error("Couldn't move playback", (e as Error).message);
        } finally {
            moving = null;
        }
    }

    function dragVolume(v: number) {
        localVolume = v;
        volumeAt = Date.now();
    }

    async function commitVolume(v: number) {
        const id = playing?.device_id;
        if (!id) return;
        localVolume = v;
        volumeAt = Date.now();
        try {
            await api.spotifyConnectVolume(id, v);
        } catch (e) {
            toasts.error("Volume failed", (e as Error).message);
        }
    }

    /** What kind of thing this is, in the words Spotify uses for it. */
    function deviceKind(d: SpotifyConnectDevice): string {
        if (d.homehub) return "HomeHub";
        if (d.speaker) return `${d.speaker} · HomeHub speaker`;
        return d.type || "Device";
    }

    /** The icon set has no phone glyph, so a handset falls back to the
     *  generic device one rather than borrowing a speaker's — a phone drawn
     *  as a speaker on a list of speakers is a small lie that costs a tap. */
    function iconFor(d: SpotifyConnectDevice): "speaker" | "devices" | "monitor" {
        const t = (d.type || "").toLowerCase();
        if (t.includes("computer")) return "monitor";
        if (t.includes("speaker") || t.includes("tv") || t.includes("avr")) return "speaker";
        return "devices";
    }
</script>

<Modal
    title="Spotify Connect"
    subtitle="Where this account is playing, and where else it could."
>
    {#snippet body()}
        {#if !loaded}
            <div class="skeleton sc-skeleton"></div>
        {:else if error}
            <p class="sc-error">{error}</p>
        {:else if !view}
            <p class="sc-quiet">Nothing could be read from Spotify.</p>
        {:else}
            <!-- ── What's playing, and where ────────────────────────────── -->
            <div class="eyrow">Playing now</div>
            {#if playing}
                <div class="sc-now">
                    {#if playing.item?.art_url}
                        <img class="sc-art" src={playing.item.art_url} alt="" />
                    {:else}
                        <span class="sc-art placeholder" aria-hidden="true"></span>
                    {/if}
                    <span class="sc-now-meta">
                        <span class="sc-now-title">
                            {playing.item?.name ?? "Something without a name"}
                        </span>
                        {#if playing.item?.sub}
                            <span class="sc-now-sub">{playing.item.sub}</span>
                        {/if}
                        <span class="sc-now-where mono">
                            {playing.playing ? "▶" : "❙❙"}
                            {playing.device_name ?? "an unnamed device"}
                        </span>
                    </span>
                </div>
                <!-- Only where there is one to move: a device with no volume
                     of its own reports -1, and a slider drawn at silence for
                     it would be a lie about the audio. -->
                {#if shownVolume >= 0}
                    <div class="sc-vol">
                        <Icon name="volume" size={14} />
                        <Slider
                            value={shownVolume}
                            label={`${playing.device_name ?? "Device"} volume`}
                            valueText={`${shownVolume}%`}
                            onInput={dragVolume}
                            onChange={(v) => void commitVolume(v)}
                        />
                        <span class="sc-vol-num mono">{shownVolume}</span>
                    </div>
                {/if}
            {:else}
                <p class="sc-quiet">
                    This account isn't playing anywhere right now. Pick a device
                    below and start something from Spotify, or play to a room
                    from HomeHub.
                </p>
            {/if}

            <!-- The warning that has to come before the tap, not after. -->
            {#if view.interrupts}
                <p class="sc-warn">
                    <Icon name="info" size={13} />
                    <span>
                        Moving playback stops <strong>{view.interrupts}</strong> —
                        one Spotify session at a time.
                    </span>
                </p>
            {/if}

            <!-- ── Where it could go ────────────────────────────────────── -->
            <div class="eyrow" style="margin-top:var(--space-5)">Move it to</div>
            {#if targets.length === 0}
                <p class="sc-quiet">
                    No other device is offering itself right now. A speaker
                    appears here while it is awake and signed in to this
                    account; a phone appears while Spotify is open on it.
                </p>
            {:else}
                <div class="sc-list">
                    {#each targets as d (d.id)}
                        {@const here = d.id === playing?.device_id}
                        <button
                            type="button"
                            class="sc-row"
                            class:on={here}
                            disabled={here || moving !== null}
                            onclick={() => void transfer(d)}
                        >
                            <Icon name={iconFor(d)} size={18} />
                            <span class="sc-row-meta">
                                <span class="sc-row-name">{d.name}</span>
                                <span class="sc-row-sub">{deviceKind(d)}</span>
                            </span>
                            {#if moving === d.id}
                                <span class="sc-tag mono">MOVING…</span>
                            {:else if here}
                                <span class="sc-tag mono">HERE</span>
                            {/if}
                        </button>
                    {/each}
                </div>
            {/if}

            <!-- Listed rather than hidden: a device somebody can see in their
                 own Spotify app and not here reads as a bug. Both kinds are
                 shown with the reason they can't be picked. -->
            {#if others.length > 0}
                <div class="eyrow" style="margin-top:var(--space-4)">Also on the account</div>
                <div class="sc-list">
                    {#each others as d (d.id)}
                        <div class="sc-row static">
                            <Icon name={iconFor(d)} size={18} />
                            <span class="sc-row-meta">
                                <span class="sc-row-name">{d.name}</span>
                                <span class="sc-row-sub">
                                    {#if d.homehub}
                                        HomeHub's own decoder — it takes the session
                                        itself when you play to a room
                                    {:else}
                                        {d.type} · Spotify accepts no commands for this one
                                    {/if}
                                </span>
                            </span>
                        </div>
                    {/each}
                </div>
            {/if}
        {/if}
    {/snippet}
    {#snippet actions()}
        <button class="btn btn-ghost" onclick={() => closeModal()}>Done</button>
    {/snippet}
</Modal>

<style>
    .sc-skeleton {
        height: 200px;
        border-radius: var(--r-md);
    }
    .sc-quiet,
    .sc-error {
        font-size: 12.5px;
        line-height: 1.45;
        color: var(--text-mute);
        margin: var(--space-2) 0 0;
    }
    .sc-error {
        color: var(--bad);
    }
    .sc-now {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        margin-top: var(--space-3);
    }
    .sc-art {
        width: 56px;
        height: 56px;
        border-radius: var(--r-sm);
        object-fit: cover;
        flex-shrink: 0;
        background: var(--card-2);
    }
    .sc-art.placeholder {
        border: 1px dashed var(--border);
    }
    .sc-now-meta {
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 2px;
    }
    .sc-now-title {
        font-size: 14px;
        font-weight: 600;
        color: var(--text);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
    .sc-now-sub {
        font-size: 12px;
        color: var(--text-mute);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
    .sc-now-where {
        font-size: 11px;
        color: var(--on);
        letter-spacing: 0.02em;
    }
    .sc-vol {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        margin-top: var(--space-3);
        color: var(--text-mute);
    }
    .sc-vol :global(input[type="range"]) {
        flex: 1;
    }
    .sc-vol-num {
        font-size: 11px;
        min-width: 2.5ch;
        text-align: right;
        color: var(--text-dim);
    }
    .sc-warn {
        display: flex;
        align-items: flex-start;
        gap: 6px;
        margin: var(--space-3) 0 0;
        font-size: 12px;
        line-height: 1.4;
        color: var(--text-dim);
    }
    .sc-warn :global(svg) {
        flex-shrink: 0;
        margin-top: 2px;
    }
    .sc-warn strong {
        color: var(--text);
        font-weight: 600;
    }
    .sc-list {
        display: flex;
        flex-direction: column;
        gap: 6px;
        margin-top: var(--space-3);
    }
    .sc-row {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: 10px 12px;
        min-height: 44px;
        background: var(--card-2);
        border: 1px solid var(--hairline);
        border-radius: var(--r-md);
        color: var(--text);
        text-align: left;
        font: inherit;
        cursor: pointer;
        transition:
            background 150ms ease,
            border-color 150ms ease;
    }
    .sc-row:hover:not(:disabled):not(.static) {
        background: var(--card-3);
    }
    .sc-row.on {
        border-color: var(--on);
        color: var(--on);
    }
    .sc-row:disabled {
        cursor: default;
    }
    .sc-row.static {
        cursor: default;
        opacity: 0.7;
    }
    .sc-row-meta {
        flex: 1;
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 1px;
    }
    .sc-row-name {
        font-size: 13.5px;
        font-weight: 500;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
    .sc-row-sub {
        font-size: 11.5px;
        color: var(--text-mute);
        line-height: 1.35;
    }
    .sc-tag {
        font-size: 10px;
        letter-spacing: 0.08em;
        color: var(--text-dim);
    }
    .sc-row.on .sc-tag {
        color: var(--on);
    }
</style>
