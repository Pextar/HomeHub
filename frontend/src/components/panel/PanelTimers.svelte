<script lang="ts">
    import { fade, fly } from "svelte/transition";
    import Icon from "../Icon.svelte";
    import { dur } from "../../lib/motion";
    import { fmtUntil, fmtDays } from "../../lib/music/format";
    import { clock } from "../../lib/music/clock.svelte";
    import { roomKeyOf } from "../../lib/panel-music.svelte";
    import type { PanelMusicStore } from "../../lib/panel-music.svelte";
    import type { MusicTimer, MusicTimerView } from "../../lib/types";

    /**
     * Music that starts and stops without anyone tapping anything, on the
     * featured room — the depth's Rooms pane, under the room's own settings
     * (DESIGN.md §16).
     *
     * Two gestures, and they are asked for differently, which is why they are
     * drawn differently:
     *
     *   Sleep  set in the moment by someone already in bed. It is "forty
     *          minutes", nothing else, about the room in front of them — so
     *          it is one row of chips, and once set it says when the room
     *          actually goes quiet rather than when the fade starts.
     *   Wake   arranged in advance. A time, the days, and something to put
     *          on — all three by chip, because there are no forms on a
     *          kiosk. What it will play is named before the tap: an alarm
     *          whose contents are a surprise is not something anyone sets.
     *
     * The room is whichever the panel features, and it may be any kind:
     * HomeHub's timers reach a KEF speaker and a zone, where the sleep timer
     * the wall used to set was a Sonos group's and reached neither. A Sonos
     * speaker keeping its own timer is still reported — it is going to stop
     * this room and a panel that knew and didn't say would be lying by
     * omission — but as its own line, never folded into HomeHub's number.
     */
    let { music }: { music: PanelMusicStore } = $props();

    const featured = $derived(music.featured);

    // ── Sleep ────────────────────────────────────────────────────────────
    /** The wait, not the fade: the backend spends the last fifth of it
     *  ramping down, so "30m" means quiet at thirty minutes and not at
     *  thirty-six. */
    const SLEEP_CHOICES = [15, 30, 45, 60, 90] as const;
    /** The key the backend files this room's timers under — the media
     *  layer's own vocabulary, taken from the store rather than spelled a
     *  second time here. */
    const roomKey = $derived(roomKeyOf(featured));
    const sleepBusy = $derived(!!music.busy["sleepin:" + roomKey]);

    /** When the room goes silent, as a clock reads it. The countdown beside
     *  it answers "how long have I got"; this answers "will that be before
     *  or after the record finishes", and they are different questions. */
    const quietAt = $derived.by(() => {
        void clock.beat;
        const t = music.sleepTimer;
        if (!t?.fires_at) return "";
        const at = new Date(Date.parse(t.fires_at) + (t.fade_minutes ?? 0) * 60_000);
        return at.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
    });

    // ── Wake ─────────────────────────────────────────────────────────────
    // The hours a house actually gets up at. Anything else is set in the
    // full app: a wall panel offers the four taps that cover the mornings
    // this home has, not a clock face to aim at.
    const WAKE_TIMES = ["06:30", "07:00", "07:30", "08:00"] as const;
    /** Ten minutes of ramp, arriving at the level the room sits at now. The
     *  strip says both numbers outright — a wake-up that guesses a volume
     *  for someone is how an alarm becomes a fright, so nothing here is a
     *  guess the user can't read before tapping. */
    const WAKE_FADE = 10;
    let weekdaysOnly = $state(true);
    const wakeDays = $derived(weekdaysOnly ? [1, 2, 3, 4, 5] : []);
    const wakeVolume = $derived(featured?.volume ?? 0);

    /**
     * What a wake-up here would put on, and where that came from.
     *
     * What is playing wins — "wake me to this" is the thought someone has
     * with a record on — and a room that is quiet falls back to what it
     * plays most, then to what it played last. A room with none of the
     * three has nothing to offer and says so rather than drawing four time
     * chips that would fail on the tap.
     */
    const wakeItem = $derived.by((): { item: MusicTimer["item"]; from: string } | null => {
        const f = featured;
        if (f?.trackURI && f.trackTitle) {
            return {
                item: {
                    provider: "spotify",
                    kind: "track",
                    uri: f.trackURI,
                    title: f.trackTitle,
                    sub: f.trackSub,
                    art_uri: f.art,
                },
                from: "playing now",
            };
        }
        // The room's own memory, and only its own: the household's list is
        // an honest fallback for a shelf to offer, and a poor thing to point
        // an alarm at without being asked.
        const p = music.topPlays[0] ?? (music.historyHousehold ? undefined : music.history[0]);
        if (!p) return null;
        return {
            item: {
                provider: p.provider,
                kind: p.kind,
                uri: p.uri,
                title: p.title,
                sub: p.sub,
                art_uri: p.art_uri,
            },
            from: music.topPlays[0] ? "played here most" : "played here last",
        };
    });

    /** The room's standing instructions, minus the sleep timer — that one
     *  has its own row above and would read as two different things saying
     *  the same one. */
    const standing = $derived(music.roomTimers.filter((t) => t.id !== music.sleepTimer?.id));

    function label(t: MusicTimerView): string {
        if (t.action === "stop") return "Stops";
        return t.item?.title || "Plays";
    }

    /** Recurring timers say the days; a one-shot says nothing about them. */
    function when(t: MusicTimerView): string {
        const next = t.enabled ? fmtUntil(t.next_at) : "off";
        const days = t.time ? fmtDays(t.days) : "once";
        return [t.time ?? "", days, next].filter(Boolean).join(" · ");
    }
