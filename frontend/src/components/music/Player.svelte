<script lang="ts">
    /**
     * The player — one sheet, for any room.
     *
     * There used to be three of these: a Sonos one, a KEF one and a zone one,
     * each a near-copy of the others with two or three sections swapped. They
     * drifted, they disagreed about small things, and which one you got
     * depended on a distinction the user never asked to care about. There is
     * one now, and it renders what the room says it can do:
     *
     *   queue          — a Sonos group's, because only Sonos has one
     *   seek           — where there is a track with a length behind it
     *   skip           — everywhere except a zone HomeHub is streaming to,
     *                    where the Spotify session is HomeHub's own
     *   play modes     — a Sonos coordinator's shuffle / repeat / crossfade
     *   input selector — a KEF speaker's, which is its equivalent of a source
     *   route note     — a zone's, in the backend's own words
     *   faders         — one per speaker in the room, whatever make it is,
     *                    plus the room-wide one when there is more than one
     *
     * Nothing here infers a capability from a make. A control that would be
     * refused is worse than a control that isn't there.
     */
    import { onMount } from "svelte";
    import Icon from "../Icon.svelte";
    import MusicSheet from "./MusicSheet.svelte";
    import PlayerArt from "./PlayerArt.svelte";
    import PlayerMeta from "./PlayerMeta.svelte";
    import PlayerTransport from "./PlayerTransport.svelte";
    import TrackRail from "./TrackRail.svelte";
    import QueuePane from "./QueuePane.svelte";
    import VolumeRow from "./VolumeRow.svelte";
    import ZoneRoute from "./ZoneRoute.svelte";
    import { kefSourceLabel, KEF_SOURCES } from "../../lib/kef";
    import { NEXT_REPEAT, repeatLabel } from "../../lib/music/sonos.svelte";
    import type { Room, RoomsModel } from "../../lib/music/rooms.svelte";
    import type { SonosBridge } from "../../lib/music/sonos.svelte";
    import type { KEFBridge } from "../../lib/music/kef.svelte";
    import type { Busy } from "../../lib/music/busy.svelte";
    import type { Snippet } from "svelte";

    let {
        room: r,
        rooms,
        sonos,
        kef,
        busy,
        onClose,
        /** What this room is made of — the zone editor, or a speaker's settings. */
        onConfigure,
        /** Take a natively grouped room apart. Absent when there's nothing to undo. */
        onUngroup,
        /** Clearing the queue stops playback, so the confirm is the caller's. */
        onClearQueue,
        /** Stop *and* hand the Spotify session back — a zone on the stream route. */
        onStop,
        startSomething,
        scrollEl = $bindable<HTMLElement | null>(null),
        sheetEl = $bindable<HTMLElement | null>(null),
        dismissing = $bindable(false),
    }: {
        room: Room;
        rooms: RoomsModel;
        sonos: SonosBridge;
        kef: KEFBridge;
        busy: Busy;
        onClose: () => void;
        onConfigure: () => void;
        onUngroup?: () => void;
        onClearQueue: () => void;
        onStop: () => void;
        startSomething: Snippet<[string | null]>;
        scrollEl?: HTMLElement | null;
        sheetEl?: HTMLElement | null;
        dismissing?: boolean;
    } = $props();

    const playing = $derived(rooms.isPlaying(r));
    const title = $derived(rooms.title(r));
    const duration = $derived(rooms.durationSec(r));
    const seekable = $derived(r.canSeek && duration > 0);
    const gs = $derived(rooms.groupState(r));
    const faders = $derived(rooms.faders(r));

    // From the desktop shell's breakpoint up the player is a stage: art on
    // the left, everything that drives it on the right, and the queue becomes
    // the right column rather than swapping the whole sheet. Below it, the
    // layout wrappers vanish and nothing about the phone changes.
    //
    // Read synchronously rather than starting `false` and correcting in
    // `onMount`: the sheet's entrance transition starts the instant this
    // component mounts, and on desktop that used to mean animating in at
    // phone-dialog size and then jumping to the 1120×740 stage a beat later
    // — a layout resize fighting a transform/opacity animation on the same
    // frame, which is what read as laggy.
    let wide = $state(window.matchMedia("(min-width: 901px)").matches);
    onMount(() => {
        const mq = window.matchMedia("(min-width: 901px)");
        const update = () => (wide = mq.matches);
        mq.addEventListener("change", update);
        return () => mq.removeEventListener("change", update);
    });

    let queuePane = $state(false);
    /** The queue belongs to a Sonos group; nothing else has one to show. */
    const queueLength = $derived(r.canQueue ? (gs?.queue_length ?? 0) : 0);

    /** What the room is called in the sheet's own words, in every state. */
    const meta = $derived.by(() => {
        if (title) {
            return { title, sub: rooms.subLine(r) || rooms.memberLine(r), idle: false };
        }
        if (!r.reachable) {
            return { title: "Not answering", sub: "Check it under Speakers.", idle: true };
        }
        if (r.speaker && !r.speaker.state?.powered_on) {
            return { title: "Standby", sub: "Press play to wake it.", idle: true };
        }
        return {
            title: "Nothing playing",
            sub: "Find something below to start it here.",
            idle: true,
        };
    });

    /** The name of the thing the header's action opens. */
    const configureLabel = $derived(r.kind === "zone" ? `Edit ${r.name}` : `${r.name} settings`);

    // The two panes share one scroll container, so switching has to rewind it
    // — otherwise the queue opens halfway down at the player's offset.
    $effect(() => {
        void queuePane;
        if (scrollEl) scrollEl.scrollTop = 0;
    });
    // A room whose queue disappears (regrouped, or swapped for a KEF) must not
    // leave the sheet stuck on a pane that no longer has anything in it.
    $effect(() => {
        if (queuePane && !r.canQueue) queuePane = false;
    });

    /** The transport keys this room can actually answer, and no others. */
    export function handleKey(e: KeyboardEvent, opts: { slider: boolean; onControl: boolean }) {
        const key = e.key.length === 1 ? e.key.toLowerCase() : e.key;
        // Space on a focused button belongs to that button, not to us.
        if ((key === " " || key === "k") && !(key === " " && opts.onControl)) {
            e.preventDefault();
            rooms.togglePlay(r);
            return;
        }
        if (opts.slider) return;
        const pos = rooms.livePosition(r);
        switch (key) {
            case "ArrowRight":
                if (!seekable) {
                    if (!r.canSkip) return;
                    e.preventDefault();
                    rooms.skip(r, "next");
                    return;
                }
                e.preventDefault();
                if (e.shiftKey) rooms.skip(r, "next");
                else rooms.seek(r, Math.min(duration, pos + 10));
                break;
            case "ArrowLeft":
                if (!seekable) {
                    if (!r.canSkip) return;
                    e.preventDefault();
                    rooms.skip(r, "previous");
                    return;
                }
                e.preventDefault();
                if (e.shiftKey) rooms.skip(r, "previous");
                else rooms.seek(r, Math.max(0, pos - 10));
                break;
            case "ArrowUp":
                e.preventDefault();
                rooms.nudgeVolume(r, 5);
                break;
            case "ArrowDown":
                e.preventDefault();
                rooms.nudgeVolume(r, -5);
                break;
            case "n":
                rooms.skip(r, "next");
                break;
            case "p":
                rooms.skip(r, "previous");
                break;
            case "m":
                rooms.toggleMute(r);
                break;
            case "s":
                if (r.group && gs) sonos.setPlayMode(r.group, { shuffle: !gs.shuffle });
                break;
            case "r":
                if (r.group && gs) sonos.setPlayMode(r.group, { repeat: NEXT_REPEAT[gs.repeat] });
                break;
            case "q":
                if (queuePane || queueLength > 0) queuePane = !queuePane;
                break;
        }
    }

    /** Which keys are worth advertising, for this room. */
    const keyLine = $derived(
        [
            "space play",
            seekable ? "← → seek" : r.canSkip ? "← → track" : "",
            seekable && r.canSkip ? "shift ← → track" : "",
            "↑ ↓ volume",
            "m mute",
            gs ? "s shuffle · r repeat" : "",
            queueLength > 0 ? "q queue" : "",
        ]
            .filter(Boolean)
            .join(" · "),
    );
