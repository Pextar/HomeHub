/**
 * Turning three bridge readings into the rooms on the wall.
 *
 * A panel shows one room's playback at a time, and this is what decides
 * which rooms there are to choose from. Zones come first — a zone is an
 * arrangement someone made on purpose, so it outranks the household's own
 * Sonos grouping and the speakers inside it stop appearing under their own
 * names — then the ungrouped Sonos groups, then the KEF speakers no zone
 * has claimed. Showing both would put one speaker on two cards under two
 * names.
 *
 * All of it is a pure function of the three readings, so it lives outside
 * the store's closure: the ranking above is a rule worth stating once and
 * testing directly, rather than a hundred and eighty lines buried in a
 * `$derived` that can only be reached by standing up the whole poll.
 */

import { kefSourceLabel } from "../kef";
import { kefCapabilities, sonosCapabilities, zoneCapabilities } from "../music/capabilities";
import { trackLines } from "../music/format";
import type { PanelSource } from "./types";
import type {
  SonosStatus,
  SonosGroupView,
  SonosSpeakerView,
  KEFStatus,
  MediaZone,
  MediaZoneSpeaker,
} from "../types";

/** The three readings the rooms are derived from — null while one is still
 *  out, or for good on a surface whose bridge never polls. */
export interface SourceInput {
  status: SonosStatus | null;
  kef: KEFStatus | null;
  zones: MediaZone[] | null;
}

/**
 * The destination key the media layer files a room's plays and timers
 * under: "sonos:<id>", "kef:<id>", "zone:<id>".
 *
 * Exported because it is the vocabulary shared with the backend — a shelf,
 * a timer and an insights tally all name a room this way — and a second
 * spelling of it in a component is a room that silently stops matching
 * itself.
 */
export function roomKeyOf(s: PanelSource | undefined): string {
  if (!s) return "";
  return (s.kind === "sonos" ? "sonos:" : s.kind === "kef" ? "kef:" : "zone:") + s.id;
}

/** How many speakers this home has registered, answering or not — what
 *  tells "no speakers here" from "nothing answered this minute". */
export function registeredCount({ status, kef, zones }: SourceInput): number {
  return (status?.speakers.length ?? 0) + (kef?.speakers.length ?? 0) + (zones?.length ?? 0);
}

/** Every speaker a zone already claims, so it can't also stand alone. */
function claimedBy(zones: MediaZone[] | null) {
  // Plain sets, rebuilt whole by each call and never mutated after — this
  // file is outside the reactive graph, so nothing here need be a SvelteSet.
  const sonos = new Set<string>();
  const kef = new Set<string>();
  for (const z of zones ?? []) {
    for (const sp of z.speakers) {
      if (sp.missing) continue;
      (sp.vendor === "kef" ? kef : sonos).add(sp.id);
    }
  }
  return { sonos, kef };
}

/** The zone member whose reading stands for the zone — the app's rule
 *  (zones.svelte.ts): the first that is playing something with a name. */
function leadOf(z: MediaZone): MediaZoneSpeaker | undefined {
  const live = z.speakers.filter((sp) => !sp.missing);
  return (
    live.find((sp) => sp.state?.state === "playing" && sp.state.track?.title) ??
    live.find((sp) => sp.state?.track?.title) ??
    live.find((sp) => sp.state) ??
    live[0]
  );
}

export function zoneSource(z: MediaZone): PanelSource {
  const live = z.speakers.filter((sp) => !sp.missing);
  const lead = leadOf(z);
  const withState = live.filter((sp) => sp.state);
  // The mean, because a zone-wide set writes one level to every
  // speaker: showing the loudest would jump the fader when touched.
  const volume = withState.length
    ? Math.round(withState.reduce((n, sp) => n + (sp.state?.volume ?? 0), 0) / withState.length)
    : 0;
  const at = lead?.state?.at ? Date.parse(lead.state.at) : NaN;
  const zoneLines = trackLines(lead?.state?.track);
  return {
    key: "z:" + z.id,
    kind: "zone",
    id: z.id,
    title: z.name,
    playing: live.some((sp) => sp.state?.state === "playing"),
    standby: false,
    volume,
    // Audible if any one speaker is: muting all is what the zone
    // mute does, so anything less reads as unmuted.
    muted: withState.length > 0 && withState.every((sp) => sp.state?.muted),
    canSkip: zoneCapabilities(z.route).canSkip,
    trackTitle: zoneLines.title || undefined,
    trackSub: zoneLines.sub,
    trackArtist: lead?.state?.track?.artist,
    art: lead?.state?.track?.art_uri,
    members:
      live.length > 1
        ? live.map((sp) => ({
            id: sp.id,
            name: sp.name,
            vendor: sp.vendor,
            volume: sp.state?.volume ?? 0,
            muted: !!sp.state?.muted,
            coordinator: sp.id === lead?.id,
          }))
        : undefined,
    positionMs: lead?.state?.position_ms,
    durationMs: lead?.state?.duration_ms,
    readAt: Number.isNaN(at) ? undefined : at,
    zone: z,
    route: z.route,
  };
}

