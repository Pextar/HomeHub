/**
 * What the panel's speaker brain deals in: one playable room on the wall,
 * the speakers inside it, and the surface the store presents to the kiosk.
 *
 * Split out of the store itself (`lib/panel-music.svelte.ts`) because these
 * are the vocabulary rather than the behaviour — fifteen components name
 * `PanelSource` and none of them care how one gets built. The store
 * re-exports every name here, so the import path components use is
 * unchanged.
 */

import type { PanelNowPlaying } from "../panel";
import type {
  MediaVendor,
  SonosGroupState,
  SonosQueueItem,
  SonosFavorite,
  KEFSource,
  SpotifyItem,
  MediaZone,
  MediaRoute,
  MediaPlay,
  MusicTimer,
  MusicTimerView,
  Listening,
  AnnounceStatus,
} from "../types";

/** Which bridge a member speaker's own volume/mute calls go to. Every vendor
 *  the media layer knows, because a zone can hold any mix of them and each
 *  takes its own call — an AirPlay receiver's volume travels over the RTSP
 *  session HomeHub already holds open to it. */
export type PanelVendor = MediaVendor;

/** One speaker inside a featured group or zone, coordinator/lead first. */
export interface PanelMember {
  id: string;
  name: string;
  vendor: PanelVendor;
  volume: number;
  muted: boolean;
  coordinator: boolean;
}

/** One playable source on the wall: a HomeHub zone, a reachable Sonos
 *  group, or a reachable KEF speaker not already claimed by a zone. */
export interface PanelSource {
  key: string;
  kind: "sonos" | "kef" | "zone";
  id: string; // coordinator id (sonos), speaker id (kef), zone id (zone)
  title: string; // zone / group / speaker name
  playing: boolean;
  standby: boolean; // kef only — powered off, no transport
  volume: number;
  muted: boolean;
  /** A skip would reach something: a queue, a stream, a network input. */
  canSkip: boolean;
  trackTitle?: string;
  trackSub?: string;
  /** The artist alone, where the sub line is "artist · album". What the
   *  radio seeds from and what the artist page is opened by name with. */
  trackArtist?: string;
  /** The canonical Spotify URI of what is playing, when the source is
   *  Spotify. Absent on radio, line-in and anything else — saving and
   *  "open the artist" render only where it is present (§15.1 applied to
   *  a track rather than to a room). */
  trackURI?: string;
  art?: string;
  /** Every speaker inside it, when there is more than one to balance. */
  members?: PanelMember[];
  // Sonos extras — the group's, so only on a Sonos source.
  groupState?: SonosGroupState;
  /** HomeHub's own "keep going with similar music" preference (§15.5). */
  autoplay?: boolean;
  queueTrack?: number; // 1-based position in the queue, when playing from it
  position?: string; // H:MM:SS — the wire form, parsed by the position deriveds
  duration?: string;
  // KEF extras.
  input?: string; // current physical input
  // KEF + zone extras: milliseconds. (Sonos speaks H:MM:SS, above.)
  positionMs?: number;
  durationMs?: number;
  /** Unix ms the reading was taken, on every make — what the position
   *  extrapolates from, rather than from when the poll happened to land. */
  readAt?: number;
  // Zone extras.
  zone?: MediaZone;
  /** How content reaches a zone right now — what makes skip honest. */
  route?: MediaRoute;
}

export interface PanelMusicStore {
  readonly hasSpeakers: boolean;
  /** Speakers are registered but none answered — the column stays and
   *  says so rather than vanishing and reflowing the panel grid. */
  readonly unreachable: boolean;
  readonly sources: PanelSource[];
  readonly featured: PanelSource | undefined;
  /** The user's chip pick; null falls back to whatever is playing. */
  selected: string | null;
  /** Pin the current fallback as an explicit pick, so a room that starts
   *  playing elsewhere can't move the destination under a finger. */
  latchFeatured(): void;
  readonly busy: Record<string, boolean>;
  /** What the ambient face shows while music plays. */
  readonly nowPlaying: PanelNowPlaying | null;
  /** Anything playing anywhere — what "Pause everything" is offered for. */
  readonly anyPlaying: boolean;
  /** While the panel sleeps, the poll can slow right down. */
  setIdle(idle: boolean): void;

  // ── Position (extrapolated between polls on the 1s beat) ──
  readonly posSec: number;
  readonly durSec: number;
  /** A seek endpoint exists: a Sonos track with a length behind it. */
  readonly seekable: boolean;
  seek(sec: number): void;

