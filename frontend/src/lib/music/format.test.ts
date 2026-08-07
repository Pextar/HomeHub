import { describe, it, expect } from "vitest";
import {
  fmtCount,
  fmtFollowers,
  fmtMs,
  fmtTotalMs,
  capFirst,
  playCount,
  fmtHour,
  fmtUntil,
  fmtDays,
} from "./format";

describe("fmtCount", () => {
  it("compacts the way Spotify writes counts", () => {
    expect(fmtCount(0)).toBe("0");
    expect(fmtCount(987)).toBe("987");
    expect(fmtCount(1234)).toBe("1.2K");
    expect(fmtCount(12345)).toBe("12.3K");
    expect(fmtCount(12345678)).toBe("12.3M");
    expect(fmtCount(1000000)).toBe("1M"); // no false precision
    expect(fmtCount(1500000000)).toBe("1.5B");
    expect(fmtCount(999999)).toBe("1M"); // the 999.9K boundary rolls over
  });
  it("handles junk", () => {
    expect(fmtCount(NaN)).toBe("0");
    expect(fmtCount(-5)).toBe("0");
  });
});

describe("fmtFollowers", () => {
  it("spells the exact count with a plural", () => {
    expect(fmtFollowers(10567890)).toBe("10,567,890 followers");
    expect(fmtFollowers(1)).toBe("1 follower");
  });
});

describe("fmtMs / fmtTotalMs", () => {
  it("formats a track and a listing", () => {
    expect(fmtMs(264000)).toBe("4:24");
    expect(fmtTotalMs(2580000)).toBe("43 min");
    expect(fmtTotalMs(4320000)).toBe("1 hr 12 min");
    expect(fmtTotalMs(3600000)).toBe("1 hr");
    expect(fmtTotalMs(0)).toBe("");
  });
});

describe("capFirst", () => {
  it("capitalises genre labels", () => {
    expect(capFirst("art rock")).toBe("Art rock");
    expect(capFirst("")).toBe("");
  });
});

describe("playCount", () => {
  // The store writes no tally on the entry that created a row, and the one
  // play that made it is what a missing count means. Every surface reads it
  // through here so none of them invents a second rule.
  it("reads a missing tally as the one play behind the entry", () => {
    expect(playCount({})).toBe(1);
    expect(playCount({ count: 0 })).toBe(1);
    expect(playCount({ count: 7 })).toBe(7);
  });
});

describe("fmtHour", () => {
  it("says a local hour the way a wall clock does", () => {
    expect(fmtHour(0)).toBe("00:00");
    expect(fmtHour(8)).toBe("08:00");
    expect(fmtHour(23)).toBe("23:00");
    // Junk clamps rather than printing NaN into a shelf label.
    expect(fmtHour(99)).toBe("23:00");
    expect(fmtHour(NaN)).toBe("00:00");
  });
});

describe("fmtUntil", () => {
  const from = new Date("2026-08-07T20:00:00");
  const at = (mins: number) => new Date(from.getTime() + mins * 60_000).toISOString();

  it("counts the wait rather than repeating the clock", () => {
    expect(fmtUntil(at(6), from)).toBe("in 6 min");
    expect(fmtUntil(at(180), from)).toBe("in 3 h");
    expect(fmtUntil(at(200), from)).toBe("in 3 h 20");
    expect(fmtUntil(at(11 * 60), from)).toBe("in 11 h");
  });

  // Something on its way is never behind us: a fire time a tick or two in
  // the past is the poll not having landed, not an event that was missed.
  it("never counts backwards", () => {
    expect(fmtUntil(at(-5), from)).toBe("now");
    expect(fmtUntil(at(0), from)).toBe("now");
  });

  it("names the day once the wait is longer than one", () => {
    expect(fmtUntil(at(36 * 60), from)).toMatch(/^Sun /);
  });

  it("says nothing about a timer that has no next time", () => {
    expect(fmtUntil(undefined, from)).toBe("");
    expect(fmtUntil("not a date", from)).toBe("");
  });
});

describe("fmtDays", () => {
  it("names the shapes a week actually comes in", () => {
    expect(fmtDays([])).toBe("Every day");
    expect(fmtDays(undefined)).toBe("Every day");
    expect(fmtDays([0, 1, 2, 3, 4, 5, 6])).toBe("Every day");
    expect(fmtDays([1, 2, 3, 4, 5])).toBe("Weekdays");
    expect(fmtDays([0, 6])).toBe("Weekends");
    expect(fmtDays([3, 1])).toBe("Mon Wed");
  });
});
