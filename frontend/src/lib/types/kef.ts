// KEF (local HTTP control).
//
// A second speaker bridge beside Sonos, not folded into it: a KEF speaker is
// one standalone stereo pair with an input selector, where Sonos is zones
// that group and share a queue. There is no grouping, no queue and no
// favorites here because the speaker's API has none — see internal/kef.

export interface KEFSpeaker {
  id: string;
  name: string;
  ip: string;
  /** Normalised MAC — the stable device id, since KEF has no RINCON. */
  mac: string;
  room?: string;
  model?: string;
  /**
   * Which Spotify Connect device this speaker is, when the name it registers
   * with Spotify isn't the name here. Optional: normally the names match and
   * the backend resolves it on its own. Starting music on a KEF speaker goes
   * through Connect — its local API can play and pause but has nothing to be
   * handed — so this is the pairing that makes a search result playable.
   */
  spotify_device_id?: string;
  spotify_device_name?: string;
}

/** Physical inputs. Not every model has every one — read `source` to see. */
export type KEFSource = "wifi" | "bluetooth" | "tv" | "optic" | "coaxial" | "analog" | "usb";

/** How long the speaker waits in silence before powering itself down. */
export type KEFStandbyMode = "standby_none" | "standby_20mins" | "standby_60mins";

export interface KEFTrack {
  title?: string;
  artist?: string;
  album?: string;
  /** Usually an absolute service URL; a speaker-relative path is proxied. */
  art_uri?: string;
}

export interface KEFState {
  /** The speaker's own word: playing | paused | stopped. */
  status: string;
  playing: boolean;
  source: string;
  /** False when the speaker is in standby — awake enough to answer, not to play. */
  powered_on: boolean;
  volume: number; // 0-100
  muted: boolean;
  track?: KEFTrack;
  /** Milliseconds. Duration is 0 for live streams and the physical inputs. */
  position_ms?: number;
  duration_ms?: number;
}

/** A registered speaker plus its live state (state omitted when unreachable). */
export interface KEFSpeakerView extends KEFSpeaker {
  reachable: boolean;
  state?: KEFState;
  /** Unix ms the reading was taken — extrapolate position from this, not from now. */
  read_at?: number;
}

export interface KEFStatus {
  speakers: KEFSpeakerView[];
  /**
   * True when this came from the backend's own poller rather than from a
   * read performed to answer this request. KEF speakers have no change
   * notifications to subscribe to, so unlike Sonos there is no "live" claim
   * to make — the backend polls once for everyone and pushes `music` on a
   * real change.
   */
  warm?: boolean;
}

/** A discovered (not necessarily registered) speaker on the LAN. */
export interface KEFCandidate {
  ip: string;
  mac: string;
  name: string;
  model: string;
  registered: boolean;
}

/** Read-only identity block. Support detail only. */
export interface KEFInfo {
  name?: string;
  model?: string;
  mac?: string;
  firmware?: string;
  release?: string;
}

/**
 * One speaker's settings snapshot. Every adjustable field is optional, and
 * absent means "this model doesn't have it" — a different statement from a
 * zero value, so check for `undefined` rather than truthiness. An LSX II has
 * no subwoofer output, so none of the sub fields come back for one.
 *
 * Placement and treble trim are in tenths of a dB, which is how the speaker
 * stores them: -60 is -6.0 dB.
 */
export interface KEFSettings {
  info: KEFInfo;
  bass_extension?: "less" | "standard" | "extra";
  desk_mode?: boolean;
  desk_gain?: number;   // -60…0
  wall_mode?: boolean;
  wall_gain?: number;   // -60…0
  treble?: number;      // -20…20
  phase_correction?: boolean;
  high_pass_mode?: boolean;
  high_pass_freq?: number; // 50…120 Hz
  subwoofer_out?: boolean;
  sub_lp_freq?: number;    // 40…250 Hz
  sub_gain?: number;       // -10…10 dB
  sub_phase?: "phase0" | "phase180";
  standby_mode?: KEFStandbyMode;
  max_volume?: number;  // 0…100
  volume_limit?: boolean;
}

/**
 * The writable half of a settings snapshot. Send one field per interaction:
 * the backend reports the first refusal, so a single field keeps "what did
 * the speaker refuse" unambiguous.
 */
export interface KEFSettingsPatch {
  bass_extension?: "less" | "standard" | "extra";
  desk_mode?: boolean;
  desk_gain?: number;
  wall_mode?: boolean;
  wall_gain?: number;
  treble?: number;
  phase_correction?: boolean;
  high_pass_mode?: boolean;
  high_pass_freq?: number;
  subwoofer_out?: boolean;
  sub_lp_freq?: number;
  sub_gain?: number;
  sub_phase?: "phase0" | "phase180";
  standby_mode?: KEFStandbyMode;
  max_volume?: number;
  volume_limit?: boolean;
}
