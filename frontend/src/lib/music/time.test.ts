import { describe, it, expect } from "vitest";
import { secs, trimClock, fmtSecs, toClock } from "./time";

describe("secs", () => {
  it("parses the H:MM:SS Sonos sends", () => {
    expect(secs("0:03:12")).toBe(192);
    expect(secs("1:04:00")).toBe(3840);
  });

  it("accepts the shorter forms too", () => {
    expect(secs("3:12")).toBe(192);
    expect(secs("42")).toBe(42);
  });

  it("reads nothing as zero rather than NaN", () => {
    // Radio and line-in report no position at all, and a NaN here would
    // propagate into a scrubber's `value` and blank the whole control.
    expect(secs()).toBe(0);
    expect(secs("")).toBe(0);
    expect(secs("NOT_IMPLEMENTED")).toBe(0);
    expect(secs("0:xx:12")).toBe(12);
  });
});

describe("trimClock", () => {
  it("drops the leading hour Sonos always sends", () => {
    expect(trimClock("0:03:12")).toBe("3:12");
    expect(trimClock("0:12:05")).toBe("12:05");
  });

  it("keeps an hour that is really there", () => {
    expect(trimClock("1:04:00")).toBe("1:04:00");
  });

  it("passes an absent duration through as empty", () => {
    expect(trimClock()).toBe("");
    expect(trimClock("")).toBe("");
  });
});

describe("fmtSecs", () => {
  it("formats under an hour as M:SS", () => {
    expect(fmtSecs(0)).toBe("0:00");
    expect(fmtSecs(9)).toBe("0:09");
    expect(fmtSecs(192)).toBe("3:12");
    expect(fmtSecs(3599)).toBe("59:59");
  });

  it("grows an hour field only once there is one", () => {
    expect(fmtSecs(3600)).toBe("1:00:00");
    expect(fmtSecs(3840)).toBe("1:04:00");
  });

  it("clamps a negative extrapolation to zero", () => {
    // The player extrapolates position between polls; a clock that briefly
    // read "-1:-1" would be worse than one that sits at zero for a tick.
    expect(fmtSecs(-5)).toBe("0:00");
  });

  it("rounds rather than truncating a fractional second", () => {
    expect(fmtSecs(191.6)).toBe("3:12");
  });
});

describe("toClock", () => {
  it("writes the H:MM:SS the seek endpoint takes", () => {
    expect(toClock(192)).toBe("0:03:12");
    expect(toClock(0)).toBe("0:00:00");
    expect(toClock(3840)).toBe("1:04:00");
  });

  it("clamps below zero, so a seek can't be sent a negative", () => {
    expect(toClock(-10)).toBe("0:00:00");
  });

  it("round-trips through secs", () => {
    // What the scrubber does on every drag: read a position, seek to it.
    for (const t of [0, 1, 59, 60, 192, 3599, 3600, 3840, 86399]) {
      expect(secs(toClock(t))).toBe(t);
    }
  });
});
