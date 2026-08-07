// Sonos (local UPnP control).

export interface SonosSpeaker {
  id: string;
  name: string;
  ip: string;
  uuid: string;
  room?: string;
  model?: string;
}

export interface SonosTrack {
  title?: string;
  artist?: string;
  album?: string;
  /** Absolute URL or an /api/sonos/{id}/art proxy path — usable as <img src> directly. */
  art_uri?: string;
  /** The canonical "spotify:track:…" for what is playing, recovered from the
   *  speaker's own resource string. Absent on radio, line-in and anything
   *  that isn't Spotify — surfaces that need it render only where it is. */
  spotify_uri?: string;
}

export interface SonosState {
  transport_state: string; // PLAYING | PAUSED_PLAYBACK | STOPPED | TRANSITIONING
  playing: boolean;
  volume: number; // 0-100
  muted: boolean;
  track?: SonosTrack;
  position?: string; // H:MM:SS
  duration?: string; // H:MM:SS, empty for live streams
  /** 1-based position in the group queue; absent on radio / line-in. */
  queue_track?: number;
}

export type SonosRepeat = "off" | "all" | "one";

/**
 * Playback settings that belong to a zone group rather than to one speaker.
 * Only present on a coordinator's view — followers never carry it.
 */
export interface SonosGroupState {
  shuffle: boolean;
  repeat: SonosRepeat;
  crossfade: boolean;
  queue_length: number;
  /** True when the group is playing from its queue rather than a stream. */
  from_queue: boolean;
}

/** One track in a group queue. */
export interface SonosQueueItem {
  track: number; // 1-based
  title?: string;
  artist?: string;
  album?: string;
  art_uri?: string;
  duration?: string; // H:MM:SS
}

/** A registered speaker plus its live state (state omitted when unreachable). */
export interface SonosSpeakerView extends SonosSpeaker {
  reachable: boolean;
  state?: SonosState;
  group_state?: SonosGroupState;
  /** HomeHub's own "continue with similar music once the queue runs out"
   *  setting for this coordinator — a preference layered on top of what the
   *  speaker reports, the same shape as crossfade. On unless the room opted
   *  out (§15.5), and always sent, so `false` really means "let it end".
   *  Only meaningful when group_state is present. */
  autoplay?: boolean;
}

/** One live zone group, expressed in registered speaker ids. */
export interface SonosGroupView {
  coordinator_id: string;
  member_ids: string[];
  /** Zone names grouped on the Sonos side but not registered in HomeHub. */
  unregistered?: string[];
}

export interface SonosStatus {
  speakers: SonosSpeakerView[];
  groups: SonosGroupView[];
  /**
   * True when the backend is subscribed to the speakers' own change
   * notifications, so this state arrived without anyone asking for it.
   * False means it was read on demand — the bridge couldn't subscribe —
   * and the caller should keep polling at the old rate.
   */
  live?: boolean;
}

/**
 * One speaker's push status. Timestamps are RFC3339 strings and absent when
 * the thing they describe hasn't happened — no event yet, no renewal due.
 */
export interface SonosEventSpeaker {
  id: string;
  name: string;
  ip: string;
  /** Whether this speaker currently holds event subscriptions. */
  subscribed: boolean;
  reachable: boolean;
  /** Subscribed service keys: transport, rendering, topology. */
  services?: string[];
  /** The URL this speaker was told to post its notifications to. */
  callback?: string;
  /** Notifications accepted from it since the hub started. */
  events: number;
  last_event?: string;
  renew_at?: string;
  /** Why the last subscription attempt failed; absent once one succeeds. */
  error?: string;
}

/**
 * How the push subsystem is doing, per speaker. Read by the "Live updates"
 * sheet — the surface that explains why the app is or isn't getting speaker
 * state pushed to it, and what to do about it.
 */
export interface SonosEventHealth {
  /** At least one speaker is subscribed — matches SonosStatus.live. */
  live: boolean;
  /** The subscription supervisor is up. False is a hub problem, not a network one. */
  running: boolean;
  subscribed: number;
  total: number;
  /** The address speakers are told to reach the hub on. */
  callback?: string;
  /** Why that address couldn't be worked out at all. */
  callback_error?: string;
  speakers: SonosEventSpeaker[];
}

/**
 * Which of the model-dependent controls a speaker actually answered for.
 * There is no "what can you do" action on Sonos, so the backend works this
 * out by asking and seeing what faults — render only what is true here,
 * because a control that would be refused is worse than one that isn't there.
 */
export interface SonosCapabilities {
  bass: boolean;
  treble: boolean;
  loudness: boolean;
  night_mode: boolean;
  /** Sonos' name for what its app calls speech enhancement. */
  dialog_level: boolean;
  sub: boolean;
  surround: boolean;
}

/** Read-only identity block — serial, firmware, MAC. Support detail only. */
export interface SonosZoneInfo {
  serial_number?: string;
  software_version?: string;
  hardware_version?: string;
  mac_address?: string;
  /** The speaker's own room name, which can differ from HomeHub's label for it. */
  zone_name?: string;
}

/**
 * One speaker's settings snapshot. Every adjustable field is optional:
 * absent means "this model doesn't have it", which is a different statement
 * from a zero value — check `capabilities` rather than truthiness.
 */
export interface SonosSettings {
  capabilities: SonosCapabilities;
  bass?: number;      // -10…10
  treble?: number;    // -10…10
  loudness?: boolean;
  night_mode?: boolean;
  dialog_level?: boolean;
  sub_enabled?: boolean;
  sub_gain?: number;  // -15…15
  surround?: boolean;
  /** The status light on the speaker's face. */
  led?: boolean;
  /** True when the speaker's touch controls are locked. */
  button_lock?: boolean;
  /** Whole minutes left on the group sleep timer; 0 when none is set. */
  sleep_minutes: number;
  info: SonosZoneInfo;
  model_number?: string;
  display_name?: string;
  /** The speaker publishes a picture of itself — otherwise use the placeholder. */
  has_image: boolean;
}

/**
 * The writable half of a settings snapshot. Send one field per interaction:
 * the backend applies a patch in a fixed order and stops at the first
 * refusal, so a single field keeps "what did the speaker refuse" unambiguous.
 */
export interface SonosSettingsPatch {
  bass?: number;
  treble?: number;
  loudness?: boolean;
  night_mode?: boolean;
  dialog_level?: boolean;
  sub_enabled?: boolean;
  sub_gain?: number;
  surround?: boolean;
  led?: boolean;
  button_lock?: boolean;
  /** Group-scoped — send to a coordinator. 0 cancels a running timer. */
  sleep_minutes?: number;
}

/** A discovered (not necessarily registered) speaker on the LAN. */
export interface SonosCandidate {
  ip: string;
  uuid: string;
  room: string;
  model: string;
  registered: boolean;
}

/** One "My Sonos" favorite. uri/metadata round-trip to the play endpoint untouched. */
export interface SonosFavorite {
  id: string;
  title: string;
  art_uri?: string;
  uri: string;
  metadata?: string;
  service?: string;
  /** Set when this favorite is a Spotify playlist or album — a list with
   *  songs inside it, worth browsing rather than only playing outright.
   *  Absent for a single track or a favorite from another service. */
  spotify_uri?: string;
}
