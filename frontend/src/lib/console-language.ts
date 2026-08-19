/**
 * How the console understands a name.
 *
 * The console lets you type "turn off the kitchen lamp" and have it work, and
 * everything that makes that feel forgiving rather than fussy is in here: the
 * order names are searched in, how loosely they match, and what the Tab key
 * offers. It was ~90 lines inside a view component, which meant the rules
 * below — the ones a person actually notices being right or wrong — had no
 * test and no name.
 *
 * The whole thing is pure. It reads the house as arguments and answers with
 * what it found; nothing here calls the API or touches the store, so the
 * effectful half (applying an action, printing to the tail) stays in the
 * view where it belongs.
 *
 * ## The resolution order, and why
 *
 * One name pool for devices, groups and rooms, searched **exact first, then
 * substring, each in device → group → room order**. That order is the whole
 * design: you shouldn't have to remember whether "Kitchen" is a room or a
 * group to turn it off, and exact-before-fuzzy means naming a thing exactly
 * always beats another thing that merely contains those letters. A device
 * called "Kitchen" wins over a room called "Kitchen Extension"; without the
 * two passes, whichever came first in the list would.
 *
 * Rooms match **in both directions** — "living room" finds "Living", and
 * "living" finds "Living Room" — because a room's real name and what people
 * call it are rarely the same string.
 */

import type { Group, Socket } from "./types";

/** The parts of the house a command can name. */
export interface Vocabulary {
  sockets: Socket[];
  groups: Group[];
  scenes: { name: string }[];
}

export type Action = "on" | "off" | "toggle";

export type Target =
  | { kind: "device"; socket: Socket }
  | { kind: "group"; group: Group }
  | { kind: "room"; name: string };

/** A device's stable console name: `room/device`, lowercased and hyphenated.
 *  It is what the log prints and a name you can type back in. */
export function hostOf(s: Socket): string {
  const ns = (s.room?.trim() || "unassigned").toLowerCase().replace(/\s+/g, "-");
  const name = s.name.toLowerCase().replace(/\s+/g, "-");
  return `${ns}/${name}`;
}

/** Every room a socket claims to be in, de-duplicated, in first-seen order. */
export function roomNames(sockets: Socket[]): string[] {
  return [...new Set(sockets.map((s) => s.room?.trim()).filter(Boolean) as string[])];
}

/** "the kitchen" and "my kitchen" mean the kitchen. */
function normalize(raw: string): string {
  return raw.trim().toLowerCase().replace(/^(the|my)\s+/, "");
}

/**
 * Resolve free text to something a command can act on. See the note at the
 * top for why the order is what it is.
 */
export function resolveTarget(v: Vocabulary, raw: string): Target | undefined {
  const q = normalize(raw);
  if (!q) return undefined;
  const rooms = roomNames(v.sockets);

  const sEx =
    v.sockets.find((s) => s.name.toLowerCase() === q) ?? v.sockets.find((s) => hostOf(s) === q);
  if (sEx) return { kind: "device", socket: sEx };
  const gEx = v.groups.find((g) => g.name.toLowerCase() === q);
  if (gEx) return { kind: "group", group: gEx };
  const rEx = rooms.find((r) => r.toLowerCase() === q);
  if (rEx) return { kind: "room", name: rEx };

  const sIn = v.sockets.find((s) => s.name.toLowerCase().includes(q));
  if (sIn) return { kind: "device", socket: sIn };
  const gIn = v.groups.find((g) => g.name.toLowerCase().includes(q));
  if (gIn) return { kind: "group", group: gIn };
  const rIn = rooms.find((r) => r.toLowerCase().includes(q));
  if (rIn) return { kind: "room", name: rIn };

  return undefined;
}

/** A room by name, matched leniently in both directions. */
export function resolveRoom(sockets: Socket[], raw: string): string | undefined {
  const q = raw.trim().toLowerCase();
  if (!q) return undefined;
  const rooms = roomNames(sockets);
  return (
    rooms.find((r) => r.toLowerCase() === q) ??
    rooms.find((r) => r.toLowerCase().includes(q) || q.includes(r.toLowerCase()))
  );
}

export function resolveGroup(groups: Group[], raw: string): Group | undefined {
  const q = raw.trim().toLowerCase();
  if (!q) return undefined;
  return (
    groups.find((g) => g.name.toLowerCase() === q) ??
    groups.find((g) => g.name.toLowerCase().includes(q))
  );
}

