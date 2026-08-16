/**
 * Sonos speakers: local UPnP control, the household's grouping,
 * and the one shared queue.
 *
 * One slice of the flat `api` object — lib/api.ts spreads these together,
 * so every call site's `api.foo()` is unchanged. Split by domain because a
 * thousand-line object is not a thing anyone reads.
 */

import { req, json, API_BASE } from "./http";
import type { PlayItemBody } from "./http";
import type {
  SonosCandidate,
  SonosEventHealth,
  SonosFavorite,
  SonosQueueItem,
  SonosRepeat,
  SonosSettings,
  SonosSettingsPatch,
  SonosSpeaker,
  SonosStatus,
} from "../types";

export const sonosApi = {
  // Sonos speakers (local UPnP control)
  sonosStatus() { return req<SonosStatus>("/sonos/status"); },
  sonosDiscover() { return req<SonosCandidate[]>("/sonos/discover"); },
  sonosEventHealth() { return req<SonosEventHealth>("/sonos/events"); },
  // Asks every watcher to resubscribe now instead of at its own backoff. The
  // work is asynchronous — re-read the health endpoint to see the outcome.
  sonosEventRetry() { return req<{ ok: boolean }>("/sonos/events/retry", { method: "POST" }); },
  sonosCreateSpeaker(body: { ip: string; name?: string; room?: string }) {
    return req<SonosSpeaker>("/sonos/speakers", { method: "POST", body: json(body) });
  },
  sonosUpdateSpeaker(id: string, body: { ip?: string; name?: string; room?: string }) {
    return req<SonosSpeaker>(`/sonos/speakers/${encodeURIComponent(id)}`, { method: "PUT", body: json(body) });
  },
  sonosDeleteSpeaker(id: string) {
    return req<void>(`/sonos/speakers/${encodeURIComponent(id)}`, { method: "DELETE" });
  },
  // Device settings — read on demand, not part of the status poll. The
  // response says which of the model-dependent controls this speaker has.
  sonosSettings(id: string) {
    return req<SonosSettings>(`/sonos/${encodeURIComponent(id)}/settings`);
  },
  // Send one field per interaction: the backend applies a patch in order and
  // stops at the first refusal, so a single field keeps the error unambiguous.
  sonosUpdateSettings(id: string, patch: SonosSettingsPatch) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/settings`, { method: "PUT", body: json(patch) });
  },
  /**
   * A picture of this speaker model, proxied from the speaker's own device
   * description. 404s when the speaker publishes none — render the striped
   * placeholder then, never a stand-in for another model.
   */
  sonosImageURL(id: string) {
    return `${API_BASE}/sonos/${encodeURIComponent(id)}/image`;
  },
  // Transport actions go to the group coordinator.
  sonosPlay(id: string) { return req<void>(`/sonos/${encodeURIComponent(id)}/play`, { method: "POST" }); },
  sonosPause(id: string) { return req<void>(`/sonos/${encodeURIComponent(id)}/pause`, { method: "POST" }); },
  sonosNext(id: string) { return req<void>(`/sonos/${encodeURIComponent(id)}/next`, { method: "POST" }); },
  sonosPrevious(id: string) { return req<void>(`/sonos/${encodeURIComponent(id)}/previous`, { method: "POST" }); },
  sonosSetVolume(id: string, level: number, group = false) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/volume`, { method: "PUT", body: json({ level, group }) });
  },
  sonosSetMute(id: string, muted: boolean) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/mute`, { method: "PUT", body: json({ muted }) });
  },
  sonosJoin(id: string, targetId: string) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/join`, { method: "POST", body: json({ target_id: targetId }) });
  },
  sonosLeave(id: string) { return req<void>(`/sonos/${encodeURIComponent(id)}/leave`, { method: "POST" }); },
  // Regroup a household in one ordered request: `join` land on {id}, then
  // `leave` step out into groups of their own. The order is the feature —
  // "take the music with me" is join-then-leave, because the destination has
  // to be handed the queue and the stream while the old room is still
  // coordinating. Looping over the two calls above from a browser keeps that
  // order only as long as the page does; here the run *is* the request, so a
  // panel that is navigated away from mid-gesture can't leave a household
  // half moved.
  sonosGroup(id: string, body: { join?: string[]; leave?: string[] }) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/group`, { method: "POST", body: json(body) });
  },
  sonosFavorites(id: string) { return req<SonosFavorite[]>(`/sonos/${encodeURIComponent(id)}/favorites`); },
  sonosPlayFavorite(id: string, fav: SonosFavorite) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/favorites/play`, { method: "POST", body: json(fav) });
  },
  // Transport extras — all group-level, so {id} must be the coordinator.
  sonosSeek(id: string, position: string) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/seek`, { method: "PUT", body: json({ position }) });
  },
  // Jumps to a 1-based queue position, switching the group back to its
  // queue first if it was parked on a stream.
  sonosSeekTrack(id: string, track: number) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/seek`, { method: "PUT", body: json({ track }) });
  },
  // Shuffle and repeat go together: Sonos stores them as one value.
  sonosSetPlayMode(id: string, shuffle: boolean, repeat: SonosRepeat) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/playmode`, { method: "PUT", body: json({ shuffle, repeat }) });
  },
  sonosSetCrossfade(id: string, enabled: boolean) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/crossfade`, { method: "PUT", body: json({ enabled }) });
  },
  // "Continue play similar" — see DESIGN.md's autoplay note. Group-level,
  // like every other transport extra: {id} must be the coordinator.
  sonosSetAutoplay(id: string, enabled: boolean) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/autoplay`, { method: "PUT", body: json({ enabled }) });
  },

  // Group queue. Adding never disturbs what is playing — pass next: true to
  // drop the item in after the current track instead of at the end.
  sonosQueue(id: string) { return req<SonosQueueItem[]>(`/sonos/${encodeURIComponent(id)}/queue`); },
  sonosQueueAdd(
    id: string,
    body: { service?: string; uri: string; title?: string; metadata?: string; next?: boolean },
  ) {
    return req<{ track: number; length: number }>(`/sonos/${encodeURIComponent(id)}/queue`, {
      method: "POST",
      body: json(body),
    });
  },
  // A whole run in one request. Reach for this over a loop of sonosQueueAdd
  // whenever there is more than one item: "more like this" is eight tracks,
  // and as eight requests from a wall panel — a 2015 iPad on household Wi-Fi
  // — it is eight round trips, each carrying its own position read, sent
  // backwards so that Sonos resolves each "next" into the right slot. Here
  // the order of the array is the order they land in.
  sonosQueueAddMany(
    id: string,
    items: { service?: string; uri: string; title?: string; metadata?: string }[],
    next = false,
  ) {
    return req<{ track: number; length: number; added: number }>(
      `/sonos/${encodeURIComponent(id)}/queue`,
      { method: "POST", body: json({ items, next }) },
    );
  },
  sonosQueueRemove(id: string, track: number) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/queue/${track}`, { method: "DELETE" });
  },
  sonosQueueClear(id: string) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/queue`, { method: "DELETE" });
  },
  // Moves a queued track to another position. Both numbers are read off the
  // queue as it looks now; the backend converts to the insertion point Sonos
  // actually wants.
  sonosQueueMove(id: string, track: number, to: number) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/queue/${track}`, {
      method: "PUT",
      body: json({ to }),
    });
  },

  // Plays a streaming-service item (from Spotify search) on the group led
  // by speaker {id}; the speaker streams with its own linked account.
  sonosPlayItem(id: string, body: PlayItemBody) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/play-item`, { method: "POST", body: json(body) });
  },
};
