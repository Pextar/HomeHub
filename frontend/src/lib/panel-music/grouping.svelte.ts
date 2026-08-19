/**
 * Playing together, on the wall.
 *
 * Sonos-native only, and deliberately so. Joining is the whole card rather
 * than one speaker — a room that moves takes its partners with it — and a KEF
 * speaker or a HomeHub zone is *absent* from the list rather than refused,
 * since neither joins a Sonos household and a zone is arranged in the Music
 * view. A control that would be refused is worse than one that isn't there
 * (DESIGN.md §15.1).
 *
 * Every call here goes through one endpoint, `sonosGroup`, taking the joins
 * and the leaves together. That is not tidiness: a household handed four
 * `SetAVTransportURI`s in the same instant re-elects its coordinators
 * mid-flight and lands with a speaker or two left out, so the hub has to walk
 * them in order. When the sequencing lived on this side it survived only as
 * long as the page did, and an iPad that slept halfway through "play it
 * everywhere" left the house half grouped.
 *
 * What each call does with the featured room afterwards is the other half of
 * the design: joining keeps the room you were on featured through the
 * reshuffle, and moving follows the sound to where it went.
 */

import { api } from "../api";
import type { PanelGrouping, PanelSource, PanelRunner } from "./types";

export interface PanelGroupingDeps {
  /** The room the wall is pointed at. A getter: it moves under this. */
  featured: () => PanelSource | undefined;
  /** Every room the wall knows about, for working out what could join. */
  sources: () => PanelSource[];
  /** Point the wall at a room — where the music ends up after a regroup. */
  feature: (key: string) => void;
  run: PanelRunner;
}

export function createPanelGrouping(
  deps: PanelGroupingDeps,
): Omit<PanelGrouping, "refresh"> {
  /** What a Sonos room could group with right now. */
  const joinable = $derived.by(() => {
    const f = deps.featured();
    if (!f || f.kind !== "sonos") return [];
    return deps.sources().filter((s) => s.kind === "sonos" && s.key !== f.key);
  });

  const canGroup = $derived.by(() => {
    const f = deps.featured();
    return (
      !!f && f.kind === "sonos" && (joinable.length > 0 || (f.members?.length ?? 0) > 1)
    );
  });

  /** The featured room, but only when it is one this module can act on. */
  function sonosFeatured(): PanelSource | undefined {
    const f = deps.featured();
    return f && f.kind === "sonos" ? f : undefined;
  }

  const memberIds = (s: PanelSource) => (s.members ?? []).map((m) => m.id);

  return {
    get joinable() {
      return joinable;
    },
    get canGroup() {
      return canGroup;
    },

    joinSource(src) {
      const f = sonosFeatured();
      if (!f || src.kind !== "sonos") return;
      const members = memberIds(src);
      if (!members.length) return;
      void deps.run(
        "join:" + src.id,
        () => api.sonosGroup(f.id, { join: members }),
        "Grouping failed",
        // The group stays featured through the reshuffle.
        () => deps.feature(f.key),
      );
    },

    /** Everything at once — one request, walked by the hub in order. */
    joinAll() {
      const f = sonosFeatured();
      if (!f) return;
      const ids = joinable.flatMap(memberIds);
      if (!ids.length) return;
      void deps.run(
        "joinall",
        () => api.sonosGroup(f.id, { join: ids }),
        "Grouping failed",
        () => deps.feature(f.key),
      );
    },

    /**
     * Take the music with you: the featured room's group moves to `dest` and
     * leaves where it was.
     *
     * This is the gesture a wall gets asked for on the way into the kitchen,
     * and it used to cost two — join, then split the room you walked out of.
     * Composed from the same two calls rather than given a bridge of its own:
     * Sonos has no "move", and what a move *is* on a household is this pair.
     *
     * Order is the whole reason it goes as one request. The destination joins
     * *first*, so the queue and the stream are handed over while the old room
     * is still coordinating, and only then does the old room step out. The
     * other way round stops the music in between.
     */
    moveTo(dest) {
      const f = sonosFeatured();
      if (!f || dest.kind !== "sonos" || dest.key === f.key) return;
      const leaving = memberIds(f);
      const arriving = memberIds(dest);
      if (!leaving.length || !arriving.length) return;
      void deps.run(
        "move:" + dest.id,
        () => api.sonosGroup(f.id, { join: arriving, leave: leaving }),
        "Couldn't move the music",
        // Follow the sound: the destination is what the wall should be
        // pointed at once the music is there.
        () => deps.feature(dest.key),
      );
    },

    ungroupFeatured() {
      const f = sonosFeatured();
      if (!f) return;
      const members = (f.members ?? []).filter((m) => !m.coordinator);
      if (!members.length) return;
      void deps.run(
        "ungroup:" + f.id,
        () => api.sonosGroup(f.id, { leave: members.map((m) => m.id) }),
        "Ungrouping failed",
      );
    },

    leaveMember(memberId) {
      if (!sonosFeatured()) return;
      void deps.run("leave:" + memberId, () => api.sonosLeave(memberId), "Ungrouping failed");
    },
  };
}
