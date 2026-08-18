import { describe, it, expect } from "vitest";
import { sonosCapabilities, kefCapabilities, zoneCapabilities } from "./capabilities";
import type { KEFState } from "../types";

/**
 * §15's honesty rule, stated once.
 *
 * These are the answers both surfaces now draw from, so a change here is a
 * change to the app and the panel together — which is the point. Each case
 * below is a control that must not be offered, because offering it means a
 * tap the speaker refuses.
 */

const kef = (over: Partial<KEFState> = {}): KEFState =>
  ({ powered_on: true, source: "wifi", volume: 30, muted: false, ...over }) as KEFState;

describe("what a Sonos group can do", () => {
  it("is the only make with a queue and play modes, because it is the only one with the API", () => {
    const c = sonosCapabilities();
    expect(c.canQueue).toBe(true);
    expect(c.canPlayMode).toBe(true);
    expect(c.canSeek).toBe(true);
    expect(c.canPickInput).toBe(false);
  });
});

describe("what a KEF speaker can do", () => {
  it("skips on a network source, where there is something to step through", () => {
    expect(kefCapabilities(kef({ source: "wifi" })).canSkip).toBe(true);
    expect(kefCapabilities(kef({ source: "bluetooth" })).canSkip).toBe(true);
  });

  it("withholds the skip on a physical input, which the speaker would refuse", () => {
    expect(kefCapabilities(kef({ source: "tv" })).canSkip).toBe(false);
    expect(kefCapabilities(kef({ source: "analog" })).canSkip).toBe(false);
    expect(kefCapabilities(kef({ source: "optical" })).canSkip).toBe(false);
  });

  it("withholds it while the speaker is asleep, which has no transport at all", () => {
    expect(kefCapabilities(kef({ powered_on: false, source: "wifi" })).canSkip).toBe(false);
  });

  it("withholds it before the speaker has answered", () => {
    expect(kefCapabilities(undefined).canSkip).toBe(false);
  });

  it("offers the input selector, which is how a KEF picks what plays", () => {
    expect(kefCapabilities(kef()).canPickInput).toBe(true);
    // Even asleep: choosing an input is how you wake it to something.
    expect(kefCapabilities(kef({ powered_on: false })).canPickInput).toBe(true);
  });

  it("never claims a seek — the API has none", () => {
    expect(kefCapabilities(kef()).canSeek).toBe(false);
  });
});

describe("what a HomeHub zone can do", () => {
  it("skips on the routes where the speakers are playing a track", () => {
    expect(zoneCapabilities("native").canSkip).toBe(true);
    expect(zoneCapabilities("connect").canSkip).toBe(true);
    expect(zoneCapabilities("group").canSkip).toBe(true);
  });

  it("withholds it where HomeHub is decoding and the speakers pull a stream", () => {
    expect(zoneCapabilities("stream").canSkip).toBe(false);
    expect(zoneCapabilities("airplay").canSkip).toBe(false);
  });

  it("never claims a queue or play modes, which are a Sonos group's", () => {
    const c = zoneCapabilities("native");
    expect(c.canQueue).toBe(false);
    expect(c.canPlayMode).toBe(false);
    expect(c.canSeek).toBe(false);
  });
});
