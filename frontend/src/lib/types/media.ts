// ── Media protocol ───────────────────────────────────────────────────────
// The vendor-neutral layer: speakers and services addressed uniformly, plus
// zones — sets of speakers that play together regardless of make. The types
// in sonos.ts and kef.ts stay: the per-speaker detail views need vendor
// specifics that don't generalise. See docs/MEDIA-PROTOCOL.md.

/** Which bridge is behind an endpoint. */
export type MediaVendor = "sonos" | "kef";

/**
 * One thing a speaker can do. These are the strings the backend emits, and
 * the UI keys off them rather than off the vendor — that is what lets a new
 * bridge appear without every component learning about it.
 */
export type MediaCapability =
  | "transport"
  | "volume"
  | "seek"
  | "queue"
  | "group"
  | "play_uri"
  | "native_service"
  | "connect"
  | "wake";

/** A speaker's identity and what it can do, without touching the device. */
export interface MediaEndpoint {
  id: string;
  name: string;
  room?: string;
  vendor: MediaVendor;
  model?: string;
  capabilities: MediaCapability[];
  /** Bridge-qualified id ("sonos:abc") — what a zone stores as a member. */
  member: string;
}

/** Normalised transport state, common across both bridges. */
export type MediaPlayState = "playing" | "paused" | "stopped" | "transitioning";

export interface MediaTrack {
  title?: string;
  artist?: string;
  album?: string;
  art_uri?: string;
}

export interface MediaNowPlaying {
  state: MediaPlayState;
  volume: number;
  muted: boolean;
  track?: MediaTrack;
  position_ms?: number;
  duration_ms?: number;
  /** When the reading was taken — extrapolate position from this, not now. */
  at: string;
}

/**
 * How content reaches a zone. Ordered best-first by the backend, and the
 * ordering is a guarantee: `stream` is only ever chosen when nothing else
 * can serve the whole zone, so a Sonos-only zone never lands on it.
 */
export type MediaRoute = "native" | "connect" | "group" | "stream";

/**
 * How well a route keeps speakers together. `buffered` is the honest label
 * for the stream route — a few hundred milliseconds apart, stable once
 * running, and not the sample-locked sync a native group gets.
 */
export type MediaSync = "exact" | "single" | "buffered";

/** Whether something is usable right now, and if not what to do about it. */
export interface MediaAvailability {
  ok: boolean;
  configured: boolean;
  reason?: string;
}

export interface MediaProvider {
  id: string;
  name: string;
  availability: MediaAvailability;
  routes: MediaRoute[];
  /**
   * Whether the cross-vendor route works. A separate question from
   * `availability` — a service can search perfectly and be unable to stream,
   * and the two need different prompts.
   */
  streaming: MediaAvailability;
}

/** One member of a zone, with whatever live state it reported. */
export interface MediaZoneSpeaker extends MediaEndpoint {
  state?: MediaNowPlaying;
  /** Set when the speaker behind this member no longer exists. */
  missing?: boolean;
}

export interface MediaZone {
  id: string;
  name: string;
  /** Bridge-qualified speaker ids, in the order the user arranged them. */
  members: string[];
  room?: string;
  speakers: MediaZoneSpeaker[];
  /** What a play would use right now; absent when nothing can serve it. */
  route?: MediaRoute;
  sync?: MediaSync;
  /** Why that route, in words fit to show. Always set when route is. */
  reason?: string;
  /** Why nothing can serve this zone. Set instead of route/reason. */
  problem?: string;
}

export type MediaItemKind = "track" | "album" | "playlist" | "artist" | "station";

export interface MediaItem {
  provider: string;
  kind: MediaItemKind;
  uri: string;
  title: string;
  subtitle?: string;
  art_uri?: string;
}

export interface MediaResults {
  tracks: MediaItem[];
  albums: MediaItem[];
  playlists: MediaItem[];
  artists: MediaItem[];
}

/** What a play answered: which route it took, and why. */
export interface MediaPlayResult {
  route: MediaRoute;
  sync: MediaSync;
  reason: string;
  /** Present only on the stream route. */
  stream_url?: string;
  speakers: string[];
}

/** One route that couldn't serve a zone, and the reason naming the speaker. */
export interface MediaRouteBlock {
  route: MediaRoute;
  reason: string;
}

export interface MediaZoneRoutes {
  speakers: string[];
  route?: MediaRoute;
  sync?: MediaSync;
  reason?: string;
  problem?: string;
  /** Every rejected route, in preference order. Present with `problem`. */
  blocked?: MediaRouteBlock[];
}

/**
 * One thing a room was asked to play, as HomeHub remembers it.
 *
 * This is the app's own memory rather than Spotify's, because Spotify's is
 * one list for the whole household and cannot say that the kitchen gets
 * radio at breakfast. `provider` decides how a tile replays it: "spotify"
 * goes back through the play path it came from, "sonos" is a household
 * favorite and is matched against the current favorites list by URI (a
 * favorite that has since been deleted simply stops appearing).
 */
export interface MediaPlay {
  provider: string;
  kind?: MediaItemKind;
  uri: string;
  title: string;
  sub?: string;
  art_uri?: string;
  /** What the room was called when this played. */
  room_name?: string;
  at: string;
}

/** One room's play history, or the household's when it has none of its own. */
export interface MediaHistory {
  plays: MediaPlay[];
  /** True when these are the household's plays rather than this room's. */
  household: boolean;
}

/** What the announcement control needs before anyone taps it. */
export interface AnnounceStatus {
  /** False when no speaker is answering — there is nowhere to announce to. */
  available: boolean;
  /** The rooms that would hear it, in the order they will be addressed. */
  rooms: string[];
  /** False when no voice service is configured: a chime, and no words. */
  voice: boolean;
  max_text: number;
}

/** What an announcement did. */
export interface AnnounceResult {
  rooms: string[];
  /** Rooms that didn't take it, so the surface can be honest about reach. */
  unreachable: string[];
  /** False when the words were dropped and only the chime played. */
  spoken: boolean;
  duration_ms: number;
}
