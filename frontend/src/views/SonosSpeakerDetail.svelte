<script lang="ts">
    // One speaker's device settings — the half of the Sonos app that isn't
    // playback. A screen, not a sheet: it opens from the Speakers screen,
    // which is a screen precisely so this one can push from it — a sheet must
    // never open another sheet (DESIGN.md §15). The
    // §11 detail shape (back chip · centered title · action chip, then a hero
    // card and secondary cards) rather than the §11 form shape, because there
    // is nothing to save — every control applies the moment it is touched.
    //
    // Playback deliberately isn't here: volume, mute and transport already
    // have a home in the full player, and a second identical set of controls
    // is exactly what DESIGN.md §15 warns against.
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
    import type { SonosSpeakerView, SonosSettings, SonosSettingsPatch } from "../lib/types";

    interface Props {
        speaker: SonosSpeakerView;
        /**
         * The zone that owns this speaker's sleep timer when it isn't this
         * speaker — the timer is group-scoped, like shuffle and repeat, and
         * lives on the coordinator. Null when this speaker leads its own
         * group (or stands alone), which is when the control is offered.
         */
        sleepTimerOwner?: string | null;
        onBack: () => void;
        /** Opens the speaker's HomeHub registration (name, room, address). */
        onEdit: () => void;
        /**
         * True when the speaker list is on screen beside this pane (desktop).
         * Then there is nothing to go "back" to and no need to repeat the list
         * as switcher chips — both only earn their space on a phone, where
         * this screen has replaced the list rather than sitting next to it.
         */
        paned?: boolean;
        /** The other speakers, for the phone switcher. */
        siblings?: SonosSpeakerView[];
        onPick?: (id: string) => void;
    }
    let {
        speaker,
        sleepTimerOwner = null,
        onBack,
        onEdit,
        paned = false,
        siblings = [],
        onPick,
    }: Props = $props();

    // Reloading when the pane switches to another speaker is the whole point
    // of the switcher, so the fetch is keyed on the id rather than on mount.
    $effect(() => {
        const id = speaker.id;
        settings = null;
        loaded = false;
        loadError = null;
        dragging = {};
        imageFailed = false;
        void load(id);
    });

    let settings = $state<SonosSettings | null>(null);
    let loaded = $state(false);
    let loadError = $state<string | null>(null);
    // Controls with a call in flight, keyed by field name.
    let busy = $state<Record<string, boolean>>({});
    let imageFailed = $state(false);

    const caps = $derived(settings?.capabilities);
    // The home-theatre block is empty on most models, so the whole card only
    // exists where the speaker answered for at least one of its parts.
    const hasTheatre = $derived(
        !!caps && (caps.night_mode || caps.dialog_level || caps.sub || caps.surround),
    );
    const canSleep = $derived(!sleepTimerOwner);

    async function load(id: string) {
        try {
            const s = await api.sonosSettings(id);
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

    /**
     * Slider positions the user is currently dragging, keyed by field. A
     * slider reads from here while it has an entry so the value follows the
     * thumb between the drag and the speaker's answer — and so a refusal can
     * put the *original* value back rather than the one that was refused.
     */
    let dragging = $state<Record<string, number>>({});
    /** What a slider should show: the live drag if there is one, else state. */
    function shown(field: PatchField, value: number | undefined): number {
        return dragging[field] ?? value ?? 0;
    }

    type PatchField = keyof SonosSettingsPatch;

    /**
     * Apply one field. The value moves on screen first so the control answers
     * the tap immediately, then rolls back — that field only — if the speaker
     * refuses. One field per call is also the backend's contract: it applies a
     * patch in order and stops at the first refusal, so a single field keeps
     * "what was refused" unambiguous.
     */
    async function apply(field: PatchField, value: number | boolean, label: string) {
        if (!settings || busy[field]) return;
        const before = settings[field];
        Object.assign(settings, { [field]: value }); // optimistic
        busy[field] = true;
        try {
            await api.sonosUpdateSettings(speaker.id, { [field]: value });
        } catch (e) {
            if (settings) Object.assign(settings, { [field]: before });
            toasts.error(`Couldn't change ${label}`, (e as Error).message);
        } finally {
            busy[field] = false;
            // Hand the slider back to state, whichever way the call went.
            delete dragging[field];
        }
    }

    // Sonos stores the lock, the Sonos app shows the controls — so the switch
    // reads "touch controls are on" and the wire value is its inverse. The
    // flip lives here rather than making every reader remember it.
    const touchOn = $derived(settings?.button_lock === false);
    function setTouch(on: boolean) {
        void apply("button_lock", !on, "touch controls");
    }

    // Tone values read as offsets, so the sign is part of the number.
    function signed(v: number): string {
        return v > 0 ? `+${v}` : String(v);
    }

    const SLEEP_CHOICES = [0, 15, 30, 45, 60, 90];
    function sleepLabel(mins: number): string {
        if (mins === 0) return "Off";
        if (mins % 60 === 0) return `${mins / 60} h`;
        return `${mins} min`;
    }

    // Read-only device block. Built as pairs so the markup stays one loop and
    // an absent field simply doesn't appear.
    const infoRows = $derived.by(() => {
        const i = settings?.info;
        const rows: { label: string; value: string }[] = [
            { label: "Model", value: speaker.model || settings?.display_name || "" },
            { label: "Model number", value: settings?.model_number ?? "" },
            { label: "Sonos room", value: i?.zone_name ?? "" },
            { label: "Serial", value: i?.serial_number ?? "" },
            { label: "Firmware", value: i?.software_version ?? "" },
            { label: "Hardware", value: i?.hardware_version ?? "" },
            { label: "MAC", value: i?.mac_address ?? "" },
            { label: "Address", value: speaker.ip },
            { label: "Device id", value: speaker.uuid },
        ];
        return rows.filter((r) => r.value !== "");
    });

    const subline = $derived(
        [settings?.display_name || speaker.model, speaker.room].filter(Boolean).join(" · ")
            || "Speaker settings",
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
         speakers ride along as chips. Changing one setting across the house
         shouldn't mean going back and forward for every room. On desktop the
         list is right there and this would just repeat it. -->
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

    <!-- Hero: the speaker's own portrait, published by the device in its
         description. No picture means the striped placeholder — never a
         stand-in that might show the wrong model (DESIGN.md §2). -->
    <section class="card hero">
        {#if imageFailed || (loaded && settings && !settings.has_image)}
            <!-- §6.7's striped fill, without its caption: the box is an
                 avatar and no wording fits it. The model name sits right
                 beside it, which is what a caption would have had to say. -->
            <div class="shot placeholder" aria-hidden="true"></div>
        {:else}
            <img
                class="shot"
                src={api.sonosImageURL(speaker.id)}
                alt=""
                loading="lazy"
                onerror={() => (imageFailed = true)}
            />
        {/if}
        <div class="hero-meta">
            <span class="hero-name">{settings?.display_name || speaker.model || speaker.name}</span>
            {#if settings?.model_number}
                <span class="hero-sub mono">{settings.model_number}</span>
            {/if}
            <span class="hero-ip mono">{speaker.ip}</span>
        </div>
    </section>

    {#if !loaded}
        <section class="card"><div class="skeleton sk"></div></section>
    {:else if loadError}
        <!-- The speaker didn't answer. Everything below would be a dead
             control, so say so instead of drawing it. -->
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
        <!-- ── Sound ──────────────────────────────────────────────────── -->
        <section class="card">
            <div class="card-header">
                <span class="c-ico" aria-hidden="true"><Icon name="sliders" size={16} /></span>
                <h2>Sound</h2>
            </div>

            {#if caps?.bass}
                <div class="row slider">
                    <span class="r-label" id="set-bass">Bass</span>
                    <input
                        type="range" min="-10" max="10" step="1"
                        aria-labelledby="set-bass"
                        disabled={busy.bass}
                        value={shown("bass", settings.bass)}
                        oninput={(e) => (dragging.bass = e.currentTarget.valueAsNumber)}
                        onchange={(e) => apply("bass", e.currentTarget.valueAsNumber, "bass")}
                    />
                    <span class="r-num mono">{signed(shown("bass", settings.bass))}</span>
                </div>
            {/if}

            {#if caps?.treble}
                <div class="row slider">
                    <span class="r-label" id="set-treble">Treble</span>
                    <input
                        type="range" min="-10" max="10" step="1"
                        aria-labelledby="set-treble"
                        disabled={busy.treble}
                        value={shown("treble", settings.treble)}
                        oninput={(e) => (dragging.treble = e.currentTarget.valueAsNumber)}
                        onchange={(e) => apply("treble", e.currentTarget.valueAsNumber, "treble")}
                    />
                    <span class="r-num mono">{signed(shown("treble", settings.treble))}</span>
                </div>
            {/if}

            {#if caps?.loudness}
                <div class="row">
                    <span class="r-meta">
                        <span class="r-label">Loudness</span>
                        <span class="r-help">Lifts bass and treble at low volume</span>
                    </span>
                    <Switch
                        checked={settings.loudness ?? false}
                        disabled={busy.loudness}
                        ariaLabel="Loudness"
                        onChange={(v) => apply("loudness", v, "loudness")}
                    />
                </div>
            {/if}

            {#if !caps?.bass && !caps?.treble && !caps?.loudness}
                <p class="c-none">This speaker reports no adjustable tone controls.</p>
            {/if}
        </section>

        <!-- ── Home theatre — only on models that answered for it ──────── -->
        {#if hasTheatre}
            <section class="card">
                <div class="card-header">
                    <span class="c-ico" aria-hidden="true"><Icon name="moon" size={16} /></span>
                    <h2>Home theatre</h2>
                </div>

                {#if caps?.night_mode}
                    <div class="row">
                        <span class="r-meta">
                            <span class="r-label">Night mode</span>
                            <span class="r-help">Softens loud effects, keeps quiet ones audible</span>
                        </span>
                        <Switch
                            checked={settings.night_mode ?? false}
                            disabled={busy.night_mode}
                            ariaLabel="Night mode"
                            onChange={(v) => apply("night_mode", v, "night mode")}
                        />
                    </div>
                {/if}

                {#if caps?.dialog_level}
                    <div class="row">
                        <span class="r-meta">
                            <span class="r-label">Speech enhancement</span>
                            <span class="r-help">Brings voices forward in the mix</span>
                        </span>
                        <Switch
                            checked={settings.dialog_level ?? false}
                            disabled={busy.dialog_level}
                            ariaLabel="Speech enhancement"
                            onChange={(v) => apply("dialog_level", v, "speech enhancement")}
                        />
                    </div>
                {/if}

                {#if caps?.surround}
                    <div class="row">
                        <span class="r-label">Surround speakers</span>
                        <Switch
                            checked={settings.surround ?? false}
                            disabled={busy.surround}
                            ariaLabel="Surround speakers"
                            onChange={(v) => apply("surround", v, "surround speakers")}
                        />
                    </div>
                {/if}

                {#if caps?.sub}
                    <div class="row">
                        <span class="r-label">Sub</span>
                        <Switch
                            checked={settings.sub_enabled ?? false}
                            disabled={busy.sub_enabled}
                            ariaLabel="Sub"
                            onChange={(v) => apply("sub_enabled", v, "the sub")}
                        />
                    </div>
                    {#if settings.sub_enabled && settings.sub_gain !== undefined}
                        <div class="row slider">
                            <span class="r-label" id="set-subgain">Sub level</span>
                            <input
                                type="range" min="-15" max="15" step="1"
                                aria-labelledby="set-subgain"
                                disabled={busy.sub_gain}
                                value={shown("sub_gain", settings.sub_gain)}
                                oninput={(e) => (dragging.sub_gain = e.currentTarget.valueAsNumber)}
                                onchange={(e) => apply("sub_gain", e.currentTarget.valueAsNumber, "the sub level")}
                            />
                            <span class="r-num mono">{signed(shown("sub_gain", settings.sub_gain))}</span>
                        </div>
                    {/if}
                {/if}
            </section>
        {/if}

        <!-- ── The speaker itself ──────────────────────────────────────── -->
        <section class="card">
            <div class="card-header">
                <span class="c-ico" aria-hidden="true"><Icon name="lock" size={16} /></span>
                <h2>Controls</h2>
            </div>

            {#if settings.led !== undefined}
                <div class="row">
                    <span class="r-meta">
                        <span class="r-label">Status light</span>
                        <span class="r-help">The small white light on the speaker</span>
                    </span>
                    <Switch
                        checked={settings.led}
                        disabled={busy.led}
                        ariaLabel="Status light"
                        onChange={(v) => apply("led", v, "the status light")}
                    />
                </div>
            {/if}

            {#if settings.button_lock !== undefined}
                <div class="row">
                    <span class="r-meta">
                        <span class="r-label">Touch controls</span>
                        <span class="r-help">
                            {touchOn
                                ? "Play, skip and volume respond on the speaker"
                                : "The speaker's own buttons are locked"}
                        </span>
                    </span>
                    <Switch
                        checked={touchOn}
                        disabled={busy.button_lock}
                        ariaLabel="Touch controls"
                        onChange={setTouch}
                    />
                </div>
            {/if}
        </section>

        <!-- ── Sleep timer ─────────────────────────────────────────────────
             Group-scoped, like shuffle and repeat: it lives on the
             coordinator. A follower gets a label naming the zone that owns
             it rather than a control that would be refused. -->
        <section class="card">
            <div class="card-header">
                <span class="c-ico" aria-hidden="true"><Icon name="timer" size={16} /></span>
                <h2>Sleep timer</h2>
                {#if canSleep && settings.sleep_minutes > 0}
                    <span class="c-tag mono">{settings.sleep_minutes} min left</span>
                {/if}
            </div>
            {#if canSleep}
                <div class="chips" role="radiogroup" aria-label="Sleep timer">
                    {#each SLEEP_CHOICES as mins (mins)}
                        <!-- Nothing is marked once a timer has ticked down
                             past its preset: 29 minutes left is not the
                             "30 min" choice, and the tag above says so. -->
                        {@const on = settings.sleep_minutes === mins}
                        <button
                            class="chip" class:on
                            role="radio" aria-checked={on}
                            disabled={busy.sleep_minutes}
                            onclick={() => apply("sleep_minutes", mins, "the sleep timer")}
                        >
                            {sleepLabel(mins)}
                        </button>
                    {/each}
                </div>
                <p class="c-none">Playback fades out and stops when the timer runs down.</p>
            {:else}
                <p class="c-none">
                    Grouped with <strong>{sleepTimerOwner}</strong> — the sleep timer belongs to
                    the whole zone, so set it there.
                </p>
            {/if}
        </section>

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
    /* Beside the list it is a pane header, not a detail head: nothing to go
       back to, so the title leads from the left instead of centring over an
       empty slot. */
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

    /* ── Hero: the device's portrait ── */
    .hero { display: flex; flex-direction: row; align-items: center; gap: var(--space-4); }
    .shot {
        width: 72px;
        height: 72px;
        flex-shrink: 0;
        border-radius: var(--r-md);
        object-fit: contain;
        background: var(--surface);
    }
    .hero-meta { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
    .hero-name { font-size: 15px; font-weight: 600; letter-spacing: -0.01em; }
    .hero-sub { font-size: 11px; color: var(--text-mute); }
    .hero-ip { font-size: 11px; color: var(--text-dim); }

    /* ── Cards ──
       The global rule is space-between, which would fling the title to the
       far edge away from its icon; here the icon leads the title. */
    .card-header {
        display: flex;
        align-items: center;
        justify-content: flex-start;
        gap: var(--space-2);
    }
    .c-ico { display: inline-flex; color: var(--text-dim); }
    .c-tag { margin-left: auto; font-size: 11px; color: var(--on); }
    .c-none {
        font-size: 12px;
        color: var(--text-mute);
        line-height: 1.5;
        margin-top: var(--space-2);
    }
    .c-none strong { color: var(--text); font-weight: 500; }

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
    .r-meta { display: flex; flex-direction: column; gap: 1px; flex: 1; min-width: 0; }
    .r-label { font-size: 13.5px; font-weight: 500; }
    /* A bare label (no sub-line) still has to push its control to the right
       edge, so it takes the free space rather than hugging its text. */
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
        background: var(--knob); border: 2px solid var(--knob-edge);
        cursor: pointer; box-shadow: 0 1px 4px rgba(0, 0, 0, 0.3);
    }
    input[type="range"]::-moz-range-thumb {
        width: 18px; height: 18px; border-radius: 50%;
        background: var(--knob); border: 2px solid var(--knob-edge);
        cursor: pointer; box-shadow: 0 1px 4px rgba(0, 0, 0, 0.3);
    }
    input[type="range"]:focus-visible { box-shadow: 0 0 0 2px var(--on-soft); }
    input[type="range"]:disabled { opacity: 0.5; }

    .chips { display: flex; flex-wrap: wrap; gap: var(--space-2); }

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
        /* Room for a bigger portrait once the column is wide. */
        .shot { width: 88px; height: 88px; }
        .dtitle h1 { font-size: 19px; }
    }
</style>
