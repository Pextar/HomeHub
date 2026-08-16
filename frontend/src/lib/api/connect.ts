/**
 * Spotify Connect: the account's single playback session — where
 * it can go, where it is, and moving it.
 *
 * One slice of the flat `api` object — lib/api.ts spreads these together,
 * so every call site's `api.foo()` is unchanged. Split by domain because a
 * thousand-line object is not a thing anyone reads.
 */

import { req, json } from "./http";
import type {
  SpotifyConnectView,
} from "../types";

export const connectApi = {
  // Spotify Connect: the account's single playback session — where it can
  // go, where it is, and moving it. A remote control rather than a speaker
  // bridge: these devices are Spotify's, and HomeHub only asks the cloud to
  // move the session between them.
  spotifyConnect() { return req<SpotifyConnectView>("/spotify/connect"); },
  // Moving the session stops it wherever it was. When that is a room HomeHub
  // is decoding for, the read above says so in `interrupts` — say it before
  // the tap.
  spotifyConnectTransfer(deviceID: string, play = true) {
    return req<void>("/spotify/connect/transfer", {
      method: "PUT", body: json({ device_id: deviceID, play }),
    });
  },
  // Only meaningful for a device that has a volume of its own; the read
  // reports -1 for the ones that don't.
  spotifyConnectVolume(deviceID: string, level: number) {
    return req<void>("/spotify/connect/volume", {
      method: "PUT", body: json({ device_id: deviceID, level }),
    });
  },
};
