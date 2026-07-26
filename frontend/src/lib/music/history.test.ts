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

describe("search history", () => {
  it("lists newest first", () => {
    const h = make();
    h.add("bowie");
    h.add("eno");
    expect(h.list).toEqual(["eno", "bowie"]);
  });

  it("moves a repeated search up instead of listing it twice", () => {
    const h = make();
    h.add("bowie");
    h.add("eno");
    h.add("bowie");
    expect(h.list).toEqual(["bowie", "eno"]);
  });

  it("treats a different capitalisation as the same search", () => {
    const h = make();
    h.add("Bowie");
    h.add("bowie");
    expect(h.list).toEqual(["bowie"]);
  });

  it("keeps only the last eight", () => {
    const h = make();
    for (let i = 0; i < 12; i++) h.add(`q${i}`);
    expect(h.list).toHaveLength(8);
    expect(h.list[0]).toBe("q11");
    expect(h.list.at(-1)).toBe("q4");
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
    expect(h.list).toEqual(["eno"]);
    key = "sonos:kitchen";
    expect(h.list).toEqual(["bowie"]);
  });

  it("clears only the room it is pointed at", () => {
    const h = make();
    h.add("bowie");
    key = "kef:study";
    h.add("eno");
    h.clear();
    expect(h.list).toEqual([]);
    key = "sonos:kitchen";
    expect(h.list).toEqual(["bowie"]);
  });

  it("removes one entry", () => {
    const h = make();
    h.add("bowie");
    h.add("eno");
    h.remove("bowie");
    expect(h.list).toEqual(["eno"]);
  });

  it("falls back to one shared bucket before a destination exists", () => {
    // A house whose speakers haven't answered yet still gets a history.
    key = null;
    const h = make();
    h.add("bowie");
    expect(h.list).toEqual(["bowie"]);
  });

  it("survives a reload", () => {
    make().add("bowie");
    expect(make().list).toEqual(["bowie"]);
  });

  it("starts empty when storage holds junk", () => {
    localStorage.setItem("music.searchHistory.v1", "not json");
    expect(make().list).toEqual([]);
  });
});
