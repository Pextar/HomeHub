<script lang="ts">
    /**
     * A zone's player — the third one, and the first that isn't about a make of
     * speaker.
     *
     * It is the same object as the other two minus what a zone hasn't got, and
     * what it hasn't got depends on the route the backend chose, not on the
     * makes in it:
     *
     *   No queue. A zone's queue would be its coordinator's, and on the stream
     *   route there is no coordinator at all. Sonos queue work stays on the
     *   Sonos player, where the queue actually lives.
     *
     *   No scrubber. There is no zone seek endpoint, because seeking a fan-out
     *   of a live stream is not a thing the transport can do. The position is a
     *   read-only line, and a source reporting no duration gets no line rather
     *   than a made-up one — the rule both other players follow.
     *
     *   No skips on the stream route. While HomeHub is decoding, the Spotify
     *   session belongs to HomeHub, and the speakers are pulling a live stream:
     *   `next` sent to a speaker mid-stream is a call it refuses. So the skips
     *   are absent and a line says where track changes come from instead — a
     *   control that would be refused is worse than no control (§15).
     *
     * What it adds is the route note. A streamed mixed zone is genuinely a
     * different thing from a natively grouped one and says so in the backend's
     * own words; a zone playing natively says one quiet line about sync and
     * nothing more.
     */
    import MusicSheet from "./MusicSheet.svelte";
    import PlayerArt from "./PlayerArt.svelte";
    import PlayerMeta from "./PlayerMeta.svelte";
    import PlayerTransport from "./PlayerTransport.svelte";
    import VolumeRow from "./VolumeRow.svelte";
    import ZoneRoute from "./ZoneRoute.svelte";
    import { fmtSecs } from "../../lib/music/time";
    import type { MediaZone } from "../../lib/types";
    import type { ZonesBridge } from "../../lib/music/zones.svelte";
    import type { SonosBridge } from "../../lib/music/sonos.svelte";
    import type { KEFBridge } from "../../lib/music/kef.svelte";
    import type { Busy } from "../../lib/music/busy.svelte";
    import type { Snippet } from "svelte";

    let {
        zone: z,
        zones,
        sonos,
        kef,
        busy,
        onClose,
        /** The way out to this zone's membership — a swap, never a second sheet. */
        onEdit,
        startSomething,
        scrollEl = $bindable<HTMLElement | null>(null),
        sheetEl = $bindable<HTMLElement | null>(null),
        dismissing = $bindable(false),
    }: {
        zone: MediaZone;
        zones: ZonesBridge;
        sonos: SonosBridge;
        kef: KEFBridge;
        busy: Busy;
        onClose: () => void;
        onEdit: () => void;
        startSomething: Snippet;
        scrollEl?: HTMLElement | null;
        sheetEl?: HTMLElement | null;
        dismissing?: boolean;
    } = $props();

    const speakers = $derived(zones.speakersOf(z));
    const lead = $derived(zones.leadOf(z));
    const st = $derived(lead?.state);
    const durMs = $derived(zones.durationMs(z));
    const playing = $derived(zones.isPlaying(z));

    /**
     * Whether stepping between tracks is something this zone can answer. On
     * the stream route it isn't: the transport would send `next` to speakers
     * playing a live stream.
     */
    const skippable = $derived(z.route !== "stream");

    const meta = $derived.by(() => {
        if (st?.track?.title) {
            return {
                title: st.track.title,
                sub: [st.track.artist, st.track.album].filter(Boolean).join(" · ") ||
                    zones.memberLine(z),
                idle: false,
            };
        }
        if (speakers.length === 0) {
            return {
                title: "No speakers in this zone",
                sub: "Add some and it can play.",
                idle: true,
            };
        }
        return {
            title: "Nothing playing",
            sub: "Search Spotify to start something here.",
            idle: true,
        };
    });

    /** The transport keys a zone can actually answer. */
    export function handleKey(e: KeyboardEvent, opts: { slider: boolean; onControl: boolean }) {
        const key = e.key.length === 1 ? e.key.toLowerCase() : e.key;
        if ((key === " " || key === "k") && !(key === " " && opts.onControl)) {
            e.preventDefault();
            void zones.togglePlay(z);
            return;
        }
        if (opts.slider) return;
        switch (key) {
            case "ArrowRight":
                if (!skippable) return;
                e.preventDefault();
                zones.skip(z, "next");
                break;
            case "ArrowLeft":
                if (!skippable) return;
                e.preventDefault();
                zones.skip(z, "previous");
                break;
            case "ArrowUp":
                e.preventDefault();
                zones.setVolume(z, zones.shownVolume(z) + 5);
                break;
            case "ArrowDown":
                e.preventDefault();
                zones.setVolume(z, zones.shownVolume(z) - 5);
                break;
            case "n": if (skippable) zones.skip(z, "next"); break;
            case "p": if (skippable) zones.skip(z, "previous"); break;
            case "m": zones.toggleMute(z); break;
        }
    }
</script>

<MusicSheet
    label="Now playing"
    eyebrow="Playing on"
    title={z.name}
    sub={zones.memberLine(z)}
    backLabel="Collapse player"
    onBack={onClose}
    onDismiss={onClose}
    action={{
        // What a zone *is* — its members — is the zone's equivalent of a
        // speaker's settings, so it rides where the KEF player's settings chip
        // does. The sheets swap, so a sheet never opens another sheet.
        icon: "sliders",
        label: `Edit ${z.name}`,
        onClick: onEdit,
    }}
    bind:scrollEl
    bind:sheetEl
    bind:dismissing
