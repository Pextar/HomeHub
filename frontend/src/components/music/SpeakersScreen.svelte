<script lang="ts">
    /**
     * Speakers — the device inventory and its settings.
     *
     * Zones answers "what plays together"; this answers "what is each of these,
     * and how is it set up". It is a screen, not a sheet, because its rows open
     * a speaker's settings one level further and a sheet must never open
     * another sheet (DESIGN.md §15). The content is also too long to spend its
     * life at 92vh.
     *
     * Two panes where the width allows, one where it doesn't: from 1024px the
     * list column stays put and the settings open beside it, because the
     * dominant job is the same change across several rooms and drilling in and
     * out for each one is what makes that tedious. Below that the settings
     * replace the list and carry a switcher chip row instead.
     *
     * The two selections are separate rather than one keyed by id: the bridges'
     * detail views take different props and answer different questions, and a
     * shared selection would mean deciding which component to render from the
     * shape of an id.
     */
    import Icon from "../Icon.svelte";
    import NavRow from "./NavRow.svelte";
    import Waveform from "./Waveform.svelte";
    import SonosSpeakerDetail from "../../views/SonosSpeakerDetail.svelte";
    import KEFSpeakerDetail from "../../views/KEFSpeakerDetail.svelte";
    import { api } from "../../lib/api";
    import { kefSourceLabel } from "../../lib/kef";
    import Slider from "./Slider.svelte";
    import type { SonosBridge } from "../../lib/music/sonos.svelte";
    import type { KEFBridge } from "../../lib/music/kef.svelte";
    import type { AirPlayBridge } from "../../lib/music/airplay.svelte";
    import type {
        SonosSpeakerView,
        KEFSpeakerView,
        AirPlaySpeakerView,
    } from "../../lib/types";

    let {
        sonos,
        kef,
        airplay,
        totalSpeakers,
        readyCount,
        onBack,
        onAdd,
        onEditSonos,
        onEditKEF,
        onEditAirPlay,
        onOpenEvents,
        onOpenQuality,
        /** Opening a KEF speaker points the destination at it, when it can take one. */
        onKEFOpened,
        /** Bound so the KEF player's settings chip can push straight into a pane. */
        detailId = $bindable<string | null>(null),
        kefDetailId = $bindable<string | null>(null),
    }: {
        sonos: SonosBridge;
        kef: KEFBridge;
        airplay: AirPlayBridge;
        totalSpeakers: number;
        readyCount: number;
        onBack: () => void;
        onAdd: () => void;
        onEditSonos: (sp: SonosSpeakerView) => void;
        onEditKEF: (sp: KEFSpeakerView) => void;
        onEditAirPlay: (sp: AirPlaySpeakerView) => void;
        onOpenQuality: () => void;
        onOpenEvents: () => void;
        onKEFOpened: (sp: KEFSpeakerView) => void;
        detailId?: string | null;
        kefDetailId?: string | null;
    } = $props();

    const detailSpeaker = $derived(
        detailId ? (sonos.status?.speakers.find((s) => s.id === detailId) ?? null) : null,
    );
    const kefDetailSpeaker = $derived(
        kefDetailId ? (kef.speakers.find((s) => s.id === kefDetailId) ?? null) : null,
    );
    const kefDetailSiblings = $derived(kef.speakers.filter((s) => s.id !== kefDetailId));
    /** Whichever pane is open — the split layout folds the list away for both. */
    const anyDetail = $derived(!!detailSpeaker || !!kefDetailSpeaker);
    /** Speakers other than the open one, for the phone switcher. */
    const detailSiblings = $derived(sonos.allSpeakers.filter((s) => s.id !== detailId));

    /**
     * The sleep timer belongs to the zone, not the speaker (DESIGN.md §15), so
     * a follower is told which room owns it rather than being given a control
     * the coordinator would answer for.
     */
    const detailSleepOwner = $derived.by(() => {
        const sp = detailSpeaker;
        if (!sp) return null;
        const g = sonos.groupOfSpeaker(sp.id);
        return g && g.coordinator_id !== sp.id
            ? (sonos.speakerById.get(g.coordinator_id)?.name ?? null)
            : null;
    });

    let paned = $state(false);
    $effect(() => {
        const mq = window.matchMedia("(min-width: 1024px)");
        const sync = () => (paned = mq.matches);
        sync();
        mq.addEventListener("change", sync);
        return () => mq.removeEventListener("change", sync);
    });

    // A blank right-hand pane is dead space, so the wide layout opens on the
    // first speaker that can answer. On a phone nothing is selected until the
    // user picks a row — there, selecting means leaving the list.
    $effect(() => {
        if (!paned || detailId || kefDetailId) return;
        const first = sonos.allSpeakers.find((s) => s.reachable);
        if (first) {
            detailId = first.id;
            return;
        }
        // A house with only KEF speakers still deserves an open pane.
        const firstKEF = kef.speakers.find((s) => s.reachable);
        if (firstKEF) kefDetailId = firstKEF.id;
    });

    /**
     * An unreachable speaker has no settings to read, so its row goes where the
     * only useful action is instead: the registration form, which is where a
     * wrong address gets fixed. A form is a sheet, per §11.
     */
    function openSonosSpeaker(sp: SonosSpeakerView) {
        if (!sp.reachable) {
            onEditSonos(sp);
            return;
        }
        kefDetailId = null; // one pane at a time
        detailId = sp.id;
    }
    function openKEFSpeaker(sp: KEFSpeakerView) {
        detailId = null;
        kefDetailId = sp.id;
        if (sp.reachable) onKEFOpened(sp);
    }

    // Speakers that turned out to have no picture of their own. Remembered per
    // id so a 404 isn't re-requested every time the list re-renders.
    let noImage = $state<Record<string, boolean>>({});

    /** Escape backs out of an open pane before it leaves the screen. */
    export function closeDetail(): boolean {
        if (detailId) {
            detailId = null;
            return true;
        }
        if (kefDetailId) {
            kefDetailId = null;
            return true;
        }
        return false;
    }
