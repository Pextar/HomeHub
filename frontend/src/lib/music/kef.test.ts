import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { createKEFBridge } from "./kef.svelte";
import { createBusy } from "./busy.svelte";
import type { KEFSpeakerView } from "../types";

/**
 * The volume slider's local-value window.
 *
 * A KEF slider shows the value the user is dragging, and lets the poll take
 * over again once they have stopped. Both halves matter: the first because a
 * control that fights the finger reads as broken, the second because the
 * speaker is the authority on its own volume.
 */
function speaker(volume: number): KEFSpeakerView {
    return {
        id: "kef-study",
        name: "Study",
        ip: "192.0.2.11",
        reachable: true,
        state: { volume, muted: false, powered_on: true },
    } as KEFSpeakerView;
}

describe("KEF volume", () => {
    beforeEach(() => vi.useFakeTimers());
    afterEach(() => vi.useRealTimers());

    const make = () => createKEFBridge(createBusy());

    it("shows the polled volume before anything is dragged", () => {
        expect(make().shownVolume(speaker(30))).toBe(30);
    });

    it("shows the dragged value straight away", () => {
        // The regression: the window was stamped only on release, so the very
        // first drag read as stale and the slider snapped back to the poll
        // under the finger.
        const kef = make();
        kef.dragVolume(speaker(30), 55);
        expect(kef.shownVolume(speaker(30))).toBe(55);
    });

    it("holds the dragged value through a drag longer than the window", () => {
        const kef = make();
        const sp = speaker(30);
        kef.dragVolume(sp, 40);
        vi.advanceTimersByTime(3000);
        kef.dragVolume(sp, 50);
        vi.advanceTimersByTime(3000); // 6s since the drag began
        expect(kef.shownVolume(sp)).toBe(50);
    });

    it("lets the poll take over once the window lapses", () => {
        const kef = make();
        kef.dragVolume(speaker(30), 55);
        expect(kef.shownVolume(speaker(30))).toBe(55);

        vi.advanceTimersByTime(4001);
        // A later poll reports someone turning it up on the speaker itself.
        // The speaker is the authority on its own volume; the local value was
        // only ever a bridge across the round trip.
        expect(kef.shownVolume(speaker(70))).toBe(70);
    });

    it("falls back to zero for a speaker that reported no state", () => {
        const kef = make();
        expect(kef.shownVolume({ id: "kef-study", name: "Study" } as KEFSpeakerView)).toBe(0);
    });
});
