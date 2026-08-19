import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { createAirPlayBridge } from "./airplay.svelte";
import { api } from "../api";
import type { AirPlaySpeakerView } from "../types";

function receiver(volume: number): AirPlaySpeakerView {
    return {
        id: "airplay-study",
        name: "Study Pi",
        ip: "192.0.2.30",
        port: 7000,
        pcm: true,
        alac: true,
        volume,
        member: "airplay:airplay-study",
        casting: false,
    };
}

describe("AirPlay volume", () => {
    beforeEach(() => {
        vi.useFakeTimers();
        vi.spyOn(api, "airplaySetVolume").mockResolvedValue(undefined);
        vi.spyOn(api, "airplayStatus").mockResolvedValue([]);
    });
    afterEach(() => {
        vi.useRealTimers();
        vi.restoreAllMocks();
    });

    it("shows the stored level before anything is dragged", () => {
        expect(createAirPlayBridge().shownVolume(receiver(35))).toBe(35);
    });

    it("shows the dragged value straight away", () => {
        // Same rule as the other two bridges: a slider that fights the finger
        // reads as broken.
        const airplay = createAirPlayBridge();
        airplay.dragVolume(receiver(35), 60);
        expect(airplay.shownVolume(receiver(35))).toBe(60);
    });

    it("holds the value under a finger that has stopped moving", () => {
        const airplay = createAirPlayBridge();
        airplay.dragVolume(receiver(35), 60);
        vi.advanceTimersByTime(10_000);
        expect(airplay.shownVolume(receiver(35))).toBe(60);
    });

    it("lets the stored level take over again once the finger lifts", () => {
        const airplay = createAirPlayBridge();
        airplay.setVolume(receiver(35), 60);
        vi.advanceTimersByTime(3001);
        expect(airplay.shownVolume(receiver(35))).toBe(35);
    });

    it("throttles a drag rather than sending every movement", () => {
        const airplay = createAirPlayBridge();
        const sp = receiver(35);
        for (const v of [40, 45, 50, 55]) airplay.dragVolume(sp, v);
        vi.advanceTimersByTime(200);
        // One call for the burst, carrying the value the finger ended on.
        expect(api.airplaySetVolume).toHaveBeenCalledTimes(1);
        expect(api.airplaySetVolume).toHaveBeenLastCalledWith("airplay-study", 55);
    });

    it("sends the released value even mid-throttle", () => {
        const airplay = createAirPlayBridge();
        const sp = receiver(35);
        airplay.dragVolume(sp, 40);
        airplay.setVolume(sp, 72);
        // The value let go of is the one that sticks: the queued 40 is
        // cancelled rather than landing after it.
        expect(api.airplaySetVolume).toHaveBeenCalledTimes(1);
        expect(api.airplaySetVolume).toHaveBeenLastCalledWith("airplay-study", 72);
        vi.advanceTimersByTime(500);
        expect(api.airplaySetVolume).toHaveBeenCalledTimes(1);
    });

    it("clamps out-of-range levels", () => {
        const airplay = createAirPlayBridge();
        const sp = receiver(35);
        airplay.setVolume(sp, 140);
        expect(api.airplaySetVolume).toHaveBeenLastCalledWith("airplay-study", 100);
    });
});

describe("AirPlay inventory", () => {
    afterEach(() => vi.restoreAllMocks());

    it("tells 'none' apart from 'not read yet'", async () => {
        vi.spyOn(api, "airplayStatus").mockResolvedValue([]);
        const airplay = createAirPlayBridge();
        // Before the first read, "no receivers" is a claim nothing supports —
        // the same distinction the zones list keeps.
        expect(airplay.loaded).toBe(false);
        await airplay.refresh();
        expect(airplay.loaded).toBe(true);
        expect(airplay.receivers).toEqual([]);
    });

    it("keeps the last good list when a read fails", async () => {
        const list = [receiver(35)];
        const spy = vi.spyOn(api, "airplayStatus").mockResolvedValue(list);
        const airplay = createAirPlayBridge();
        await airplay.refresh();
        expect(airplay.receivers).toHaveLength(1);

        spy.mockRejectedValueOnce(new Error("offline"));
        await airplay.refresh();
        // A failed background poll must not empty a screen the user is
        // looking at.
        expect(airplay.receivers).toHaveLength(1);
    });

    it("reports which receivers are being sent to", async () => {
        vi.spyOn(api, "airplayStatus").mockResolvedValue([
            receiver(35),
            { ...receiver(50), id: "airplay-kitchen", casting: true },
        ]);
        const airplay = createAirPlayBridge();
        await airplay.refresh();
        expect(airplay.casting.map((r) => r.id)).toEqual(["airplay-kitchen"]);
        expect(airplay.byId("airplay-kitchen")?.casting).toBe(true);
        expect(airplay.byId("nobody")).toBeNull();
    });
});
