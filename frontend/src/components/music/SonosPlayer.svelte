<script lang="ts">
    /**
     * A Sonos zone's player: art-led, with the scrubber, play modes, the
     * queue and the group's per-speaker volumes.
     *
     * Two panes inside one sheet — now playing, and the queue — reached from
     * an "Up next" row that names the actual next track. Not a segmented
     * control; §2 has no exception left to lean on. The header's left button
     * becomes a back chevron, the close X stays put, and Escape still leaves
     * the player outright rather than stepping back a level.
     */
    import Icon from "../Icon.svelte";
    import MusicSheet from "./MusicSheet.svelte";
    import PlayerArt from "./PlayerArt.svelte";
    import PlayerMeta from "./PlayerMeta.svelte";
    import PlayerTransport from "./PlayerTransport.svelte";
    import QueuePane from "./QueuePane.svelte";
    import Slider from "./Slider.svelte";
    import VolumeRow from "./VolumeRow.svelte";
    import { fmtSecs, secs } from "../../lib/music/time";
    import { NEXT_REPEAT, repeatLabel } from "../../lib/music/sonos.svelte";
    import type { SonosBridge } from "../../lib/music/sonos.svelte";
    import type { Busy } from "../../lib/music/busy.svelte";
    import type { SonosGroupView } from "../../lib/types";
    import type { Snippet } from "svelte";

    let {
        group: g,
        sonos,
        busy,
        onClose,
        /** Clearing the queue is destructive, so the confirm is the caller's. */
        onClearQueue,
        startSomething,
        scrollEl = $bindable<HTMLElement | null>(null),
        sheetEl = $bindable<HTMLElement | null>(null),
        dismissing = $bindable(false),
    }: {
        group: SonosGroupView;
        sonos: SonosBridge;
        busy: Busy;
        onClose: () => void;
        onClearQueue: () => void;
        startSomething: Snippet<[string | null]>;
        scrollEl?: HTMLElement | null;
        sheetEl?: HTMLElement | null;
        dismissing?: boolean;
    } = $props();

    const c = $derived(sonos.coordinatorOf(g));
    const st = $derived(c?.state);
    const gs = $derived(c?.group_state);
    const grouped = $derived(g.member_ids.length > 1);

    /** Non-null while a finger is on the scrubber; everything else about a
     *  position — the poll, the extrapolation, a just-issued seek — is the
     *  bridge's. */
    let scrubSec = $state<number | null>(null);
    let queuePane = $state(false);

    // Sources without a duration (radio, line-in, TV) can't be seeked.
    const durationSec = $derived(secs(st?.duration));
    const livePos = $derived(scrubSec ?? sonos.livePosition(g));

    function commitSeek(sec: number) {
        scrubSec = null;
        sonos.seek(g, sec);
    }

    // The two panes share one scroll container, so switching has to rewind it
    // — otherwise the queue opens halfway down at the player's offset.
    $effect(() => {
        void queuePane;
        if (scrollEl) scrollEl.scrollTop = 0;
    });

    /**
     * Drop the scrub and seek overrides when the track changes, so a new song
     * never inherits the previous one's position. The guard matters: every
     * poll replaces the status objects, so this effect re-runs on the poll —
     * without it, a drag in progress would be cancelled and a fresh seek
     * discarded each time one landed.
     */
    let lastTrack = "";
    $effect(() => {
        const key = st?.track?.title ?? "";
        if (key === lastTrack) return;
        lastTrack = key;
        scrubSec = null;
        sonos.clearSeek();
    });

    export function handleKey(e: KeyboardEvent, opts: { slider: boolean; onControl: boolean }) {
        const key = e.key.length === 1 ? e.key.toLowerCase() : e.key;
        // Space on a focused button belongs to that button, not to us.
        if ((key === " " || key === "k") && !(key === " " && opts.onControl)) {
            e.preventDefault();
            void sonos.togglePlay(g);
            return;
        }
        if (opts.slider) return;
        switch (key) {
            case "ArrowRight":
                e.preventDefault();
                if (e.shiftKey || durationSec === 0) sonos.skip(g, "next");
                else commitSeek(Math.min(durationSec, livePos + 10));
                break;
            case "ArrowLeft":
                e.preventDefault();
                if (e.shiftKey || durationSec === 0) sonos.skip(g, "previous");
                else commitSeek(Math.max(0, livePos - 10));
                break;
            case "ArrowUp": e.preventDefault(); sonos.nudgeVolume(g, 5); break;
            case "ArrowDown": e.preventDefault(); sonos.nudgeVolume(g, -5); break;
            case "n": sonos.skip(g, "next"); break;
            case "p": sonos.skip(g, "previous"); break;
            case "m": sonos.toggleMuteGroup(g); break;
            case "s": if (gs) sonos.setPlayMode(g, { shuffle: !gs.shuffle }); break;
            case "r": if (gs) sonos.setPlayMode(g, { repeat: NEXT_REPEAT[gs.repeat] }); break;
            case "q":
                if (queuePane || (gs?.queue_length ?? 0) > 0) queuePane = !queuePane;
                break;
        }
    }
