import type {
  Socket,
  Schedule,
  Group,
  Scene,
  Timer,
  RoomSummary,
  BulkResult,
  TargetType,
  SocketAction,
} from "./types";

const BASE = "/api";

class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

async function req<T>(path: string, opts: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = { ...((opts.headers as Record<string, string>) ?? {}) };
  if (opts.body && !headers["Content-Type"]) headers["Content-Type"] = "application/json";

  const res = await fetch(BASE + path, { ...opts, headers });
  if (res.status === 204) return undefined as T;

  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) {
    const msg = (data && (data as { error?: string }).error) || res.statusText || "Request failed";
    throw new ApiError(msg, res.status);
  }
  return data as T;
}

const json = (body: unknown) => JSON.stringify(body);

export const api = {
  health() {
    return req<{ status: string; sockets: number; schedules: number; groups: number; scenes: number; timers: number; time: string }>("/health");
  },

  // Sockets
  listSockets() { return req<Socket[]>("/sockets"); },
  createSocket(body: Partial<Socket>) { return req<Socket>("/sockets", { method: "POST", body: json(body) }); },
  updateSocket(id: string, body: Partial<Socket>) { return req<Socket>(`/sockets/${encodeURIComponent(id)}`, { method: "PUT", body: json(body) }); },
  deleteSocket(id: string) { return req<void>(`/sockets/${encodeURIComponent(id)}`, { method: "DELETE" }); },
  socketOn(id: string) { return req<Socket>(`/sockets/${encodeURIComponent(id)}/on`, { method: "POST" }); },
  socketOff(id: string) { return req<Socket>(`/sockets/${encodeURIComponent(id)}/off`, { method: "POST" }); },
  socketToggle(id: string) { return req<Socket>(`/sockets/${encodeURIComponent(id)}/toggle`, { method: "POST" }); },
  socketTimer(id: string, body: { action: SocketAction; in_seconds: number; note?: string }) {
    return req<Timer>(`/sockets/${encodeURIComponent(id)}/timer`, { method: "POST", body: json(body) });
  },
  allOn() { return req<BulkResult>("/sockets/all/on", { method: "POST" }); },
  allOff() { return req<BulkResult>("/sockets/all/off", { method: "POST" }); },

  // Rooms
  listRooms() { return req<RoomSummary[]>("/rooms"); },
  roomOn(room: string) { return req<BulkResult>(`/rooms/${encodeURIComponent(room)}/on`, { method: "POST" }); },
  roomOff(room: string) { return req<BulkResult>(`/rooms/${encodeURIComponent(room)}/off`, { method: "POST" }); },

  // Schedules
  listSchedules() { return req<Schedule[]>("/schedules"); },
  createSchedule(body: Partial<Schedule>) { return req<Schedule>("/schedules", { method: "POST", body: json(body) }); },
  updateSchedule(id: string, body: Partial<Schedule>) { return req<Schedule>(`/schedules/${encodeURIComponent(id)}`, { method: "PUT", body: json(body) }); },
  deleteSchedule(id: string) { return req<void>(`/schedules/${encodeURIComponent(id)}`, { method: "DELETE" }); },

  // Groups
  listGroups() { return req<Group[]>("/groups"); },
  createGroup(body: Partial<Group>) { return req<Group>("/groups", { method: "POST", body: json(body) }); },
  updateGroup(id: string, body: Partial<Group>) { return req<Group>(`/groups/${encodeURIComponent(id)}`, { method: "PUT", body: json(body) }); },
  deleteGroup(id: string) { return req<void>(`/groups/${encodeURIComponent(id)}`, { method: "DELETE" }); },
  groupAction(id: string, action: SocketAction) {
    return req<{ group: string; updated: number; failures: unknown[] }>(`/groups/${encodeURIComponent(id)}/${action}`, { method: "POST" });
  },

  // Scenes
  listScenes() { return req<Scene[]>("/scenes"); },
  createScene(body: Partial<Scene>) { return req<Scene>("/scenes", { method: "POST", body: json(body) }); },
  updateScene(id: string, body: Partial<Scene>) { return req<Scene>(`/scenes/${encodeURIComponent(id)}`, { method: "PUT", body: json(body) }); },
  deleteScene(id: string) { return req<void>(`/scenes/${encodeURIComponent(id)}`, { method: "DELETE" }); },
  activateScene(id: string) {
    return req<{ scene: string; updated: number; failures: unknown[] }>(`/scenes/${encodeURIComponent(id)}/activate`, { method: "POST" });
  },

  // Timers
  listTimers() { return req<Timer[]>("/timers"); },
  createTimer(body: { target_type: TargetType; target_id: string; action: string; in_seconds?: number; fires_at?: string; note?: string }) {
    return req<Timer>("/timers", { method: "POST", body: json(body) });
  },
  deleteTimer(id: string) { return req<void>(`/timers/${encodeURIComponent(id)}`, { method: "DELETE" }); },
};

export type Api = typeof api;
export { ApiError };