  // ── Volume ──
  /** Group (or single-speaker) fader value — the finger's while it's down. */
  readonly vol: number;
  /** Live, on every movement: shows at once, sends on a short throttle. */
  dragVolume(s: PanelSource, level: number): void;
  /** On release — the authoritative value. */
  setVolume(s: PanelSource, level: number): void;
  /** One step of the ± buttons — a fader is imprecise at arm's length. */
  nudgeVolume(s: PanelSource, delta: number): void;
  /** Per-member faders, when a group or zone has more than one speaker. */
  readonly memVol: Record<string, number>;
  dragMemberVolume(id: string, level: number): void;
  setMemberVolume(id: string, level: number): void;
  /** No memberId mutes the whole room; one mutes that speaker. */
  toggleMute(s: PanelSource, memberId?: string): void;

  // ── Transport & play modes ──
  togglePlay(s: PanelSource): void;
  skip(s: PanelSource, dir: "next" | "previous"): void;
  /** Wake a KEF speaker out of standby — the panel's own job, not the
   *  full view's: waking a speaker isn't configuring one. */
  wake(s: PanelSource): void;
  /** Pause every source that is playing, in one tap. */
  pauseAll(): void;
  toggleShuffle(): void;
  cycleRepeat(): void;
  toggleCrossfade(): void;
  /** "Play similar": keep the room going once the queue runs out (§15.5). */
  toggleAutoplay(): void;

  // ── KEF ──
  setKefSource(s: PanelSource, source: KEFSource): void;

  // ── Sleep and wake (HomeHub's own timers, any room) ──────────────────
  /** Every music timer in the house, soonest first. */
  readonly timers: MusicTimerView[];
  /** The featured room's, soonest first. */
  readonly roomTimers: MusicTimerView[];
  /** The featured room's sleep timer — the one-shot stop — if it has one. */
  readonly sleepTimer: MusicTimerView | undefined;
  /** Whole minutes until the featured room goes quiet, counted down
   *  locally between reads; 0 when nothing is going to quiet it. */
  readonly sleepMinutesLeft: number;
  /** A ramp is walking the featured room right now: the state between
   *  "sleep timer set" and "room quiet", otherwise visible only as the
   *  volume drifting on its own. */
  readonly fading: boolean;
  /** "Quiet in forty minutes", on any room — the gesture this whole
   *  mechanism exists for. Replaces the room's existing sleep timer. */
  setSleepIn(minutes: number): void;
  /** Call the whole thing off: the timer goes, and a ramp already in
   *  flight is cancelled, which puts the volume back and leaves the music
   *  playing. */
  clearSleep(): void;
  /** "I'm still up" — stop the ramp without deleting the timer. */
  cancelFade(): void;
  /** Wake this room to something at a time of day. */
  setWake(opts: {
    time: string;
    days: number[];
    volume?: number;
    fadeMinutes?: number;
    item: MusicTimer["item"];
    name?: string;
  }): void;
  setTimerEnabled(t: MusicTimerView, enabled: boolean): void;
  deleteTimer(t: MusicTimerView): void;

  // ── Sonos' own sleep timer (set in the Sonos app, reported by it) ────
  /** Whole minutes left on a timer the *speaker* is keeping, which is not
   *  the same thing as HomeHub's — the panel says so rather than folding
   *  two different clocks into one number. 0 for none. */
  readonly sonosSleepMinutes: number;
  /** Clear the speaker's own timer. Only ever called with 0 from the
   *  panel: HomeHub's timers are what the wall now sets. */
  setSonosSleep(minutes: number): void;

  // ── Queue (a Sonos group's — empty for anything else) ──
  readonly queue: SonosQueueItem[];
  readonly queueLoading: boolean;
  /** The first queued track after the one playing — the Up-next row. */
  readonly nextInQueue: SonosQueueItem | undefined;
  /** False when shuffle or repeat-one means queue order isn't play order,
   *  so nothing on the wall may claim to know what comes next. */
  readonly queueOrderKnown: boolean;
  jumpTo(track: number): void;
  removeQueued(track: number): void;
  /** Move a queued track one place up (-1) or down (+1). One place at a
   *  time, by tap: the app's drag is an imprecise aim at arm's length. */
  moveQueued(track: number, dir: -1 | 1): void;
  clearQueue(): void;
  /** Add a search result without disturbing what's playing. */
  enqueue(item: SpotifyItem, next: boolean): void;
  /** The last thing queued, for the player column's inline confirmation —
   *  a queued track changes nothing visible on its own. */
  readonly lastQueued: { title: string; next: boolean; at: number } | null;

