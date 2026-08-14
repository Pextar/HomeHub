import { describe, it, expect, beforeEach, vi } from "vitest";

const mediaHeard = vi.fn();
const mediaForgetHeard = vi.fn();
vi.mock("../api", () => ({
  api: {
    mediaHeard: (...a: unknown[]) => mediaHeard(...a),
    mediaForgetHeard: (...a: unknown[]) => mediaForgetHeard(...a),
  },
}));
vi.mock("../stores.svelte", () => ({ toasts: { error: vi.fn() } }));

const { createHeardLog } = await import("./heard.svelte");

const track = (title: string) => ({ title, at: new Date().toISOString() });

/** A load that only resolves when the test says so. */
function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((r) => (resolve = r));
  return { promise, resolve };
}

beforeEach(() => {
  mediaHeard.mockReset();
  mediaForgetHeard.mockReset();
});

describe("the listening log", () => {
  it("reads a room's log and remembers whose it is", async () => {
    mediaHeard.mockResolvedValue({ tracks: [track("Song")], household: false });
    const log = createHeardLog();
    await log.load("sonos:kitchen");

    expect(mediaHeard).toHaveBeenCalledWith("sonos:kitchen");
    expect(log.list.map((t) => t.title)).toEqual(["Song"]);
    expect(log.household).toBe(false);
    expect(log.key).toBe("sonos:kitchen");
    expect(log.loading).toBe(false);
  });

  it("carries the household flag through, so the pane can say whose it is", async () => {
    mediaHeard.mockResolvedValue({ tracks: [track("Elsewhere")], household: true });
    const log = createHeardLog();
    await log.load("kef:study");
    expect(log.household).toBe(true);
  });

  // The skeleton is for a room being looked at for the first time. Re-reading
  // the same room keeps what is on screen rather than blinking it away.
  it("shows a skeleton for a new room and not for a re-read", async () => {
    const first = deferred<{ tracks: unknown[]; household: boolean }>();
    mediaHeard.mockReturnValueOnce(first.promise);
    const log = createHeardLog();

    const loading = log.load("sonos:kitchen");
    expect(log.loading).toBe(true);
    first.resolve({ tracks: [track("Song")], household: false });
    await loading;
    expect(log.loading).toBe(false);

    const second = deferred<{ tracks: unknown[]; household: boolean }>();
    mediaHeard.mockReturnValueOnce(second.promise);
    const again = log.load("sonos:kitchen");
    expect(log.loading).toBe(false);
    expect(log.list).toHaveLength(1); // the old rows stay up
    second.resolve({ tracks: [track("Song"), track("Newer")], household: false });
    await again;
    expect(log.list).toHaveLength(2);
  });

  // The same guard the queue load has: a room switched twice while the first
  // read is in flight must not be overwritten by the stale answer.
  it("ignores an answer to a question nobody is asking any more", async () => {
    const slow = deferred<{ tracks: unknown[]; household: boolean }>();
    mediaHeard.mockReturnValueOnce(slow.promise);
    mediaHeard.mockResolvedValueOnce({ tracks: [track("Study")], household: false });

    const log = createHeardLog();
    const stale = log.load("sonos:kitchen");
    await log.load("kef:study");
    slow.resolve({ tracks: [track("Kitchen")], household: false });
    await stale;

    expect(log.key).toBe("kef:study");
    expect(log.list.map((t) => t.title)).toEqual(["Study"]);
  });

  it("empties itself for a room that isn't there, without asking", async () => {
    mediaHeard.mockResolvedValue({ tracks: [track("Song")], household: false });
    const log = createHeardLog();
    await log.load("sonos:kitchen");
    await log.load(null);

    expect(log.list).toEqual([]);
    expect(log.key).toBeNull();
    expect(mediaHeard).toHaveBeenCalledTimes(1);
  });

  it("survives a hub that won't answer", async () => {
    mediaHeard.mockRejectedValue(new Error("no"));
    const log = createHeardLog();
    await log.load("sonos:kitchen");
    expect(log.list).toEqual([]);
    expect(log.loading).toBe(false);
  });

  it("re-reads after clearing, so the pane shows the emptiness", async () => {
    mediaHeard.mockResolvedValueOnce({ tracks: [track("Song")], household: false });
    const log = createHeardLog();
    await log.load("sonos:kitchen");

    mediaForgetHeard.mockResolvedValue(undefined);
    mediaHeard.mockResolvedValueOnce({ tracks: [], household: false });
    await log.clear("sonos:kitchen");

    expect(mediaForgetHeard).toHaveBeenCalledWith("sonos:kitchen");
    expect(log.list).toEqual([]);
  });

  it("keeps the list when clearing fails", async () => {
    mediaHeard.mockResolvedValueOnce({ tracks: [track("Song")], household: false });
    const log = createHeardLog();
    await log.load("sonos:kitchen");

    mediaForgetHeard.mockRejectedValue(new Error("nope"));
    await log.clear("sonos:kitchen");
    expect(log.list.map((t) => t.title)).toEqual(["Song"]);
  });
});
