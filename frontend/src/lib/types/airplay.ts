// AirPlay receivers — RoPieee boxes, Apple TVs, anything running
// shairport-sync.
//
// A third speaker bridge beside Sonos and KEF, and the shape of it is thinner
// for a reason: those two are players that hold something, so their types
// carry queues, sources and settings. An AirPlay receiver is a *sink*. It has
// an address, a volume, and whatever HomeHub is currently sending it — there
// is no state on the device to read, so there is none to model here.
//
// Everything about playing to one is a zone operation through the media
// types, because pushing audio to receivers is a route, not a per-speaker
// command. See internal/airplay.

export interface AirPlaySpeaker {
  id: string;
  name: string;
  ip: string;
  /** The RTSP port from the receiver's advertisement. 7000 for most. */
  port: number;
  /** The receiver's own identity, normalised — the stable id across DHCP
   *  leases, the job `mac` does for KEF. Empty for a typed-in receiver. */
  device_id?: string;
  room?: string;
  model?: string;
  /** The formats the receiver advertised. Both are bit-exact, so this is
   *  about how the samples are packed, not about quality. */
  pcm: boolean;
  alac: boolean;
  /** A receiver that will not accept cleartext audio. */
  needs_encryption?: boolean;
  /** Whether it has a display worth sending track info to. */
  metadata?: boolean;
  /** The level the household last set, 0-100. Stored because a receiver only
   *  takes a volume inside a session: this is what the next cast opens with. */
  volume?: number;
}

/** A registered receiver, as the status endpoint reports it. */
export interface AirPlaySpeakerView extends AirPlaySpeaker {
  /** Bridge-qualified id ("airplay:abc") — what a zone stores. */
  member: string;
  /** Whether HomeHub is sending to it right now. This is as close to
   *  "reachable" as a receiver gets: asking one whether it is there means
   *  opening a session, which would take it away from whatever else is
   *  playing to it. */
  casting: boolean;
}

/** One receiver found by a scan. */
export interface AirPlayCandidate {
  name: string;
  ip: string;
  port: number;
  id?: string;
  model?: string;
  version?: string;
  audio: { sample_rate: number; bit_depth: number; channels: number };
  needs_password: boolean;
  metadata: boolean;
  registered: boolean;
  /** Whether HomeHub could actually drive it. A scan that lists a
   *  FairPlay-only Apple TV as addable sets up a failure two taps later. */
  supported: boolean;
  /** Why not, in words meant for a person. Set when `supported` is false. */
  problem?: string;
}
