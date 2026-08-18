/**
 * Calling the house from the wall.
 *
 * The panel's own feature more than the app's: "dinner's ready" is shouted
 * from a hallway, not typed on a phone. It goes to every reachable Sonos
 * room at once, and the status read is what says whether there is anywhere
 * to announce to and whether there will be words or only a chime.
 *
 * Split out of the store's closure because it shares nothing with the rest
 * of it beyond the guarded runner and one re-read.
 */

import { api } from "../api";
import type { AnnounceStatus } from "../types";
import type { PanelRunner } from "./types";

export interface PanelAnnounceStore {
  /** Where an announcement would go and whether it would be spoken.
   *  Null while the read is out or when the server has no answer. */
  readonly status: AnnounceStatus | null;
  readonly last: { text: string; rooms: string[]; spoken: boolean; at: number } | null;
  send(text: string, rooms?: string[]): void;
}

export interface PanelAnnounceDeps {
  /** The kid surface: announcing is a household action and the endpoint
   *  is admin-only, so asking would be one guaranteed 403 on every load
   *  of a screen that has no control to draw with the answer. */
  sonosOnly: boolean;
  run: PanelRunner;
  /** Re-read the speakers once the interruption is over. */
  refresh: () => Promise<void>;
}

export function createPanelAnnounce(deps: PanelAnnounceDeps): PanelAnnounceStore {
  let status = $state<AnnounceStatus | null>(null);
  let last = $state<{ text: string; rooms: string[]; spoken: boolean; at: number } | null>(null);

  if (!deps.sonosOnly) {
    void api
      .announceStatus()
      .then((st) => {
        status = st;
      })
      .catch(() => {
        status = null;
      });
  }

  return {
    get status() {
      return status;
    },
    get last() {
      return last;
    },
    send(text, rooms) {
      void deps.run(
        "announce",
        async () => {
          const res = await api.announce(text, rooms);
          last = { text, rooms: res.rooms, spoken: res.spoken, at: Date.now() };
          // Every room has been interrupted and will be put back a
          // few seconds from now; re-read once that has happened so
          // the panel doesn't show the announcement as what's
          // playing.
          setTimeout(() => void deps.refresh(), res.duration_ms + 1500);
        },
        "Couldn't announce that",
      );
    },
  };
}
