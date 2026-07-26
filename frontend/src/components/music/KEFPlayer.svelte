<script lang="ts">
    /**
     * A KEF speaker's player.
     *
     * The same object as the Sonos one minus the two things KEF hasn't got —
     * a queue and a group — plus the input selector, which sits in the slot
     * where the group's per-speaker volumes sit, because both answer "where is
     * this coming out". Every room chip on Home opens a player; the chips sit
     * side by side and looked identical, so sending one of them to a settings
     * screen two levels away was the module's worst seam (DESIGN.md §15).
     *
     * No scrubber: KEF's API has no seek, so the position line is read-only.
     * The keyboard binds what exists — space, skip, volume, mute — and leaves
     * seek, queue and play-mode unbound rather than doing something
     * almost-right.
     */
    import MusicSheet from "./MusicSheet.svelte";
    import PlayerArt from "./PlayerArt.svelte";
    import PlayerMeta from "./PlayerMeta.svelte";
    import PlayerTransport from "./PlayerTransport.svelte";
    import VolumeRow from "./VolumeRow.svelte";
    import { kefSourceLabel, KEF_SOURCES } from "../../lib/kef";
    import { fmtSecs } from "../../lib/music/time";
    import type { KEFBridge } from "../../lib/music/kef.svelte";
    import type { Busy } from "../../lib/music/busy.svelte";
    import type { KEFSpeakerView } from "../../lib/types";
    import type { Snippet } from "svelte";

    let {
        speaker: sp,
        kef,
        busy,
        onClose,
        /** The way out to this speaker's settings screen. */
        onSettings,
        /** "Start something" — the row above the input selector. */
        startSomething,
        scrollEl = $bindable<HTMLElement | null>(null),
        sheetEl = $bindable<HTMLElement | null>(null),
        dismissing = $bindable(false),
    }: {
        speaker: KEFSpeakerView;
        kef: KEFBridge;
        busy: Busy;
        onClose: () => void;
        onSettings: () => void;
        startSomething: Snippet;
        scrollEl?: HTMLElement | null;
        sheetEl?: HTMLElement | null;
        dismissing?: boolean;
    } = $props();

    const st = $derived(sp.state);
    const durMs = $derived(st?.duration_ms ?? 0);

    /** What the meta block says when there is no track to name. */
    const meta = $derived.by(() => {
        if (st?.track?.title) {
            return {
                title: st.track.title,
                sub:
                    [st.track.artist, st.track.album].filter(Boolean).join(" · ") ||
                    kefSourceLabel(st.source),
                idle: false,
            };
        }
        if (!st?.powered_on) {
            return { title: "Standby", sub: "Press play to wake it.", idle: true };
        }
        return {
            title: kef.nowLine(sp),
            sub: "Pick an input below, or search Spotify.",
            idle: true,
        };
    });

    /** The transport keys this speaker can actually answer. */
    export function handleKey(e: KeyboardEvent, opts: { slider: boolean; onControl: boolean }) {
        const key = e.key.length === 1 ? e.key.toLowerCase() : e.key;
        if ((key === " " || key === "k") && !(key === " " && opts.onControl)) {
            e.preventDefault();
            void kef.togglePlay(sp);
            return;
        }
        if (opts.slider) return;
        switch (key) {
            case "ArrowRight": e.preventDefault(); kef.skip(sp, "next"); break;
            case "ArrowLeft": e.preventDefault(); kef.skip(sp, "previous"); break;
            case "ArrowUp": e.preventDefault(); kef.setVolume(sp, kef.shownVolume(sp) + 5); break;
            case "ArrowDown": e.preventDefault(); kef.setVolume(sp, kef.shownVolume(sp) - 5); break;
            case "n": kef.skip(sp, "next"); break;
            case "p": kef.skip(sp, "previous"); break;
            case "m": kef.toggleMute(sp); break;
        }
    }
</script>

<MusicSheet
    label="Now playing"
    eyebrow="Playing on"
    title={sp.name}
    backLabel="Collapse player"
    onBack={onClose}
    onDismiss={onClose}
    action={{
        // Tone, EQ and the rest are the speaker's own settings, and they live
        // on its screen. This is the way there, and the sheet stands down
        // first so a screen can push without a sheet ever opening one.
        icon: "sliders",
        label: `${sp.name} settings`,
        onClick: onSettings,
    }}
    bind:scrollEl
    bind:sheetEl
    bind:dismissing
