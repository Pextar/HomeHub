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
  /**
   * Whether this login may save a track to the account's library. Reading
   * whether one is saved has always been in the grant, so a heart can show
   * the truth on an older login and simply not offer the tap.
   */
  library?: boolean;
  /**
   * Whether this login may be asked what the account has been playing — the
   * shelves a search box idles on. Same story as `playback`: an older grant
   * searches perfectly well and simply has nothing to say here, so the
   * shelves are absent rather than empty and the offer to reconnect is made
   * once, quietly, where they would have been.
   */
  listening: boolean;
}

/** What this account has been playing, for the idle shelves. Either half
 *  can be empty: a new account has no history, and a refused read costs its
 *  own shelf and nothing else (DESIGN.md §15.9). */
export interface SpotifyListening {
  recent: SpotifyItem[];
  top: SpotifyItem[];
}

/**
 * The rest of the collection: what the account saved, kept and actually
 * listens to. Any one shelf can be empty on its own — a refused read costs
 * that shelf and nothing else (DESIGN.md §15.9), so a login whose grant
 * predates one scope still fills the other two.
 */
export interface SpotifyLibrary {
  albums: SpotifyItem[];
  playlists: SpotifyItem[];
  artists: SpotifyItem[];
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

  // Whatever else the endpoint answered for, so a row can be as informative
  // as Spotify's own. Absent means "the service didn't say" — drop the
  // field rather than fabricating one.
  album?: string;        // tracks: the record it sits on
  duration_ms?: number;  // tracks
  explicit?: boolean;    // tracks
  year?: string;         // albums: release year
  total_tracks?: number; // albums + playlists
  followers?: number;    // artists + playlists
  genres?: string[];     // artists
  popularity?: number;   // artists, 0–100
}

export interface SpotifyResults {
  tracks: SpotifyItem[];
  albums: SpotifyItem[];
  playlists: SpotifyItem[];
  artists: SpotifyItem[];
}

/** An artist's page: the following and genres up top, the most-played
 *  tracks, the discography split the way Spotify splits it, and the artists
 *  their listeners also play. */
export interface SpotifyArtistDetail {
  uri: string;
  name: string;
  art_url?: string;
  genres?: string[];
  followers?: number;
  popularity?: number; // 0–100
  top_tracks: SpotifyItem[];
  albums: SpotifyItem[];
  singles: SpotifyItem[];
  related: SpotifyItem[];
}

/** A playlist or album's own track listing — the drill-in behind a favorite
 *  or a search result that turns out to be a list rather than one song. */
export interface SpotifyContextDetail {
  kind: "playlist" | "album";
  uri: string;
  name: string;
  sub?: string;         // album: artists · playlist: owner
  art_url?: string;
  year?: string;        // album
  followers?: number;   // playlist
  total_tracks?: number;
  description?: string; // playlist
  artist_uri?: string;  // album: first artist, for "More by"
  tracks: SpotifyItem[];
}
