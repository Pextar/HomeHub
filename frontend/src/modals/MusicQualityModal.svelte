<script module lang="ts">
    import type { MediaQuality } from "../lib/types";

    /**
     * A quality as equipment labels it. Kept here rather than in the backend's
     * own `label` because this sheet shows codec and numbers in different
     * weights, and re-splitting a formatted string to do that would be worse
     * than formatting twice.
     */
    export function formatQuality(q: MediaQuality): string {
        const codec = codecName(q.codec);
        if (q.lossless && q.sample_rate && q.bit_depth) {
            return `${codec} ${khz(q.sample_rate)} · ${q.bit_depth}-bit`;
        }
        if (q.bitrate_kbps) {
            return `${codec} ${q.approximate ? "up to " : ""}${q.bitrate_kbps} kbps`;
        }
        if (q.sample_rate) return `${codec} ${khz(q.sample_rate)}`;
        return codec;
    }

    function codecName(c: MediaQuality["codec"]): string {
        switch (c) {
            case "pcm":
                return "PCM";
            case "alac":
                return "ALAC";
            case "flac":
                return "FLAC";
            case "vorbis":
                return "Ogg Vorbis";
            case "aac":
                return "AAC";
            case "mp3":
                return "MP3";
            default:
                return "Unknown";
        }
    }

    /** 44.1 kHz, the way the number is written on the equipment. */
    function khz(hz: number): string {
        return hz % 1000 === 0 ? `${hz / 1000} kHz` : `${(hz / 1000).toFixed(1)} kHz`;
    }
</script>

<script lang="ts">
    /**
     * "Am I hearing lossless, and if not, why not?"
     *
     * The honest answer has two halves, and this sheet is shaped around
     * keeping them apart. The audio passes through two hands — the service
     * that encoded it, and the path it took to the speaker — so a lossless
     * path carrying a lossy source is not lossless. A single badge would have
     * to pick one of those to lie about; a chain with the weak link named
     * doesn't.
     *
     * The second half is what can actually be changed. There is exactly one
     * lever in this system: how hard HomeHub's own decoder asks the service to
     * compress, and it only moves the two routes HomeHub decodes for. On the
     * others the speaker holds the account and negotiates for itself. So the
     * picker sits under a heading that says what it reaches, and the routes it
     * doesn't reach say so in their own rows rather than being quietly absent.
     *
     * What this sheet must never do is offer to make Spotify lossless. It
     * cannot be done, the backend says so, and a control that implies
     * otherwise is worse than no control.
     */
    import { onMount } from "svelte";
    import Modal from "../components/Modal.svelte";
    import Icon from "../components/Icon.svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { toasts } from "../lib/stores.svelte";
    import type { MediaQualityReport, StreamQuality } from "../lib/types";

    let report = $state<MediaQualityReport | null>(null);
    let loaded = $state(false);
    let saving = $state<StreamQuality | null>(null);

    async function refresh() {
        try {
            report = await api.mediaQuality();
        } catch (e) {
            if (!loaded) toasts.error("Couldn't read quality", (e as Error).message);
        } finally {
            loaded = true;
        }
    }

    onMount(refresh);

    async function choose(value: StreamQuality) {
        if (saving || report?.stream_quality === value) return;
        saving = value;
        try {
            const res = await api.setMediaQuality(value);
            // Said out loud, because it is the surprising part: the bitrate is
            // baked into the decoder's command line, so what is playing keeps
            // playing at the old one rather than being cut off to improve it.
            toasts.success("Quality changed", `Applies ${res.applies}.`);
            await refresh();
        } catch (e) {
            toasts.error("Couldn't change quality", (e as Error).message);
        } finally {
            saving = null;
        }
    }

    /** The routes HomeHub decodes for, which are the ones the picker moves. */
    const decoded = $derived(
        report?.providers.flatMap((p) => p.routes.filter((r) => r.decoded)) ?? [],
    );
    const speakerServed = $derived(
        report?.providers.flatMap((p) => p.routes.filter((r) => !r.decoded)) ?? [],
    );
</script>

<Modal
    title="Sound quality"
    subtitle="What reaches your speakers, and the one part of it you can change."
