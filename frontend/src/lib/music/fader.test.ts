import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { createFader } from "./fader.svelte";

/**
 * Who owns a volume slider's value at any moment — the finger, or the
 * device.
 *
 * Six copies of this rule disagreed on all three of its parts: how long the
 * finger's value outlives the gesture, whether a finger that stops moving
 * still counts as a gesture, and what happens when a drag never ends. These
 * are those three answers.
 */

const sent: [string, number][] = [];
const make = (opts?: { holdMs?: number }) =>
  createFader((id, level) => sent.push([id, level]), opts);

beforeEach(() => {
  sent.length = 0;
  vi.useFakeTimers();
});
afterEach(() => vi.useRealTimers());

describe("volume fader", () => {
  it("shows what the device reported before anything is touched", () => {
    expect(make().shown("sp1", 30)).toBe(30);
  });

  it("shows nothing as zero rather than as undefined", () => {
    expect(make().shown("sp1", undefined)).toBe(0);
  });

  it("answers the finger from the first frame", () => {
    const f = make();
    f.drag("sp1", 55);
    expect(f.shown("sp1", 30)).toBe(55);
  });

  it("keeps each slider's value to itself", () => {
    const f = make();
    f.drag("sp1", 55);
    expect(f.shown("sp2", 30)).toBe(30);
  });

  it("clamps to the ends of the rail", () => {
    const f = make();
    expect(f.drag("sp1", 140)).toBe(100);
    expect(f.commit("sp1", -20)).toBe(0);
  });

  describe("while the finger is down", () => {
    it("holds through a drag longer than the window", () => {
      const f = make();
      f.drag("sp1", 40);
      vi.advanceTimersByTime(3000);
      f.drag("sp1", 50);
      vi.advanceTimersByTime(3000);
      expect(f.shown("sp1", 30)).toBe(50);
    });

    it("holds under a finger that has stopped moving", () => {
      // The bug in four of the six copies: the window was re-stamped by each
      // drag frame and nothing else, so resting on the slider past it handed
      // the value back to the poll — the rail jumping out from under the
      // thumb that was holding it.
      const f = make();
      f.drag("sp1", 55);
      vi.advanceTimersByTime(10_000);
      expect(f.shown("sp1", 30)).toBe(55);
    });

    it("gives up on a drag whose release never arrived", () => {
      // A touch cancelled out from under the element leaves no `onchange`.
      // The claim expires rather than holding the slider for the session.
      const f = make();
      f.drag("sp1", 55);
      vi.advanceTimersByTime(31_000);
      expect(f.shown("sp1", 30)).toBe(30);
    });
  });

  describe("after the finger lifts", () => {
    it("bridges the gap until the device agrees", () => {
      const f = make();
      f.commit("sp1", 55);
      vi.advanceTimersByTime(2000);
      // The speaker hasn't caught up and still reports the old level.
      expect(f.shown("sp1", 30)).toBe(55);
    });

    it("hands the value back once the window lapses", () => {
      const f = make();
      f.commit("sp1", 55);
      vi.advanceTimersByTime(3001);
      // Someone turned it up on the device itself; it is the authority.
      expect(f.shown("sp1", 70)).toBe(70);
    });

    it("takes a window of its own where a surface asks for one", () => {
      const f = make({ holdMs: 10_000 });
      f.commit("sp1", 55);
      vi.advanceTimersByTime(3001);
      expect(f.shown("sp1", 70)).toBe(55);
    });
  });

  describe("what goes out on the wire", () => {
    it("sends while the finger is still down, not one call per pixel", () => {
      const f = make();
      f.drag("sp1", 40);
      f.drag("sp1", 45);
      f.drag("sp1", 50);
      vi.advanceTimersByTime(200);
      expect(sent).toHaveLength(1);
      expect(sent[0]).toEqual(["sp1", 50]);
    });

    it("drops a queued frame on release, so a stale one can't land after it", () => {
      // The caller sends the committed value itself. A mid-drag frame
      // arriving behind it would set the speaker back to where the finger
      // passed through.
      const f = make();
      f.drag("sp1", 40);
      f.drag("sp1", 45);
      f.commit("sp1", 50);
      vi.advanceTimersByTime(1000);
      expect(sent).toEqual([]);
    });
  });

  describe("holds", () => {
    it("is false for a slider nobody has touched", () => {
      expect(make().holds("sp1")).toBe(false);
    });

    it("is true through the gesture and the window after it", () => {
      const f = make();
      f.drag("sp1", 55);
      expect(f.holds("sp1")).toBe(true);
      f.commit("sp1", 55);
      expect(f.holds("sp1")).toBe(true);
      vi.advanceTimersByTime(3001);
      expect(f.holds("sp1")).toBe(false);
    });
  });
});
