/**
 * Where the queue is cut.
 *
 * A queue pane opened on track one is an answer to a question nobody asked:
 * a room forty tracks into a playlist wants to know what is *next*. So every
 * surface that shows a queue splits it at the track playing — what just
 * played kept in view above it, the rest of the history folded, and what's
 * next under it.
 *
 * The history is a first-class part of the pane, not a leftover: "what was
 * that song?" is asked of the queue as often as "what's coming", and it is
 * asked about the last few tracks. So the last couple stay on screen and the
 * fold names how many more there are, rather than the pane starting flatly at
 * the track playing.
 *
 * Pure on purpose, and here rather than inlined in the pane: three surfaces
 * render the same list (the player's second pane, the panel's full player,
 * the panel's browse depth) and the arithmetic is worth having under test.
 */
import type { SonosQueueItem } from "../types";

export interface QueueSplit {
  /** Index of the playing track in `items`; -1 when it isn't in the list. */
  currentIdx: number;
  /** Tracks before the one playing — the fold's contents. */
  earlier: SonosQueueItem[];
  /** The playing track and everything after it, in queue order. */
  ahead: SonosQueueItem[];
  /** How many tracks are still to come, not counting the one playing. */
  upNext: number;
}

/**
 * Split `items` at `currentTrack`.
 *
 * With nothing playing out of the queue — radio, line-in, a coordinator that
 * reports no queue position, or a position past the window the backend
 * fetched — there is no cut to make: the whole list is `ahead`, `earlier` is
 * empty, and `upNext` is 0 because "next" has no anchor to be next to.
 */
export function splitQueue(items: SonosQueueItem[], currentTrack?: number): QueueSplit {
  const currentIdx = currentTrack ? items.findIndex((i) => i.track === currentTrack) : -1;
  if (currentIdx < 0) {
    return { currentIdx: -1, earlier: [], ahead: items, upNext: 0 };
  }
  return {
    currentIdx,
    earlier: items.slice(0, currentIdx),
    ahead: items.slice(currentIdx),
    upNext: items.length - currentIdx - 1,
  };
}

/**
 * How many played tracks stay on screen above the playing one.
 *
 * Not zero: "what was that one I liked?" is asked about the song that just
 * finished more often than about anything else in the queue, and an answer
 * that needs a tap to reach is an answer you have to know is there. Two rows
 * carry the last few minutes of listening without pushing the playing track
 * out of the first screenful.
 */
export const QUEUE_PEEK = 2;

/**
 * Split the history into the part that stays visible and the part behind the
 * fold. A fold that hides one or two rows costs a row to save a row, so it
 * doesn't happen: a short history is simply all there.
 */
export function foldEarlier(
  earlier: SonosQueueItem[],
  peek: number = QUEUE_PEEK,
): { hidden: SonosQueueItem[]; shown: SonosQueueItem[] } {
  if (earlier.length <= peek + 1) return { hidden: [], shown: earlier };
  return { hidden: earlier.slice(0, earlier.length - peek), shown: earlier.slice(-peek) };
}

/**
 * How many tracks are left after the one playing, from the *group's* numbers
 * rather than the fetched window: a queue can run past what one browse
 * returns (`sonos.MaxQueueFetch`), and the player's "Up next" row is read at
 * a glance, so it counts the real remainder. No position reported means
 * nothing has been played out of it yet — the whole queue is still ahead.
 */
export function tracksAhead(queueLength: number, currentTrack?: number): number {
  if (!currentTrack) return queueLength;
  return Math.max(0, queueLength - currentTrack);
}
