import { describe, it, expect } from "vitest";
import { LIGHT_PRESETS, ctToCss, ctToRgb, kelvinLabel, tintForLevel } from "./light";

describe("ctToRgb", () => {
  it("maps the mired endpoints to the cool and warm anchors", () => {
    expect(ctToRgb(153)).toEqual([206, 233, 255]);
    expect(ctToRgb(500)).toEqual([255, 184, 107]);
  });

  it("clamps outside the 153-500 range rather than extrapolating", () => {
    // Some bulbs report a wider range than they can actually render; letting
    // the interpolation run past the anchors would produce impossible colours.
    expect(ctToRgb(0)).toEqual(ctToRgb(153));
    expect(ctToRgb(9000)).toEqual(ctToRgb(500));
  });

  it("warms monotonically as mireds rise", () => {
    const [r1, , b1] = ctToRgb(200);
    const [r2, , b2] = ctToRgb(400);
    expect(r2).toBeGreaterThan(r1);
    expect(b2).toBeLessThan(b1);
  });
});

describe("tintForLevel", () => {
  it("returns the colour untouched at full brightness", () => {
    expect(tintForLevel("#3DAFFF", 100)).toBe("rgb(61, 175, 255)");
  });

  it("accepts a bare RRGGBB as well as #RRGGBB", () => {
    expect(tintForLevel("3DAFFF", 100)).toBe(tintForLevel("#3DAFFF", 100));
  });

  it("floors the dimming so a low level keeps its hue", () => {
    // Scaling straight to level/100 would render 1% as pure black and lose
    // the colour the user picked, so the scale bottoms out at 0.18.
    expect(tintForLevel("#FFFFFF", 1)).toBe(tintForLevel("#FFFFFF", 18));
    expect(tintForLevel("#FFFFFF", 1)).toBe("rgb(46, 46, 46)");
  });

  it("darkens as the level drops", () => {
    expect(tintForLevel("#FFFFFF", 100)).toBe("rgb(255, 255, 255)");
    expect(tintForLevel("#FFFFFF", 50)).toBe("rgb(128, 128, 128)");
  });
});

describe("ctToCss", () => {
  it("scales the colour-temperature tint by brightness", () => {
    expect(ctToCss(153, 100)).toBe("rgb(206, 233, 255)");
    expect(ctToCss(500, 100)).toBe("rgb(255, 184, 107)");
  });

  it("shares the brightness floor with tintForLevel", () => {
    expect(ctToCss(300, 1)).toBe(ctToCss(300, 18));
  });
});

describe("kelvinLabel", () => {
  it("converts mireds to kelvin rounded to the nearest 50", () => {
    expect(kelvinLabel(500)).toBe("2000K");
    expect(kelvinLabel(370)).toBe("2700K");
    expect(kelvinLabel(153)).toBe("6550K");
  });
});

describe("LIGHT_PRESETS", () => {
  it("gives every preset the channel its kind drives", () => {
    // The control surface filters presets by device capability using `kind`,
    // so a mismatch here would offer a preset that writes nothing.
    for (const p of LIGHT_PRESETS) {
      if (p.kind === "white") {
        expect(p.ct, p.key).toBeDefined();
        expect(p.color, p.key).toBeUndefined();
      } else {
        expect(p.color, p.key).toBeDefined();
        expect(p.ct, p.key).toBeUndefined();
      }
    }
  });

  it("keeps every level and mired value in range", () => {
    for (const p of LIGHT_PRESETS) {
      expect(p.level, p.key).toBeGreaterThanOrEqual(1);
      expect(p.level, p.key).toBeLessThanOrEqual(100);
      if (p.ct !== undefined) {
        expect(p.ct, p.key).toBeGreaterThanOrEqual(153);
        expect(p.ct, p.key).toBeLessThanOrEqual(500);
      }
      if (p.color !== undefined) expect(p.color, p.key).toMatch(/^[0-9A-F]{6}$/);
    }
  });

  it("has unique keys, since they key the rendered list", () => {
    expect(new Set(LIGHT_PRESETS.map(p => p.key)).size).toBe(LIGHT_PRESETS.length);
  });
});
