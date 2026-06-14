import type { Group, Scene, Schedule, Socket, Timer, SocketAction, Automation } from "./types";
import { api } from "./api";
import { data, toasts } from "./stores.svelte";

// Count automations that reference an entity, so delete confirmations can warn
// that the cascade will remove or prune them (mirrors the backend cleanup).
export function automationsUsingSocket(autos: Automation[], id: string): number {
  return autos.filter(a => a.rules.some(r =>
    (r.trigger.type === "device" && r.trigger.socket_id === id) ||
    (r.conditions?.some(c => c.type === "device" && c.socket_id === id) ?? false) ||
    r.actions.some(ac => ac.target_type === "socket" && ac.target_id === id),
  )).length;
}
export function automationsUsingSensor(autos: Automation[], id: string): number {
  return autos.filter(a => a.rules.some(r => r.trigger.type === "sensor" && r.trigger.sensor_id === id)).length;
}
export function automationsUsingTarget(autos: Automation[], type: "group" | "scene", id: string): number {
  return autos.filter(a => a.rules.some(r => r.actions.some(ac => ac.target_type === type && ac.target_id === id))).length;
}

// "1 automation" / "3 automations" — for warning copy.
export function plural(n: number, word: string): string {
  return `${n} ${word}${n === 1 ? "" : "s"}`;
}

export const DAY_SHORT = ["S", "M", "T", "W", "T", "F", "S"];
export const DAY_NAMES = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

export const PROTOCOLS: { value: string; label: string }[] = [
  { value: "nexa", label: "Nexa / Proove" },
  { value: "kaku", label: "KlikAanKlikUit (KAKU)" },
  { value: "intertechno", label: "Intertechno" },
  { value: "raw", label: "Raw / custom" },
  { value: "tasmota", label: "Tasmota (Wi-Fi)" },
  { value: "matter", label: "Matter (Wi-Fi)" },
  { value: "matter-thread", label: "Matter (Thread)" },
  { value: "mqtt", label: "MQTT" },
];

// Protocol classification, shared by every view that branches on transport.
// Previously each view kept its own copy of this sniffing and they had
// already drifted (only Console knew about a literal "wifi" protocol).
export type ProtocolKind = "rf" | "wifi" | "matter" | "mqtt";
export function protocolKind(p: string): ProtocolKind {
  if (p === "tasmota" || p === "wifi") return "wifi";
  if (p.startsWith("matter")) return "matter";
  if (p === "mqtt") return "mqtt";
  return "rf";
}

// Smart protocols are driven through a bridge (Tasmota/Matter) and expose
// brightness/colour beyond plain on/off.
export function isSmartProtocol(p: string): boolean {
  return p === "tasmota" || p.startsWith("matter");
}

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
      : { kind: "Socket", label: "(removed device)", sub: "—" };
  }
  if (tt === "group") {
    const g = data.groupById(tid);
    return g
      ? { kind: "Group", label: g.name, sub: `${g.socket_ids.length} socket${g.socket_ids.length === 1 ? "" : "s"}` }
      : { kind: "Group", label: "(removed group)", sub: "—" };
  }
  if (tt === "scene") {
    const sc = data.sceneById(tid);
    return sc
      ? { kind: "Scene", label: sc.name, sub: sc.steps.length > 1 ? `${sc.steps.length} steps` : `${sc.steps[0]?.actions.length ?? 0} action${(sc.steps[0]?.actions.length ?? 0) === 1 ? "" : "s"}` }
      : { kind: "Scene", label: "(removed scene)", sub: "—" };
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

// Format a past Date as a short "time ago" string ("just now", "5m ago",
// "3h ago", "2d ago"). Returns "" for falsy input.
export function formatAgo(when: string | undefined): string {
  if (!when) return "";
  const ms = Date.now() - new Date(when).getTime();
  if (ms < 0) return "just now";
  const s = Math.floor(ms / 1000);
  if (s < 60) return "just now";
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.floor(h / 24);
  return `${d}d ago`;
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

// Toggle/turn a single socket on or off with an optimistic flip. The store
// updates instantly, the API response is merged in when it lands, and a
// failure rolls the state back — all without a full data refresh.
export async function socketAction(socket: Socket, action: SocketAction, successMessage?: string): Promise<boolean> {
  const prev = socket.state;
  const next = action === "toggle" ? !prev : action === "on";
  data.applySocket({ ...socket, state: next });
  try {
    const updated = action === "on"
      ? await api.socketOn(socket.id)
      : action === "off"
        ? await api.socketOff(socket.id)
        : await api.socketToggle(socket.id);
    data.applySocket(updated);
    if (successMessage) toasts.success(successMessage);
    return true;
  } catch (e) {
    data.applySocket({ ...socket, state: prev });
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
