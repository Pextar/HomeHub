/**
 * Both bridges normalise volume to 0–100, and both take arrow-key nudges that
 * can walk off either end, so the clamp is shared rather than written twice
 * with a chance of drifting.
 */
export const clampVol = (v: number) => Math.max(0, Math.min(100, Math.round(v)));

/**
 * Lets a drag send live, without one network call per pixel: the first move
 * in a quiet window goes out right away, further moves within `intervalMs`
 * collapse into a single trailing call carrying whatever value is current
 * when the window closes. Keyed by whatever id a surface uses for its faders
 * (a speaker, a group coordinator, a zone).
 *
 * `cancel` drops any queued trailing call, and a release has to call it
 * before sending the authoritative value or a stale mid-drag frame can land
 * after it and undo the release. That ordering is easy to get wrong once per
 * bridge, so `fader.svelte.ts` owns it — reach for that rather than for this
 * directly.
 */
export function createVolumeThrottle(send: (id: string, level: number) => void, intervalMs = 150) {
  const timers: Record<string, ReturnType<typeof setTimeout>> = {};
  const pending: Record<string, number> = {};
  const lastSentAt: Record<string, number> = {};

  function fire(id: string) {
    delete timers[id];
    const level = pending[id];
    delete pending[id];
    lastSentAt[id] = Date.now();
    send(id, level);
  }

  return {
    schedule(id: string, level: number) {
      pending[id] = level;
      if (timers[id]) return; // a trailing call is already queued
      const wait = Math.max(0, intervalMs - (Date.now() - (lastSentAt[id] ?? 0)));
      timers[id] = setTimeout(() => fire(id), wait);
    },
    cancel(id: string) {
      clearTimeout(timers[id]);
      delete timers[id];
      delete pending[id];
    },
  };
}
