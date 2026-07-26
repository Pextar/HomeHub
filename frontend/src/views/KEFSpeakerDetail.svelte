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
    import Switch from "../components/Switch.svelte";
    import { api } from "../lib/api";
    import { toasts } from "../lib/stores.svelte";
    import { dur } from "../lib/motion";
    import { KEF_SOURCES, kefSourceLabel } from "../lib/kef";
    import type {
        KEFSpeakerView,
        KEFSettings,
        KEFSettingsPatch,
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
        dragging = {};
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

    function setVolume(v: number) {
        volDrag = v;
        volSentAt = Date.now();
        const sentAt = volSentAt;
        void run("volume", () => api.kefSetVolume(speaker.id, v), "Couldn't set the volume")
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
            .then(() => {
                toasts.success(
                    d ? "Spotify device set" : "Matching by name again",
                    d ? `${speaker.name} starts on "${d.name}"` : `Uses the name "${speaker.name}"`,
                );
                return loadConnect(speaker.id);
            })
            .catch((e) => toasts.error("Couldn't set the Spotify device", (e as Error).message))
            .finally(() => (connectBusy = false));
    }

    // ── Settings ─────────────────────────────────────────────────────────

    /**
     * Slider positions the user is currently dragging, keyed by field. A
     * slider reads from here while it has an entry so the value follows the
     * thumb between the drag and the speaker's answer — and so a refusal can
     * put the *original* value back rather than the one that was refused.
     */
    let dragging = $state<Record<string, number>>({});
    function shown(field: PatchField, value: number | undefined): number {
        return dragging[field] ?? value ?? 0;
    }

    type PatchField = keyof KEFSettingsPatch;

    /**
     * Apply one field. The value moves on screen first so the control answers
     * the tap immediately, then rolls back — that field only — if the speaker
     * refuses. One field per call keeps "what was refused" unambiguous.
     */
    async function apply(field: PatchField, value: number | boolean | string, label: string) {
        if (!settings || busy[field]) return;
        const before = settings[field];
        Object.assign(settings, { [field]: value }); // optimistic
        busy[field] = true;
        try {
            await api.kefUpdateSettings(speaker.id, { [field]: value } as KEFSettingsPatch);
        } catch (e) {
            if (settings) Object.assign(settings, { [field]: before });
            toasts.error(`Couldn't change ${label}`, (e as Error).message);
        } finally {
            busy[field] = false;
            delete dragging[field];
        }
    }

    /** Tenths of a dB → the signed number the KEF app shows. */
    function db(tenths: number): string {
        const v = tenths / 10;
        const s = v.toFixed(1);
        return v > 0 ? `+${s}` : s;
    }
    function signed(v: number): string {
        return v > 0 ? `+${v}` : String(v);
    }

    const BASS_CHOICES = [
        { value: "less", label: "Less" },
        { value: "standard", label: "Standard" },
        { value: "extra", label: "Extra" },
    ] as const;

    const STANDBY_CHOICES = [
        { value: "standby_none", label: "Never" },
        { value: "standby_20mins", label: "20 min" },
        { value: "standby_60mins", label: "60 min" },
    ] as const;

    // The subwoofer block only exists on models that answered for it — an
    // LSX II has no sub output, and a control the speaker would refuse is
    // worse than one that isn't there.
    const hasSub = $derived(
        !!settings &&
            (settings.subwoofer_out !== undefined ||
                settings.high_pass_mode !== undefined ||
                settings.sub_gain !== undefined),
    );
    const hasSound = $derived(
        !!settings &&
            (settings.bass_extension !== undefined ||
                settings.desk_mode !== undefined ||
                settings.wall_mode !== undefined ||
                settings.treble !== undefined ||
                settings.phase_correction !== undefined),
    );
    const hasPower = $derived(
        !!settings &&
            (settings.standby_mode !== undefined ||
                settings.volume_limit !== undefined ||
                settings.max_volume !== undefined),
    );

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
                        <span class="wave" aria-hidden="true"><i></i><i></i><i></i><i></i></span>
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
                    <input
                        type="range" min="0" max="100" step="1"
                        aria-label="Volume"
                        disabled={busy.volume}
                        value={volume}
                        oninput={(e) => (volDrag = e.currentTarget.valueAsNumber)}
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
            {#if hasSound}
                <section class="card">
                    <div class="card-header">
                        <span class="c-ico" aria-hidden="true"><Icon name="sliders" size={16} /></span>
                        <h2>Sound</h2>
                    </div>

                    {#if settings.bass_extension !== undefined}
                        <div class="row wrap">
                            <span class="r-meta">
                                <span class="r-label">Bass extension</span>
                                <span class="r-help">How far down the speaker reaches</span>
                            </span>
                            <div class="chips" role="radiogroup" aria-label="Bass extension">
                                {#each BASS_CHOICES as c (c.value)}
                                    {@const on = settings.bass_extension === c.value}
                                    <button
                                        class="chip" class:on
                                        role="radio" aria-checked={on}
                                        disabled={busy.bass_extension}
                                        onclick={() => apply("bass_extension", c.value, "bass extension")}
                                    >
                                        {c.label}
                                    </button>
                                {/each}
                            </div>
                        </div>
                    {/if}

                    {#if settings.desk_mode !== undefined}
                        <div class="row">
                            <span class="r-meta">
                                <span class="r-label">Desk mode</span>
                                <span class="r-help">Cuts the boost a desktop surface adds</span>
                            </span>
                            <Switch
                                checked={settings.desk_mode}
                                disabled={busy.desk_mode}
                                ariaLabel="Desk mode"
                                onChange={(v) => apply("desk_mode", v, "desk mode")}
                            />
                        </div>
                        {#if settings.desk_mode && settings.desk_gain !== undefined}
                            <div class="row slider">
                                <span class="r-label" id="kef-desk">Desk</span>
                                <input
                                    type="range" min="-60" max="0" step="5"
                                    aria-labelledby="kef-desk"
                                    disabled={busy.desk_gain}
                                    value={shown("desk_gain", settings.desk_gain)}
                                    oninput={(e) => (dragging.desk_gain = e.currentTarget.valueAsNumber)}
                                    onchange={(e) => apply("desk_gain", e.currentTarget.valueAsNumber, "the desk trim")}
                                />
                                <span class="r-num wide mono">{db(shown("desk_gain", settings.desk_gain))}</span>
                            </div>
                        {/if}
                    {/if}

                    {#if settings.wall_mode !== undefined}
                        <div class="row">
                            <span class="r-meta">
                                <span class="r-label">Wall mode</span>
                                <span class="r-help">Cuts the boost a nearby wall adds</span>
                            </span>
                            <Switch
                                checked={settings.wall_mode}
                                disabled={busy.wall_mode}
                                ariaLabel="Wall mode"
                                onChange={(v) => apply("wall_mode", v, "wall mode")}
                            />
                        </div>
                        {#if settings.wall_mode && settings.wall_gain !== undefined}
                            <div class="row slider">
                                <span class="r-label" id="kef-wall">Wall</span>
                                <input
                                    type="range" min="-60" max="0" step="5"
                                    aria-labelledby="kef-wall"
                                    disabled={busy.wall_gain}
                                    value={shown("wall_gain", settings.wall_gain)}
                                    oninput={(e) => (dragging.wall_gain = e.currentTarget.valueAsNumber)}
                                    onchange={(e) => apply("wall_gain", e.currentTarget.valueAsNumber, "the wall trim")}
                                />
                                <span class="r-num wide mono">{db(shown("wall_gain", settings.wall_gain))}</span>
                            </div>
                        {/if}
                    {/if}

                    {#if settings.treble !== undefined}
                        <div class="row slider">
                            <span class="r-label" id="kef-treble">Treble</span>
                            <input
                                type="range" min="-20" max="20" step="5"
                                aria-labelledby="kef-treble"
                                disabled={busy.treble}
                                value={shown("treble", settings.treble)}
                                oninput={(e) => (dragging.treble = e.currentTarget.valueAsNumber)}
                                onchange={(e) => apply("treble", e.currentTarget.valueAsNumber, "treble")}
                            />
                            <span class="r-num wide mono">{db(shown("treble", settings.treble))}</span>
                        </div>
                    {/if}

                    {#if settings.phase_correction !== undefined}
                        <div class="row">
                            <span class="r-meta">
                                <span class="r-label">Phase correction</span>
                                <span class="r-help">Tightens timing across the drivers</span>
                            </span>
                            <Switch
                                checked={settings.phase_correction}
                                disabled={busy.phase_correction}
                                ariaLabel="Phase correction"
                                onChange={(v) => apply("phase_correction", v, "phase correction")}
                            />
                        </div>
                    {/if}
                </section>
            {/if}

            <!-- ── Subwoofer — only on models with an output ─────────────── -->
            {#if hasSub}
                <section class="card">
                    <div class="card-header">
                        <span class="c-ico" aria-hidden="true"><Icon name="chart" size={16} /></span>
                        <h2>Subwoofer</h2>
                    </div>

                    {#if settings.subwoofer_out !== undefined}
                        <div class="row">
                            <span class="r-meta">
                                <span class="r-label">Subwoofer output</span>
                                <span class="r-help">Sends low frequencies to the sub out</span>
                            </span>
                            <Switch
                                checked={settings.subwoofer_out}
                                disabled={busy.subwoofer_out}
                                ariaLabel="Subwoofer output"
                                onChange={(v) => apply("subwoofer_out", v, "the subwoofer output")}
                            />
                        </div>
                    {/if}

                    {#if settings.subwoofer_out}
                        {#if settings.sub_gain !== undefined}
                            <div class="row slider">
                                <span class="r-label" id="kef-subgain">Sub level</span>
                                <input
                                    type="range" min="-10" max="10" step="1"
                                    aria-labelledby="kef-subgain"
                                    disabled={busy.sub_gain}
                                    value={shown("sub_gain", settings.sub_gain)}
                                    oninput={(e) => (dragging.sub_gain = e.currentTarget.valueAsNumber)}
                                    onchange={(e) => apply("sub_gain", e.currentTarget.valueAsNumber, "the sub level")}
                                />
                                <span class="r-num mono">{signed(shown("sub_gain", settings.sub_gain))}</span>
                            </div>
                        {/if}
                        {#if settings.sub_lp_freq !== undefined}
                            <div class="row slider">
                                <span class="r-label" id="kef-sublp">Low-pass</span>
                                <input
                                    type="range" min="40" max="250" step="5"
                                    aria-labelledby="kef-sublp"
                                    disabled={busy.sub_lp_freq}
                                    value={shown("sub_lp_freq", settings.sub_lp_freq)}
                                    oninput={(e) => (dragging.sub_lp_freq = e.currentTarget.valueAsNumber)}
                                    onchange={(e) => apply("sub_lp_freq", e.currentTarget.valueAsNumber, "the low-pass")}
                                />
                                <span class="r-num wide mono">{shown("sub_lp_freq", settings.sub_lp_freq)} Hz</span>
                            </div>
                        {/if}
                        {#if settings.sub_phase !== undefined}
                            <div class="row">
                                <span class="r-meta">
                                    <span class="r-label">Invert phase</span>
                                    <span class="r-help">Flips the sub 180° against the speakers</span>
                                </span>
                                <Switch
                                    checked={settings.sub_phase === "phase180"}
                                    disabled={busy.sub_phase}
                                    ariaLabel="Invert subwoofer phase"
                                    onChange={(v) => apply("sub_phase", v ? "phase180" : "phase0", "the sub phase")}
                                />
                            </div>
                        {/if}
                    {/if}

                    {#if settings.high_pass_mode !== undefined}
                        <div class="row">
                            <span class="r-meta">
                                <span class="r-label">High-pass</span>
                                <span class="r-help">Relieves the speakers of the lowest octave</span>
                            </span>
                            <Switch
                                checked={settings.high_pass_mode}
                                disabled={busy.high_pass_mode}
                                ariaLabel="High-pass"
                                onChange={(v) => apply("high_pass_mode", v, "the high-pass")}
                            />
                        </div>
                        {#if settings.high_pass_mode && settings.high_pass_freq !== undefined}
                            <div class="row slider">
                                <span class="r-label" id="kef-hp">Cut below</span>
                                <input
                                    type="range" min="50" max="120" step="5"
                                    aria-labelledby="kef-hp"
                                    disabled={busy.high_pass_freq}
                                    value={shown("high_pass_freq", settings.high_pass_freq)}
                                    oninput={(e) => (dragging.high_pass_freq = e.currentTarget.valueAsNumber)}
                                    onchange={(e) => apply("high_pass_freq", e.currentTarget.valueAsNumber, "the high-pass point")}
                                />
                                <span class="r-num wide mono">{shown("high_pass_freq", settings.high_pass_freq)} Hz</span>
                            </div>
                        {/if}
                    {/if}
                </section>
            {/if}

            <!-- ── Power & volume ─────────────────────────────────────────── -->
            {#if hasPower}
                <section class="card">
                    <div class="card-header">
                        <span class="c-ico" aria-hidden="true"><Icon name="power" size={16} /></span>
                        <h2>Power &amp; volume</h2>
                    </div>

                    {#if settings.standby_mode !== undefined}
                        <div class="row wrap">
                            <span class="r-meta">
                                <span class="r-label">Auto standby</span>
                                <span class="r-help">How long the speaker waits in silence</span>
                            </span>
                            <div class="chips" role="radiogroup" aria-label="Auto standby">
                                {#each STANDBY_CHOICES as c (c.value)}
                                    {@const on = settings.standby_mode === c.value}
                                    <button
                                        class="chip" class:on
                                        role="radio" aria-checked={on}
                                        disabled={busy.standby_mode}
                                        onclick={() => apply("standby_mode", c.value, "auto standby")}
                                    >
                                        {c.label}
                                    </button>
                                {/each}
                            </div>
                        </div>
                    {/if}

                    {#if settings.volume_limit !== undefined}
                        <div class="row">
                            <span class="r-meta">
                                <span class="r-label">Volume limit</span>
                                <span class="r-help">Caps how loud the speaker will go</span>
                            </span>
                            <Switch
                                checked={settings.volume_limit}
                                disabled={busy.volume_limit}
                                ariaLabel="Volume limit"
                                onChange={(v) => apply("volume_limit", v, "the volume limit")}
                            />
                        </div>
                        {#if settings.volume_limit && settings.max_volume !== undefined}
                            <div class="row slider">
                                <span class="r-label" id="kef-maxvol">Maximum</span>
                                <input
                                    type="range" min="0" max="100" step="1"
                                    aria-labelledby="kef-maxvol"
                                    disabled={busy.max_volume}
                                    value={shown("max_volume", settings.max_volume)}
                                    oninput={(e) => (dragging.max_volume = e.currentTarget.valueAsNumber)}
                                    onchange={(e) => apply("max_volume", e.currentTarget.valueAsNumber, "the maximum volume")}
                                />
                                <span class="r-num mono">{shown("max_volume", settings.max_volume)}</span>
                            </div>
                        {/if}
                    {/if}

                    <div class="row">
                        <span class="r-meta">
                            <span class="r-label">Standby now</span>
                            <span class="r-help">Sends the speaker to sleep straight away</span>
                        </span>
                        <button class="chip" disabled={busy.power || !poweredOn} onclick={() => setPower(false)}>
                            <Icon name="power" size={14} /> Standby
                        </button>
                    </div>
                </section>
            {/if}

            <!-- ── Device ─────────────────────────────────────────────────── -->
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
    .detail { display: flex; flex-direction: column; gap: var(--space-4); }
    .sk { height: 92px; border-radius: var(--r-md); }

    /* ── §11 detail head ── */
    .dhead {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        min-height: 44px;
    }
    .dtitle {
        flex: 1;
        min-width: 0;
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: 1px;
        text-align: center;
    }
    .dtitle h1 {
        font-size: 17px;
        font-weight: 600;
        letter-spacing: -0.02em;
        line-height: 1.1;
        max-width: 100%;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
    .dsub {
        font-size: 12px;
        color: var(--text-mute);
        max-width: 100%;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
    /* Beside the list it is a pane header, not a detail head. */
    .dhead.paned .dtitle { align-items: flex-start; text-align: left; }

    .switch-row {
        display: flex;
        gap: var(--space-2);
        overflow-x: auto;
        scrollbar-width: none;
        padding-bottom: 2px;
    }
    .switch-row::-webkit-scrollbar { display: none; }
    .switch-row .chip { flex-shrink: 0; }

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
    .t-btn:hover:not(:disabled), .t-play:hover:not(:disabled) { transform: scale(1.04); }
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

    /* ── Waveform (§6.8) — Music's "audio is moving" motif ── */
    .wave { display: flex; align-items: flex-end; gap: 2.5px; height: 13px; }
    .wave i {
        width: 2.5px; border-radius: 1px; background: var(--on); height: 4px;
        animation: wv 950ms ease-in-out infinite;
    }
    .wave i:nth-child(1) { animation-delay: 0s; }
    .wave i:nth-child(2) { animation-delay: 0.15s; }
    .wave i:nth-child(3) { animation-delay: 0.3s; }
    .wave i:nth-child(4) { animation-delay: 0.1s; }
    @keyframes wv { 0%, 100% { height: 3px; } 50% { height: 13px; } }
    @media (prefers-reduced-motion: reduce) {
        .wave i { animation: none; height: 8px; }
        .t-btn, .t-play { transition-duration: 0.001ms; }
    }

    /* ── Cards ── */
    .card-header {
        display: flex;
        align-items: center;
        justify-content: flex-start;
        gap: var(--space-2);
    }
    .c-ico { display: inline-flex; color: var(--text-dim); }
    .c-none {
        font-size: 12px;
        color: var(--text-mute);
        line-height: 1.5;
        margin-top: var(--space-2);
    }

    /* ── Unreachable ── */
    .unreach {
        display: flex;
        flex-direction: column;
        align-items: flex-start;
        gap: var(--space-2);
    }
    .unreach-title { font-size: 13.5px; font-weight: 600; }
    .unreach-msg { font-size: 12px; color: var(--text-mute); line-height: 1.5; }

    /* ── Rows: the §11 list shape, label left, control right ── */
    .row {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        min-height: 44px;
        padding: 6px 0;
        border-bottom: 1px solid var(--hairline);
    }
    .row:last-child { border-bottom: 0; }
    /* A chip group can't share a line with its label on a phone. */
    .row.wrap { flex-wrap: wrap; }
    .r-meta { display: flex; flex-direction: column; gap: 1px; flex: 1; min-width: 0; }
    .r-label { font-size: 13.5px; font-weight: 500; }
    .row > .r-label { flex: 1; min-width: 0; }
    .r-meta .r-label { flex: none; }
    .r-help { font-size: 11.5px; color: var(--text-mute); line-height: 1.4; }
    .r-num {
        font-size: 12px;
        color: var(--text-mute);
        width: 3ch;
        text-align: right;
        flex-shrink: 0;
    }
    /* dB and Hz readouts carry a unit or a decimal, so they need the room. */
    .r-num.wide { width: 6ch; }

    /* A slider row is the exception: the label is pinned narrow so the track
       keeps its travel instead of being squeezed by a long word. */
    .row.slider > .r-label { flex: none; width: 78px; }
    input[type="range"] {
        flex: 1;
        min-width: 60px;
        appearance: none;
        height: 6px;
        border-radius: 3px;
        outline: none;
        background: var(--card-3);
        accent-color: var(--on);
    }
    input[type="range"]::-webkit-slider-thumb {
        appearance: none;
        width: 18px; height: 18px; border-radius: 50%;
        background: #fff; border: 2px solid rgba(0, 0, 0, 0.35);
        cursor: pointer; box-shadow: 0 1px 4px rgba(0, 0, 0, 0.3);
    }
    input[type="range"]::-moz-range-thumb {
        width: 18px; height: 18px; border-radius: 50%;
        background: #fff; border: 2px solid rgba(0, 0, 0, 0.35);
        cursor: pointer; box-shadow: 0 1px 4px rgba(0, 0, 0, 0.3);
    }
    input[type="range"]:focus-visible { box-shadow: 0 0 0 2px var(--on-soft); }
    input[type="range"]:disabled { opacity: 0.5; }

    .chips { display: flex; flex-wrap: wrap; gap: var(--space-2); }

    /* ── Spotify block ── */
    .sp-dev {
        display: inline-flex; align-items: center; gap: var(--space-2);
        font-size: 12px; color: var(--text);
    }
    .sp-dev .mono { font-size: 11.5px; overflow-wrap: anywhere; }
    .sp-dev.none { color: var(--text-mute); }

    /* ── Device block ── */
    .info {
        display: grid;
        grid-template-columns: auto 1fr;
        gap: 6px var(--space-4);
        margin: 0;
        font-size: 12px;
    }
    .info dt { color: var(--text-mute); }
    .info dd {
        margin: 0;
        color: var(--text);
        font-size: 11.5px;
        overflow-wrap: anywhere;
        text-align: right;
    }

    @media (pointer: coarse) {
        input[type="range"] { height: 10px; border-radius: 5px; }
        input[type="range"]::-webkit-slider-thumb { width: 26px; height: 26px; }
        input[type="range"]::-moz-range-thumb { width: 26px; height: 26px; }
        .row.slider > .r-label { width: 66px; }
    }

    @media (min-width: 601px) {
        .dtitle h1 { font-size: 19px; }
    }
</style>
