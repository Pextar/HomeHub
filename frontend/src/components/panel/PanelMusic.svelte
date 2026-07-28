<script lang="ts">
    import Icon from "../Icon.svelte";
    import Waveform from "../music/Waveform.svelte";
    import Slider from "../music/Slider.svelte";
    import { api } from "../../lib/api";
    import { route, session, toasts } from "../../lib/stores.svelte";
    import { onLive } from "../../lib/live";
    import { kefSourceLabel } from "../../lib/kef";
    import { haptic } from "../../lib/utils";
    import type { PanelNowPlaying } from "../../lib/panel";
    import type { SonosStatus, SonosGroupView, SonosSpeakerView, KEFStatus } from "../../lib/types";

    // The panel's right column: what's playing, plus the transport and
    // volume that answer from across the room. Music's second satellite
    // outside its own view (after Home's "Playing now" card, DESIGN.md
    // §6.8) — same data deal: speaker state isn't in the shared store, so
    // it arrives pushed on the "music" SSE topic with a slow poll behind.
    let {
        hasSpeakers = $bindable(false),
        // No default: the parent binds its own $state in, and the effect
        // below writes before anything reads. Only written here — the
        // reader is the parent's ambient face, invisible to the linter.
        // eslint-disable-next-line no-useless-assignment
        playing = $bindable(),
    }: {
        hasSpeakers?: boolean;
        playing?: PanelNowPlaying | null;
    } = $props();

    const POLL_MS = 15_000;
    const LIVE_POLL_MS = 45_000;

    let status = $state<SonosStatus | null>(null);
    let kef = $state<KEFStatus | null>(null);
    let failed = $state(false);
    let busy = $state<Record<string, boolean>>({});
    let seq = 0;

    async function refresh() {
        const mine = ++seq;
        // Both bridges in one pass, settled: one brand being absent or down
        // must not blank the other.
        const [sonosRes, kefRes] = await Promise.allSettled([api.sonosStatus(), api.kefStatus()]);
        if (mine !== seq) return;
        if (sonosRes.status === "fulfilled") status = sonosRes.value;
        if (kefRes.status === "fulfilled") kef = kefRes.value;
        failed = sonosRes.status === "rejected" && kefRes.status === "rejected";
        // Keep the "speakers-seen" memory fresh — the panel's parent reads
        // it to size the grid before the first poll lands (NowPlaying is
        // the other writer).
        if (!failed) {
            try {
                const any = (status?.speakers.length ?? 0) + (kef?.speakers.length ?? 0) > 0;
                localStorage.setItem("speakers-seen", String(any));
            } catch {
                /* private browsing */
            }
        }
    }

    // The Sonos endpoints are admin-only. Derived, not read straight off
    // `status`: this effect calls refresh(), which reassigns `status` —
    // reading it here directly would retrigger the effect forever.
    const livePush = $derived(!!status?.live);

    $effect(() => {
        if (!session.isAdmin) return;
        void refresh();
        const onVisible = () => {
            if (!document.hidden) void refresh();
        };
        const t = setInterval(onVisible, livePush ? LIVE_POLL_MS : POLL_MS);
        const stopLive = onLive("music", () => {
            if (!document.hidden) void refresh();
        });
        document.addEventListener("visibilitychange", onVisible);
        return () => {
            clearInterval(t);
            stopLive();
            document.removeEventListener("visibilitychange", onVisible);
        };
    });

    // ── Sources and the featured one ─────────────────────────────────────
    // A panel shows one room's playback at a time. Every reachable Sonos
    // group and KEF speaker is a source; the featured one is the user's
    // pick, else whatever is playing, else the first.
    interface Source {
        key: string;
        kind: "sonos" | "kef";
        id: string; // coordinator id (sonos) or speaker id (kef)
        title: string; // zone / speaker name
        playing: boolean;
        standby: boolean; // kef only — powered off, no transport
        volume: number;
        muted: boolean;
        trackTitle?: string;
        trackSub?: string;
        art?: string;
    }

    const speakers = $derived(status?.speakers ?? []);
    const byId = $derived(new Map(speakers.map((s) => [s.id, s])));

    function coordinatorOf(g: SonosGroupView): SonosSpeakerView | undefined {
        return byId.get(g.coordinator_id) ?? byId.get(g.member_ids[0]);
    }
    function groupTitle(g: SonosGroupView): string {
        const names = g.member_ids.map((id) => byId.get(id)?.name).filter((n): n is string => !!n);
        if (names.length <= 2) return names.join(" + ");
        return `${names[0]} + ${names.length - 1} more`;
    }

    const sources = $derived.by((): Source[] => {
        const out: Source[] = [];
        for (const g of status?.groups ?? []) {
            const c = coordinatorOf(g);
            if (!c?.reachable) continue;
            const st = c.state;
            out.push({
                key: "s:" + g.coordinator_id,
                kind: "sonos",
                id: g.coordinator_id,
                title: groupTitle(g),
                playing: !!st?.playing,
                standby: false,
                volume: st?.volume ?? 0,
                muted: !!st?.muted,
                trackTitle: st?.track?.title,
                trackSub: [st?.track?.artist, st?.track?.album].filter(Boolean).join(" · "),
                art: st?.track?.art_uri,
            });
        }
        for (const sp of kef?.speakers ?? []) {
            if (!sp.reachable) continue;
            const st = sp.state;
            out.push({
                key: "k:" + sp.id,
                kind: "kef",
                id: sp.id,
                title: sp.name,
                playing: !!st?.playing,
                standby: st ? !st.powered_on : false,
                volume: st?.volume ?? 0,
                muted: !!st?.muted,
                trackTitle:
                    st?.track?.title ??
                    (st?.playing && st.source ? `${kefSourceLabel(st.source)} input` : undefined),
                trackSub: [st?.track?.artist, st?.track?.album].filter(Boolean).join(" · "),
                art: st?.track?.art_uri,
            });
        }
        return out;
    });

    let selected = $state<string | null>(null);
    const featured = $derived(
        sources.find((s) => s.key === selected) ?? sources.find((s) => s.playing) ?? sources[0],
    );

    // The parent sizes its grid on this: no speakers → no third column.
    $effect(() => {
        hasSpeakers = !failed && sources.length > 0;
    });

    // And the ambient face reads what's playing through the same binding
    // path — one sentence about playback, derived once, shown in two places.
    $effect(() => {
        playing = featured?.playing
            ? {
                  title: featured.trackTitle ?? "Playing",
                  sub: [featured.trackSub, featured.title].filter(Boolean).join(" · "),
                  art: featured.art,
              }
            : null;
    });

    // ── Actions ──────────────────────────────────────────────────────────
    async function run(key: string, fn: () => Promise<unknown>, errTitle: string) {
        if (busy[key]) return;
        busy[key] = true;
        haptic();
        try {
            await fn();
            await refresh();
        } catch (e) {
            toasts.error(errTitle, (e as Error).message);
        } finally {
            busy[key] = false;
        }
    }

    function togglePlay(s: Source) {
        const label = s.playing ? "Pause failed" : "Play failed";
        if (s.kind === "sonos") {
            void run(
                "play:" + s.id,
                () => (s.playing ? api.sonosPause(s.id) : api.sonosPlay(s.id)),
                label,
            );
        } else {
            void run(
                "play:" + s.id,
                () => (s.playing ? api.kefPause(s.id) : api.kefPlay(s.id)),
                label,
            );
        }
    }

    // Sonos-only on this surface, matching Home's card: KEF has no queue,
    // so on most of its sources there is nothing for a skip to step
    // through.
    function skip(s: Source, dir: "next" | "previous") {
        void run(
            dir + ":" + s.id,
            () => (dir === "next" ? api.sonosNext(s.id) : api.sonosPrevious(s.id)),
            "Skip failed",
        );
    }

    // Volume: local while dragging, sent on release; group volume for
    // Sonos so the whole zone answers, not just the coordinator.
    let vol = $state(0);
    let dragging = false;
    $effect(() => {
        if (!dragging && featured) vol = featured.volume;
    });
    function setVolume(s: Source, level: number) {
        void run(
            "vol:" + s.id,
            () =>
                s.kind === "sonos"
                    ? api.sonosSetVolume(s.id, level, true)
                    : api.kefSetVolume(s.id, level),
            "Volume failed",
        );
    }
