/**
 * The wall's own clocks: "quiet in forty minutes", and the other end of the
 * same mechanism — "wake this room at 06:45".
 *
 * Both reach every kind of room the panel can feature — a Sonos group, a
 * KEF speaker, a HomeHub zone — where a speaker's own sleep timer reaches
 * only the one make that has one. That is why these are HomeHub's timers
 * and not a passthrough.
 *
 * The household's listening picture rides along on the same beat, and is
 * here rather than in a file of its own for exactly that reason: it changes
 * when something is played, which is the same event a timer read is already
 * waiting on, and giving it a second interval would double the polling to
 * learn the same thing twice.
 *
 * Split out of the store's closure because it needs remarkably little of
 * it: which room is featured, and the guarded runner that every action in
 * the panel goes through.
 */

import { api } from "../api";
import { session } from "../stores.svelte";
import { clock } from "../music/clock.svelte";
import type { Listening, MusicTimer, MusicTimerView } from "../types";

/** The store's own guarded runner: claims the key, re-reads on success,
 *  toasts on failure. Passed in rather than rebuilt so a timer action
 *  disables the same control every other panel action does. */
export type PanelRunner = (
    key: string,
    fn: () => Promise<unknown>,
    errTitle: string,
    ok?: () => void,
) => Promise<void>;

export interface PanelTimersDeps {
    /** The kid surface: the endpoints are admin-only, and asking would be a
     *  guaranteed 403 on every load of a screen with no control to draw. */
    sonosOnly: boolean;
    /** The featured room's destination key, or "" when there is none. A
     *  getter: the featured room moves under this. */
    roomKey: () => string;
    run: PanelRunner;
}

export interface PanelTimersStore {
    readonly timers: MusicTimerView[];
    readonly roomTimers: MusicTimerView[];
    readonly sleepTimer: MusicTimerView | undefined;
    readonly sleepMinutesLeft: number;
    readonly fading: boolean;
    readonly insights: Listening | null;
    setSleepIn(minutes: number): void;
    clearSleep(): void;
    cancelFade(): void;
    setWake(o: {
        time: string;
        days: number[];
        volume?: number;
        fadeMinutes?: number;
        item: MusicTimer["item"];
        name?: string;
    }): void;
    setTimerEnabled(t: MusicTimerView, enabled: boolean): void;
    deleteTimer(t: MusicTimerView): void;
}

