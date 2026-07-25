<script lang="ts">
    import Icon from "./Icon.svelte";
    import { api } from "../lib/api";
    import { route, session, toasts } from "../lib/stores.svelte";
    import { dur, stagger } from "../lib/motion";
    import { fly } from "svelte/transition";
    import { cubicOut } from "svelte/easing";
    import type { SonosStatus, SonosGroupView, SonosSpeakerView } from "../lib/types";

    // Home's window on the Music module. It carries its own poll because
    // speaker state doesn't live in the shared data store — it's read off the
    // speakers themselves. Slower than the Music view's 5s: home shows no
    // position, so a stale second costs nothing.
    const POLL_MS = 10_000;

    let status = $state<SonosStatus | null>(null);
    let loaded = $state(false);
    // Sonos being unconfigured, unreachable, or refused must never make the
    // home page complain: the section simply doesn't render. Only an action
    // the user actually took (play, skip) earns a toast.
    let failed = $state(false);
    let busy = $state<Record<string, boolean>>({});
    let seq = 0;

    // Whether the last poll (in any earlier session) found speakers at all.
    // A home with Sonos gets a skeleton in the right place instead of a
    // section that pops in; a home without one gets no flash of a section it
    // will never have.
    const SEEN_KEY = "sonos-seen";
    function seenSpeakers(): boolean {
        try { return localStorage.getItem(SEEN_KEY) === "true"; }
        catch { return false; } // private browsing
    }
    function rememberSpeakers(any: boolean) {
        try { localStorage.setItem(SEEN_KEY, String(any)); }
        catch { /* private browsing */ }
    }

    async function refresh() {
        const mine = ++seq;
        try {
            const st = await api.sonosStatus();
            if (mine !== seq) return;
            status = st;
            failed = false;
            rememberSpeakers(st.speakers.length > 0);
        } catch {
            if (mine !== seq) return;
            failed = true;
        } finally {
            if (mine === seq) loaded = true;
        }
    }

    // The Sonos endpoints are admin-only, and a backgrounded PWA shouldn't
    // keep waking the speakers — so the poll runs only while both hold.
    $effect(() => {
        if (!session.isAdmin) return;
        void refresh();
        const onVisible = () => { if (!document.hidden) void refresh(); };
        const t = setInterval(onVisible, POLL_MS);
        document.addEventListener("visibilitychange", onVisible);
        return () => {
            clearInterval(t);
            document.removeEventListener("visibilitychange", onVisible);
        };
    });

    const speakers = $derived(status?.speakers ?? []);
    const groups = $derived(status?.groups ?? []);
    const byId = $derived(new Map(speakers.map((s) => [s.id, s])));
    const reachable = $derived(speakers.filter((s) => s.reachable));

    function coordinatorOf(g: SonosGroupView): SonosSpeakerView | undefined {
        return byId.get(g.coordinator_id) ?? byId.get(g.member_ids[0]);
    }
    function groupTitle(g: SonosGroupView): string {
        const names = g.member_ids
            .map((id) => byId.get(id)?.name)
            .filter((n): n is string => !!n);
        if (names.length <= 2) return names.join(" + ");
        return `${names[0]} + ${names.length - 1} more`;
    }

    const playing = $derived(groups.filter((g) => coordinatorOf(g)?.state?.playing));

    // Nothing registered, no permission, or the bridge is down — stay out of
    // the way entirely rather than showing a dead section.
    const hidden = $derived(
        !session.isAdmin ||
        failed ||
        (loaded ? speakers.length === 0 : !seenSpeakers()),
    );

    async function run(key: string, fn: () => Promise<unknown>, errTitle: string) {
        if (busy[key]) return;
        busy[key] = true;
        try {
            await fn();
            await refresh();
        } catch (e) {
            toasts.error(errTitle, (e as Error).message);
        } finally {
            busy[key] = false;
        }
    }

    function togglePlay(g: SonosGroupView) {
        const c = coordinatorOf(g);
        if (!c) return;
        const isPlaying = c.state?.playing;
        void run(
            "play:" + c.id,
            () => (isPlaying ? api.sonosPause(c.id) : api.sonosPlay(c.id)),
            isPlaying ? "Pause failed" : "Play failed",
        );
    }
    function skip(g: SonosGroupView, dir: "next" | "previous") {
        const c = coordinatorOf(g);
        if (!c) return;
        void run(
            dir + ":" + c.id,
            () => (dir === "next" ? api.sonosNext(c.id) : api.sonosPrevious(c.id)),
            "Skip failed",
        );
    }