</script>

<!-- On a phone an open speaker replaces the list, and that pane carries its own
     §11 head — two back chips on one screen would be one too many, so this one
     stands down. -->
{#if !(anyDetail && !paned)}
    <div class="screen-head">
        <button class="icon-btn" aria-label="Back to Music" onclick={onBack}>
            <Icon name="chevronLeft" size={18} />
        </button>
        <div class="screen-title">
            <h1>Speakers</h1>
            <span class="screen-sub">
                <span class="mono">{totalSpeakers}</span>
                registered · <span class="mono">{readyCount}</span> reachable
            </span>
        </div>
        <button class="icon-btn" aria-label="Add speaker" onclick={onAdd}>
            <Icon name="plus" size={16} />
        </button>
    </div>
{/if}

<div class="sp-split" class:has-detail={anyDetail}>
    <div class="sp-col">
    {#if sonos.allSpeakers.length > 0}
    <section class="block">
        <div class="block-head">
            <!-- Named by bridge once there are two: "what is this thing and
                 how is it configured" has a different answer per protocol,
                 and the two lists don't interleave into anything meaningful. -->
            <div class="eyrow">{kef.speakers.length > 0 ? "Sonos" : "Speakers"}</div>
            <span class="hint">
                <span class="mono">{sonos.reachable.length}</span>
                of <span class="mono">{sonos.allSpeakers.length}</span> reachable
            </span>
        </div>
        <div class="sp-list">
            <!-- One target per row, the §11 shape: chevron right, into that
                 speaker's settings. Editing its registration lives on the
                 detail's action chip rather than as a second control here. -->
            {#each sonos.allSpeakers as sp (sp.id)}
                {@const playing = sonos.speakerPlaying(sp.id)}
                <button
                    class="sp-row"
                    class:off={!sp.reachable}
                    class:sel={detailId === sp.id}
                    aria-current={detailId === sp.id ? "true" : undefined}
                    onclick={() => openSonosSpeaker(sp)}
                >
                    <!-- The speaker's own portrait, served by the device.
                         No picture published means the striped placeholder
                         — never a guess at which model this is (§2). -->
                    {#if noImage[sp.id]}
                        <!-- §6.7's striped fill, without its caption: no
                             wording fits a 40px box, and the row's name
                             and model already say what this is. -->
                        <span class="shot placeholder" aria-hidden="true"></span>
                    {:else}
                        <img
                            class="shot"
                            src={api.sonosImageURL(sp.id)}
                            alt=""
                            loading="lazy"
                            onerror={() => (noImage[sp.id] = true)}
                        />
                    {/if}
                    <span class="sp-meta">
                        <span class="sp-name">{sp.name}</span>
                        <span class="sp-sub">
                            {#if !sp.reachable}
                                Unreachable · <span class="mono">{sp.ip}</span>
                            {:else}
                                {[sp.model, sp.room].filter(Boolean).join(" · ") || sp.ip}
                            {/if}
                        </span>
                    </span>
                    {#if playing}
                        <Waveform />
                    {/if}
                    <span class="sp-chev" aria-hidden="true"><Icon name="chevronDown" size={18} /></span>
                </button>
            {/each}
        </div>
        <p class="hint">
            Tone, night mode, the status light and the touch controls are the
            speaker's own settings — they stay set whatever is playing.
        </p>
    </section>
    {/if}

    <!-- ── KEF ─────────────────────────────────────────────────────────
         Its own list, not interleaved with the Sonos one: the row's sub-line
         means different things (a Sonos row leads with its zone, a KEF row
         with its input), and the screen each one opens answers a different
         set of questions. -->
    {#if kef.speakers.length > 0}
    <section class="block">
        <div class="block-head">
            <div class="eyrow">KEF</div>
            <span class="hint">
                <span class="mono">{kef.reachable.length}</span>
                of <span class="mono">{kef.speakers.length}</span> reachable
            </span>
        </div>
        <div class="sp-list">
            {#each kef.speakers as sp (sp.id)}
                <button
                    class="sp-row"
                    class:off={!sp.reachable}
                    class:sel={kefDetailId === sp.id}
                    aria-current={kefDetailId === sp.id ? "true" : undefined}
                    onclick={() => (sp.reachable ? openKEFSpeaker(sp) : onEditKEF(sp))}
                >
                    <!-- KEF publishes no picture of itself the way Sonos
                         does, so this is the §6.7 striped fill rather than a
                         stock photo that might show the wrong model (§2). -->
                    <span class="shot placeholder" aria-hidden="true"></span>
                    <span class="sp-meta">
                        <span class="sp-name">{sp.name}</span>
                        <span class="sp-sub">
                            {#if !sp.reachable}
                                Unreachable · <span class="mono">{sp.ip}</span>
                            {:else if !sp.state?.powered_on}
                                Standby · {[sp.model, sp.room].filter(Boolean).join(" · ") || sp.ip}
                            {:else}
                                {[kefSourceLabel(sp.state?.source), sp.model, sp.room]
                                    .filter(Boolean).join(" · ") || sp.ip}
                            {/if}
                        </span>
                    </span>
                    {#if kef.isPlaying(sp)}
                        <Waveform />
                    {/if}
                    <span class="sp-chev" aria-hidden="true"><Icon name="chevronDown" size={18} /></span>
                </button>
            {/each}
        </div>
        <p class="hint">
            KEF speakers stand alone — no grouping, no shared queue — so their
            input, volume and EQ all live on the speaker's own screen.
        </p>
    </section>
    {/if}

    <!-- ── AirPlay ─────────────────────────────────────────────────────
         A list and nothing more, because a receiver is a sink: it has no
         settings screen to open, no input to pick and no now-playing to read
         — what it is playing is whatever HomeHub is sending it, which the
         room's own player already says. The row carries the one control that
         is genuinely the receiver's own, and the row itself opens the
         registration form, since that is the only thing left to change. -->
    {#if airplay.receivers.length > 0}
    <section class="block">
        <div class="block-head">
            <div class="eyrow">AirPlay</div>
            <span class="hint">
                <span class="mono">{airplay.receivers.length}</span> registered
            </span>
        </div>
        <div class="sp-list">
            {#each airplay.receivers as sp (sp.id)}
                <div class="sp-row ap-row" class:on={sp.casting}>
                    <span class="shot placeholder" aria-hidden="true"></span>
                    <span class="sp-meta">
                        <button class="ap-open" onclick={() => onEditAirPlay(sp)}>
                            <span class="sp-name">{sp.name}</span>
                            <span class="sp-sub">
                                {[sp.model, sp.room].filter(Boolean).join(" · ") || sp.ip}
                            </span>
                        </button>
                        <!-- Sent when a cast is running, remembered when one
                             isn't: a receiver only takes a level inside a
                             session, so this is what the next cast opens
                             with rather than a control that does nothing. -->
                        <Slider
                            value={airplay.shownVolume(sp)}
                            label={`${sp.name} volume`}
                            valueText={`${airplay.shownVolume(sp)}%`}
                            onInput={(v) => airplay.dragVolume(sp, v)}
                            onChange={(v) => airplay.setVolume(sp, v)}
                        />
                    </span>
                    {#if sp.casting}
                        <Waveform />
                    {/if}
                </div>
            {/each}
        </div>
        <p class="hint">
            AirPlay receivers are sent audio by HomeHub rather than playing for
            themselves, so they have no settings of their own here. Add one to a
            room to play to it.
        </p>
    </section>
    {/if}

    <!-- ── Live updates ────────────────────────────────────────────────
         Speakers is where the devices are managed, so it is where the
         plumbing behind them belongs. The topbar chip says which state we're
         in; this row is the discoverable way in for someone who never
         noticed it. -->
    {#if sonos.allSpeakers.length > 0}
    <NavRow
        icon={sonos.livePush ? "bolt" : "radio"}
        on={sonos.livePush}
        title="Live updates"
        onClick={onOpenEvents}
    >
        {#snippet sub()}
            {#if sonos.livePush}
                Speakers push their changes — this app keeps up in real time
            {:else}
                Speakers are being polled — changes take a few seconds to show
            {/if}
        {/snippet}
    </NavRow>
    {/if}

    <!-- ── Sound quality ───────────────────────────────────────────────
         Here rather than in Settings because it is a fact about these
         devices: what reaches a speaker depends on which of them are in the
         room, and this is the screen where that is being arranged. -->
    <NavRow icon="radio" title="Sound quality" onClick={onOpenQuality}>
        {#snippet sub()}
            What actually reaches each speaker, and the one part of it you can change
        {/snippet}
    </NavRow>
    </div><!-- /.sp-col -->

    {#if detailSpeaker}
        <div class="sp-pane">
            <SonosSpeakerDetail
                speaker={detailSpeaker}
                sleepTimerOwner={detailSleepOwner}
                {paned}
                siblings={detailSiblings}
                onPick={(id) => (detailId = id)}
                onBack={() => (detailId = null)}
                onEdit={() => onEditSonos(detailSpeaker)}
            />
        </div>
    {:else if kefDetailSpeaker}
        <div class="sp-pane">
            <KEFSpeakerDetail
                speaker={kefDetailSpeaker}
                {paned}
                siblings={kefDetailSiblings}
                onPick={(id) => (kefDetailId = id)}
                onBack={() => (kefDetailId = null)}
                onEdit={() => onEditKEF(kefDetailSpeaker)}
                onChanged={() => void kef.refresh()}
            />
        </div>
    {/if}
    </div><!-- /.sp-split -->
<style>
    /* An AirPlay row is a container rather than a button: the name opens the
       registration form and the slider is its own control, and nesting a
       slider inside a button would make the whole row swallow the drag. */
    .ap-row { cursor: default; }
    .ap-row .sp-meta { gap: 6px; }
    .ap-open {
        display: flex; flex-direction: column; gap: 1px;
        background: none; border: 0; padding: 0; margin: 0;
        font: inherit; color: inherit; text-align: left; cursor: pointer;
        min-height: 32px;
    }
    @media (pointer: coarse) {
        .ap-open { min-height: 44px; justify-content: center; }
    }
    /* ── Screen head (Speakers) ──
       The §11 detail shape — back chip, centered title, action chip — because
       Speakers is a screen pushed from Home, not a sheet lifted over it. */
    .screen-head {
        display: flex; align-items: center; gap: var(--space-3);
    }
    .screen-title {
        flex: 1; min-width: 0;
        display: flex; flex-direction: column; gap: 2px;
        text-align: center;
    }
    .screen-title h1 {
        font-family: var(--font-sans);
        font-size: 20px; font-weight: 600; letter-spacing: -0.02em;
        color: var(--text);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .screen-sub { font-size: 12px; color: var(--text-mute); }

    /* ── Speaker list (Speakers) ──────────────────────────────────────
       The §11 list-row shape, with the device's own portrait standing in for
       the 36px icon — it is the one place in the app where a real photograph
       beats a glyph, because telling a Sonos One from a Five is exactly what
       the user is doing here. */
    /* Two panes from 1024px, one below it. The list column folds away on a
       phone once a speaker is open, which is what turns the same markup into
       a drill-down without a second copy of it. */
    .sp-split { display: flex; flex-direction: column; gap: var(--space-4); }
    .sp-col { display: flex; flex-direction: column; gap: var(--space-4); min-width: 0; }
    .sp-pane { min-width: 0; }
    @media (max-width: 1023px) {
        .sp-split.has-detail > .sp-col { display: none; }
    }
    @media (min-width: 1024px) {
        .sp-split {
            display: grid;
            grid-template-columns: minmax(260px, 330px) minmax(0, 1fr);
            gap: var(--space-5);
            align-items: start;
        }
        /* The list is the shorter column; let the settings scroll past it
           rather than stretching the rows to match. */
        .sp-col { position: sticky; top: 76px; }
    }

    .sp-list { display: flex; flex-direction: column; gap: 6px; }
    .sp-row {
        width: 100%;
        display: flex;
        align-items: center;
        gap: var(--space-3);
        min-height: 60px;
        padding: var(--space-3) var(--space-4);
        text-align: left;
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        border-radius: var(--radius-lg);
        color: inherit;
        font: inherit;
        cursor: pointer;
        transition: background var(--t-fast), border-color var(--t-fast);
    }
    .shot {
        width: 40px;
        height: 40px;
        flex-shrink: 0;
        border-radius: var(--radius-md);
        object-fit: contain;
        background: var(--surface);
    }
    /* Caption dropped (see markup); the striped fill carries the meaning. */
    .sp-meta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
    .sp-name {
        font-size: 14px; font-weight: 600; letter-spacing: -0.01em;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .sp-sub {
        font-size: 12px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .sp-row.off .shot { opacity: 0.45; }
    .sp-row.off .sp-name { color: var(--text-mute); }
    /* Which row the pane is showing. An amber edge, not the .tile.on
       gradient — that treatment means "this device is on", and a selected
       row is a statement about the screen, not about the speaker. */
    .sp-row.sel { border-color: var(--on); background: var(--card-2); }
    .sp-chev { flex-shrink: 0; display: flex; color: var(--text-dim); transform: rotate(-90deg); }
    /* Beside the pane the chevron is redundant — the row's amber edge already
       says which one is open, and there is nowhere further to go. */
    @media (min-width: 1024px) {
        .sp-chev { display: none; }
    }
    @media (hover: hover) {
        .sp-row:hover { background: var(--bg-raised); border-color: var(--border-strong); }
    }

</style>
