import { api } from "./api";

/**
 * The rooms a scene or an automation can aim music at.
 *
 * One list, built from the two things the house actually has: the speakers
 * the bridges report and the zones somebody arranged. The key is the media
 * layer's own — "sonos:<id>", "kef:<id>", "zone:<id>" — which is the same
 * vocabulary the music timers, the play shelves and the listening tallies
 * use. A second spelling anywhere would be a room that silently stops
 * matching itself.
 *
 * Unlike the Music module's room model (lib/music/rooms.svelte.ts) this does
 * *not* hide a speaker a zone has claimed. That rule exists so one sound is
 * listed once on a surface you drive; here you are writing an instruction,
 * and "quiet the kitchen speaker" and "quiet the whole downstairs zone" are
 * both things somebody may reasonably mean.
 */
export interface MediaRoomOption {
  key: string;
  name: string;
  kind: "sonos" | "kef" | "airplay" | "zone";
  /** For a zone: how many speakers it holds, so two similar names differ. */
  members?: number;
}

/**
 * Loads the list. Zones first — an arrangement someone made on purpose is
 * the likelier target — then the speakers by name.
 *
 * Either half failing costs only its own rows: a house with no zones and a
 * house whose zone read failed should not both come back empty.
 */
export async function loadMediaRooms(): Promise<MediaRoomOption[]> {
  const [epRes, zoneRes] = await Promise.allSettled([api.mediaEndpoints(), api.mediaZones()]);

  const zones: MediaRoomOption[] =
    zoneRes.status === "fulfilled"
      ? zoneRes.value.map((z) => ({
          key: `zone:${z.id}`,
          name: z.name,
          kind: "zone" as const,
          members: z.speakers?.length ?? z.members?.length ?? 0,
        }))
      : [];

  const speakers: MediaRoomOption[] =
    epRes.status === "fulfilled"
      ? epRes.value.map((e) => ({
          key: e.member,
          name: e.name,
          // The endpoint's own vendor, not a guess from the id: filing an
          // AirPlay receiver as Sonos would put the wrong word next to a
          // room a user is about to write an instruction for.
          kind: e.vendor,
        }))
      : [];

  zones.sort((a, b) => a.name.localeCompare(b.name));
  speakers.sort((a, b) => a.name.localeCompare(b.name));
  return [...zones, ...speakers];
}
