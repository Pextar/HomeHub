<script lang="ts">
    // One KEF speaker's screen: playback at the top, then the settings that
    // stay put whatever is playing. The §11 detail shape (back chip · centred
    // title · action chip, then a hero card and secondary cards) and the
    // §11 detail branch rather than the form branch, because there is nothing
    // to save — every control applies the moment it is touched.
    //
    // It differs from SonosSpeakerDetail in one deliberate way: transport,
    // volume and the input selector *are* here. DESIGN.md §15 keeps them off
    // the Sonos pane because the full player already owns them — but a KEF
    // speaker has no full player. It has no queue, no grouping and no
    // favorites to fill one, so this screen is the only place its playback
    // lives, and leaving it out would mean the speaker could be configured
    // but not played. That is the exception, not a licence to duplicate the
    // Sonos controls.
    //
    // Each control is optimistic — the switch or slider moves immediately and
    // rolls back if the speaker refuses, because a control that waits for a
    // LAN round trip reads as a dropped tap.
    import { fly } from "svelte/transition";
    import { cubicOut } from "svelte/easing";
    import Icon from "../components/Icon.svelte";
    import Waveform from "../components/music/Waveform.svelte";
    import KEFSettingsCards from "./KEFSettingsCards.svelte";
    import { api } from "../lib/api";
    import { toasts } from "../lib/stores.svelte";
    import { dur } from "../lib/motion";
    import { KEF_SOURCES, kefSourceLabel } from "../lib/kef";
    import { clampVol, createVolumeThrottle } from "../lib/music/volume";
    import type {
        KEFSpeakerView,
        KEFSettings,
        KEFSource,
        KEFSpotifyView,
        SpotifyDevice,
    } from "../lib/types";

    interface Props {
        speaker: KEFSpeakerView;
        onBack: () => void;
        /** Opens the speaker's HomeHub registration (name, room, address). */
        onEdit: () => void;
        /**
         * True when the speaker list is on screen beside this pane (desktop).
         * Then there is nothing to go "back" to and no need to repeat the list
         * as switcher chips.
         */
        paned?: boolean;
        /** The other KEF speakers, for the phone switcher. */
        siblings?: KEFSpeakerView[];
        onPick?: (id: string) => void;
        /** Ask the parent to re-poll after a transport action. */
        onChanged?: () => void;
    }
    let {
        speaker,
        onBack,
        onEdit,
        paned = false,
        siblings = [],
        onPick,
        onChanged,
    }: Props = $props();

    // Reloading when the pane switches to another speaker is the whole point
    // of the switcher, so the fetch is keyed on the id rather than on mount.
    $effect(() => {
        const id = speaker.id;
        settings = null;
        loaded = false;
        loadError = null;
        connect = null;
        void load(id);
        void loadConnect(id);
    });

    let settings = $state<KEFSettings | null>(null);
    let loaded = $state(false);
    let loadError = $state<string | null>(null);
    /** Controls with a call in flight, keyed by field name. */
    let busy = $state<Record<string, boolean>>({});

    async function load(id: string) {
        try {
            const s = await api.kefSettings(id);
            // A slower answer for a speaker the user has already left must not
            // overwrite the one they are looking at now.
            if (id !== speaker.id) return;
            settings = s;
            loadError = null;
        } catch (e) {
            if (id !== speaker.id) return;
            loadError = (e as Error).message;
        } finally {
            if (id === speaker.id) loaded = true;
        }
    }

    // ── Playback ─────────────────────────────────────────────────────────

    const now = $derived(speaker.state);
    const poweredOn = $derived(now?.powered_on ?? false);
    /**
     * Optimistic overrides for the two controls whose answer arrives on the
     * next poll rather than in the response. Held for a few seconds so the
     * poll has time to agree; after that the speaker is the truth.
     */
    let playOverride = $state<{ playing: boolean; at: number } | null>(null);
    let volDrag = $state<number | null>(null);
    let volSentAt = 0;

    const playing = $derived.by(() => {
        const ov = playOverride;
        if (ov && Date.now() - ov.at < 6000) return ov.playing;
        return now?.playing ?? false;
    });
    const volume = $derived(volDrag ?? now?.volume ?? 0);
    /** Only the transport sources can be played; the analogue inputs can't. */
    const canTransport = $derived(
        poweredOn && (now?.source === "wifi" || now?.source === "bluetooth"),
    );

    async function run(key: string, fn: () => Promise<unknown>, errTitle: string) {
        if (busy[key]) return;
        busy[key] = true;
        try {
            await fn();
            onChanged?.();
        } catch (e) {
            toasts.error(errTitle, (e as Error).message);
            throw e;
        } finally {
            busy[key] = false;
        }
    }

    async function togglePlay() {
        const next = !playing;
        playOverride = { playing: next, at: Date.now() };
        try {
            await run(
                "play",
                () => (next ? api.kefPlay(speaker.id) : api.kefPause(speaker.id)),
                next ? "Couldn't start playback" : "Couldn't pause",
            );
        } catch {
            playOverride = null;
        }
    }

    function skip(dir: "next" | "previous") {
        void run(
            dir,
            () => (dir === "next" ? api.kefNext(speaker.id) : api.kefPrevious(speaker.id)),
            "Couldn't skip",
        ).catch(() => {});
    }

    // The speaker follows the finger down the slider rather than waiting for
    // it to lift — the same throttled send the Music view's faders use, so
    // one drag is a handful of calls instead of one per pixel.
    const volThrottle = createVolumeThrottle((_id, level) => {
        void api.kefSetVolume(speaker.id, level).catch(() => {});
    });

    function dragVolume(v: number) {
        volDrag = clampVol(v);
        volSentAt = Date.now();
        volThrottle.schedule(speaker.id, volDrag);
    }

    function setVolume(v: number) {
        volDrag = clampVol(v);
        volThrottle.cancel(speaker.id);
        volSentAt = Date.now();
        const sentAt = volSentAt;
        void run("volume", () => api.kefSetVolume(speaker.id, volDrag!), "Couldn't set the volume")
            .catch(() => {})
            .finally(() => {
                // Hand the slider back to the poll, unless the user has moved
                // it again in the meantime.
                if (volSentAt === sentAt) setTimeout(() => {
                    if (volSentAt === sentAt) volDrag = null;
                }, 2500);
            });
    }

    function toggleMute() {
        void run(
            "mute",
            () => api.kefSetMute(speaker.id, !(now?.muted ?? false)),
            "Couldn't change mute",
        ).catch(() => {});
    }

    function setSource(source: KEFSource) {
        void run("source", () => api.kefSetSource(speaker.id, source), "Couldn't switch input")
            .catch(() => {});
    }

    function setPower(on: boolean) {
        void run("power", () => api.kefSetPower(speaker.id, on), "Couldn't change power")
            .catch(() => {});
    }

    /** ms → M:SS. Used for the position line; KEF reports milliseconds. */
    function clock(ms: number): string {
        const total = Math.max(0, Math.round(ms / 1000));
        const s = String(total % 60).padStart(2, "0");
        const m = Math.floor(total / 60);
        if (m < 60) return `${m}:${s}`;
        return `${Math.floor(m / 60)}:${String(m % 60).padStart(2, "0")}:${s}`;
    }

    const nowLine = $derived.by(() => {
        if (!poweredOn) return "In standby";
        const t = now?.track;
        if (t?.title) return t.title;
        if (!now?.source) return "Idle";
        return kefSourceLabel(now.source) + " input";
    });
    const nowSub = $derived.by(() => {
        const t = now?.track;
        return [t?.artist, t?.album].filter(Boolean).join(" · ");
    });

    // ── Spotify Connect ──────────────────────────────────────────────────
    // The one thing on this screen that isn't the speaker's own doing. Its
    // local API can play, pause and skip but has no way to be *handed*
    // something, so starting music on it goes through Spotify Connect — and
    // that only works once HomeHub knows which Connect device this speaker is.
    // Normally the name matches and there is nothing to do here; this card
    // exists for when it doesn't, and to say plainly where the music will
    // come from when the Search screen sends something.
    //
    // Read on demand with the settings, never polled: which device a speaker
    // is doesn't change on its own. The card renders only when the read
    // succeeded — a home with no Spotify account must not see a dead section,
    // and setting Spotify up already has one home, on the Search screen.
    let connect = $state<KEFSpotifyView | null>(null);
    let connectBusy = $state(false);

    async function loadConnect(id: string) {
        try {
            const v = await api.kefSpotifyDevices(id);
            if (id !== speaker.id) return;
            connect = v;
        } catch {
            if (id === speaker.id) connect = null;
        }
    }

    /** Pin a device, or clear the pin by passing null (back to name matching). */
    function pinDevice(d: SpotifyDevice | null) {
        if (connectBusy) return;
        connectBusy = true;
        void api
            .kefSetSpotifyDevice(speaker.id, d?.id ?? "", d?.name ?? "")
            .then(() => loadConnect(speaker.id))
            .catch((e) => toasts.error("Couldn't set the Spotify device", (e as Error).message))
            .finally(() => (connectBusy = false));
    }

    // Read-only device block. Built as pairs so the markup stays one loop and
    // an absent field simply doesn't appear.
    const infoRows = $derived.by(() => {
        const i = settings?.info;
        const rows: { label: string; value: string }[] = [
            { label: "Model", value: speaker.model || i?.model || "" },
            { label: "Speaker name", value: i?.name ?? "" },
            { label: "Firmware", value: i?.firmware ?? "" },
            { label: "Release", value: i?.release ?? "" },
            { label: "MAC", value: i?.mac || speaker.mac },
            { label: "Address", value: speaker.ip },
        ];
        return rows.filter((r) => r.value !== "");
    });

    const subline = $derived(
        [speaker.model, speaker.room].filter(Boolean).join(" · ") || "Speaker settings",
    );
