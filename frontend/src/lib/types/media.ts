// ── Media protocol ───────────────────────────────────────────────────────
// The vendor-neutral layer: speakers and services addressed uniformly, plus
// zones — sets of speakers that play together regardless of make. The types
// in sonos.ts and kef.ts stay: the per-speaker detail views need vendor
// specifics that don't generalise. See docs/MEDIA-PROTOCOL.md.

/** Which bridge is behind an endpoint. `airplay` is a protocol rather than a
 *  make — a RoPieee, an Apple TV and any shairport-sync box are all driven
 *  identically, which is exactly what a vendor means to the route engine. */
export type MediaVendor = "sonos" | "kef" | "airplay";

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
  | "wake"
  /** Can be *pushed* audio rather than handed something to fetch. The
   *  inverse of play_uri in who holds the audio: HomeHub does the sending
   *  and keeps the clock. */
  | "airplay";

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
  /** A live source describes itself differently — see SonosTrack. Only the
   *  Sonos bridge fills these; a KEF reports a title for everything. */
  stream?: string;
  station?: string;
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
export type MediaRoute = "native" | "connect" | "group" | "airplay" | "stream";

/**
 * How well a route keeps speakers together.
 *
 * Three honest labels rather than a good/bad flag. `buffered` is the stream
 * route: a few hundred milliseconds apart, stable once running, and not the
 * sample-locked sync a native group gets. `clocked` is AirPlay: one sender,
 * one clock, every receiver told which sample belongs at which moment —
 * materially tighter than buffered, still not a vendor's own multi-room bus.
 */
export type MediaSync = "exact" | "single" | "buffered" | "clocked";

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
  /** What a play here would actually sound like, source to speaker. */
  quality?: MediaChain;
}

// ── Quality ──────────────────────────────────────────────────────────────
// "Am I hearing lossless?" has no single answer: the audio passes through two
// hands — the service that encoded it and the path it took to the speaker —
// and a lossless path carrying a lossy source is not lossless. So the backend
// reports a chain with the weakest link named, and the UI shows that rather
// than a badge, because a badge would have to pick a lie.

export type MediaCodec = "pcm" | "alac" | "flac" | "vorbis" | "aac" | "mp3" | "";

export interface MediaQuality {
  codec?: MediaCodec;
  sample_rate?: number;
  bit_depth?: number;
  channels?: number;
  /** Absent for lossless and for unknown, which `lossless` tells apart. */
  bitrate_kbps?: number;
  lossless: boolean;
  /** Something HomeHub could not measure — a speaker's own session, whose
   *  parameters it never sees. Show as "up to", never as a measurement. */
  approximate?: boolean;
}

/** One link in the chain, with whatever is responsible for it named. */
export interface MediaQualityStage {
  name: string;
  quality: MediaQuality;
  detail?: string;
}

/** An improvement that is actually available. Absent is a real answer and
 *  must render as one rather than as a disabled button. */
export interface MediaQualityFix {
  /** The lever. "stream_quality" is the only value that maps to a control;
   *  empty means the improvement is something the listener does to their
   *  zone, so render it as a sentence and never as a button. */
  setting: string;
  label: string;
  detail?: string;
}

/** The end-to-end answer, in the states it actually has.
 *
 *  `up_to` is the one a boolean couldn't hold: on a route where the speaker
 *  holds the service account, nothing in the chain is lossy but nobody
 *  measured the far end either. Rendering it as "lossless" invents a reading;
 *  rendering it as "not lossless" tells someone with a lossless plan their
 *  system is worse than it is. It gets its own badge. */
export type MediaVerdict = "lossless" | "up_to" | "capped" | "unknown";

export interface MediaChain {
  source: MediaQualityStage;
  transport: MediaQualityStage;
  verdict: MediaVerdict;
  /** Bit-exact *and* known to be — true only for verdict `lossless`.
   *  Deliberately false for `up_to`; switch on `verdict` for the badge. */
  lossless: boolean;
  /** The stage that caps the result. What the UI leads with on `capped` —
   *  "not lossless" alone is not actionable. Empty on `up_to`: nothing is
   *  capping it, HomeHub just can't see the far end. */
  limited_by?: string;
  summary: string;
  fix?: MediaQualityFix;
}

