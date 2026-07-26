/**
 * Both bridges normalise volume to 0–100, and both take arrow-key nudges that
 * can walk off either end, so the clamp is shared rather than written twice
 * with a chance of drifting.
 */
export const clampVol = (v: number) => Math.max(0, Math.min(100, Math.round(v)));
