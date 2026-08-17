<script lang="ts">
    /**
     * Grouping, on the screen you are already listening from
     * (DESIGN.md §16, the full-screen player's fourth pane).
     *
     * The wall's grouping used to live one depth back, in the music
     * screen's Rooms list — the right home for *choosing* a room, and the
     * wrong one for the sentence people actually walk up to the panel to
     * say: "put this in the kitchen too". You had to leave the record you
     * were listening to, find the room list, and aim at a chip that named
     * a group you could no longer see. This pane is that sentence, said
     * where the music is.
     *
     * Three ideas hold it together:
     *
     * - **One list, membership by tap.** Every Sonos room in the house is
     *   a row here. The rows above the rule are in the group; the ones
     *   below are not, and a tap moves a room across. No mode, no drag, no
     *   Done button to forget: the wall's whole gesture is glance + tap
     *   (§16), and the row moving *is* the confirmation. (The app's own
     *   Music view groups by dragging one card onto another — right for a
     *   phone with a room grid on screen; wrong for an iPad on a wall at
     *   arm's length, where a hold-then-drag over a five-second poll is the
     *   least reliable gesture there is.)
     * - **Grouping and balance are one surface.** The moment a second room
     *   joins, the next thing anyone wants is the kitchen quieter than the
     *   living room — so a member row carries its own fader and mute. The
     *   room-wide fader stays where it was, on the player above.
     * - **It says what a tap will cost before the tap.** A room that is
     *   playing something else says so on its row; joining stops that and
     *   follows this record instead. Sonos will do it either way; the wall
     *   is what has to be honest about it (§15.1).
     *
     * Only Sonos rooms appear, because only they group natively. A KEF
     * speaker and a HomeHub zone are absent rather than dead: a zone is an
     * arrangement someone built on purpose and is edited in the Music view,
     * never assembled from a kiosk.
     */
    import Icon from "../Icon.svelte";
    import Slider from "../music/Slider.svelte";
    import Waveform from "../music/Waveform.svelte";
    import type { PanelGrouping, PanelRooms, PanelSource, PanelVolume } from "../../lib/panel-music.svelte";

    let { music }: { music: PanelRooms & PanelGrouping & PanelVolume } = $props();

    const featured = $derived(music.featured);
    /** Lead first: it is the speaker the group's queue, modes and stream
     *  hang off, and the one row that can't step out on its own. */
    const members = $derived(
        [...(featured?.members ?? [])].sort(
            (a, b) => Number(b.coordinator) - Number(a.coordinator),
        ),
    );
    const joinable = $derived(music.joinable);
    /** A room of one speaker has nothing to balance against and no lead to
     *  be lead *of*: its fader is the player's own, three rows up, and a
     *  second copy of a control is worse than none (§15.5). So the row
     *  states the destination and stops there — the mixer appears with the
     *  company that gives it a job. */
    const multi = $derived(members.length > 1);

    /** What a room is doing right now, in the words the row has space for.
     *  Joining it stops this — worth reading before the tap, not after. */
    function busyWith(s: PanelSource): string {
        if (!s.playing)
            return s.members && s.members.length > 1 ? `${s.members.length} speakers` : "Idle";
        return s.trackTitle ? `Playing ${s.trackTitle}` : "Playing";
    }

    // Split is the one gesture here that stops sound in rooms you did not
    // aim at, and a kiosk has no confirm dialog (§16) — so it arms for a
    // few seconds and asks for the second tap instead. Same shape the
    // music depth's Rooms pane uses, deliberately: one gesture, one
    // meaning, wherever the wall offers it.
    let splitArmed = $state(false);
    let splitTimer: ReturnType<typeof setTimeout> | undefined;
    function splitClick() {
        if (splitArmed) {
            splitArmed = false;
            clearTimeout(splitTimer);
            say(`Split — every speaker is on its own again.`);
            music.ungroupFeatured();
            return;
        }
        splitArmed = true;
        clearTimeout(splitTimer);
        splitTimer = setTimeout(() => (splitArmed = false), 3000);
    }

    // A join takes a moment to come back through the poll, and a room that
    // was silent joins a silent group with nothing on screen moving at all.
    // One line, announced, then gone — the kiosk's version of a toast,
    // which it has nobody to dismiss.
    let flash = $state("");
    let flashTimer: ReturnType<typeof setTimeout> | undefined;
    function say(msg: string) {
        flash = msg;
        clearTimeout(flashTimer);
        flashTimer = setTimeout(() => (flash = ""), 4000);
    }
    $effect(() => () => {
        clearTimeout(flashTimer);
        clearTimeout(splitTimer);
    });

    function join(s: PanelSource) {
        say(`${s.title} joined ${featured?.title ?? "the group"}.`);
        music.joinSource(s);
    }
    /** Take the music with you. Two calls under the hood — the destination
     *  joins, then this room steps out — because that is what a move *is*
     *  on a Sonos household: there is no move action, only membership. The
     *  store does them in that order so the queue is handed over before the
     *  old room stops coordinating, and the panel follows the sound. */
    function moveHere(s: PanelSource) {
        say(`Moved to ${s.title}.`);
        music.moveTo(s);
    }
    function joinEverywhere() {
        say("Playing everywhere.");
        music.joinAll();
    }
    function leave(id: string, name: string) {
        say(`${name} left the group.`);
        music.leaveMember(id);
    }