/** How hard HomeHub's own decoder asks the service to compress. Household-wide
 *  because the decoder is one process holding one service session. */
export type StreamQuality = "best" | "balanced" | "saver";

export interface StreamQualityOption {
  value: StreamQuality;
  label: string;
  bitrate_kbps: number;
  /** What this choice costs and gains, in the terms the choice is made in. */
  detail: string;
}

/** One route and what it would sound like. */
export interface MediaRouteQuality {
  route: MediaRoute;
  label: string;
  /** True on the routes HomeHub decodes for — the only ones the setting
   *  reaches. Elsewhere the speaker holds the account and picks for itself. */
  decoded: boolean;
  chain: MediaChain;
}

export interface MediaProviderQuality {
  id: string;
  name: string;
  routes: MediaRouteQuality[];
}

/** GET /api/media/quality — the setting, the choices, and what every path
 *  through the house currently sounds like. */
export interface MediaQualityReport {
  stream_quality: StreamQuality;
  bitrate_kbps: number;
  options: StreamQualityOption[];
  providers: MediaProviderQuality[];
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
  /** What it will sound like, source to speaker. */
  quality?: MediaChain;
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
  /**
   * How many times this room has started it. Absent on entries written
   * before the tally existed, which read as the one play that created them —
   * `playCount()` in lib/music/format.ts is the one place that decides so.
   */
  count?: number;
  /** When this room first played it — "since March" rather than "an hour
   *  ago". Absent on entries that predate the field. */
  first_at?: string;
  /** Plays by local hour, 24 slots. Absent on older entries, which is
   *  honest: they are not evidence about any hour in particular. */
  hours?: number[];
}

/** One room's play history, or the household's when it has none of its own. */
export interface MediaHistory {
  plays: MediaPlay[];
  /** True when these are the household's plays rather than this room's. */
  household: boolean;
}

/**
 * One track a room was heard playing — the other half of MediaPlay.
 *
 * A play is what someone *chose*: an album, a playlist, a station. This is
 * what came out of the speaker, written from what the speakers report rather
 * than from what HomeHub was asked to do. It is the only one of the two that
 * survives a queue being replaced, and the only one that knows about track
 * nine of something nobody picked by name.
 */
export interface HeardTrack {
  title: string;
  artist?: string;
  album?: string;
  art_uri?: string;
  /** The service URI where the source had one. Its absence is what makes a
   *  row a name rather than something to play again — radio, line-in and a
   *  KEF's own reporting all land here. */
  uri?: string;
  provider?: string;
  /** What the room was called when this played. */
  room_name?: string;
  at: string;
}

export interface MediaHeard {
  tracks: HeardTrack[];
  /** True when these are the household's tracks rather than this room's. */
  household: boolean;
}

/**
 * What a room keeps coming back to, rather than what it happened to play
 * last — and, when it has a habit at the hour it currently is, what it plays
 * *then*. The difference between offering the kitchen its breakfast radio at
 * eight in the morning and offering it last night's dinner record.
 */
export interface MediaTopPlays {
  plays: MediaPlay[];
  /** True when these are this room's habit at `hour`, false when they are
   *  its favourites overall. The shelf's label depends on it: a wall that
   *  says "you play this now" about a record this room has never played in
   *  the morning is exactly the confident wrongness §15 rules out. */
  by_hour: boolean;
  /** The local hour the ranking was asked for, 0–23. */
  hour: number;
}

/** One name and how many plays sit behind it. */
export interface ListeningTally {
  /** A room key for rooms, the artist's own line for artists — so a row can
   *  be acted on rather than only printed. */
  key: string;
  name: string;
  plays: number;
  /** The most recent play in this tally. */
  at: string;
}