export function createPanelTimers(deps: PanelTimersDeps): PanelTimersStore {
    const { run } = deps;
    const opts = { sonosOnly: deps.sonosOnly };

// ── HomeHub's music timers ───────────────────────────────────────────
// The wall's own "quiet in forty minutes", and the other end of the same
// mechanism: "wake this room at 06:45". Both reach every kind of room
// the panel can feature — a Sonos group, a KEF speaker, a HomeHub zone —
// where the speaker's own timer reaches only the one make that has one.
//
// Read on a slow beat of its own rather than on the speaker poll: a
// timer changes when somebody sets one and when one fires, and the
// countdown between reads is arithmetic, not a round trip. The one thing
// that genuinely moves on its own is a ramp in flight, and a minute's
// granularity is right for something that takes five.
//
// Not on the kid surface: the endpoints are admin-only, and asking would
// be a guaranteed 403 on every load of a screen with no control to draw
// with the answer.
const TIMERS_MS = 60_000;
let timers = $state<MusicTimerView[]>([]);

async function loadTimers() {
    if (opts.sonosOnly) return;
    try {
        timers = await api.musicTimers();
    } catch {
        timers = [];
    }
}

const roomTimers = $derived.by(() => {
    const key = deps.roomKey();
    return key ? timers.filter((t) => t.room === key) : [];
});
/** A sleep timer is the one-shot stop: no time of day, nothing to play.
 *  A recurring "stop at 23:00" is a standing instruction and belongs in
 *  the list with the wake-ups, not on the "quiet in…" row. */
const sleepTimer = $derived(
    roomTimers.find((t) => t.action === "stop" && !t.time && !!t.fires_at),
);
const fading = $derived(roomTimers.some((t) => t.fading));

/** Minutes until the room is actually quiet: the timer fires when the
 *  fade *starts*, so the fade's own length is still to come. */
const sleepMinutesLeft = $derived.by(() => {
    void clock.beat;
    const t = sleepTimer;
    if (!t?.fires_at || !t.enabled) return 0;
    const quietAt = Date.parse(t.fires_at) + (t.fade_minutes ?? 0) * 60_000;
    return Math.max(0, Math.ceil((quietAt - Date.now()) / 60_000));
});

function setSleepIn(minutes: number) {
    const key = deps.roomKey();
    if (!key) return;
    void run(
        "sleepin:" + key,
        () => api.musicSleep({ room: key, minutes }).then(loadTimers),
        "Couldn't set the sleep timer",
    );
}

function clearSleep() {
    const t = sleepTimer;
    if (!t) return;
    // Deleting cancels the ramp too, which puts the volume back and
    // leaves the music playing — "I'm still up" said with the timer.
    void run(
        "sleepin:" + t.room,
        () => api.musicDeleteTimer(t.id).then(loadTimers),
        "Couldn't clear the sleep timer",
    );
}

function cancelFade() {
    const key = deps.roomKey();
    if (!key) return;
    void run(
        "fade:" + key,
        () => api.musicCancelFade(key).then(loadTimers),
        "Couldn't stop the fade",
    );
}

function setWake(o: {
    time: string;
    days: number[];
    volume?: number;
    fadeMinutes?: number;
    item: MusicTimer["item"];
    name?: string;
}) {
    const key = deps.roomKey();
    if (!key || !o.item?.uri) return;
    void run(
        "wake:" + key + ":" + o.time,
        () =>
            api
                .musicCreateTimer({
                    room: key,
                    action: "start",
                    enabled: true,
                    time: o.time,
                    days: o.days,
                    item: o.item,
                    volume: o.volume,
                    fade_minutes: o.fadeMinutes,
                    name: o.name,
                })
                .then(loadTimers),
        "Couldn't set that alarm",
    );
}

function setTimerEnabled(t: MusicTimerView, enabled: boolean) {
    void run(
        "timer:" + t.id,
        () =>
            api
                .musicUpdateTimer(t.id, {
                    room: t.room,
                    action: t.action,
                    enabled,
                    fires_at: t.fires_at,
                    time: t.time,
                    days: t.days,
                    item: t.item,
                    volume: t.volume,
                    fade_minutes: t.fade_minutes,
                    name: t.name,
                })
                .then(loadTimers),
        "Couldn't change that timer",
    );
}

function deleteTimer(t: MusicTimerView) {
    void run(
        "timer:" + t.id,
        () => api.musicDeleteTimer(t.id).then(loadTimers),
        "Couldn't remove that timer",
    );
}

// ── What the household listens to ────────────────────────────────────
// The one picture no single room can give, and the reason it rides on
// the timers' beat rather than the speakers': it changes when something
// is played, which the panel finds out about anyway.
let insights = $state<Listening | null>(null);
async function loadInsights() {
    if (opts.sonosOnly) return;
    try {
        insights = await api.mediaInsights(8);
    } catch {
        insights = null;
    }
}

$effect(() => {
    if (opts.sonosOnly) return;
    if (!session.isAdmin) return;
    void loadTimers();
    void loadInsights();
    const t = setInterval(() => {
        if (document.hidden) return;
        void loadTimers();
        void loadInsights();
    }, TIMERS_MS);
    return () => clearInterval(t);
});
    return {
        get timers() {
            return timers;
        },
        get roomTimers() {
            return roomTimers;
        },
        get sleepTimer() {
            return sleepTimer;
        },
        get sleepMinutesLeft() {
            return sleepMinutesLeft;
        },
        get fading() {
            return fading;
        },
        get insights() {
            return insights;
        },
        setSleepIn,
        clearSleep,
        cancelFade,
        setWake,
        setTimerEnabled,
        deleteTimer,
    };
}
