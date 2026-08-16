/**
 * The media protocol: speakers and services addressed uniformly,
 * zones that span makes, the timers that start and stop a room on its own,
 * and calling the house.
 *
 * One slice of the flat `api` object — lib/api.ts spreads these together,
 * so every call site's `api.foo()` is unchanged. Split by domain because a
 * thousand-line object is not a thing anyone reads.
 */

import { req, json } from "./http";
import type {
  AnnounceResult,
  AnnounceStatus,
  Listening,
  MediaEndpoint,
  MediaHeard,
  MediaHistory,
  MediaPlayResult,
  MediaProvider,
  MediaQualityReport,
  MediaResults,
  MediaTopPlays,
  MediaZone,
  MediaZoneRoutes,
  MusicSleepResult,
  MusicTimer,
  MusicTimerView,
  QobuzStatus,
  StreamQuality,
  UPnPDescription,
  UPnPRenderer,
} from "../types";

export const mediaApi = {
  // ── Media protocol ─────────────────────────────────────────────────────
  // Speakers and services addressed uniformly, plus zones — sets of speakers
  // that play together regardless of make. The sonos*/kef* calls above stay:
  // they carry vendor specifics the detail views need.
  // See docs/MEDIA-PROTOCOL.md.
  mediaEndpoints() { return req<MediaEndpoint[]>("/media/endpoints"); },
  // UPnP/DLNA renderers. Registration is a *describe* rather than a probe:
  // a renderer publishes its control URLs inside a device description at a
  // URL of its choosing, so adding one means reading that document. Describe
  // first to show what was found, then create.
  upnpRenderers() { return req<UPnPRenderer[]>("/upnp/renderers"); },
  upnpDescribe(location: string) {
    return req<UPnPDescription>("/upnp/describe", { method: "POST", body: json({ location }) });
  },
  upnpCreateRenderer(body: { location: string; name?: string; room?: string }) {
    return req<UPnPRenderer>("/upnp/renderers", { method: "POST", body: json(body) });
  },
  upnpUpdateRenderer(id: string, body: { name?: string; room?: string }) {
    return req<UPnPRenderer>(`/upnp/renderers/${encodeURIComponent(id)}`, {
      method: "PUT", body: json(body),
    });
  },
  /** Re-read the device description — the fix for a renderer that rebooted
   *  onto a different port and stopped answering the URLs we remembered. */
  upnpRefreshRenderer(id: string) {
    return req<UPnPRenderer>(`/upnp/renderers/${encodeURIComponent(id)}/refresh`, { method: "POST" });
  },
  upnpDeleteRenderer(id: string) {
    return req<void>(`/upnp/renderers/${encodeURIComponent(id)}`, { method: "DELETE" });
  },
  upnpSetVolume(id: string, level: number) {
    return req<void>(`/upnp/${encodeURIComponent(id)}/volume`, {
      method: "PUT", body: json({ level }),
    });
  },
  upnpSetMute(id: string, muted: boolean) {
    return req<void>(`/upnp/${encodeURIComponent(id)}/mute`, {
      method: "PUT", body: json({ muted }),
    });
  },

  // Qobuz. Setup is two calls because the credentials come from two parties;
  // see QobuzStatus. The password is sent once and never stored — what
  // persists server-side is the token Qobuz returns for it.
  qobuzStatus() { return req<QobuzStatus>("/qobuz/status"); },
  qobuzSetConfig(appId: string, appSecret: string) {
    return req<QobuzStatus>("/qobuz/config", {
      method: "PUT",
      body: json({ app_id: appId, app_secret: appSecret }),
    });
  },
  qobuzLogin(email: string, password: string) {
    return req<QobuzStatus>("/qobuz/login", { method: "POST", body: json({ email, password }) });
  },
  qobuzDisconnect() { return req<QobuzStatus>("/qobuz/disconnect", { method: "POST" }); },

  mediaProviders() { return req<MediaProvider[]>("/media/providers"); },
  mediaSearch(q: string, opts?: { provider?: string; limit?: number }) {
    const p = new URLSearchParams({ q });
    if (opts?.provider) p.set("provider", opts.provider);
    if (opts?.limit) p.set("limit", String(opts.limit));
    return req<MediaResults>(`/media/search?${p}`);
  },
  mediaZones() { return req<MediaZone[]>("/media/zones"); },
  mediaCreateZone(body: { name: string; members: string[]; room?: string }) {
    return req<MediaZone>("/media/zones", { method: "POST", body: json(body) });
  },
  mediaUpdateZone(id: string, body: { name?: string; members?: string[]; room?: string }) {
    return req<MediaZone>(`/media/zones/${encodeURIComponent(id)}`, { method: "PUT", body: json(body) });
  },
  mediaDeleteZone(id: string) {
    return req<void>(`/media/zones/${encodeURIComponent(id)}`, { method: "DELETE" });
  },
  // What this zone could do, and for anything it can't, which speaker blocked
  // it. Read before playing so the UI can explain a limitation rather than
  // letting the user discover it as a failure.
  mediaZoneRoutes(id: string, provider?: string) {
    const p = provider ? `?provider=${encodeURIComponent(provider)}` : "";
    return req<MediaZoneRoutes>(`/media/zones/${encodeURIComponent(id)}/routes${p}`);
  },
  // Starts content on a zone. The response says which route was chosen and
  // why — a streamed zone genuinely differs from a natively grouped one, and
  // the UI is expected to say so rather than present them as equivalent.
  // A 409 means something the user can fix: connect an account, wake a
  // speaker, install librespot, or pick different speakers.
  mediaZonePlay(
    id: string,
    body: { provider?: string; uri: string; title?: string; kind?: string; sub?: string; art_uri?: string },
  ) {
    return req<MediaPlayResult>(`/media/zones/${encodeURIComponent(id)}/play`, {
      method: "POST", body: json(body),
    });
  },
  // What the audio actually is on every path through the house, and the one
  // setting that changes it. Read rather than inferred: whether something is
  // lossless depends on the service *and* the route, and only the backend
  // knows both.
  mediaQuality() { return req<MediaQualityReport>("/media/quality"); },
  // Lands on the next thing played, not on what is playing: the bitrate is
  // baked into the decoder's command line, so applying it now would mean
  // cutting off the music to improve it.
  setMediaQuality(quality: StreamQuality) {
    return req<{ stream_quality: StreamQuality; bitrate_kbps: number; applies: string }>(
      "/media/quality", { method: "PUT", body: json({ stream_quality: quality }) },
    );
  },
  // What a room has been asked to play, newest first. A room with no history
  // of its own answers with the household's, flagged as such — the shelf says
  // "Played here" for one and "Played recently" for the other, because a wall
  // must never imply a room played something it didn't.
  mediaHistory(room: string, limit = 12) {
    return req<MediaHistory>(
      `/media/history?room=${encodeURIComponent(room)}&limit=${limit}`,
    );
  },
  // One room stops remembering one thing; without a uri it forgets the lot.
  // The shelves are *ranked*, so a record started by mistake doesn't sink —
  // it competes for the first shelf the wall offers, and every accidental
  // replay pushes it up. Per room, never household-wide: the same record is
  // the kids' room's favourite and the living room's mistake.
  mediaForgetPlay(room: string, uri?: string) {
    const p = new URLSearchParams({ room });
    if (uri) p.set("uri", uri);
    return req<void>(`/media/history?${p}`, { method: "DELETE" });
  },
  // What a room was *heard* playing, newest first — written from what the
  // speakers report rather than from what anyone asked for, which is what
  // makes it survive a queue being replaced. A room with nothing of its own
  // answers with the household's, flagged, exactly as the shelf above does.
  mediaHeard(room: string, limit = 40) {
    return req<MediaHeard>(`/media/heard?room=${encodeURIComponent(room)}&limit=${limit}`);
  },
  // One room stops keeping a log. Whole-room only: nothing ranks this list or
  // offers it back, so there is no single row worth surgery.
  mediaForgetHeard(room: string) {
    return req<void>(`/media/heard?room=${encodeURIComponent(room)}`, { method: "DELETE" });
  },
  // What a room keeps coming back to, rather than what it happened to play
  // last. `hour` takes a local hour or "now", and ranks by what this room
  // plays at that hour — the difference between offering the kitchen its
  // breakfast radio at eight and offering it last night's dinner record. The
  // answer says which of the two it gave (`by_hour`).
  mediaTopPlays(room: string, opts: { limit?: number; hour?: number | "now" } = {}) {
    const p = new URLSearchParams({ room, limit: String(opts.limit ?? 8) });
    if (opts.hour !== undefined) p.set("hour", String(opts.hour));
    return req<MediaTopPlays>(`/media/history/top?${p}`);
  },
  // What the household listens to, summed over every room: who does the
  // listening, which artists it keeps coming back to, and when in the day it
  // is loud. Deliberately not per-room — the per-room questions are already
  // answered above, and this is the one picture none of them can give.
  mediaInsights(limit = 8) {
    return req<Listening>(`/media/insights?limit=${limit}`);
  },

  // ── Music timers ───────────────────────────────────────────────────────
  // Music that starts and stops on its own: the half the socket scheduler
  // could never reach. A wake-up is arranged in advance and described in
  // full, so it is ordinary CRUD; a sleep timer is set by someone already in
  // bed and is "forty minutes, this room", so it has a call of its own that
  // does the arithmetic.
  musicTimers() { return req<MusicTimerView[]>("/media/timers"); },
  musicCreateTimer(body: Omit<MusicTimer, "id">) {
    return req<MusicTimer>("/media/timers", { method: "POST", body: json(body) });
  },
  // The body replaces the timer wholesale: the two schedules are mutually
  // exclusive, so a partial update would have to define what clearing each
  // of them looks like.
  musicUpdateTimer(id: string, body: Omit<MusicTimer, "id">) {
    return req<MusicTimer>(`/media/timers/${encodeURIComponent(id)}`, {
      method: "PUT", body: json(body),
    });
  },
  musicDeleteTimer(id: string) {
    return req<void>(`/media/timers/${encodeURIComponent(id)}`, { method: "DELETE" });
  },
  // "Quiet in forty minutes." `minutes` is when the room goes silent and the
  // fade is the tail of that wait rather than time added to it, so the room
  // is quiet at forty and not at forty-eight. Setting one twice replaces it.
  // The engine puts the volume back afterwards — a room faded to two and
  // paused is inaudible the next morning.
  musicSleep(body: { room: string; minutes: number; fade_minutes?: number; volume?: number }) {
    return req<MusicSleepResult>("/media/timers/sleep", { method: "POST", body: json(body) });
  },
  // Stop a ramp in flight without deleting anything — "I'm still up". The
  // room keeps whatever volume it started the fade at.
  musicCancelFade(room: string) {
    return req<{ cancelled: boolean }>("/media/timers/fade/cancel", {
      method: "POST", body: json({ room }),
    });
  },

  // Calling the house. The status read is what decides whether the control is
  // drawn at all and whether it offers words or only a chime.
  announceStatus() { return req<AnnounceStatus>("/announce"); },
  announce(text: string, rooms?: string[]) {
    return req<AnnounceResult>("/announce", {
      method: "POST",
      body: json(rooms?.length ? { text, rooms } : { text }),
    });
  },

  mediaZoneResume(id: string) {
    return req<void>(`/media/zones/${encodeURIComponent(id)}/resume`, { method: "POST" });
  },
  mediaZonePause(id: string) {
    return req<void>(`/media/zones/${encodeURIComponent(id)}/pause`, { method: "POST" });
  },
  mediaZoneNext(id: string) {
    return req<void>(`/media/zones/${encodeURIComponent(id)}/next`, { method: "POST" });
  },
  mediaZonePrevious(id: string) {
    return req<void>(`/media/zones/${encodeURIComponent(id)}/previous`, { method: "POST" });
  },
  // Stop, unlike pause, also releases a stream session — so librespot stops
  // holding the account's Spotify device.
  mediaZoneStop(id: string) {
    return req<void>(`/media/zones/${encodeURIComponent(id)}/stop`, { method: "POST" });
  },
  mediaZoneVolume(id: string, level: number) {
    return req<void>(`/media/zones/${encodeURIComponent(id)}/volume`, { method: "PUT", body: json({ level }) });
  },
  mediaZoneMute(id: string, muted: boolean) {
    return req<void>(`/media/zones/${encodeURIComponent(id)}/mute`, { method: "PUT", body: json({ muted }) });
  },
};
