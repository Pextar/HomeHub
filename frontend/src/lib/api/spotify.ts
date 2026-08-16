/**
 * The catalog, searched and browsed through the account's own
 * PKCE grant.
 *
 * One slice of the flat `api` object — lib/api.ts spreads these together,
 * so every call site's `api.foo()` is unchanged. Split by domain because a
 * thousand-line object is not a thing anyone reads.
 */

import { req, json } from "./http";
import type {
  SpotifyArtistDetail,
  SpotifyContextDetail,
  SpotifyItem,
  SpotifyLibrary,
  SpotifyListening,
  SpotifyResults,
  SpotifyStatus,
} from "../types";

export const spotifyApi = {
  // Spotify search/browse (user's own account via PKCE — configured in the Music view)
  spotifyStatus() { return req<SpotifyStatus>("/spotify/status"); },
  spotifySetConfig(clientId: string) {
    return req<void>("/spotify/config", { method: "PUT", body: json({ client_id: clientId }) });
  },
  spotifyLoginURL() { return req<{ url: string }>("/spotify/login"); },
  // Manual-flow finish: pass the full address the browser landed on after consent.
  spotifyExchange(url: string) {
    return req<void>("/spotify/exchange", { method: "POST", body: json({ url }) });
  },
  spotifyDisconnect() { return req<void>("/spotify/disconnect", { method: "POST" }); },
  // `kind` narrows to one of tracks/albums/playlists/artists and `offset`
  // pages into it — what a shelf's "Show more" needs, since Spotify caps a
  // search at ten results per kind. `signal` lets a superseded search be
  // called off rather than left to finish and be discarded.
  spotifySearch(
    q: string,
    limit = 8,
    opts: { kind?: string; offset?: number; signal?: AbortSignal } = {},
  ) {
    const p = new URLSearchParams({ q, limit: String(limit) });
    if (opts.kind) p.set("kind", opts.kind);
    if (opts.offset) p.set("offset", String(opts.offset));
    return req<SpotifyResults>(`/spotify/search?${p}`, { signal: opts.signal });
  },
  spotifyMyPlaylists() { return req<SpotifyItem[]>("/spotify/playlists"); },
  // What the account has been playing, for the idle shelves. 409 means the
  // login predates the listening scopes — a reconnect, not a fault.
  spotifyListening() { return req<SpotifyListening>("/spotify/listening"); },
  // Songs to continue with, seeded from an artist name — the same engine
  // "play similar" uses when a queue runs dry, asked for on purpose. By
  // name because that is what a speaker reports about what it is playing.
  spotifySimilar(artist: string, limit = 8) {
    return req<SpotifyItem[]>(
      `/spotify/similar?artist=${encodeURIComponent(artist)}&limit=${limit}`,
    );
  },
  // An artist's page — top tracks and albums, behind a search result.
  spotifyArtist(uri: string) {
    return req<SpotifyArtistDetail>(`/spotify/artist?uri=${encodeURIComponent(uri)}`);
  },
  // A playlist or album's own tracks — behind a favorite that turns out to
  // be a list rather than one song.
  // Whether a track is in the account's library, and putting it there. The
  // read needs no scope the login didn't already have; the write does, which
  // is why status.library exists — hide the control rather than offer a tap
  // that will be refused.
  spotifySaved(uri: string) {
    return req<{ saved: boolean }>(`/spotify/saved?uri=${encodeURIComponent(uri)}`);
  },
  spotifySetSaved(uri: string, saved: boolean) {
    return req<void>("/spotify/saved", { method: "PUT", body: json({ uri, saved }) });
  },
  spotifyContext(uri: string) {
    return req<SpotifyContextDetail>(`/spotify/context?uri=${encodeURIComponent(uri)}`);
  },
  // The rest of the collection — saved albums, kept playlists, the artists
  // this account actually plays — read as three shelves in one round trip.
  // A shelf whose scope was refused comes back empty rather than failing the
  // request; only all three failing is the account's problem rather than one
  // permission's.
  spotifyLibrary(limit = 20) {
    return req<SpotifyLibrary>(`/spotify/library?limit=${limit}`);
  },
  // What came out lately. The one shelf that is about the catalog rather
  // than about this household, and so the only answer available on an
  // evening when nobody wants to hear anything they already know.
  spotifyNewReleases(limit = 20) {
    return req<SpotifyItem[]>(`/spotify/new-releases?limit=${limit}`);
  },
};
