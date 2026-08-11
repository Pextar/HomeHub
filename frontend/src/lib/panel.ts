// Shared constants for the kiosk surface (DESIGN.md §16). The Panel view
// and the app-level kiosk behaviours (idle auto-return) read the same
// numbers here so the two idle clocks can't drift.

export const PANEL_IDLE_MS = 120_000;
export const PANEL_NIGHT_IDLE_MS = 45_000;

// Night window: 22:00–06:00. The panel falls asleep sooner and shows its
// ambient face dimmed — kinder to the room and to the backlight.
const NIGHT_START = 22;
const NIGHT_END = 6;

export function isPanelNight(d: Date = new Date()): boolean {
    const h = d.getHours();
    return h >= NIGHT_START || h < NIGHT_END;
}

export function panelIdleMs(d: Date = new Date()): number {
    return isPanelNight(d) ? PANEL_NIGHT_IDLE_MS : PANEL_IDLE_MS;
}

/** The track after this one, where the room's play order is actually known. */
export interface PanelNextUp {
    title: string;
    sub?: string; // "Artist · Album" — whichever parts the queue carried
}

/**
 * What the ambient face shows while music plays. Fed by PanelMusic.
 *
 * The face is the panel's most-looked-at surface and the one nobody is
 * standing at, so it says everything about the record that can be said
 * without asking for a tap: what is on, where it is playing, how far
 * through the queue it is, and what comes after it.
 *
 * The ticking position is deliberately absent. It is a live value the face
 * reads off the store (`posSec` / `durSec`, DESIGN.md §15.1) rather than a
 * field, so a rail creeping once a second can't rebuild this whole object
 * — and everything in here changes only when the poll brings a new track.
 */
export interface PanelNowPlaying {
    title: string; // track title (or "Playing")
    sub: string;   // "Artist · Album" — the record, without the room
    art?: string;
    /** The room making the noise — its own row on the face, not the tail
     *  of the artist line: *where* is a different fact from *what*. */
    room: string;
    /** Where the room is in its queue, when it is playing one. Absent on
     *  radio and line-in, which have no queue to be anywhere in. */
    queueTrack?: number;
    queueLength?: number;
    /** Absent under shuffle and repeat-one, where the wall doesn't guess. */
    next?: PanelNextUp;
    /** Other rooms playing something of their own, by name. */
    elsewhere: string[];
}
