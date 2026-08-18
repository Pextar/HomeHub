/**
 * AirPlay receivers. Registration and volume only: a receiver
 * holds nothing to control, so playing to one is a zone operation.
 *
 * One slice of the flat `api` object — lib/api.ts spreads these together,
 * so every call site's `api.foo()` is unchanged. Split by domain because a
 * thousand-line object is not a thing anyone reads.
 */

import { req, json } from "./http";
import type { AirPlayCandidate, AirPlaySpeaker, AirPlaySpeakerView } from "../types";

export const airplayApi = {
  // AirPlay receivers (RoPieee, shairport-sync, Apple TV). Registration and
  // volume only: a receiver holds nothing to control, so playing to one is a
  // zone operation under /media. See internal/airplay.
  airplayStatus() {
    return req<AirPlaySpeakerView[]>("/airplay/status");
  },
  airplayDiscover() {
    return req<AirPlayCandidate[]>("/airplay/discover");
  },
  // The body may carry everything a scan learned, or nothing but an address.
  // A bare address is probed, which proves something answers AirPlay there
  // and no more — the codecs live in the mDNS advertisement a direct
  // connection never sees.
  airplayCreateSpeaker(body: {
    ip: string;
    name?: string;
    room?: string;
    port?: number;
    device_id?: string;
    model?: string;
    pcm?: boolean;
    alac?: boolean;
    needs_encryption?: boolean;
    metadata?: boolean;
  }) {
    return req<AirPlaySpeaker>("/airplay/speakers", { method: "POST", body: json(body) });
  },
  airplayUpdateSpeaker(
    id: string,
    body: { ip?: string; name?: string; room?: string; port?: number },
  ) {
    return req<AirPlaySpeaker>(`/airplay/speakers/${encodeURIComponent(id)}`, {
      method: "PUT",
      body: json(body),
    });
  },
  airplayDeleteSpeaker(id: string) {
    return req<void>(`/airplay/speakers/${encodeURIComponent(id)}`, { method: "DELETE" });
  },
  // Always remembered, and sent as well when a cast is running: a receiver
  // only accepts a level inside a session, and the stored one is what the
  // next cast opens with.
  airplaySetVolume(id: string, level: number) {
    return req<void>(`/airplay/${encodeURIComponent(id)}/volume`, {
      method: "PUT",
      body: json({ level }),
    });
  },
};
