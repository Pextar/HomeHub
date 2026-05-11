import type { Group, Scene, Schedule, Socket, Timer } from "./types";
import { data, toasts } from "./stores.svelte";

export const DAY_SHORT = ["S", "M", "T", "W", "T", "F", "S"];
export const DAY_NAMES = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

export const PROTOCOLS: { value: string; label: string }[] = [
  { value: "nexa", label: "Nexa / Proove" },
  { value: "kaku", label: "KlikAanKlikUit (KAKU)" },
  { value: "intertechno", label: "Intertechno" },
  { value: "raw", label: "Raw / custom" },
];

export function formatDays(days: number[] | undefined): string {
  if (!days || days.length === 0 || days.length === 7) return "Every day";
  const sorted = [...days].sort((a, b) => a - b);
  const isWeekdays = sorted.length === 5 && sorted.every((d, i) => d === i + 1);
  if (isWeekdays) return "Weekdays";
  const isWeekends = sorted.length === 2 && sorted[0] === 0 && sorted[1] === 6;
  if (isWeekends) return "Weekends";
  return sorted.map(d => DAY_NAMES[d]).join(", ");
}

export interface TargetDescription {
  kind: "Socket" | "Group" | "Scene" | "?";
  label: string;
  sub: string;
}

export function describeTarget(
  targetType: string | undefined,
  targetId: string | undefined,
  fallbackSocketId?: string,
): TargetDescription {
  const tt = targetType || (fallbackSocketId ? "socket" : "");
  const tid = targetId || fallbackSocketId;
  if (!tid) return { kind: "?", label: "Unknown target", sub: "" };

  if (tt === "socket") {
    const s = data.socketById(tid);
    return s
      ? { kind: "Socket", label: s.name, sub: s.room || "Unassigned" }
      : { kind: "Socket", label: `(missing socket: ${tid})`, sub: "—" };
  }
  if (tt === "group") {
    const g = data.groupById(tid);
    return g
      ? { kind: "Group", label: g.name, sub: `${g.socket_ids.length} socket${g.socket_ids.length === 1 ? "" : "s"}` }
      : { kind: "Group", label: `(missing group: ${tid})`, sub: "—" };
  }
  if (tt === "scene") {
    const sc = data.sceneById(tid);
    return sc
      ? { kind: "Scene", label: sc.name, sub: `${sc.actions.length} action${sc.actions.length === 1 ? "" : "s"}` }
      : { kind: "Scene", label: `(missing scene: ${tid})`, sub: "—" };
  }
  return { kind: "?", label: "Unknown target", sub: "" };
}

// Format a future Date as a short countdown ("28s", "5m 12s", "1h 23m").
export function formatCountdown(when: Date | string): string {
  const t = typeof when === "string" ? new Date(when) : when;
  const ms = t.getTime() - Date.now();
  if (ms <= 0) return "now";
  const s = Math.floor(ms / 1000);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ${s % 60}s`;
  const h = Math.floor(m / 60);
  return `${h}h ${m % 60}m`;
}

// Wraps an async action, refreshes data, and shows a toast on failure.
export async function runAction(
  fn: () => Promise<unknown>,
  successMessage?: string,
): Promise<boolean> {
  try {
    await fn();
    if (successMessage) toasts.success(successMessage);
    await data.refresh();
    return true;
  } catch (e) {
    toasts.error("Action failed", (e as Error).message);
    return false;
  }
}

// Sort sockets by room, then name.
export function sortedSockets(sockets: Socket[]): Socket[] {
  return [...sockets].sort((a, b) => {
    const ar = (a.room || "").toLowerCase();
    const br = (b.room || "").toLowerCase();
    if (ar !== br) return ar.localeCompare(br);
    return a.name.localeCompare(b.name);
  });
}

export function groupSocketsByRoom(sockets: Socket[]): Map<string, Socket[]> {
  const map = new Map<string, Socket[]>();
  for (const s of sockets) {
    const room = s.room || "Unassigned";
    if (!map.has(room)) map.set(room, []);
    map.get(room)!.push(s);
  }
  return map;
}

export type Entity = Socket | Group | Scene | Schedule | Timer;