</script>

<MusicSheet
    label="Now playing"
    eyebrow={queuePane ? "Queue" : "Playing on"}
    title={sonos.groupTitle(g)}
    backIcon={queuePane ? "chevronLeft" : "chevronDown"}
    backLabel={queuePane ? "Back to now playing" : "Collapse player"}
    onBack={() => (queuePane ? (queuePane = false) : onClose())}
    onDismiss={onClose}
    action={{ icon: "close", label: "Close player", onClick: onClose }}
    bind:scrollEl
    bind:sheetEl
    bind:dismissing
>
    {#if queuePane}
        <QueuePane
            items={sonos.queue}
            loading={sonos.queueLoading}
            total={gs?.queue_length ?? sonos.queue.length}
            currentTrack={st?.queue_track}
            playing={!!st?.playing}
            clearBusy={!c || busy.is("qclear:" + c?.id)}
            isBusy={(k) => busy.is(k)}
            onJump={(track) => sonos.jumpTo(g, track)}
            onRemove={(track) => sonos.removeQueued(g, track)}
            onClear={onClearQueue}
        />
    {:else}
        <PlayerArt
            artUri={st?.track?.art_uri}
            sheetDismissing={dismissing}
            onSkip={(dir) => sonos.skip(g, dir)}
        />

        <PlayerMeta
            title={st?.track?.title ?? "Nothing playing"}
            sub={st?.track?.title
                ? [st.track.artist, st.track.album].filter(Boolean).join(" · ")
                : "Start a favorite below, or search Spotify."}
            idle={!st?.track?.title}
        />

        <!-- The rail is a real control only where the source reports a
             duration. Radio and line-in don't, so they get an honest label
             instead of a scrubber that would be refused. -->
        {#if durationSec > 0}
            <div class="p-scrub">
                <Slider
                    variant="scrub"
                    max={durationSec}
                    value={livePos}
                    label="Seek"
                    valueText="{fmtSecs(livePos)} of {fmtSecs(durationSec)}"
                    disabled={!c}
                    onInput={(v) => (scrubSec = v)}
                    onChange={commitSeek}
                />
                <div class="p-times mono">
                    <span>{fmtSecs(livePos)}</span><span>{fmtSecs(durationSec)}</span>
                </div>
            </div>
        {:else if st?.track?.title}
            <div class="p-live mono">live stream — no track position</div>
        {/if}

        <PlayerTransport
            playing={sonos.isPlaying(g)}
            onToggle={() => sonos.togglePlay(g)}
            toggleBusy={!c || busy.is("play:" + c?.id)}
            onPrev={() => sonos.skip(g, "previous")}
            prevBusy={!c || busy.is("previous:" + c?.id)}
            onNext={() => sonos.skip(g, "next")}
            nextBusy={!c || busy.is("next:" + c?.id)}
            seekable={durationSec > 0}
            modes={gs
                ? {
                      shuffle: gs.shuffle,
                      repeat: gs.repeat,
                      repeatLabel: repeatLabel(gs.repeat),
                      busy: !c || busy.is("mode:" + c?.id),
                      onShuffle: () => sonos.setPlayMode(g, { shuffle: !gs.shuffle }),
                      onRepeat: () => sonos.setPlayMode(g, { repeat: NEXT_REPEAT[gs.repeat] }),
                  }
                : undefined}
        />

        <!-- The keys are only worth advertising where there is a keyboard;
             phones get the swipe gesture instead. -->
        <p class="p-keys mono" aria-hidden="true">
            space play · ← → seek · shift ← → track · ↑ ↓ volume · m mute · s shuffle · r repeat · q queue
        </p>

        {#if gs}
            <div class="p-extras">
                <!-- Crossfade is a preference, not a device state, so it is a
                     chip rather than a switch. -->
                <button
                    class="chip"
                    class:on={gs.crossfade}
                    aria-pressed={gs.crossfade}
                    disabled={!c || busy.is("xfade:" + c?.id)}
                    onclick={() => sonos.toggleCrossfade(g)}
                >
                    Crossfade
                </button>
                <!-- "Continue play similar": once the queue runs out, keep
                     going with similar tracks rather than falling silent —
                     Sonos has no such concept, so this is a HomeHub
                     preference, the same "chip, not a switch" shape as
                     crossfade. -->
                <button
                    class="chip"
                    class:on={!!c?.autoplay}
                    aria-pressed={!!c?.autoplay}
                    disabled={!c || busy.is("autoplay:" + c?.id)}
                    onclick={() => sonos.toggleAutoplay(g)}
                >
                    Autoplay
                </button>
                {#if gs.queue_length > 0}
                    <button class="p-upnext" onclick={() => (queuePane = true)}>
                        <Icon name="queue" size={17} />
                        <span class="up-body">
                            <span class="up-label">Up next</span>
                            <span class="up-track">
                                {sonos.nextInQueue(st?.queue_track)?.title ?? "End of the queue"}
                            </span>
                        </span>
                        <span class="up-count mono">{gs.queue_length}</span>
                        <span class="up-go" aria-hidden="true">
                            <Icon name="chevronLeft" size={16} />
                        </span>
                    </button>
                {/if}
            </div>
        {/if}

        <!-- Somewhere to go, playing or not: swapping a song out is as
             ordinary a thing to want here as starting the first one.
             Favorites still only stand in for an empty player — with a track
             up, the row that matters is the search. -->
        {@render startSomething(st?.track?.title ? null : g.coordinator_id)}

        <div class="p-speakers">
            <div class="eyrow">Volume</div>
            {#if grouped}
                <VolumeRow
                    name="All rooms"
                    value={sonos.shownGroupVolume(g.coordinator_id)}
                    label="Group volume"
                    onInput={(v) => sonos.dragGroupVolume(g.coordinator_id, v)}
                    onChange={(v) => sonos.setGroupVolume(g.coordinator_id, v)}
                />
                <div class="m-divider" aria-hidden="true"></div>
            {/if}
            {#each g.member_ids as id (id)}
                {@const sp = sonos.speakerById.get(id)}
                {#if sp}
                    <VolumeRow
                        name={sp.name}
                        value={sonos.shownVolume(sp)}
                        label="{sp.name} volume"
                        mute={{
                            muted: !!sp.state?.muted,
                            busy: busy.is("mute:" + sp.id),
                            onToggle: () => sonos.toggleMute(sp),
                        }}
                        onRemove={grouped ? () => sonos.leave(sp.id) : undefined}
                        removeBusy={busy.is("leave:" + sp.id)}
                        onInput={(v) => sonos.dragVolume(sp.id, v)}
                        onChange={(v) => sonos.setVolume(sp.id, v)}
                    />
                {/if}
            {/each}
            {#if sonos.joinables(g).length > 0}
                <div class="joiners">
                    {#each sonos.joinables(g) as sp (sp.id)}
                        <button
                            class="chip"
                            disabled={busy.is("join:" + sp.id)}
                            onclick={() => sonos.join(sp.id, g)}
                        >
                            <Icon name="plus" size={13} /> {sp.name}
                        </button>
                    {/each}
                </div>
            {/if}
            {#if g.unregistered?.length}
                <div class="unreg mono">
                    also in this group: {g.unregistered.join(", ")} — add them to control here
                </div>
            {/if}
        </div>
    {/if}
</MusicSheet>

<style>
    .p-scrub { display: flex; flex-direction: column; gap: 6px; }
    .p-times { display: flex; justify-content: space-between; font-size: 11px; color: var(--text-dim); }
    .p-live {
        text-align: center; font-size: 10.5px; letter-spacing: 0.08em;
        text-transform: uppercase; color: var(--text-dim);
    }

    .p-keys { display: none; }
    @media (hover: hover) and (pointer: fine) {
        .p-keys {
            display: block; text-align: center;
            font-size: 10px; letter-spacing: 0.06em;
            color: var(--text-dim);
        }
    }

    .p-extras { display: flex; flex-direction: column; gap: var(--space-3); }
    .p-extras .chip { align-self: flex-start; }
    /* Up next doubles as the way into the queue pane. */
    .p-upnext {
        display: flex; align-items: center; gap: var(--space-3);
        min-height: 56px; padding: 10px var(--space-3);
        background: var(--card); border: 1px solid var(--hairline);
        border-radius: var(--r-md);
        color: var(--text-mute); cursor: pointer; text-align: left; font: inherit;
        transition: border-color var(--t-fast);
    }
    @media (hover: hover) { .p-upnext:hover { border-color: var(--border-strong); } }
    .up-body { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
    .up-label {
        font-family: var(--font-mono);
        font-size: 10px; letter-spacing: 0.1em; text-transform: uppercase;
        color: var(--text-dim);
    }
    .up-track {
        font-size: 13px; color: var(--text);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .up-count { font-size: 12px; color: var(--text-dim); flex-shrink: 0; }
    .up-go { display: flex; transform: rotate(180deg); flex-shrink: 0; }

    .p-speakers { display: flex; flex-direction: column; gap: 2px; }
    .p-speakers .eyrow { margin-bottom: var(--space-1); }
    .m-divider { height: 1px; background: var(--hairline); margin: var(--space-2) 0; }
    .joiners { display: flex; flex-wrap: wrap; gap: var(--space-2); margin-top: var(--space-2); }
    .unreg { font-size: 11px; color: var(--text-dim); margin-top: var(--space-2); }

    @media (prefers-reduced-motion: reduce) {
        .p-upnext { transition-duration: 0.001ms; }
    }
</style>