/**
 * What the household's listening adds up to, summed over every room.
 *
 * Bounded by what the per-room lists still hold, so every number here is
 * "plays we remember" and never a lifetime total — `since` is what lets a
 * surface say which window it is describing instead of implying it covers
 * everything.
 */
export interface Listening {
  plays: number;
  /** How many distinct things those plays were. */
  items: number;
  /** Which rooms did the listening, busiest first. */
  rooms: ListeningTally[];
  /** The artist lines of the tracks and albums played, busiest first.
   *  Playlists and stations are left out — their second line is an owner or
   *  a service, and counting those would put "Spotify" at the top. */
  artists: ListeningTally[];
  /** The most-played items themselves, merged across rooms. */
  top: MediaPlay[];
  /** When the house listens: 24 slots, local time. */
  hours: number[];
  /** The oldest play still remembered. Absent when nothing has played. */
  since?: string;
}

// ── Music timers ─────────────────────────────────────────────────────────
// Music that starts and stops without anyone tapping anything: the half the
// socket scheduler could never reach. One type covers both uses because they
// differ only in which end of the fade they are on — arrive at 20 over ten
// minutes at 06:45, or take the room down to nothing in forty.

export type MusicTimerAction = "start" | "stop";

/** What a starting timer puts on. Carried in full rather than as a URI so
 *  06:45 is not the moment a catalog round trip has to succeed. */
export interface MusicTimerItem {
  provider?: string;
  kind?: string;
  uri?: string;
  title?: string;
  sub?: string;
  art_uri?: string;
}

export interface MusicTimer {
  id: string;
  name?: string;
  /** The media layer's destination key — "sonos:…", "kef:…", "zone:…" — the
   *  same vocabulary the play history uses. */
  room: string;
  action: MusicTimerAction;
  enabled: boolean;
  /** Set makes this a one-shot: it runs once and is deleted. This is what
   *  "sleep in forty minutes" is. */
  fires_at?: string;
  /** "HH:MM" plus days makes it recurring. Empty days means every day. */
  time?: string;
  days?: number[];
  item?: MusicTimerItem;
  /** Where the room ends up: the level to arrive at for a start, the level
   *  to fade down to for a stop. Absent leaves the volume alone. */
  volume?: number;
  /** How long to take getting there. Zero is a jump. */
  fade_minutes?: number;
  last_fired_at?: string;
}

/** A timer plus what the backend already knows and the wall would otherwise
 *  have to work out for itself. */
export interface MusicTimerView extends MusicTimer {
  /** What the house calls the room now — not necessarily what it was called
   *  when the timer was set. */
  room_name: string;
  /** When this next fires, so a row can say "in 6 hours" without
   *  reimplementing weekday arithmetic. Absent when it never will. */
  next_at?: string;
  /** A ramp is walking this room right now — the state between "sleep timer
   *  set" and "room quiet", otherwise invisible except as volume drifting
   *  on its own. */
  fading: boolean;
}

/** What setting a sleep timer answered. */
export interface MusicSleepResult {
  timer: MusicTimer;
  /** When the room actually goes quiet — the number worth reading back,
   *  rather than when the fade starts. */
  quiet_at: string;
}

/** One room an announcement could be sent to. */
export interface AnnounceRoom {
  id: string;
  name: string;
}

/** What the announcement control needs before anyone taps it. */
export interface AnnounceStatus {
  /** False when no speaker is answering — there is nowhere to announce to. */
  available: boolean;
  /** The rooms that would hear it, in the order they will be addressed. */
  rooms: AnnounceRoom[];
  /** False when no voice service is configured: a chime, and no words. */
  voice: boolean;
  max_text: number;
  /** The sentences the wall offers before its box, from household settings
   *  (edited in Settings, only read by the kiosk). They come from the server
   *  rather than a constant here for two reasons pulling the same way:
   *  typing is the worst thing a wall asks anyone to do, so the presets are
   *  most of what the control is — and they are read out by a voice that
   *  speaks one language, which the household picks. An empty list is a
   *  household that wants the box and nothing above it. */
  presets: string[];
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