</script>

{#if featured}
    <div class="tm">
        <!-- ── Sleep ──────────────────────────────────────────────────── -->
        <div class="tm-row">
            <span class="tm-label">
                <Icon name="moon" size={15} />
                <span>Sleep</span>
            </span>
            <div class="tm-chips" role="group" aria-label="Sleep timer">
                {#each SLEEP_CHOICES as mins (mins)}
                    <button
                        class="p-chip"
                        disabled={sleepBusy}
                        onclick={() => music.setSleepIn(mins)}
                    >
                        {mins}m
                    </button>
                {/each}
                {#if music.sleepTimer}
                    <button class="p-chip" disabled={sleepBusy} onclick={() => music.clearSleep()}>
                        Off
                    </button>
                {/if}
            </div>
        </div>

        {#if music.sleepTimer}
            <!-- Set: the fact worth reading back is when the room is quiet,
                 not when the ramp starts. While the ramp is actually walking
                 the volume there is one more thing to offer, and it is the
                 sentence someone says out loud — the fade stops, the volume
                 goes back, and the music keeps playing. -->
            <!-- A line that arrives because a tap set something going, and
                 leaves when it is called off. Both directions here: it is
                 one line of text in a column, so nothing else has to hold
                 its place while it goes. -->
            <p
                class="tm-state"
                class:live={music.fading}
                transition:fly={{ y: -4, duration: dur(150) }}
            >
                {#if music.fading}
                    <!-- The one looping animation this block has, and it
                         earns it the way §6.8's waveform does: a ramp is
                         *happening right now*, and nothing else on screen
                         says so — the volume is moving on its own, over
                         minutes, with no other tell. Opacity on a 14px
                         icon, and only while the ramp is actually in
                         flight. -->
                    <span class="tm-ramp"><Icon name="activity" size={14} /></span>
                    <span>Fading out — quiet at {quietAt}</span>
                    <button
                        class="p-chip tm-still"
                        disabled={!!music.busy["fade:" + roomKey]}
                        onclick={() => music.cancelFade()}
                    >
                        I'm still up
                    </button>
                {:else}
                    <Icon name="moon" size={14} />
                    <span
                        >Quiet at {quietAt} — <span class="mono"
                            >{music.sleepMinutesLeft} min</span
                        > left</span
                    >
                {/if}
            </p>
        {/if}

        {#if music.sonosSleepMinutes > 0}
            <!-- The speaker's own timer, set somewhere that isn't here. Its
                 own line, because it is a different clock kept by a
                 different thing and folding the two into one number would
                 be the panel inventing a timer nobody set. -->
            <p class="tm-state">
                <Icon name="info" size={14} />
                <span
                    >This speaker is also keeping its own timer —
                    <span class="mono">{music.sonosSleepMinutes} min</span></span
                >
                <button
                    class="p-chip"
                    disabled={!!music.busy["sleep:" + featured.id]}
                    onclick={() => music.setSonosSleep(0)}
                >
                    Clear it
                </button>
            </p>
        {/if}

        <!-- ── Wake ───────────────────────────────────────────────────── -->
        {#if wakeItem}
            <div class="tm-row">
                <span class="tm-label">
                    <Icon name="sunrise" size={15} />
                    <span>Wake</span>
                </span>
                <div class="tm-chips" role="group" aria-label="Wake to music">
                    {#each WAKE_TIMES as t (t)}
                        <button
                            class="p-chip"
                            disabled={!!music.busy["wake:" + roomKey + ":" + t]}
                            onclick={() =>
                                music.setWake({
                                    time: t,
                                    days: wakeDays,
                                    volume: wakeVolume,
                                    fadeMinutes: WAKE_FADE,
                                    item: wakeItem?.item,
                                })}
                        >
                            {t}
                        </button>
                    {/each}
                    <!-- Which mornings, as one chip that says what it is
                         rather than a pair that have to be compared. -->
                    <button
                        class="p-chip"
                        aria-pressed={weekdaysOnly}
                        onclick={() => (weekdaysOnly = !weekdaysOnly)}
                    >
                        {weekdaysOnly ? "Weekdays" : "Every day"}
                    </button>
                </div>
            </div>
            <p class="tm-note">
                Wakes {featured.title} to <strong>{wakeItem.item?.title}</strong> ({wakeItem.from}),
                coming up to <span class="mono">{wakeVolume}</span> over
                <span class="mono">{WAKE_FADE}</span> minutes.
            </p>
        {:else}
            <p class="tm-note">
                Play something in {featured.title} first — a wake-up needs something to put on, and
                this room has nothing it has played.
            </p>
        {/if}

        <!-- ── What is already set ────────────────────────────────────── -->
        {#if standing.length > 0}
            <p class="tm-sub mono">Set for this room</p>
            <div class="tm-list">
                {#each standing as t (t.id)}
                    <div
                        class="tm-item"
                        class:off={!t.enabled}
                        in:fly={{ y: -6, duration: dur(160) }}
                        out:fade={{ duration: dur(120) }}
                    >
                        <Icon name={t.action === "start" ? "sunrise" : "moon"} size={15} />
                        <span class="tm-meta">
                            <span class="tm-name">{label(t)}</span>
                            <span class="tm-when mono">{when(t)}</span>
                        </span>
                        <button
                            class="p-chip"
                            aria-pressed={t.enabled}
                            disabled={!!music.busy["timer:" + t.id]}
                            onclick={() => music.setTimerEnabled(t, !t.enabled)}
                        >
                            {t.enabled ? "On" : "Off"}
                        </button>
                        <button
                            class="tm-x"
                            aria-label="Remove this timer"
                            disabled={!!music.busy["timer:" + t.id]}
                            onclick={() => music.deleteTimer(t)}
                        >
                            <Icon name="close" size={14} />
                        </button>
                    </div>
                {/each}
            </div>
        {/if}
    </div>
{/if}

<style>
    .tm {
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
        padding-bottom: var(--space-4);
        margin-bottom: var(--space-2);
        border-bottom: 1px solid var(--hairline);
        flex-shrink: 0;
    }
    .tm-row {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
        flex-wrap: wrap;
    }
    .tm-label {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        font-size: 12.5px;
        color: var(--text-mute);
    }
    .tm-chips {
        display: flex;
        flex-wrap: wrap;
        gap: var(--space-2);
    }
    /* One state line under the row it describes. Quiet by default — it is a
       fact, not a control — and it takes the ON ink only while a ramp is
       actually moving, which is the one state on this block that is
       happening right now rather than scheduled. */
    .tm-state {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        margin: 0;
        font-size: 12.5px;
        color: var(--text-mute);
    }
    .tm-state.live {
        color: var(--on);
    }
    /* Slow enough to read as breathing rather than blinking, and opacity
       only — a 14px icon is the cheapest thing on this surface that could
       carry a loop, which is the whole reason it is the thing carrying it
       (§16's motion budget). */
    .tm-ramp {
        display: inline-flex;
        animation: ramp-breathe 2.4s ease-in-out infinite;
    }
    @keyframes ramp-breathe {
        0%,
        100% {
            opacity: 1;
        }
        50% {
            opacity: 0.4;
        }
    }
    .tm-still {
        margin-left: auto;
    }
    .tm-note {
        margin: 0;
        font-size: 12.5px;
        line-height: 1.5;
        color: var(--text-dim);
    }
    .tm-note strong {
        color: var(--text-mute);
        font-weight: 600;
    }
    .tm-sub {
        margin: var(--space-2) 0 0;
        font-size: 10.5px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-dim);
    }
    .tm-list {
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
    }
    .tm-item {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: var(--space-2);
        border: 1px solid var(--hairline);
        border-radius: var(--r-md);
        background: var(--card);
        color: var(--text-mute);
    }
    /* A disabled timer is still worth listing — it is a thing someone set —
       so it dims rather than disappearing. */
    .tm-item.off {
        opacity: 0.6;
    }
    .tm-meta {
        flex: 1;
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 2px;
    }
    .tm-name {
        font-size: 14px;
        color: var(--text);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .tm-when {
        font-size: 11.5px;
        color: var(--text-dim);
    }
    .tm-x {
        position: relative;
        width: 32px;
        height: 32px;
        flex-shrink: 0;
        display: grid;
        place-items: center;
        border: 0;
        border-radius: 50%;
        background: var(--card-3);
        color: var(--text-mute);
        cursor: pointer;
    }
    .tm-x:disabled {
        opacity: 0.4;
    }

    .p-chip {
        display: inline-flex;
        align-items: center;
        gap: 6px;
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
    .p-chip:active {
        transform: scale(0.95);
        transition-duration: 80ms;
    }
    .p-chip[aria-pressed="true"] {
        border-color: var(--border-strong);
        color: var(--text);
    }
    .p-chip:disabled {
        opacity: 0.55;
    }

    @media (prefers-reduced-motion: reduce) {
        /* The breathing stops rather than slowing: a loop is the one kind
           of motion someone who asked for less of it will keep seeing. The
           line still says what is happening. */
        .tm-ramp {
            animation: none;
        }
    }

    /* Distance-scaled targets: this is a wall, so every chip clears the §2
       floor rather than inheriting a phone's sizing, and the one control too
       small to grow takes its hit area from the space around it. */
    @media (pointer: coarse) {
        .p-chip {
            min-height: 44px;
            padding-inline: 16px;
        }
        .tm-x::after {
            content: "";
            position: absolute;
            inset: -8px;
        }
    }
</style>
