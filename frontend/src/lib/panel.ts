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

/** What the ambient face shows while music plays. Fed by PanelMusic. */
export interface PanelNowPlaying {
    title: string; // track title (or "Playing")
    sub: string;   // "Artist · Album · Zone" — whichever parts exist
    art?: string;
}
