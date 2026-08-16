/**
 * KEF speakers: local HTTP control. No grouping, queue or
 * favorites — the speaker's API has none; the input selector picks a source.
 *
 * One slice of the flat `api` object — lib/api.ts spreads these together,
 * so every call site's `api.foo()` is unchanged. Split by domain because a
 * thousand-line object is not a thing anyone reads.
 */

import { req, json } from "./http";
import type { PlayItemBody } from "./http";
import type {
  KEFCandidate,
  KEFSettings,
  KEFSettingsPatch,
  KEFSource,
  KEFSpeaker,
  KEFSpotifyView,
  KEFStatus,
} from "../types";

export const kefApi = {
  // KEF speakers (local HTTP control). No grouping, queue or favorites —
  // the speaker's API has none; the input selector is what picks a source.
  kefStatus() { return req<KEFStatus>("/kef/status"); },
  kefDiscover() { return req<KEFCandidate[]>("/kef/discover"); },
  kefCreateSpeaker(body: { ip: string; name?: string; room?: string }) {
    return req<KEFSpeaker>("/kef/speakers", { method: "POST", body: json(body) });
  },
  kefUpdateSpeaker(id: string, body: { ip?: string; name?: string; room?: string }) {
    return req<KEFSpeaker>(`/kef/speakers/${encodeURIComponent(id)}`, { method: "PUT", body: json(body) });
  },
  kefDeleteSpeaker(id: string) {
    return req<void>(`/kef/speakers/${encodeURIComponent(id)}`, { method: "DELETE" });
  },
  kefPlay(id: string) { return req<void>(`/kef/${encodeURIComponent(id)}/play`, { method: "POST" }); },
  kefPause(id: string) { return req<void>(`/kef/${encodeURIComponent(id)}/pause`, { method: "POST" }); },
  kefNext(id: string) { return req<void>(`/kef/${encodeURIComponent(id)}/next`, { method: "POST" }); },
  kefPrevious(id: string) { return req<void>(`/kef/${encodeURIComponent(id)}/previous`, { method: "POST" }); },
  kefSetVolume(id: string, level: number) {
    return req<void>(`/kef/${encodeURIComponent(id)}/volume`, { method: "PUT", body: json({ level }) });
  },
  kefSetMute(id: string, muted: boolean) {
    return req<void>(`/kef/${encodeURIComponent(id)}/mute`, { method: "PUT", body: json({ muted }) });
  },
  // Switching the input is how you pick what plays: there is no queue to
  // point somewhere, so selecting "optic" *is* the "play the TV" action.
  kefSetSource(id: string, source: KEFSource) {
    return req<void>(`/kef/${encodeURIComponent(id)}/source`, { method: "PUT", body: json({ source }) });
  },
  kefSetPower(id: string, on: boolean) {
    return req<void>(`/kef/${encodeURIComponent(id)}/power`, { method: "PUT", body: json({ on }) });
  },
  // Device settings — read on demand, not part of the status poll. Fields
  // the model doesn't have are simply absent from the response.
  kefSettings(id: string) {
    return req<KEFSettings>(`/kef/${encodeURIComponent(id)}/settings`);
  },
  // One field per interaction, so "what did the speaker refuse" stays clear.
  kefUpdateSettings(id: string, patch: KEFSettingsPatch) {
    return req<void>(`/kef/${encodeURIComponent(id)}/settings`, { method: "PUT", body: json(patch) });
  },
  // Starts a Spotify item on a KEF speaker. Same body as sonosPlayItem, a
  // different road underneath: the speaker's own API can't be handed content,
  // so this asks Spotify to point Connect playback at it. The backend wakes
  // the speaker onto Wi-Fi first. A 409 means something the user can fix —
  // reconnect Spotify, or pick which Connect device this speaker is.
  kefPlayItem(id: string, body: PlayItemBody) {
    return req<void>(`/kef/${encodeURIComponent(id)}/play-item`, { method: "POST", body: json(body) });
  },
  // The Connect pairing for one speaker, plus the account's visible devices.
  kefSpotifyDevices(id: string) {
    return req<KEFSpotifyView>(`/kef/${encodeURIComponent(id)}/spotify`);
  },
  // Pin which Connect device a speaker is; an empty id goes back to matching
  // on the speaker's name.
  kefSetSpotifyDevice(id: string, device_id: string, device_name = "") {
    return req<KEFSpeaker>(`/kef/${encodeURIComponent(id)}/spotify`, {
      method: "PUT",
      body: json({ device_id, device_name }),
    });
  },
};