/** A device by name, but only among the ones in `room`. */
export function deviceInRoom(
  sockets: Socket[],
  subject: string,
  room: string,
): Socket | undefined {
  const n = subject.trim().toLowerCase();
  if (!n) return undefined;
  const inRoom = sockets.filter((s) => (s.room?.trim().toLowerCase() ?? "") === room.toLowerCase());
  return inRoom.find((s) => s.name.toLowerCase() === n) ?? inRoom.find((s) => s.name.toLowerCase().includes(n));
}

// ── Tab completion ───────────────────────────────────────────────────

/** What Tab offers on an empty line: the verbs, before any name. */
export const VERBS = [
  "turn on ", "turn off ", "toggle ", "on ", "off ", "set ", "scene ",
  "all off", "all on", "status", "list", "rooms", "groups", "scenes", "help", "clear",
];

/** A line already carrying a verb — everything after it is being named. */
const VERB_HEAD =
  /^(\s*(?:(?:turn|switch)\s+(?:on|off)|on|off|toggle|set|dim|brighten|scene|activate|room|group)\s+)(.*)$/i;

/** Every name in the house, in the order Tab offers them. */
export function vocabulary(v: Vocabulary): string[] {
  return [
    ...v.sockets.map((s) => s.name),
    ...v.groups.map((g) => g.name),
    ...roomNames(v.sockets),
    ...v.scenes.map((s) => s.name),
  ];
}

export interface Completion {
  /** The part of the line to keep — the verb already typed. */
  head: string;
  /** What could follow it, in offer order. Empty when nothing matches. */
  matches: string[];
}

/**
 * What Tab has to offer for `line`.
 *
 * Once a verb has been typed only names are offered, because no second verb
 * can follow one; before that the verbs come first, since an empty line is
 * far more often the start of a command than of a device's name.
 *
 * A fragment that already *is* a whole name offers nothing — completing it to
 * itself would look like Tab doing nothing, and cycling past it to a longer
 * name would take away what was typed.
 */
export function complete(v: Vocabulary, line: string): Completion {
  const m = line.match(VERB_HEAD);
  const head = m ? m[1] : "";
  const frag = (m ? m[2] : line).replace(/^\s+/, "").toLowerCase();
  const names = vocabulary(v);
  const pool = m ? names : [...VERBS, ...names];
  return {
    head,
    matches: pool.filter((x) => x.toLowerCase().startsWith(frag) && x.toLowerCase() !== frag),
  };
}

// ── Understanding a sentence ─────────────────────────────────────────

/** Words meaning "the whole scope" — what drives "all"/whole-room commands. */
export const WHOLE = new Set([
  "everything", "all", "all lights", "lights", "light", "them", "all of them", "everything else",
]);

/**
 * Strip filler so a sentence parses like a command: "turn off the lamp"
 * becomes "turn off lamp". Words that carry meaning stay — "in" separates a
 * device from its room, and "all" is a scope — so only the ones that never
 * change what a line does are dropped.
 *
 * Punctuation is taken off the end **twice**, and that is the fix rather than
 * a belt-and-braces: dropping a trailing filler word can expose punctuation
 * that was in the middle of the line when the first pass ran. "Turn off the
 * lamp, please." used to come out as `turn off lamp,` — a name with a comma
 * welded on, which matches no device.
 */
export function norm(s: string): string {
  return s
    .toLowerCase()
    .replace(/[.!?,]+$/g, "")
    .replace(/\b(the|a|an|please|just|to|of)\b/g, " ")
    .replace(/\s+/g, " ")
    .replace(/[\s.!?,]+$/g, "")
    .trim();
}

/**
 * Pull the verb out of a token list, wherever it sits.
 *
 * People write both ways round — "turn off the lamp" and "the lamp off" — and
 * a console that only took one of them would be a console you have to
 * remember the grammar of. So the verb is looked for at the front and at the
 * back, and what is left over is the thing being named.
 *
 * "turn"/"switch" are the one case needing care: they are not verbs on their
 * own, so the on/off after them is the action and a bare "turn lamp" means
 * nothing rather than defaulting to something.
 */
export function extractAction(tokens: string[]): { action: Action; rest: string[] } | null {
  const lead = tokens[0];
  const last = tokens[tokens.length - 1];
  if (lead === "turn" || lead === "switch") {
    if (tokens[1] === "on" || tokens[1] === "off") {
      return { action: tokens[1] as Action, rest: tokens.slice(2) };
    }
    if (last === "on" || last === "off") {
      return { action: last as Action, rest: tokens.slice(1, -1) };
    }
    return null;
  }
  if (lead === "on" || lead === "off" || lead === "toggle") {
    return { action: lead as Action, rest: tokens.slice(1) };
  }
  if (last === "on" || last === "off" || last === "toggle") {
    return { action: last as Action, rest: tokens.slice(0, -1) };
  }
  return null;
}