>
    <PlayerArt
        artUri={st?.track?.art_uri}
        sheetDismissing={dismissing}
        onSkip={skippable ? (dir) => zones.skip(z, dir) : undefined}
    />

    <PlayerMeta title={meta.title} sub={meta.sub} idle={meta.idle} />

    <!-- What a play here does, in the backend's words. It sits above the
         transport because it qualifies the transport: on the stream route the
         skips below are absent, and this is the sentence that explains why. -->
    <div class="z-route">
        <ZoneRoute route={z.route} sync={z.sync} reason={z.reason} problem={z.problem} />
    </div>

    <!-- Read-only, like the KEF player's: there is no zone seek, so a
         scrubber here would be a control the transport can't back. -->
    {#if durMs > 0}
        <div class="p-scrub">
            <span class="z-rail" aria-hidden="true">
                <i style:width="{zones.progress(z) * 100}%"></i>
            </span>
            <div class="p-times mono">
                <span>{fmtSecs(zones.positionMs(z) / 1000)}</span><span
                    >{fmtSecs(durMs / 1000)}</span
                >
            </div>
        </div>
    {:else if st?.track?.title}
        <div class="p-live mono">live stream — no track position</div>
    {/if}

    <PlayerTransport
        {playing}
        onToggle={() => zones.togglePlay(z)}
        toggleBusy={busy.is("zplay:" + z.id)}
        onPrev={skippable ? () => zones.skip(z, "previous") : undefined}
        prevBusy={busy.is("zprevious:" + z.id)}
        onNext={skippable ? () => zones.skip(z, "next") : undefined}
        nextBusy={busy.is("znext:" + z.id)}
    />

    {#if !skippable}
        <p class="hint z-skip-note">
            HomeHub is the Spotify device while this zone plays, so track changes come from
            Spotify itself — skip there, and it follows here.
        </p>
    {/if}

    <!-- Stop, not just pause: it also hands the Spotify session back, which is
         what frees another zone to take it. Only worth offering while there is
         something to stop. -->
    {#if playing || st?.track?.title}
        <div class="z-stop">
            <button class="chip" disabled={busy.is("zstop:" + z.id)} onclick={() => void zones.stop(z)}>
                Stop and release Spotify
            </button>
        </div>
    {/if}

    {@render startSomething()}

    <div class="p-speakers">
        <div class="eyrow">Volume</div>
        {#if speakers.length > 1}
            <VolumeRow
                name="All speakers"
                value={zones.shownVolume(z)}
                label="Zone volume"
                onInput={(v) => zones.dragVolume(z, v)}
                onChange={(v) => zones.setVolume(z, v)}
            />
            <div class="m-divider" aria-hidden="true"></div>
        {/if}
        <!-- Per speaker, through its own bridge: a zone-wide set writes one
             level everywhere, and the reason to open this section is usually
             that one room wants to be quieter than the rest. -->
        {#each speakers as sp (sp.member)}
            {#if sp.vendor === "kef"}
                {@const k = kef.byId(sp.id)}
                {#if k}
                    <VolumeRow
                        name={k.name}
                        value={kef.shownVolume(k)}
                        label="{k.name} volume"
                        mute={{
                            muted: !!k.state?.muted,
                            busy: busy.is("kefmute:" + k.id),
                            onToggle: () => kef.toggleMute(k),
                        }}
                        onInput={(v) => kef.dragVolume(k, v)}
                        onChange={(v) => kef.setVolume(k, v)}
                    />
                {/if}
            {:else}
                {@const s = sonos.speakerById.get(sp.id)}
                {#if s}
                    <VolumeRow
                        name={s.name}
                        value={sonos.shownVolume(s)}
                        label="{s.name} volume"
                        mute={{
                            muted: !!s.state?.muted,
                            busy: busy.is("mute:" + s.id),
                            onToggle: () => sonos.toggleMute(s),
                        }}
                        onInput={(v) => sonos.dragVolume(s.id, v)}
                        onChange={(v) => sonos.setVolume(s.id, v)}
                    />
                {/if}
            {/if}
        {/each}
        {#if z.speakers.some((sp) => sp.missing)}
            <div class="z-missing mono">
                a speaker in this zone no longer exists — edit the zone to drop it
            </div>
        {/if}
    </div>
</MusicSheet>

<style>
    .z-route { display: flex; flex-direction: column; gap: var(--space-1); }

    .p-scrub { display: flex; flex-direction: column; gap: 6px; }
    .p-times {
        display: flex; justify-content: space-between;
        font-size: 11px; color: var(--text-dim);
    }
    .p-live {
        text-align: center; font-size: 10.5px; letter-spacing: 0.08em;
        text-transform: uppercase; color: var(--text-dim);
    }

    /* Same read-only rail the KEF player uses, for the same reason. */
    .z-rail {
        display: block; height: 6px; border-radius: 3px;
        background: var(--card-3); overflow: hidden;
    }
    .z-rail i {
        display: block; height: 100%; background: var(--on);
        transition: width 1s linear;
    }

    .z-skip-note { text-align: center; }
    .z-stop { display: flex; justify-content: center; }

    .p-speakers { display: flex; flex-direction: column; gap: 2px; }
    .p-speakers .eyrow { margin-bottom: var(--space-1); }
    .m-divider { height: 1px; background: var(--hairline); margin: var(--space-2) 0; }
    .z-missing { font-size: 11px; color: var(--bad); margin-top: var(--space-2); }

    @media (prefers-reduced-motion: reduce) {
        .z-rail i { transition-duration: 0.001ms; }
    }
</style>