</script>

<div class="detail" in:fly={{ y: 10, duration: dur(240), easing: cubicOut }}>
    <!-- §11 detail head: back chip left, centered title block, action chip
         right. This is one level below the Speakers screen, which is itself
         one level below Home — a chain of pushes, never a sheet on a sheet. -->
    <div class="dhead" class:paned>
        {#if !paned}
            <button class="icon-btn" aria-label="Back to speakers" onclick={onBack}>
                <Icon name="chevronLeft" size={18} />
            </button>
        {/if}
        <div class="dtitle">
            <h1>{speaker.name}</h1>
            <span class="dsub">{subline}</span>
        </div>
        <button class="icon-btn" aria-label="Edit {speaker.name} in HomeHub" onclick={onEdit}>
            <Icon name="edit" size={16} />
        </button>
    </div>

    <!-- Phone switcher: the list is gone from the screen, so the other
         speakers ride along as chips. -->
    {#if !paned && siblings.length > 0}
        <div class="switch-row" role="tablist" aria-label="Other speakers">
            {#each siblings as sp (sp.id)}
                <button
                    class="chip"
                    role="tab"
                    aria-selected="false"
                    disabled={!sp.reachable}
                    onclick={() => onPick?.(sp.id)}
                >
                    {sp.name}
                </button>
            {/each}
        </div>
    {/if}

    {#if !speaker.reachable}
        <!-- The speaker didn't answer at all. Every control below would be
             dead, so say so instead of drawing them. -->
        <section class="card">
            <div class="unreach">
                <span class="unreach-title">Speaker unreachable</span>
                <span class="unreach-msg">
                    Nothing answered at <span class="mono">{speaker.ip}</span>. If it moved to a
                    new address, edit the registration above.
                </span>
                <button class="chip on" onclick={onEdit}>Edit address</button>
            </div>
        </section>
    {:else}
        <!-- ── Hero: playback ──────────────────────────────────────────
             The exception noted at the top of this file: a KEF speaker has
             no full player, so its transport lives here or nowhere. The
             playing surface is the sanctioned `.tile.on` look — the same
             "ON" gradient a lit device gets (§15). -->
        <section class="card hero" class:playing>
            <div class="hero-top">
                <span class="hero-ico" class:on={playing}>
                    {#if playing}
                        <Waveform />
                    {:else}
                        <Icon name="speaker" size={18} />
                    {/if}
                </span>
                <span class="hero-meta">
                    <span class="hero-name">{nowLine}</span>
                    {#if nowSub}<span class="hero-sub">{nowSub}</span>{/if}
                </span>
                {#if poweredOn && now?.duration_ms}
                    <span class="hero-time mono">
                        {clock(now.position_ms ?? 0)} / {clock(now.duration_ms)}
                    </span>
                {/if}
            </div>

            {#if poweredOn}
                <div class="transport">
                    <button
                        class="t-btn"
                        aria-label="Previous track"
                        disabled={!canTransport || busy.previous}
                        onclick={() => skip("previous")}
                    >
                        <Icon name="skipPrev" size={18} />
                    </button>
                    <button
                        class="t-play"
                        aria-label={playing ? "Pause" : "Play"}
                        disabled={!canTransport || busy.play}
                        onclick={togglePlay}
                    >
                        <Icon name={playing ? "pause" : "play"} size={20} />
                    </button>
                    <button
                        class="t-btn"
                        aria-label="Next track"
                        disabled={!canTransport || busy.next}
                        onclick={() => skip("next")}
                    >
                        <Icon name="skipNext" size={18} />
                    </button>
                    <!-- Honest about the source: the analogue and optical
                         inputs have no transport to drive, so the buttons say
                         why they're inert rather than pretending. -->
                    {#if !canTransport}
                        <span class="t-note">
                            Transport works on the {kefSourceLabel("wifi")} and
                            {kefSourceLabel("bluetooth")} sources
                        </span>
                    {/if}
                </div>

                <div class="vol-row">
                    <button
                        class="icon-btn"
                        aria-label={now?.muted ? "Unmute" : "Mute"}
                        disabled={busy.mute}
                        onclick={toggleMute}
                    >
                        <Icon name={now?.muted ? "volumeOff" : "volume"} size={18} />
                    </button>
                    <!-- Not disabled while a send is in flight: with a call
                         going out every throttle window, that would grey the
                         slider out under the finger holding it. -->
                    <input
                        type="range" min="0" max="100" step="1"
                        aria-label="Volume"
                        value={volume}
                        oninput={(e) => dragVolume(e.currentTarget.valueAsNumber)}
                        onchange={(e) => setVolume(e.currentTarget.valueAsNumber)}
                    />
                    <span class="r-num mono">{volume}</span>
                </div>
            {:else}
                <div class="standby">
                    <span class="standby-msg">
                        The speaker is in standby. Waking it selects the
                        {kefSourceLabel("wifi")} source.
                    </span>
                    <button class="chip on" disabled={busy.power} onclick={() => setPower(true)}>
                        <Icon name="power" size={14} /> Wake
                    </button>
                </div>
            {/if}
        </section>

        <!-- ── Source ─────────────────────────────────────────────────────
             KEF's input selector *is* the "play this" control: there is no
             queue to point somewhere, so switching to the optical input is
             the "play the TV" action. Chips, not a segmented control (§2). -->
        <section class="card">
            <div class="card-header">
                <span class="c-ico" aria-hidden="true"><Icon name="devices" size={16} /></span>
                <h2>Source</h2>
            </div>
            <div class="chips" role="radiogroup" aria-label="Source">
                {#each KEF_SOURCES as s (s.value)}
                    {@const on = poweredOn && now?.source === s.value}
                    <button
                        class="chip" class:on
                        role="radio" aria-checked={on}
                        disabled={busy.source}
                        onclick={() => setSource(s.value)}
                    >
                        {s.label}
                    </button>
                {/each}
            </div>
            <p class="c-none">
                Every model lists the same inputs here; one this speaker doesn't
                have will simply be refused.
            </p>
        </section>

        <!-- ── Spotify ────────────────────────────────────────────────────
             Where music comes from when the Search screen sends something
             here. The speaker's own API can't be handed content, so a search
             result reaches it through Spotify Connect — which means HomeHub
             has to know which Connect device this speaker is. It matches on
             the name by itself; this card is for when that isn't enough, and
             for saying which device it landed on when it is. Absent entirely
             when there is no Spotify account to ask (setup lives on Search). -->
        {#if connect}
            <section class="card">
                <div class="card-header">
                    <span class="c-ico" aria-hidden="true"><Icon name="musicNotes" size={16} /></span>
                    <h2>Spotify</h2>
                </div>

                <div class="row wrap">
                    <span class="r-meta">
                        <span class="r-label">Starts music on</span>
                        <span class="r-help">
                            Search plays here through Spotify Connect — the speaker's own
                            controls can't be handed a track.
                        </span>
                    </span>
                    {#if connect.device}
                        <span class="sp-dev">
                            <span class="mono">{connect.device.name}</span>
                            {#if connect.pinned_id}
                                <button class="chip" disabled={connectBusy}
                                    onclick={() => pinDevice(null)}>Match by name</button>
                            {/if}
                        </span>
                    {:else}
                        <span class="sp-dev none">Not paired yet</span>
                    {/if}
                </div>

                {#if connect.reason}
                    <p class="c-none">{connect.reason}</p>
                {/if}

                {#if connect.devices.length > 0}
                    <div class="row wrap">
                        <span class="r-meta">
                            <span class="r-label">Pick the device</span>
                            <span class="r-help">
                                Only speakers that are awake and signed in to this Spotify
                                account show up here.
                            </span>
                        </span>
                        <div class="chips" role="radiogroup" aria-label="Spotify Connect device">
                            {#each connect.devices as d (d.id)}
                                {@const on = connect.device?.id === d.id}
                                <button
                                    class="chip" class:on
                                    role="radio" aria-checked={on}
                                    disabled={connectBusy || d.restricted}
                                    onclick={() => pinDevice(d)}
                                >
                                    {d.name}
                                </button>
                            {/each}
                        </div>
                    </div>
                {/if}
            </section>
        {/if}

        {#if !loaded}
            <section class="card"><div class="skeleton sk"></div></section>
        {:else if loadError}
            <section class="card">
                <div class="unreach">
                    <span class="unreach-title">Settings unavailable</span>
                    <span class="unreach-msg">{loadError}</span>
                    <button class="chip on" onclick={() => { loaded = false; void load(speaker.id); }}>
                        Try again
                    </button>
                </div>
            </section>
        {:else if settings}
            <!-- ── Sound — only the parts this model answered for ───────── -->
            {#if settings}
                <KEFSettingsCards
                    {settings}
                    {busy}
                    {poweredOn}
                    speakerId={speaker.id}
                    onSetPower={setPower}
                />
            {/if}
            {#if infoRows.length > 0}
                <section class="card">
                    <div class="card-header">
                        <span class="c-ico" aria-hidden="true"><Icon name="info" size={16} /></span>
                        <h2>Device</h2>
                    </div>
                    <dl class="info">
                        {#each infoRows as r (r.label)}
                            <dt>{r.label}</dt>
                            <dd class="mono">{r.value}</dd>
                        {/each}
                    </dl>
                </section>
            {/if}
        {/if}
    {/if}
</div>

<style>
    /* ── §11 detail head ── */
    /* Beside the list it is a pane header, not a detail head. */

    /* ── Hero: playback ──
       Playing takes the sanctioned `.tile.on` surface — the same warm
       gradient a lit device gets. No music-only gradient exists (§15). */
    .hero { display: flex; flex-direction: column; gap: var(--space-3); }
    .hero.playing {
        background: var(--tile-on-gradient);
        border-color: var(--tile-on-border);
    }
    .hero-top { display: flex; align-items: center; gap: var(--space-3); }
    .hero-ico {
        width: 38px; height: 38px;
        flex-shrink: 0;
        display: grid; place-items: center;
        border-radius: var(--r-md);
        background: var(--card-2);
        color: var(--text-mute);
    }
    .hero-ico.on { background: var(--on-soft); color: var(--on); }
    .hero-meta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
    .hero-name {
        font-size: 14.5px; font-weight: 600; letter-spacing: -0.01em;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .hero-sub {
        font-size: 12px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .hero-time { font-size: 11px; color: var(--text-mute); flex-shrink: 0; }

    /* ── Transport ── */
    .transport {
        display: flex;
        align-items: center;
        justify-content: center;
        flex-wrap: wrap;
        gap: var(--space-3);
    }
    .t-btn, .t-play {
        display: grid;
        place-items: center;
        border-radius: 50%;
        border: 1px solid var(--hairline);
        background: var(--card-2);
        color: var(--text);
        cursor: pointer;
        width: 44px; height: 44px;
        transition: background 150ms ease, transform 150ms var(--spring);
    }
    .t-play {
        width: 52px; height: 52px;
        background: var(--on);
        border-color: transparent;
        color: var(--bg);
    }
    @media (hover: hover) {
        .t-btn:hover:not(:disabled), .t-play:hover:not(:disabled) { transform: scale(1.04); }
    }
    .t-btn:disabled, .t-play:disabled { opacity: 0.45; cursor: default; }
    .t-note {
        flex-basis: 100%;
        text-align: center;
        font-size: 11.5px;
        color: var(--text-mute);
    }

    .vol-row { display: flex; align-items: center; gap: var(--space-3); }

    .standby { display: flex; align-items: center; gap: var(--space-3); flex-wrap: wrap; }
    .standby-msg { flex: 1; min-width: 160px; font-size: 12px; color: var(--text-mute); line-height: 1.5; }

    @media (prefers-reduced-motion: reduce) {
        .t-btn, .t-play { transition-duration: 0.001ms; }
    }

    /* ── Cards ── */

    /* ── Unreachable ── */

    /* ── Rows: the §11 list shape, label left, control right ── */
    /* A chip group can't share a line with its label on a phone. */
    .row.wrap { flex-wrap: wrap; }

    /* ── Spotify block ── */
    .sp-dev {
        display: inline-flex; align-items: center; gap: var(--space-2);
        font-size: 12px; color: var(--text);
    }
    .sp-dev .mono { font-size: 11.5px; overflow-wrap: anywhere; }
    .sp-dev.none { color: var(--text-mute); }

    /* ── Device block ── */

    @media (pointer: coarse) {
        input[type="range"] { height: 10px; border-radius: 5px; }
        input[type="range"]::-webkit-slider-thumb { width: 26px; height: 26px; }
        input[type="range"]::-moz-range-thumb { width: 26px; height: 26px; }
    }

    @media (min-width: 601px) {
        .dtitle h1 { font-size: 19px; }
    }
</style>
