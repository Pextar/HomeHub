import { describe, it, expect } from "vitest";
import { fmtCount, fmtFollowers, fmtMs, fmtTotalMs, capFirst } from "./format";

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