function coordinatorOf(
  g: SonosGroupView,
  byId: Map<string, SonosSpeakerView>,
): SonosSpeakerView | undefined {
  return byId.get(g.coordinator_id) ?? byId.get(g.member_ids[0]);
}

function groupTitle(g: SonosGroupView, byId: Map<string, SonosSpeakerView>): string {
  const names = g.member_ids.map((id) => byId.get(id)?.name).filter((n): n is string => !!n);
  if (names.length <= 2) return names.join(" + ");
  return `${names[0]} + ${names.length - 1} more`;
}

/** Every room the wall may feature, zones first, then ungrouped Sonos
 *  groups, then the KEF speakers no zone has claimed. */
export function buildSources({ status, kef, zones }: SourceInput): PanelSource[] {
  const claimed = claimedBy(zones);
  const byId = new Map((status?.speakers ?? []).map((s) => [s.id, s]));
  const out: PanelSource[] = [];
  for (const z of zones ?? []) {
    if (z.speakers.some((sp) => !sp.missing)) out.push(zoneSource(z));
  }
  for (const g of status?.groups ?? []) {
    if (g.member_ids.some((id) => claimed.sonos.has(id))) continue;
    const c = coordinatorOf(g, byId);
    if (!c?.reachable) continue;
    const st = c.state;
    const members = g.member_ids
      .map((id) => byId.get(id))
      .filter((x): x is SonosSpeakerView => !!x);
    // Group volume isn't reported by the status poll — only each
    // member's own — so the fader's value is the members' average,
    // same as the full Music view's Sonos bridge (sonos.svelte.ts).
    // Reading the coordinator's own volume here would desync the
    // fader the moment a group's members sit at different levels.
    const memberVols = members
      .map((x) => x.state?.volume)
      .filter((v): v is number => v !== undefined);
    const groupVolume = memberVols.length
      ? Math.round(memberVols.reduce((a, b) => a + b, 0) / memberVols.length)
      : (st?.volume ?? 0);
    const lines = trackLines(st?.track);
    out.push({
      key: "s:" + g.coordinator_id,
      kind: "sonos",
      id: g.coordinator_id,
      title: groupTitle(g, byId),
      playing: !!st?.playing,
      standby: false,
      volume: groupVolume,
      // A group is muted only when every speaker in it is: one
      // audible speaker means the room is audible. Reading the
      // coordinator's own flag here made the icon disagree with
      // what the button then did.
      muted: members.length > 0 && members.every((x) => !!x.state?.muted),
      canSkip: sonosCapabilities().canSkip,
      // Radio names itself in its own fields, so the two lines
      // are composed once for every make (`trackLines`) rather
      // than assembled from artist and album here.
      trackTitle: lines.title || undefined,
      trackSub: lines.sub,
      trackArtist: st?.track?.artist,
      trackURI: st?.track?.spotify_uri,
      art: st?.track?.art_uri,
      members: members.map((x) => ({
        id: x.id,
        name: x.name,
        vendor: "sonos" as const,
        volume: x.state?.volume ?? 0,
        muted: !!x.state?.muted,
        coordinator: x.id === g.coordinator_id,
      })),
      groupState: c.group_state,
      autoplay: c.autoplay,
      queueTrack: st?.queue_track,
      position: st?.position,
      duration: st?.duration,
      // The coordinator's, because its state is the group's.
      readAt: c.read_at,
    });
  }
  for (const sp of kef?.speakers ?? []) {
    if (!sp.reachable || claimed.kef.has(sp.id)) continue;
    const st = sp.state;
    const kefLines = trackLines(st?.track);
    out.push({
      key: "k:" + sp.id,
      kind: "kef",
      id: sp.id,
      title: sp.name,
      playing: !!st?.playing,
      standby: st ? !st.powered_on : false,
      volume: st?.volume ?? 0,
      muted: !!st?.muted,
      canSkip: kefCapabilities(st).canSkip,
      trackTitle:
        kefLines.title ||
        (st?.playing && st.source ? `${kefSourceLabel(st.source)} input` : undefined),
      trackSub: kefLines.sub,
      trackArtist: st?.track?.artist,
      art: st?.track?.art_uri,
      input: st?.source,
      positionMs: st?.position_ms,
      durationMs: st?.duration_ms,
      readAt: sp.read_at,
    });
  }
  return out;
  return out;
}
