import { describe, it, expect, beforeEach } from "vitest";
import {
  HOME_SECTIONS,
  defaultLayout,
  homeSensors,
  homeTemperature,
  normalizeLayout,
  reorder,
  sectionsFor,
  type HomeLayout,
  type HomeSectionId,
} from "./home-layout";
import { createHomeLayoutStore } from "./home-layout.svelte";
import type { Sensor } from "./types";

const ids = HOME_SECTIONS.map((s) => s.id);

const layout = (over: Partial<HomeLayout> = {}): HomeLayout => ({ ...defaultLayout(), ...over });

const sensor = (over: Partial<Sensor> & { id: string }): Sensor => ({
  name: over.id,
  kind: "temperature",
  unit: "°C",
  code: "",
  protocol: "rtl_433",
  ...over,
});

describe("reading a stored layout", () => {
  it("gives a home that was never customised the canonical order", () => {
    expect(normalizeLayout(null).order).toEqual(ids);
    expect(normalizeLayout(undefined)).toEqual(defaultLayout());
    expect(normalizeLayout("not an object")).toEqual(defaultLayout());
  });

  it("keeps an order the user built", () => {
    const stored = { order: ["sensors", "hero", ...ids.filter((i) => i !== "sensors" && i !== "hero")] };
    expect(normalizeLayout(stored).order.slice(0, 2)).toEqual(["sensors", "hero"]);
  });

  it("drops ids it no longer knows, and duplicates", () => {
    const stored = { order: ["hero", "hero", "a-section-we-removed", "sensors"] };
    const got = normalizeLayout(stored).order;
    expect(got).toContain("hero");
    expect(got).not.toContain("a-section-we-removed");
    expect(got.filter((x) => x === "hero")).toHaveLength(1);
    expect([...got].sort()).toEqual([...ids].sort());
  });

  it("lands a newly shipped section where the design put it, not at the bottom", () => {
    // Someone customised before "sensors" existed: it belongs after "rooms".
    const stored = { order: ids.filter((i) => i !== "sensors") };
    const got = normalizeLayout(stored).order;
    expect(got.indexOf("sensors")).toBe(got.indexOf("rooms") + 1);
  });

  it("puts a new first section first even when everything above it is gone", () => {
    const stored = { order: ["timers"] };
    const got = normalizeLayout(stored).order;
    expect(got[0]).toBe("hero");
    expect(got).toHaveLength(ids.length);
  });

  it("takes only known ids as hidden", () => {
    const got = normalizeLayout({ hidden: ["timers", "nonsense", 7] });
    expect(got.hidden).toEqual(["timers"]);
  });

  it("tells a chosen sensor list from never having chosen", () => {
    expect(normalizeLayout({}).sensors).toBeNull();
    expect(normalizeLayout({ sensors: "bad" }).sensors).toBeNull();
    expect(normalizeLayout({ sensors: [] }).sensors).toEqual([]);
    expect(normalizeLayout({ sensors: ["a", "b", "a"] }).sensors).toEqual(["a", "b"]);
  });
});

describe("moving a section", () => {
  const three: HomeSectionId[] = ["hero", "nowplaying", "favorites"];

  it("moves it to the index asked for", () => {
    expect(reorder(three, "favorites", 0)).toEqual(["favorites", "hero", "nowplaying"]);
    expect(reorder(three, "hero", 2)).toEqual(["nowplaying", "favorites", "hero"]);
  });

  it("clamps past either end rather than dropping the section", () => {
    expect(reorder(three, "nowplaying", -5)).toEqual(["nowplaying", "hero", "favorites"]);
    expect(reorder(three, "nowplaying", 99)).toEqual(["hero", "favorites", "nowplaying"]);
  });

  it("leaves a list it doesn't contain alone", () => {
    expect(reorder(three, "devices", 0)).toEqual(three);
  });
});

describe("what a profile may arrange", () => {
  it("hides the sections a non-admin never fetches", () => {
    const got = sectionsFor(defaultLayout(), false).map((s) => s.id);
    expect(got).not.toContain("sensors");
    expect(got).not.toContain("groups");
    expect(got).not.toContain("timers");
    expect(got).toContain("hero");
  });

  it("gives an admin every section, in layout order", () => {
    const l = layout({ order: reorder(ids, "devices", 0) });
    expect(sectionsFor(l, true).map((s) => s.id)).toEqual(l.order);
  });
});

