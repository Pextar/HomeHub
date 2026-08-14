import { describe, it, expect } from "vitest";
import { splitQueue, tracksAhead } from "./queue";
import type { SonosQueueItem } from "../types";

/** A queue of `n` tracks, numbered the way Sonos numbers them (1-based). */
function q(n: number): SonosQueueItem[] {
  return Array.from({ length: n }, (_, i) => ({ track: i + 1, title: `Track ${i + 1}` }));
}

describe("splitQueue", () => {
  it("puts what already played behind the fold and leads with the playing track", () => {
    const s = splitQueue(q(40), 38);
    expect(s.currentIdx).toBe(37);
    expect(s.earlier).toHaveLength(37);
    expect(s.ahead.map((i) => i.track)).toEqual([38, 39, 40]);
    expect(s.upNext).toBe(2);
  });

  it("folds nothing away on the first track", () => {
    const s = splitQueue(q(5), 1);
    expect(s.earlier).toEqual([]);
    expect(s.ahead).toHaveLength(5);
    expect(s.upNext).toBe(4);
  });

  it("says nothing is next on the last track", () => {
    const s = splitQueue(q(5), 5);
    expect(s.ahead.map((i) => i.track)).toEqual([5]);
    expect(s.upNext).toBe(0);
  });

  it("shows the whole list when nothing is playing out of the queue", () => {
    for (const current of [undefined, 0]) {
      const s = splitQueue(q(3), current);
      expect(s.currentIdx).toBe(-1);
      expect(s.earlier).toEqual([]);
      expect(s.ahead).toHaveLength(3);
      expect(s.upNext).toBe(0);
    }
  });

  it("shows the whole list when the position is past the fetched window", () => {
    // The backend caps a browse at MaxQueueFetch; a queue longer than that
    // can be playing a track this list has never seen.
    const s = splitQueue(q(200), 340);
    expect(s.currentIdx).toBe(-1);
    expect(s.ahead).toHaveLength(200);
  });

  it("cuts by track number, not by index — a removal renumbers the rest", () => {
    const items = [q(1)[0], { track: 3, title: "Track 3" }, { track: 4, title: "Track 4" }];
    const s = splitQueue(items, 3);
    expect(s.earlier.map((i) => i.track)).toEqual([1]);
    expect(s.ahead.map((i) => i.track)).toEqual([3, 4]);
    expect(s.upNext).toBe(1);
  });

  it("survives an empty queue", () => {
    const s = splitQueue([], 2);
    expect(s).toEqual({ currentIdx: -1, earlier: [], ahead: [], upNext: 0 });
  });
});

describe("tracksAhead", () => {
  it("counts what is left, not how long the queue is", () => {
    expect(tracksAhead(40, 38)).toBe(2);
    expect(tracksAhead(40, 40)).toBe(0);
  });

  it("counts the whole queue when nothing has been played out of it", () => {
    expect(tracksAhead(12, undefined)).toBe(12);
    expect(tracksAhead(12, 0)).toBe(12);
  });

  it("never goes negative when the count and the position disagree", () => {
    // The group state and the queue length are read from the same poll but
    // can still cross while a queue is being cleared.
    expect(tracksAhead(3, 9)).toBe(0);
  });
});