</script>

{#if hasSpeakers}
    <section class="music" aria-label="Now playing">
        <header class="m-head">
            <h2>Music</h2>
        </header>

        {#if sources.length > 1}
            <div class="m-sources">
                {#each sources as s (s.key)}
                    <button
                        class="m-chip"
                        class:active={featured?.key === s.key}
                        onclick={() => (selected = s.key)}
                    >
                        {s.title}
                    </button>
                {/each}
            </div>
        {/if}

        {#if featured}
            <article class="m-card" class:playing={featured.playing}>
                <!-- Art + meta are the tap-through to the full Music view
                     (search, queue, grouping); transport and volume stay
                     out of the button so the player still answers on the
                     panel itself. -->
                <button
                    class="m-open"
                    onclick={() => route.go("music")}
                    aria-label="Open {featured.trackTitle ?? 'playback'} in Music"
                >
                    <span class="m-artwrap">
                        {#if featured.art}
                            <img class="m-art" src={featured.art} alt="" loading="lazy" />
                        {:else}
                            <span class="m-art placeholder">[ art ]</span>
                        {/if}
                        {#if featured.playing}
                            <span class="m-wave"><Waveform /></span>
                        {/if}
                    </span>

                    <span class="m-track">
                        <span class="m-title">
                            {featured.trackTitle ?? (featured.playing ? "Playing" : "Not playing")}
                        </span>
                        <span class="m-subrow">
                            <span class="m-sub">{featured.trackSub || featured.title}</span>
                            <span class="m-go" aria-hidden="true"
                                ><Icon name="chevronLeft" size={16} /></span
                            >
                        </span>
                    </span>
                </button>

                {#if featured.standby}
                    <!-- A refused control renders as a label, never dead. -->
                    <div class="m-standby">In standby — wake it from the Music view</div>
                {:else}
                    <div class="m-transport">
                        {#if featured.kind === "sonos"}
                            <button
                                class="t-btn"
                                aria-label="Previous track"
                                disabled={busy["previous:" + featured.id]}
                                onclick={() => skip(featured, "previous")}
                            >
                                <Icon name="skipPrev" size={24} />
                            </button>
                        {/if}
                        <button
                            class="t-btn primary"
                            class:on={featured.playing}
                            aria-label={featured.playing ? "Pause" : "Play"}
                            disabled={busy["play:" + featured.id]}
                            onclick={() => togglePlay(featured)}
                        >
                            <Icon name={featured.playing ? "pause" : "play"} size={30} />
                        </button>
                        {#if featured.kind === "sonos"}
                            <button
                                class="t-btn"
                                aria-label="Next track"
                                disabled={busy["next:" + featured.id]}
                                onclick={() => skip(featured, "next")}
                            >
                                <Icon name="skipNext" size={24} />
                            </button>
                        {/if}
                    </div>

                    <div class="m-volume">
                        <span class="v-ico" class:mute={featured.muted}>
                            <Icon name={featured.muted ? "volumeOff" : "volume"} size={18} />
                        </span>
                        <Slider
                            value={vol}
                            label="Volume"
                            valueText="{vol}%"
                            onInput={(v) => {
                                dragging = true;
                                vol = v;
                            }}
                            onChange={(v) => {
                                dragging = false;
                                setVolume(featured, v);
                            }}
                        />
                        <span class="v-val mono">{vol}</span>
                    </div>
                {/if}
            </article>
        {/if}
    </section>
{/if}

<style>
    .music {
        display: flex;
        flex-direction: column;
        min-height: 0;
        min-width: 0;
    }
    .m-head {
        display: flex;
        align-items: center;
        justify-content: space-between;
        margin-bottom: var(--space-3);
        flex-shrink: 0;
    }
    h2 {
        margin: 0;
        font-size: 17px;
        font-weight: 600;
        letter-spacing: -0.02em;
    }

    .m-sources {
        display: flex;
        flex-wrap: wrap;
        gap: var(--space-2);
        margin-bottom: var(--space-3);
        flex-shrink: 0;
    }
    .m-chip {
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
    .m-chip:active {
        transform: scale(0.95);
        transition-duration: 80ms;
    }
    .m-chip.active {
        background: var(--text);
        color: var(--bg);
        border-color: var(--text);
    }

    .m-card {
        flex: 1;
        min-height: 0;
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
        padding: var(--space-5);
        border-radius: var(--r-lg);
        border: 1px solid var(--hairline);
        background: var(--card);
        transition:
            background var(--t-med),
            border-color var(--t-med);
    }
    .m-card.playing {
        background: var(--tile-on-gradient);
        border-color: var(--tile-on-border);
    }

    /* The tap-through button: reset to a plain flex column so it reads as
       the art + meta it contains, not as a button. */
    .m-open {
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
        padding: 0;
        border: 0;
        background: none;
        color: inherit;
        font: inherit;
        text-align: left;
        cursor: pointer;
        border-radius: var(--r-md);
        min-width: 0;
    }
    .m-open:focus-visible {
        box-shadow: var(--focus-ring);
    }

    .m-artwrap {
        position: relative;
        display: block;
        flex-shrink: 0;
    }
    .m-art {
        width: 100%;
        height: 200px;
        object-fit: cover;
        border-radius: var(--r-md);
        display: block;
    }
    span.m-art {
        font-size: 11px;
    }
    .m-wave {
        position: absolute;
        left: var(--space-3);
        bottom: var(--space-3);
        padding: 6px 8px;
        border-radius: var(--r-sm);
        background: var(--bg-bar);
        display: inline-flex;
    }

    .m-track {
        display: flex;
        flex-direction: column;
        gap: 4px;
        min-width: 0;
    }
    .m-title {
        font-size: 21px;
        font-weight: 600;
        letter-spacing: -0.02em;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .m-subrow {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-2);
        min-width: 0;
    }
    .m-sub {
        font-size: 14px;
        color: var(--text-mute);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .m-go {
        color: var(--text-dim);
        flex-shrink: 0;
        transform: rotate(180deg);
        display: inline-flex;
    }

    .m-standby {
        font-size: 13px;
        color: var(--text-dim);
        text-align: center;
        padding: var(--space-4) 0;
    }

    /* Transport sized for a wall poke: 64px sides, 80px centre. */
    .m-transport {
        display: flex;
        align-items: center;
        justify-content: center;
        gap: var(--space-5);
    }
    .t-btn {
        width: 64px;
        height: 64px;
        border-radius: 50%;
        border: 1px solid var(--hairline);
        background: var(--card-2);
        color: var(--text);
        display: grid;
        place-items: center;
        cursor: pointer;
        transition:
            background var(--t-fast),
            color var(--t-fast),
            border-color var(--t-fast),
            transform var(--t-fast);
    }
    .t-btn:active {
        transform: scale(0.94);
        transition-duration: 80ms;
    }
    .t-btn:disabled {
        opacity: 0.5;
    }
    .t-btn.primary {
        width: 80px;
        height: 80px;
        background: var(--on);
        border-color: var(--on);
        color: var(--primary-fg);
    }

    .m-volume {
        display: flex;
        align-items: center;
        gap: var(--space-3);
    }
    .v-ico {
        color: var(--text-mute);
        display: inline-flex;
        flex-shrink: 0;
    }
    .v-ico.mute {
        color: var(--bad);
    }
    .v-val {
        font-size: 13px;
        color: var(--text-mute);
        width: 3ch;
        text-align: right;
        flex-shrink: 0;
    }

    /* Portrait stack: the art shrinks so the transport stays reachable. */
    @media (orientation: portrait), (max-width: 760px) {
        .m-art {
            height: 160px;
        }
        .m-card {
            flex: none;
        }
    }
</style>
