<script lang="ts">
    /**
     * A KEF speaker's own settings, in three cards: how it sounds, what it
     * sends the subwoofer, and when it goes to sleep.
     *
     * Each card renders only where the speaker answered for the fields in
     * it — an LSX II has no sub output, and a control the speaker would
     * refuse is worse than one that isn't there (§15). The capability flags
     * live here rather than on the page because they are a property of
     * these cards and nothing else asks.
     *
     * Writing lives here too, with the sliders it serves: one field per
     * call so "what was refused" stays unambiguous, moved on screen first
     * so the control answers the tap, and rolled back — that field only —
     * if the speaker says no.
     */
    import Icon from "../components/Icon.svelte";
    import Switch from "../components/Switch.svelte";
    import { api } from "../lib/api";
    import { toasts } from "../lib/stores.svelte";
    import type { KEFSettings, KEFSettingsPatch } from "../lib/types";

    let {
        settings,
        busy,
        speakerId,
        poweredOn,
        onSetPower,
    }: {
        settings: KEFSettings;
        /**
         * Which write is in flight, keyed by field. Shared with the page,
         * whose transport controls key the same map — the namespaces are
         * disjoint, and one map is what keeps a settings write from
         * grewing out a second idea of what "busy" means.
         */
        busy: Record<string, boolean>;
        speakerId: string;
        poweredOn: boolean;
        onSetPower: (on: boolean) => void;
    } = $props();

    type PatchField = keyof KEFSettingsPatch;

    /**
     * Slider positions the user is currently dragging, keyed by field. A
     * slider reads from here while it has an entry so the value follows the
     * thumb between the drag and the speaker's answer — and so a refusal can
     * put the *original* value back rather than the one that was refused.
     */
    let dragging = $state<Record<string, number>>({});
    // The pane switches between speakers without remounting this component,
    // and a half-finished drag on one speaker is not a position on the next.
    $effect(() => {
        void speakerId;
        dragging = {};
    });
    function shown(field: PatchField, value: number | undefined): number {
        return dragging[field] ?? value ?? 0;
    }

    async function apply(field: PatchField, value: number | boolean | string, label: string) {
        if (busy[field]) return;
        const before = settings[field];
        Object.assign(settings, { [field]: value }); // optimistic
        busy[field] = true;
        try {
            await api.kefUpdateSettings(speakerId, { [field]: value } as KEFSettingsPatch);
        } catch (e) {
            Object.assign(settings, { [field]: before });
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
    // LSX II has no sub output.
    const hasSub = $derived(
        settings.subwoofer_out !== undefined ||
            settings.high_pass_mode !== undefined ||
            settings.sub_gain !== undefined,
    );
    const hasSound = $derived(
        settings.bass_extension !== undefined ||
            settings.desk_mode !== undefined ||
            settings.wall_mode !== undefined ||
            settings.treble !== undefined ||
            settings.phase_correction !== undefined,
    );
    const hasPower = $derived(
        settings.standby_mode !== undefined ||
            settings.volume_limit !== undefined ||
            settings.max_volume !== undefined,
    );
</script>

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
            <button class="chip" disabled={busy.power || !poweredOn} onclick={() => onSetPower(false)}>
                <Icon name="power" size={14} /> Standby
            </button>
        </div>
    </section>
{/if}

<style>
    /* dB and Hz readouts carry a unit or a decimal, so they need the room. */
    .r-num.wide {
        width: 6ch;
    }
    /* A slider row is the exception to the §11 list shape: the label is
       pinned narrow so the track keeps its travel instead of being squeezed
       by a long word. Wider still on touch, where the thumb is bigger. */
    @media (pointer: coarse) {
        .row.slider > .r-label {
            width: 66px;
        }
    }
</style>
