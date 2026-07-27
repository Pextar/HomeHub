// Spotify search (plays back through the speakers' linked account).

export interface SpotifyStatus {
  configured: boolean; // client ID set
  connected: boolean;  // OAuth completed
  display_name?: string;
  /** Exact redirect URI to register in the Spotify developer dashboard. */
  redirect_uri: string;
  /**
   * True when HomeHub is served over plain HTTP: the redirect URI is a
   * parked loopback address, and the user finishes the login by pasting
   * the address they land on (see api.spotifyExchange). Over HTTPS the
   * callback completes automatically.
   */
  manual: boolean;
  /**
   * Whether this login may start playback (KEF speakers go through Spotify
   * Connect). False on a login granted before HomeHub asked for the player
   * scopes: search still works, playing doesn't, and only reconnecting
   * fixes it — so it is worth saying before the tap rather than after.
   */
  playback: boolean;
}

/** One Spotify Connect endpoint the connected account can play to. */
export interface SpotifyDevice {
  id: string;
  name: string;
  type: string; // Speaker | Computer | Smartphone | …
  active: boolean;
  /** Spotify accepts no commands for these at all (some car/TV integrations). */
  restricted: boolean;
  volume?: number;
}

/** The Connect pairing for one KEF speaker, plus what else is on offer. */
export interface KEFSpotifyView {
  pinned_id?: string;
  pinned_name?: string;
  /** What a play would use right now; absent when nothing matches. */
  device?: SpotifyDevice;
  /** Why `device` is absent, in words worth showing. */
  reason?: string;
  devices: SpotifyDevice[];
}

export interface SpotifyItem {
  kind: "track" | "album" | "playlist" | "artist";
  /** Canonical Spotify URI (spotify:track:…) — what the play endpoint takes. */
  uri: string;
  name: string;
  sub?: string;     // artist / owner line
  art_url?: string; // https CDN image
}

export interface SpotifyResults {
  tracks: SpotifyItem[];
  albums: SpotifyItem[];
  playlists: SpotifyItem[];
  artists: SpotifyItem[];
}

/** An artist's page: enough to browse from without typing. */
export interface SpotifyArtistDetail {
  uri: string;
  name: string;
  art_url?: string;
  top_tracks: SpotifyItem[];
  albums: SpotifyItem[];
}

/** A playlist or album's own track listing — the drill-in behind a favorite
 *  that turns out to be a list rather than one song. */
export interface SpotifyContextDetail {
  kind: "playlist" | "album";
  uri: string;
  name: string;
  sub?: string;
  art_url?: string;
  tracks: SpotifyItem[];
}