>
    {#snippet body()}
        {#if !loaded}
            <div class="skeleton q-skeleton"></div>
        {:else if !report}
            <p class="q-empty">Nothing could be read about playback quality.</p>
        {:else}
            <!-- ── What it sounds like now ──────────────────────────────
                 Per route, because the answer genuinely differs by route and
                 a single figure would be wrong in at least one room. -->
            <div class="eyrow">Right now</div>
            <div class="q-chains">
                {#each [...decoded, ...speakerServed] as r (r.route)}
                    {@const chain = r.chain}
                    <div class="q-chain">
                        <div class="q-chain-head">
                            <span class="q-route">{r.label}</span>
                            <span class="q-tag mono" class:lossless={chain.lossless}>
                                {chain.lossless ? "LOSSLESS" : "NOT LOSSLESS"}
                            </span>
                        </div>
                        <!-- Two stages, always both. Showing only the weak one
                             would leave a reader unable to tell a lossy
                             service from a lossy path, which are fixed in
                             completely different places. -->
                        <div class="q-stage">
                            <span class="q-stage-name">{chain.source.name}</span>
                            <span class="q-stage-val mono">{formatQuality(chain.source.quality)}</span>
                        </div>
                        <div class="q-stage">
                            <span class="q-stage-name">{chain.transport.name}</span>
                            <span class="q-stage-val mono"
                                >{formatQuality(chain.transport.quality)}</span
                            >
                        </div>
                        <p class="q-why">{chain.summary}</p>
                        {#if chain.fix}
                            <p class="q-fix">
                                <Icon name="info" size={13} />
                                <span>{chain.fix.detail || chain.fix.label}</span>
                            </p>
                        {/if}
                    </div>
                {/each}
            </div>

            <!-- ── The one lever ────────────────────────────────────────── -->
            <div class="eyrow" style="margin-top:var(--space-5)">
                What HomeHub decodes
            </div>
            <p class="q-note">
                Only the paths above where HomeHub holds the audio — the AirPlay
                and HomeHub-stream routes. Where a speaker streams a service
                from its own account, the speaker negotiates its own quality and
                this setting reaches nothing.
            </p>
            <div class="q-options" role="radiogroup" aria-label="Decode quality">
                {#each report.options as o (o.value)}
                    {@const on = report.stream_quality === o.value}
                    <button
                        type="button"
                        class="q-option"
                        class:on
                        role="radio"
                        aria-checked={on}
                        disabled={saving !== null}
                        onclick={() => choose(o.value)}
                    >
                        <span class="q-option-meta">
                            <span class="q-option-name">{o.label}</span>
                            <span class="q-option-detail">{o.detail}</span>
                        </span>
                        <span class="q-option-rate mono">{o.bitrate_kbps} kbps</span>
                        {#if on}<Icon name="check" size={16} />{/if}
                    </button>
                {/each}
            </div>
        {/if}
    {/snippet}
    {#snippet actions()}
        <button class="btn btn-ghost" onclick={() => closeModal()}>Done</button>
    {/snippet}
</Modal>


<style>
    .q-skeleton {
        height: 180px;
        border-radius: var(--r-md);
    }
    .q-empty,
    .q-note {
        font-size: 12.5px;
        color: var(--text-mute);
        margin: var(--space-2) 0 0;
        line-height: 1.45;
    }
    .q-chains {
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
        margin-top: var(--space-3);
    }
    .q-chain {
        background: var(--card-2);
        border: 1px solid var(--hairline);
        border-radius: var(--r-md);
        padding: var(--space-3);
    }
    .q-chain-head {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-2);
        margin-bottom: 6px;
    }
    .q-route {
        font-size: 13px;
        font-weight: 600;
        color: var(--text);
    }
    .q-tag {
        font-size: 10px;
        letter-spacing: 0.08em;
        color: var(--text-dim);
    }
    .q-tag.lossless {
        color: var(--on);
    }
    .q-stage {
        display: flex;
        align-items: baseline;
        justify-content: space-between;
        gap: var(--space-3);
        font-size: 12px;
        padding: 2px 0;
    }
    .q-stage-name {
        color: var(--text-mute);
    }
    .q-stage-val {
        color: var(--text);
        font-size: 11.5px;
    }
    .q-why {
        margin: 6px 0 0;
        font-size: 12px;
        line-height: 1.45;
        color: var(--text-mute);
    }
    .q-fix {
        display: flex;
        align-items: flex-start;
        gap: 6px;
        margin: 6px 0 0;
        font-size: 12px;
        line-height: 1.4;
        color: var(--cool);
    }
    .q-fix :global(svg) {
        flex-shrink: 0;
        margin-top: 2px;
    }
    .q-options {
        display: flex;
        flex-direction: column;
        gap: 6px;
        margin-top: var(--space-3);
    }
    .q-option {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: 10px 12px;
        min-height: 44px;
        background: var(--card-2);
        border: 1px solid var(--hairline);
        border-radius: var(--r-md);
        color: var(--text);
        cursor: pointer;
        text-align: left;
        font: inherit;
        transition:
            background 150ms ease,
            border-color 150ms ease;
    }
    .q-option:hover:not(:disabled) {
        background: var(--card-3);
    }
    .q-option.on {
        border-color: var(--on);
        color: var(--on);
    }
    .q-option:disabled {
        opacity: 0.6;
        cursor: default;
    }
    .q-option-meta {
        flex: 1;
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 2px;
    }
    .q-option-name {
        font-size: 13.5px;
        font-weight: 500;
        color: var(--text);
    }
    .q-option.on .q-option-name {
        color: var(--on);
    }
    .q-option-detail {
        font-size: 11.5px;
        color: var(--text-mute);
        line-height: 1.35;
    }
    .q-option-rate {
        font-size: 11px;
        color: var(--text-dim);
    }
</style>