</script>

<!-- The §6.8 waveform — "audio is moving", the Music module's stand-in for a
     status dot. This card is Music's surface on Home, so it comes along. -->
{#snippet wave()}
    <span class="wave" aria-hidden="true"><i></i><i></i><i></i><i></i></span>
{/snippet}

{#if !hidden}
    <section class="home-section">
        <div class="section-head">
            <h2><span class="section-ico"><Icon name="music" size={15} /></span>Playing now</h2>
            <button class="chip" onclick={() => route.go("music")}>Music</button>
        </div>

        {#if !loaded}
            <div class="skeleton np-skel" aria-hidden="true"></div>
        {:else if playing.length === 0}
            <button class="np idle" onclick={() => route.go("music")}>
                <span class="np-ico"><Icon name="speaker" size={20} /></span>
                <span class="np-meta">
                    <span class="np-title">Nothing playing</span>
                    <span class="np-sub">
                        <span class="mono">{reachable.length}</span>
                        speaker{reachable.length === 1 ? "" : "s"} ready
                    </span>
                </span>
                <span class="np-go" aria-hidden="true"><Icon name="chevronLeft" size={16} /></span>
            </button>
        {:else}
            <div class="np-grid">
                {#each playing as g, i (g.coordinator_id)}
                    {@const c = coordinatorOf(g)}
                    {@const st = c?.state}
                    {@const track = st?.track}
                    <article class="np playing"
                        in:fly={{ y: 8, duration: dur(220), delay: stagger(i), easing: cubicOut }}>
                        <button class="np-open" onclick={() => route.go("music")}
                            aria-label="Open {track?.title ?? 'playback'} in Music">
                            {#if track?.art_uri}
                                <img class="np-art" src={track.art_uri} alt="" loading="lazy" />
                            {:else}
                                <span class="np-art placeholder">[ art ]</span>
                            {/if}
                            <span class="np-meta">
                                <span class="np-where">
                                    {@render wave()}
                                    <span class="np-room">{groupTitle(g)}</span>
                                </span>
                                <span class="np-title">{track?.title ?? "Playing"}</span>
                                <span class="np-sub">
                                    {[track?.artist, track?.album].filter(Boolean).join(" · ") || "Live audio"}
                                </span>
                            </span>
                        </button>
                        <div class="np-transport">
                            <button class="np-btn skip" aria-label="Previous track"
                                disabled={!c || busy["previous:" + c?.id]}
                                onclick={() => skip(g, "previous")}>
                                <Icon name="skipPrev" size={18} />
                            </button>
                            <button class="np-btn on" aria-label="Pause"
                                disabled={!c || busy["play:" + c?.id]}
                                onclick={() => togglePlay(g)}>
                                <Icon name="pause" size={18} />
                            </button>
                            <button class="np-btn skip" aria-label="Next track"
                                disabled={!c || busy["next:" + c?.id]}
                                onclick={() => skip(g, "next")}>
                                <Icon name="skipNext" size={18} />
                            </button>
                        </div>
                    </article>
                {/each}
            </div>
        {/if}
    </section>
{/if}

<style>
    /* Section chrome matches Dashboard's other sections exactly — this card
       is a Home section that happens to own its own data. */
    .home-section { display: flex; flex-direction: column; gap: var(--space-3); }
    .section-head {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
    }
    .section-head h2 {
        display: inline-flex;
        align-items: center;
        gap: 8px;
        font-size: 17px;
        font-weight: 600;
    }
    .section-ico {
        width: 24px; height: 24px;
        border-radius: var(--r-sm);
        display: grid; place-items: center;
        background: var(--on-soft);
        color: var(--on);
        flex-shrink: 0;
    }

    .np-skel { height: 96px; border-radius: var(--r-lg); }

    .np-grid {
        display: grid;
        grid-template-columns: 1fr;
        gap: var(--space-3);
    }
    /* Wide enough that the track title isn't crushed between the art and the
       transport — a 340px track would truncate almost every song. */
    @media (min-width: 900px) {
        .np-grid { grid-template-columns: repeat(auto-fill, minmax(460px, 1fr)); }
    }

    .np {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: 14px;
        background: var(--card);
        border: 1px solid var(--hairline);
        border-radius: var(--r-lg);
        min-width: 0;
        transition: background var(--t-med), border-color var(--t-med);
    }
    /* Playing is an "ON" state — the same sanctioned surface a lit device
       gets (DESIGN.md §15), never a music-only colour. */
    .np.playing {
        background: var(--tile-on-gradient);
        border-color: var(--tile-on-border);
    }

    /* ── Idle card ──────────────────────────────────────────────────── */
    .np.idle {
        width: 100%;
        text-align: left;
        font: inherit;
        color: var(--text);
        cursor: pointer;
        touch-action: manipulation;
    }
    .np.idle:active { transform: scale(0.99); transition: transform 80ms ease; }
    .np.idle:focus-visible { box-shadow: var(--focus-ring); }
    @media (hover: hover) {
        .np.idle:hover { border-color: var(--border-strong); }
    }
    .np-ico {
        width: 44px; height: 44px;
        border-radius: var(--r-md);
        background: var(--card-3);
        color: var(--text-mute);
        display: grid; place-items: center;
        flex-shrink: 0;
    }
    .np-go { color: var(--text-dim); flex-shrink: 0; transform: rotate(180deg); }

    /* ── Playing card ───────────────────────────────────────────────── */
    .np-open {
        flex: 1;
        min-width: 0;
        display: flex;
        align-items: center;
        gap: var(--space-3);
        background: none;
        border: 0;
        padding: 0;
        color: var(--text);
        text-align: left;
        cursor: pointer;
        touch-action: manipulation;
        transition: transform var(--t-fast);
    }
    .np-open:active { transform: scale(0.99); }
    .np-open:focus-visible { box-shadow: var(--focus-ring); border-radius: var(--r-md); }

    .np-art {
        width: 60px; height: 60px;
        border-radius: var(--r-md);
        object-fit: cover;
        background: var(--card-3);
        border: 1px solid var(--hairline);
        flex-shrink: 0;
        /* The same light a lit device gives off (§15), dialled down for a
           card this size. */
        box-shadow: 0 0 20px -4px var(--on-glow);
    }
    span.np-art { font-size: 9px; }

    .np-meta {
        flex: 1;
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 2px;
    }
    .np-where { display: flex; align-items: center; gap: 6px; min-width: 0; }
    .np-room {
        font-family: var(--font-mono);
        font-size: 10.5px;
        font-weight: 500;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--on);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .np-title {
        font-size: 15px;
        font-weight: 600;
        letter-spacing: -0.01em;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .np-sub {
        font-size: 12.5px;
        color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }

    .np-transport { display: flex; align-items: center; gap: 6px; flex-shrink: 0; }
    .np-btn {
        width: 38px; height: 38px;
        border-radius: 50%;
        display: grid; place-items: center;
        background: var(--card-3);
        border: 1px solid var(--hairline);
        color: var(--text);
        cursor: pointer;
        touch-action: manipulation;
        transition: background var(--t-fast), transform var(--t-fast);
    }
    .np-btn.on { background: var(--on); color: var(--primary-fg); border-color: transparent; }
    .np-btn:active { transform: scale(0.94); }
    .np-btn:disabled { opacity: 0.5; cursor: default; }
    .np-btn:focus-visible { box-shadow: var(--focus-ring); }
    /* Phones keep play/pause and drop the skips rather than crushing the
       track title — Home is a glance surface, the Music view has the full
       transport. */
    @media (max-width: 430px) {
        .np-btn.skip { display: none; }
    }
    @media (pointer: coarse) {
        .np-btn { width: 44px; height: 44px; }
    }

    /* ── Waveform (§6.8) ────────────────────────────────────────────── */
    .wave { display: flex; align-items: flex-end; gap: 2.5px; height: 13px; flex-shrink: 0; }
    .wave i {
        display: block;
        width: 2.5px;
        border-radius: 1px;
        background: var(--on);
        height: 4px;
        animation: wv 950ms ease-in-out infinite;
    }
    .wave i:nth-child(1) { animation-delay: 0s; }
    .wave i:nth-child(2) { animation-delay: 0.15s; }
    .wave i:nth-child(3) { animation-delay: 0.3s; }
    .wave i:nth-child(4) { animation-delay: 0.1s; }
    @keyframes wv { 0%, 100% { height: 3px; } 50% { height: 13px; } }
    @media (prefers-reduced-motion: reduce) {
        .wave i { animation: none; height: 8px; }
    }
</style>
