// One Server-Sent Events connection for the whole app.
//
// The backend publishes named topics on /api/events — "changed" for store
// mutations, "music" for speaker state coming off the Sonos event monitor —
// and callers subscribe to the ones they care about. A music view that
// refetched on every socket toggle, or a dashboard that refetched on every
// volume nudge, would undo the point of having topics at all.
//
// Deliberately a singleton: a component-per-connection design would burn
// through the browser's per-origin connection limit, and an EventSource
// reconnects on its own, so there is nothing to gain from more of them.

let source: EventSource | null = null;
const wired = new Set<string>();
const listeners = new Map<string, Set<() => void>>();

function ensure(topic: string) {
    if (!source) {
        try {
            source = new EventSource("/api/events");
        } catch {
            // No EventSource here. Every caller polls as a backstop anyway,
            // so this degrades to "a bit slower", not "broken".
            return;
        }
    }
    if (wired.has(topic)) return;
    wired.add(topic);
    source.addEventListener(topic, () => {
        for (const fn of listeners.get(topic) ?? []) fn();
    });
}

/**
 * Call fn whenever the backend reports that topic changed. Returns a
 * teardown to run on unmount.
 */
export function onLive(topic: string, fn: () => void): () => void {
    let set = listeners.get(topic);
    if (!set) {
        set = new Set();
        listeners.set(topic, set);
    }
    set.add(fn);
    ensure(topic);
    return () => {
        set.delete(fn);
    };
}
