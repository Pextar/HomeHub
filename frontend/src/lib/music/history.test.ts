import { describe, it, expect, beforeEach } from "vitest";
import { createSearchHistory } from "./history.svelte";

// The store is keyed on a getter so it can follow the destination while a
// sheet is open; these tests drive that getter directly.
let key: string | null;
beforeEach(() => {
  localStorage.clear();
  key = "sonos:kitchen";
});
const make = () => createSearchHistory(() => key);
const queries = (h: ReturnType<typeof make>) => h.list.map((e) => e.q);

describe("search history", () => {
  it("lists newest first", () => {
    const h = make();
    h.add("bowie");
    h.add("eno");
    expect(queries(h)).toEqual(["eno", "bowie"]);
  });

  it("moves a repeated search up instead of listing it twice", () => {
    const h = make();
    h.add("bowie");
    h.add("eno");
    h.add("bowie");
    expect(queries(h)).toEqual(["bowie", "eno"]);
  });

  it("treats a different capitalisation as the same search", () => {
    const h = make();
    h.add("Bowie");
    h.add("bowie");
    expect(queries(h)).toEqual(["bowie"]);
  });

  it("keeps only the last eight", () => {
    const h = make();
    for (let i = 0; i < 12; i++) h.add(`q${i}`);
    expect(h.list).toHaveLength(8);
    expect(h.list[0].q).toBe("q11");
    expect(h.list.at(-1)?.q).toBe("q4");
  });

  it("cuts `recent` to a row", () => {
    const h = make();
    for (let i = 0; i < 8; i++) h.add(`q${i}`);
    expect(h.recent).toHaveLength(6);
  });

  it("keeps each room's searches apart", () => {
    const h = make();
    h.add("bowie");
    key = "kef:study";
    expect(h.list).toEqual([]);
    h.add("eno");
    expect(queries(h)).toEqual(["eno"]);
    key = "sonos:kitchen";
    expect(queries(h)).toEqual(["bowie"]);
  });

  it("clears only the room it is pointed at", () => {
    const h = make();
    h.add("bowie");
    key = "kef:study";
    h.add("eno");
    h.clear();
    expect(h.list).toEqual([]);
    key = "sonos:kitchen";
    expect(queries(h)).toEqual(["bowie"]);
  });

  it("removes one entry", () => {
    const h = make();
    h.add("bowie");
    h.add("eno");
    h.remove("bowie");
    expect(queries(h)).toEqual(["eno"]);
  });

  it("falls back to one shared bucket before a destination exists", () => {
    // A house whose speakers haven't answered yet still gets a history.
    key = null;
    const h = make();
    h.add("bowie");
    expect(queries(h)).toEqual(["bowie"]);
  });

  it("survives a reload", () => {
    make().add("bowie");
    expect(queries(make())).toEqual(["bowie"]);
  });

  it("starts empty when storage holds junk", () => {
    localStorage.setItem("music.searchHistory.v1", "not json");
    expect(make().list).toEqual([]);
  });

  it("has no picture until the search behind it answers", () => {
    const h = make();
    h.add("bowie");
    expect(h.list[0].art_url).toBeUndefined();
  });

  it("fills in the picture once the search answers", () => {
    const h = make();
    h.add("bowie");
    h.add("bowie", { art_url: "https://img/bowie.jpg", round: true });
    expect(h.list).toEqual([{ q: "bowie", art_url: "https://img/bowie.jpg", round: true }]);
  });

  it("keeps the picture across a later add with none of its own", () => {
    const h = make();
    h.add("bowie");
    h.add("bowie", { art_url: "https://img/bowie.jpg", round: true });
    h.add("bowie"); // e.g. a re-run whose search hasn't answered yet
    expect(h.list[0].art_url).toBe("https://img/bowie.jpg");
  });

  it("reads an older plain-string history back with no picture", () => {
    localStorage.setItem("music.searchHistory.v1", JSON.stringify({ "sonos:kitchen": ["bowie", "eno"] }));
    const h = make();
    expect(h.list).toEqual([{ q: "bowie" }, { q: "eno" }]);
  });
});