</script>

<MusicSheet
    label="Now playing"
    eyebrow={queuePane ? "Queue" : "Playing on"}
    title={r.name}
    sub={r.grouped ? rooms.memberLine(r) : undefined}
    backIcon={queuePane ? "chevronLeft" : "chevronDown"}
    backLabel={queuePane ? "Back to now playing" : "Collapse player"}
    onBack={() => (queuePane ? (queuePane = false) : onClose())}
    onDismiss={onClose}
    action={{ icon: "sliders", label: configureLabel, onClick: onConfigure }}
    wide
    backdropUri={rooms.art(r)}
    bind:scrollEl
    bind:sheetEl
    bind:dismissing
>
    {#if queuePane && r.group && !wide}
        <!-- On a phone the queue is the sheet's second pane: it swaps the
             whole surface and puts it back on the way out. -->
        {@const c = sonos.coordinatorOf(r.group)}
        <QueuePane
            items={sonos.queue}
            loading={sonos.queueLoading}
            total={queueLength || sonos.queue.length}
            currentTrack={c?.state?.queue_track}
            {playing}
            clearBusy={!c || busy.is("qclear:" + c?.id)}
            isBusy={(k) => busy.is(k)}
            onJump={(track) => r.group && sonos.jumpTo(r.group, track)}
            onRemove={(track) => r.group && sonos.removeQueued(r.group, track)}
            onClear={onClearQueue}
        />
    {:else}
        <div class="st">
            <div class="st-left">
                <PlayerArt
                    artUri={rooms.art(r)}
                    sheetDismissing={dismissing}
                    large={wide}
                    onSkip={r.canSkip ? (dir) => rooms.skip(r, dir) : undefined}
                />
            </div>

            <div class="st-right">
                {#if queuePane && r.group}
                    <!-- On the stage the queue is the right column — the art stays,
                 because what's playing is still the point of the window. -->
                    {@const c = sonos.coordinatorOf(r.group)}
                    <QueuePane
                        items={sonos.queue}
                        loading={sonos.queueLoading}
                        total={queueLength || sonos.queue.length}
                        currentTrack={c?.state?.queue_track}
                        {playing}
                        clearBusy={!c || busy.is("qclear:" + c?.id)}
                        isBusy={(k) => busy.is(k)}
                        onJump={(track) => r.group && sonos.jumpTo(r.group, track)}
                        onRemove={(track) => r.group && sonos.removeQueued(r.group, track)}
                        onClear={onClearQueue}
                    />
                {:else}
                    <PlayerMeta title={meta.title} sub={meta.sub} idle={meta.idle} large={wide} />

                    <!-- What a play here does, in the backend's words. Above the transport
             because it qualifies it: on the stream route the skips below are
             absent, and this is the sentence that explains why. -->
                    {#if r.zone}
                        <ZoneRoute
                            route={r.zone.route}
                            sync={r.zone.sync}
                            reason={r.zone.reason}
                            problem={r.zone.problem}
                        />
                    {/if}

                    <TrackRail
                        position={rooms.livePosition(r)}
                        {duration}
                        {seekable}
                        idle={!title}
                        liveLabel={r.speaker
                            ? "no track position on this input"
                            : "live stream — no track position"}
                        onSeek={(sec) => rooms.seek(r, sec)}
                    />

                    <PlayerTransport
                        {playing}
                        onToggle={() => rooms.togglePlay(r)}
                        toggleBusy={rooms.playBusy(r) || !r.reachable}
                        onPrev={r.canSkip ? () => rooms.skip(r, "previous") : undefined}
                        prevBusy={rooms.prevBusy(r)}
                        onNext={r.canSkip ? () => rooms.skip(r, "next") : undefined}
                        nextBusy={rooms.nextBusy(r)}
                        large={wide}
                        {seekable}
                        modes={gs && r.group
                            ? {
                                  shuffle: gs.shuffle,
                                  repeat: gs.repeat,
                                  repeatLabel: repeatLabel(gs.repeat),
                                  busy: busy.is("mode:" + r.id),
                                  onShuffle: () =>
                                      r.group &&
                                      sonos.setPlayMode(r.group, { shuffle: !gs.shuffle }),
                                  onRepeat: () =>
                                      r.group &&
                                      sonos.setPlayMode(r.group, {
                                          repeat: NEXT_REPEAT[gs.repeat],
                                      }),
                              }
                            : undefined}
                    />

                    {#if r.zone && !r.canSkip}
                        <p class="hint centred">
                            HomeHub is the Spotify device while this room plays, so track changes
                            come from Spotify itself — skip there, and it follows here.
                        </p>
                    {/if}

                    <!-- The keys are only worth advertising where there is a keyboard;
             phones get the art swipe instead. -->
                    <p class="p-keys mono" aria-hidden="true">{keyLine}</p>

                    <!-- Stop, not just pause: it also hands the Spotify session back, which
             is what frees another room to take it. -->
                    {#if r.zone && (playing || title)}
                        <div class="centred-row">
                            <button
                                class="chip"
                                disabled={busy.is("zstop:" + r.id)}
                                onclick={onStop}
                            >
                                Stop and release Spotify
                            </button>
                        </div>
                    {/if}

                    {#if gs && r.group}
                        {@const c = sonos.coordinatorOf(r.group)}
                        <div class="p-extras">
                            <div class="p-chips">
                                <!-- Preferences, not device states, so chips rather than switches. -->
                                <button
                                    class="chip"
                                    class:on={gs.crossfade}
                                    aria-pressed={gs.crossfade}
                                    disabled={!c || busy.is("xfade:" + c?.id)}
                                    onclick={() => r.group && sonos.toggleCrossfade(r.group)}
                                >
                                    Crossfade
                                </button>
                                <button
                                    class="chip"
                                    class:on={!!c?.autoplay}
                                    aria-pressed={!!c?.autoplay}
                                    disabled={!c || busy.is("autoplay:" + c?.id)}
                                    onclick={() => r.group && sonos.toggleAutoplay(r.group)}
                                >
                                    Autoplay
                                </button>
                            </div>
                            {#if queueLength > 0}
                                <button class="p-upnext" onclick={() => (queuePane = true)}>
                                    <Icon name="queue" size={17} />
                                    <span class="up-body">
                                        <span class="up-label">Up next</span>
                                        <span class="up-track">
                                            {sonos.nextInQueue(c?.state?.queue_track)?.title ??
                                                "End of the queue"}
                                        </span>
                                    </span>
                                    <span class="up-count mono">{queueLength}</span>
                                    <span class="up-go" aria-hidden="true"
                                        ><Icon name="chevronLeft" size={16} /></span
                                    >
                                </button>
                            {/if}
                        </div>
                    {/if}

                    <!-- Somewhere to go, playing or not: swapping a song out is as ordinary
             a thing to want here as starting the first one. Favorites only
             stand in for an empty player — with a track up, the row that
             matters is the search. -->
                    {@render startSomething(title ? null : r.kind === "sonos" ? r.id : null)}

                    <div class="p-speakers">
                        <div class="eyrow">Volume</div>
                        {#if r.grouped}
                            <VolumeRow
                                name="All speakers"
                                value={rooms.volume(r)}
                                label="{r.name} volume"
                                onInput={(v) => rooms.dragVolume(r, v)}
                                onChange={(v) => rooms.setVolume(r, v)}
                            />
                            <div class="m-divider" aria-hidden="true"></div>
                        {/if}
                        {#each faders as f (f.key)}
                            <VolumeRow
                                name={f.name}
                                value={f.value}
                                label="{f.name} volume"
                                mute={{ muted: f.muted, busy: f.muteBusy, onToggle: f.onMute }}
                                onRemove={f.onRemove}
                                removeBusy={f.removeBusy}
                                onInput={f.onInput}
                                onChange={f.onChange}
                            />
                        {/each}
                        {#if r.group?.unregistered?.length}
                            <div class="p-note mono">
                                also in this room: {r.group.unregistered.join(", ")} — add them to control
                                here
                            </div>
                        {/if}
                        {#if r.zone?.speakers.some((sp) => sp.missing)}
                            <div class="p-note bad mono">
                                a speaker in this room no longer exists — edit it to drop it
                            </div>
                        {/if}
                    </div>

                    <!-- The question a KEF speaker raises that nothing else does: which
             input. Every model shows the same list — there is no "what inputs
             do you have" call, so a model without USB refuses it rather than
             the UI guessing. -->
                    {#if r.speaker}
                        {@const sp = r.speaker}
                        <div class="p-speakers">
                            <div class="eyrow">Input</div>
                            <div class="p-chips">
                                {#each KEF_SOURCES as src (src.value)}
                                    <button
                                        class="chip"
                                        class:on={sp.state?.source === src.value}
                                        aria-pressed={sp.state?.source === src.value}
                                        disabled={busy.is("kefsrc:" + sp.id)}
                                        onclick={() => kef.setSource(sp, src.value)}
                                        >{src.label}</button
                                    >
                                {/each}
                            </div>
                            <p class="hint">
                                Currently on {kefSourceLabel(sp.state?.source)}. Grouping this
                                speaker with another makes a HomeHub room, which streams to both.
                            </p>
                        </div>
                    {/if}

                    <!-- Taking the room apart. Native grouping undoes here; a room the user
             named and built is edited or deleted, which is a form, so the
             header's action owns it instead. -->
                    {#if onUngroup}
                        <div class="centred-row">
                            <button
                                class="chip"
                                disabled={rooms.ungroupBusy(r)}
                                onclick={onUngroup}
                            >
                                Split into {r.members.length} separate rooms
                            </button>
                        </div>
                    {/if}
                {/if}
            </div>
        </div>
    {/if}
</MusicSheet>

<style>
    /* Below the desktop shell's breakpoint the stage wrappers vanish, and the
       player's children stack in the sheet exactly as they always have. */
    .st,
    .st-left,
    .st-right {
        display: contents;
    }
    @media (min-width: 901px) {
        .st {
            display: grid;
            grid-template-columns: auto minmax(0, 1fr);
            gap: 52px;
            align-items: start;
            padding-top: var(--space-2);
        }
        .st-left {
            display: block;
            /* The art stays put while a long right column scrolls under it. */
            position: sticky;
            top: 48px;
        }
        .st-right {
            display: flex;
            flex-direction: column;
            gap: var(--space-5);
            min-width: 0;
        }
        .st-right > .centred {
            text-align: left;
        }
        .st-right > .centred-row {
            justify-content: flex-start;
        }
        .st-right .p-keys {
            text-align: left;
        }
    }

    .centred {
        text-align: center;
    }
    .centred-row {
        display: flex;
        justify-content: center;
    }

    .p-keys {
        display: none;
    }
    @media (hover: hover) and (pointer: fine) {
        .p-keys {
            display: block;
            text-align: center;
            font-size: 10px;
            letter-spacing: 0.06em;
            color: var(--text-dim);
        }
    }

    .p-extras {
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
    }
    .p-chips {
        display: flex;
        flex-wrap: wrap;
        gap: var(--space-2);
    }

    /* Up next doubles as the way into the queue pane. */
    .p-upnext {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        min-height: 56px;
        padding: 10px var(--space-3);
        background: var(--card);
        border: 1px solid var(--hairline);
        border-radius: var(--r-md);
        color: var(--text-mute);
        cursor: pointer;
        text-align: left;
        font: inherit;
        transition: border-color var(--t-fast);
    }
    @media (hover: hover) {
        .p-upnext:hover {
            border-color: var(--border-strong);
        }
    }
    .up-body {
        flex: 1;
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 2px;
    }
    .up-label {
        font-family: var(--font-mono);
        font-size: 10px;
        letter-spacing: 0.1em;
        text-transform: uppercase;
        color: var(--text-dim);
    }
    .up-track {
        font-size: 13px;
        color: var(--text);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
    .up-count {
        font-size: 12px;
        color: var(--text-dim);
        flex-shrink: 0;
    }
    .up-go {
        display: flex;
        transform: rotate(180deg);
        flex-shrink: 0;
    }

    .p-speakers {
        display: flex;
        flex-direction: column;
        gap: 2px;
    }
    .p-speakers .eyrow {
        margin-bottom: var(--space-1);
    }
    .m-divider {
        height: 1px;
        background: var(--hairline);
        margin: var(--space-2) 0;
    }
    .p-note {
        font-size: 11px;
        color: var(--text-dim);
        margin-top: var(--space-2);
    }
    .p-note.bad {
        color: var(--bad);
    }

    @media (prefers-reduced-motion: reduce) {
        .p-upnext {
            transition-duration: 0.001ms;
        }
    }
</style>