describe("which sensors home shows", () => {
  const many = Array.from({ length: 8 }, (_, i) => sensor({ id: `s${i}` }));

  it("takes the first six when nobody has chosen", () => {
    expect(homeSensors(many, layout()).map((s) => s.id)).toEqual([
      "s0",
      "s1",
      "s2",
      "s3",
      "s4",
      "s5",
    ]);
  });

  it("takes exactly what was chosen, in the order it was saved", () => {
    const got = homeSensors(many, layout({ sensors: ["s7", "s1"] }));
    expect(got.map((s) => s.id)).toEqual(["s7", "s1"]);
  });

  it("shows nothing when nothing was chosen", () => {
    expect(homeSensors(many, layout({ sensors: [] }))).toEqual([]);
  });

  it("quietly forgets a sensor that has been deleted", () => {
    const got = homeSensors(many, layout({ sensors: ["s1", "gone", "s2"] }));
    expect(got.map((s) => s.id)).toEqual(["s1", "s2"]);
  });
});

describe("what the house says it is", () => {
  it("says nothing at all with no readings to say it from", () => {
    expect(homeTemperature([], layout())).toBeNull();
    expect(homeTemperature([sensor({ id: "hall" })], layout())).toBeNull(); // no last_value
  });

  it("reads the one thermometer by name, without being asked", () => {
    const got = homeTemperature([sensor({ id: "hall", name: "Hall", last_value: 21.4 })], layout());
    expect(got).toEqual({ value: 21.4, label: "Hall", named: true });
  });

  it("averages the house when nobody picked, and says so", () => {
    const got = homeTemperature(
      [
        sensor({ id: "hall", last_value: 20 }),
        sensor({ id: "out", last_value: 0 }),
        sensor({ id: "bed", last_value: 22 }),
      ],
      layout(),
    );
    expect(got?.value).toBeCloseTo(14);
    expect(got).toMatchObject({ label: "Average of 3", named: false });
  });

  it("reads the sensor it was told to, and calls it by its name", () => {
    const got = homeTemperature(
      [sensor({ id: "hall", name: "Hall", last_value: 20 }), sensor({ id: "out", name: "Outside", last_value: 0 })],
      layout({ temperature: "out" }),
    );
    expect(got).toEqual({ value: 0, label: "Outside", named: true });
  });

  it("falls back to the average when the chosen sensor has gone quiet", () => {
    const got = homeTemperature(
      [sensor({ id: "hall", last_value: 20 }), sensor({ id: "bed", last_value: 24 })],
      layout({ temperature: "unplugged" }),
    );
    expect(got).toMatchObject({ value: 22, named: false });
  });

  it("ignores anything that isn't a temperature", () => {
    const got = homeTemperature(
      [
        sensor({ id: "hall", name: "Hall", last_value: 20 }),
        sensor({ id: "meter", kind: "power", unit: "W", last_value: 900 }),
      ],
      layout(),
    );
    expect(got).toEqual({ value: 20, label: "Hall", named: true });
  });
});

describe("the layout store", () => {
  beforeEach(() => localStorage.clear());

  it("starts from the default and writes every change through", () => {
    const store = createHomeLayoutStore();
    expect(store.order).toEqual(ids);

    store.setHidden("timers", true);
    store.move("devices", 0);
    store.setSensors(["a"]);
    store.setTemperature("out");

    // A second store is a second device tab: it must read back the same home.
    const reloaded = createHomeLayoutStore();
    expect(reloaded.isHidden("timers")).toBe(true);
    expect(reloaded.order[0]).toBe("devices");
    expect(reloaded.sensors).toEqual(["a"]);
    expect(reloaded.temperature).toBe("out");
  });

  it("switching a section back on leaves it where it was", () => {
    const store = createHomeLayoutStore();
    store.move("sensors", 0);
    store.setHidden("sensors", true);
    store.setHidden("sensors", false);
    expect(store.order[0]).toBe("sensors");
    expect(store.isHidden("sensors")).toBe(false);
  });

  it("survives a storage value that isn't a layout", () => {
    localStorage.setItem("home.layout.v1", "{{{ not json");
    expect(createHomeLayoutStore().order).toEqual(ids);
  });

  it("reset puts back the order and clears the sensor choices", () => {
    const store = createHomeLayoutStore();
    store.setSensors([]);
    store.setTemperature("out");
    store.move("timers", 0);
    store.reset();
    expect(store.order).toEqual(ids);
    expect(store.sensors).toBeNull();
    expect(store.temperature).toBeNull();
    expect(createHomeLayoutStore().sensors).toBeNull();
  });
});