</script>

<div class="gp" aria-label="Speaker grouping">
    <div class="gp-head">
        <h3 class="s-label">Playing in</h3>
        {#if multi}
            <button
                class="g-chip"
                class:armed={splitArmed}
                aria-label={splitArmed ? "Confirm split" : "Split this group"}
                disabled={!!music.busy["ungroup:" + (featured?.id ?? "")]}
                onclick={splitClick}
            >
                {splitArmed ? "Split?" : "Split"}
            </button>
        {/if}
    </div>

    <!-- In the group: name, what it is doing, and the two controls that
         only make sense once a room has company — its own level, and the
         way out. A group of one still lists its speaker, so the pane never
         opens on an empty box. -->
    <div class="g-list">
        {#each members as m (m.id)}
            <div class="g-row in" class:solo={!multi}>
                <span class="g-mark" class:live={featured?.playing}>
                    {#if featured?.playing}
                        <Waveform />
                    {:else}
                        <Icon name="speaker" size={16} />
                    {/if}
                </span>
                <span class="g-name">{m.name}</span>
                {#if multi}
                    <button
                        class="g-ico"
                        class:mute={m.muted}
                        aria-label="{m.muted ? 'Unmute' : 'Mute'} {m.name}"
                        disabled={!!music.busy["mute:" + m.id]}
                        onclick={() => featured && music.toggleMute(featured, m.id)}
                    >
                        <Icon name={m.muted ? "volumeOff" : "volume"} size={16} />
                    </button>
                    <Slider
                        value={music.memVol[m.id] ?? m.volume}
                        label="Volume {m.name}"
                        valueText="{music.memVol[m.id] ?? m.volume}%"
                        onInput={(v) => music.dragMemberVolume(m.id, v)}
                        onChange={(v) => music.setMemberVolume(m.id, v)}
                    />
                    <!-- No number beside a member's fader. The room-wide one
                         three rows up carries the reading; what this list is
                         for is the *balance* between rooms — the kitchen
                         quieter than the living room — which is what the
                         faders lining up already say, and three digits per
                         row buy that at the fader's expense in a 380px
                         column (§16). -->
                    <!-- The membership column, one width for every row: a
                         badge that said "lead" mid-row and an ✕ that didn't
                         shunted every fader in the list to a different x,
                         and a mixer whose faders don't line up can't be
                         read as one. The lead carries the queue and the
                         stream and so cannot step out of its own group —
                         splitting is what ends one, and that control is at
                         the head of the pane. -->
                    <span class="g-out">
                        {#if m.coordinator}
                            <span class="g-lead mono">lead</span>
                        {:else}
                            <button
                                class="g-ico"
                                aria-label="Drop {m.name} out of the group"
                                disabled={!!music.busy["leave:" + m.id]}
                                onclick={() => leave(m.id, m.name)}
                            >
                                <Icon name="close" size={16} />
                            </button>
                        {/if}
                    </span>
                {/if}
            </div>
        {/each}
    </div>

    {#if joinable.length}
        <div class="gp-head second">
            <h3 class="s-label">Add a room</h3>
            {#if joinable.length > 1}
                <!-- The party tap. Four rooms joined one at a time is four
                     aims at a wall; the house has exactly one "everywhere"
                     and it is worth a button. -->
                <button class="g-chip" disabled={!!music.busy["joinall"]} onclick={joinEverywhere}>
                    Everywhere
                </button>
            {/if}
        </div>

        <!-- Out of the group: nearly the whole row joins, because at arm's
             length a 44px plus sign is a worse aim than a 400px row that
             does the same thing. The trailing target is the other half of
             the sentence — "take it with me" rather than "play it there
             too" — and it needs its own button rather than a mode, since
             which of the two you mean is known before you reach the wall,
             never after. -->
        <div class="g-list">
            {#each joinable as s (s.key)}
                {@const rowBusy =
                    !!music.busy["join:" + s.id] ||
                    !!music.busy["joinall"] ||
                    !!music.busy["move:" + s.id]}
                <div class="g-row add">
                    <button class="g-open" disabled={rowBusy} onclick={() => join(s)}>
                        <span class="g-mark">
                            {#if s.playing}<Waveform />{:else}<Icon name="speaker" size={16} />{/if}
                        </span>
                        <span class="g-meta">
                            <span class="g-name">{s.title}</span>
                            <span class="g-sub">{busyWith(s)}</span>
                        </span>
                        <span class="g-join">
                            <Icon name="plus" size={16} /><span>Join</span>
                        </span>
                    </button>
                    <button
                        class="g-move"
                        aria-label="Move the music to {s.title}"
                        disabled={rowBusy}
                        onclick={() => moveHere(s)}
                    >
                        <Icon name="chevronRight" size={16} /><span>Move</span>
                    </button>
                </div>
            {/each}
        </div>
    {/if}

    <!-- One line, and it earns its height twice: what a join costs while
         nothing has been tapped, and what just happened after one has. -->
    <p class="g-note" role="status" aria-live="polite">
        {#if flash}
            {flash}
        {:else if joinable.length}
            Join plays here as well; Move takes it there and leaves.
        {:else}
            Every Sonos room in the house is playing this one.
        {/if}
    </p>
</div>

<style>
    .gp {
        flex: 1 1 auto;
        min-height: 0;
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
        overflow-y: auto;
        /* Vertically, and only vertically — see `.fp-queue`, which shares
           both the slot and the rounding: these rows are flex lines of
           text too. */
        overflow-x: hidden;
        border-top: 1px solid var(--hairline);
        padding-top: var(--space-3);
    }

    .gp-head {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
        min-height: 28px;
        flex-shrink: 0;
    }
    .gp-head.second {
        margin-top: var(--space-2);
    }
    .s-label {
        margin: 0;
        font-size: 11px;
        font-weight: 500;
        font-family: var(--font-mono);
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-dim);
    }

    .g-list {
        display: flex;
        flex-direction: column;
        gap: 4px;
        flex-shrink: 0;
    }

    .g-row {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        min-height: 52px;
        padding: 6px var(--space-3);
        border-radius: var(--r-md);
        border: 1px solid var(--hairline);
        background: var(--card-2);
        color: var(--text);
        font-family: inherit;
        text-align: left;
        min-width: 0;
    }
    /* In the group wears the room's own soft fill — the same "this one is
       making noise with us" the app gives a playing surface, one step down
       from the §15.2 gradient the queue row playing needs to keep. */
    .g-row.in {
        background: var(--on-soft);
        border-color: var(--tile-on-border);
    }
    /* An addable room is two targets in one row: the row itself joins, the
       trailing chip moves. The wrapper carries the edge so the two still
       read as one row, and the padding moves onto the buttons so each has
       its full height to be hit in. */
    .g-row.add {
        padding: 0;
        transition:
            border-color var(--t-fast),
            background var(--t-fast);
    }
    .g-open {
        flex: 1 1 auto;
        min-width: 0;
        display: flex;
        align-items: center;
        gap: var(--space-2);
        align-self: stretch;
        padding: 6px var(--space-3);
        border: 0;
        border-radius: var(--r-md);
        background: none;
        color: inherit;
        font-family: inherit;
        text-align: left;
        cursor: pointer;
        transition: transform var(--t-fast);
    }
    .g-open:active {
        transform: scale(0.99);
        transition-duration: 80ms;
    }
    .g-open:disabled,
    .g-move:disabled {
        opacity: 0.55;
    }
    /* Quieter than Join, because it is the rarer of the two and the row is
       already the louder target. A hairline on the leading edge separates
       them without drawing a second box (§16 draws its own edges). */
    .g-move {
        flex: none;
        display: inline-flex;
        align-items: center;
        gap: 4px;
        align-self: stretch;
        padding: 0 var(--space-3);
        border: 0;
        border-left: 1px solid var(--hairline);
        border-radius: 0 var(--r-md) var(--r-md) 0;
        background: none;
        color: var(--text-mute);
        font-family: inherit;
        font-size: 12px;
        font-weight: 500;
        cursor: pointer;
        transition: color var(--t-fast);
    }
    .g-move:active {
        color: var(--on);
    }
    @media (hover: hover) {
        .g-row.add:hover {
            border-color: var(--border-strong);
        }
    }

    .g-mark {
        width: 28px;
        flex-shrink: 0;
        display: inline-flex;
        justify-content: center;
        color: var(--text-dim);
    }
    .g-mark.live {
        color: var(--on);
    }
    .g-meta {
        flex: 1;
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 1px;
    }
    .g-name {
        font-size: 14px;
        font-weight: 500;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    /* A member row's name shares the row with a fader, so it takes a fixed
       slice rather than pushing the control it labels off the end — and the
       slice is cut to what the 380px controls column can spare, since every
       pixel it takes comes off the fader beside it (§16). */
    .g-row.in .g-name {
        width: 92px;
        flex-shrink: 0;
    }
    /* Alone in its group there is no fader beside it to line up with, so
       the name simply takes the row. */
    .g-row.in.solo .g-name {
        width: auto;
        flex: 1;
    }
    .g-sub {
        font-size: 12px;
        color: var(--text-mute);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .g-out {
        width: 44px;
        flex-shrink: 0;
        display: inline-flex;
        align-items: center;
        justify-content: center;
    }
    .g-lead {
        font-size: 10px;
        letter-spacing: 0.06em;
        text-transform: uppercase;
        color: var(--on);
    }

    .g-ico {
        width: 44px;
        height: 44px;
        display: grid;
        place-items: center;
        border: 0;
        background: none;
        color: var(--text-mute);
        border-radius: var(--r-sm);
        cursor: pointer;
        flex-shrink: 0;
    }
    .g-ico.mute {
        color: var(--bad);
    }
    .g-ico:disabled {
        opacity: 0.5;
    }

    .g-join {
        display: inline-flex;
        align-items: center;
        gap: 4px;
        flex-shrink: 0;
        padding: 6px 12px;
        border-radius: var(--r-pill);
        border: 1px solid var(--hairline);
        background: var(--card);
        color: var(--text-mute);
        font-size: 12.5px;
        font-weight: 500;
    }

    .g-chip {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        padding: 8px 14px;
        border-radius: var(--r-pill);
        border: 1px solid var(--hairline);
        background: var(--card-2);
        color: var(--text-mute);
        font-family: inherit;
        font-size: 12.5px;
        font-weight: 500;
        cursor: pointer;
        flex-shrink: 0;
        transition:
            background var(--t-fast),
            color var(--t-fast),
            border-color var(--t-fast),
            transform var(--t-fast);
    }
    .g-chip:active {
        transform: scale(0.95);
        transition-duration: 80ms;
    }
    /* Armed: the second tap is the one that splits, and the button says so
       in the app's warning colour rather than in the ON amber, which here
       would read as "grouped". */
    .g-chip.armed {
        border-color: var(--bad);
        color: var(--bad);
    }
    .g-chip:disabled {
        opacity: 0.55;
    }

    .g-note {
        margin: var(--space-2) 0 0;
        font-size: 12px;
        line-height: 1.5;
        color: var(--text-dim);
        flex-shrink: 0;
    }

    .g-open:focus-visible,
    .g-move:focus-visible,
    .g-chip:focus-visible,
    .g-ico:focus-visible {
        outline: none;
        box-shadow: var(--focus-ring);
    }

    @media (pointer: coarse) {
        .g-chip {
            min-height: 44px;
            padding-inline: 16px;
        }
        .g-row {
            min-height: 56px;
        }
    }

    @media (pointer: coarse) {
        .g-move {
            padding-inline: var(--space-4);
        }
    }

    @media (prefers-reduced-motion: reduce) {
        .g-row.add,
        .g-open {
            transition-duration: 0.001ms;
        }
    }
</style>