  // ── Sonos favorites (a household list — radio, and what was starred) ──
  readonly favorites: SonosFavorite[];
  playFavorite(f: SonosFavorite): void;

  // ── Starting something ──
  playItem(item: SpotifyItem): Promise<void>;

  // ── What this room played before (HomeHub's own memory, per room) ──
  readonly history: MediaPlay[];
  /** True when the list is the household's rather than this room's own —
   *  the shelf says which, because a wall must never imply a room played
   *  something it didn't. */
  readonly historyHousehold: boolean;
  playFromHistory(p: MediaPlay): void;
  /** This room stops remembering something. The shelves are ranked, so a
   *  record started by mistake doesn't sink out of the way on its own — it
   *  competes for the first thing the wall offers, and every accidental
   *  replay pushes it further up. Only ever the featured room's own list;
   *  the household's fallback is not this room's to edit. */
  forgetPlay(p: MediaPlay): void;
  /** Whether `forgetPlay` would reach anything: the shelf is showing this
   *  room's own memory rather than the household's, and the login may
   *  write. Absent rather than refused, per §15.1. */
  readonly canForget: boolean;

  // ── What this room keeps coming back to ──────────────────────────────
  /** Ranked by how often this room has started them — and, when it has a
   *  habit at the hour it currently is, ranked by what it plays *then*.
   *  Empty for a room with nothing of its own; the plain history is what
   *  a shelf falls back to. */
  readonly topPlays: MediaPlay[];
  /** True when `topPlays` is this room's habit at `topPlaysHour` rather
   *  than its favourites overall. The shelf's label depends on it. */
  readonly topPlaysByHour: boolean;
  /** The local hour those plays were ranked for. */
  readonly topPlaysHour: number;

  // ── What the household listens to ────────────────────────────────────
  /** Summed over every room: who does the listening, which artists the
   *  house keeps coming back to, and how far back the numbers reach.
   *  Null while the read is out or when it was refused. */
  readonly insights: Listening | null;

  // ── Saving what's playing ──
  /** The login may write to the library. False hides the heart rather
   *  than offering a tap that will be refused. */
  readonly canSave: boolean;
  /** Whether what's playing is in the account's library. Meaningless
   *  unless the featured source has a trackURI. */
  readonly saved: boolean;
  toggleSaved(): void;

  // ── More like this ──
  /** Something is playing with an artist to seed from. */
  readonly canRadio: boolean;
  /** Queue more of what's on (Sonos), or play the first of it (anywhere
   *  else, which has no queue to fill). */
  startRadio(): void;
  /** What the last run added, for the in-place confirmation — queuing
   *  changes nothing visible on its own. */
  readonly lastRadio: { count: number; artist: string; at: number } | null;

  // ── Announcements ──
  /** Where an announcement would go and whether it would be spoken.
   *  Null while the read is out or when the server has no answer. */
  readonly announce: AnnounceStatus | null;
  sendAnnouncement(text: string, rooms?: string[]): void;
  readonly lastAnnounce: { text: string; rooms: string[]; spoken: boolean; at: number } | null;

  // ── Grouping (Sonos-native only) ──
  /** The Sonos rooms that could join the featured one — every other
   *  reachable Sonos source. Empty unless a Sonos room is featured:
   *  nothing else groups natively, and a control that would be refused
   *  is worse than one that isn't there (§15.1). */
  readonly joinable: PanelSource[];
  /** There is something to say about grouping at all: a Sonos room with
   *  either somewhere to join from or more than one speaker to split. */
  readonly canGroup: boolean;
  /** Every speaker in src's group joins the featured group. */
  joinSource(src: PanelSource): void;
  /** Every joinable room at once — the wall's "play everywhere" tap. */
  joinAll(): void;
  /** Take the music with you: the featured room's group moves to dest and
   *  leaves where it was. Sonos-native, like every other grouping call. */
  moveTo(dest: PanelSource): void;
  /** Every non-coordinator member leaves the featured group. */
  ungroupFeatured(): void;
  /** One member steps out of the featured group. */
  leaveMember(memberId: string): void;

  refresh(): Promise<void>;
}

export interface PanelMusicOptions {
  /** The kid surface: Sonos only — never poll KEF or zones, never list one. */
  sonosOnly?: boolean;
}