>
    <!-- No skips on the art: they exist, but the swipe belongs to a source
         with a queue behind it, and this one steps between whatever the
         Connect session happens to hold. -->
    <PlayerArt artUri={st?.track?.art_uri} />

    <PlayerMeta title={meta.title} sub={meta.sub} idle={meta.idle} />

    <!-- A read-only line, not a scrubber: KEF's API has no seek. The physical
         inputs and live streams report no duration at all and get no line
         rather than a made-up one — the same rule Sonos radio follows one
         sheet over. -->
    {#if durMs > 0}
        <div class="p-scrub">
            <span class="kef-rail" aria-hidden="true">
                <i style:width="{kef.progress(sp) * 100}%"></i>
            </span>
            <div class="p-times mono">
                <span>{fmtSecs(kef.positionMs(sp) / 1000)}</span><span>{fmtSecs(durMs / 1000)}</span>
            </div>
        </div>
    {:else if st?.track?.title}
        <div class="p-live mono">no track position on this input</div>
    {/if}

    <PlayerTransport
        playing={kef.isPlaying(sp)}
        onToggle={() => kef.togglePlay(sp)}
        toggleBusy={busy.is("kefplay:" + sp.id)}
        onPrev={() => kef.skip(sp, "previous")}
        prevBusy={busy.is("kefprevious:" + sp.id)}
        onNext={() => kef.skip(sp, "next")}
        nextBusy={busy.is("kefnext:" + sp.id)}
    />

    <!-- Spotify Connect is the only road content takes to a KEF speaker, so
         this row is the whole of "play something else" here — and it sits
         above the input selector, which answers the same question for the
         physical inputs. -->
    {@render startSomething()}

    <div class="p-speakers">
        <div class="eyrow">Volume</div>
        <VolumeRow
            name={sp.name}
            value={kef.shownVolume(sp)}
            label="Volume for {sp.name}"
            mute={{
                muted: !!st?.muted,
                busy: busy.is("kefmute:" + sp.id),
                onToggle: () => kef.toggleMute(sp),
            }}
            onInput={(v) => kef.dragVolume(sp, v)}
            onChange={(v) => kef.setVolume(sp, v)}
        />
    </div>

    <!-- The question a KEF speaker raises that a Sonos zone doesn't: which
         input. Every model shows the same list — there is no "what inputs do
         you have" call, so a model without USB simply refuses it rather than
         the UI hiding it. -->
    <div class="p-speakers">
        <div class="eyrow">Input</div>
        <div class="src-row">
            {#each KEF_SOURCES as src (src.value)}
                <button
                    class="chip"
                    class:on={st?.source === src.value}
                    aria-pressed={st?.source === src.value}
                    disabled={busy.is("kefsrc:" + sp.id)}
                    onclick={() => kef.setSource(sp, src.value)}>{src.label}</button
                >
            {/each}
        </div>
        <p class="hint">
            No queue and no grouping — a KEF speaker plays alone, so there is nothing to line up
            behind this or to play it with.
        </p>
    </div>
</MusicSheet>

<style>
    .p-scrub { display: flex; flex-direction: column; gap: 6px; }
    .p-times { display: flex; justify-content: space-between; font-size: 11px; color: var(--text-dim); }
    .p-live {
        text-align: center; font-size: 10.5px; letter-spacing: 0.08em;
        text-transform: uppercase; color: var(--text-dim);
    }

    /* Read-only, so a bar rather than the Sonos scrubber's range input —
       DESIGN.md §15: a control the speaker would refuse is worse than no
       control. */
    .kef-rail {
        display: block; height: 6px; border-radius: 3px;
        background: var(--card-3); overflow: hidden;
    }
    .kef-rail i {
        display: block; height: 100%; background: var(--on);
        /* Matches the 1s position tick, so the fill creeps instead of
           stepping. */
        transition: width 1s linear;
    }

    .p-speakers { display: flex; flex-direction: column; gap: 2px; }
    .p-speakers .eyrow { margin-bottom: var(--space-1); }
    .p-speakers .hint { margin-top: var(--space-2); }
    .src-row { display: flex; flex-wrap: wrap; gap: var(--space-2); }
    .src-row .chip { flex-shrink: 0; }

    @media (prefers-reduced-motion: reduce) {
        .kef-rail i { transition-duration: 0.001ms; }
    }
</style>
