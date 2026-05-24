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
  ActivityEntry,
  Sensor,
  SensorReading,
  DiscoveryState,
  Settings,
  TasmotaState,
  TasmotaStateUpdate,
  MatterState,
  MatterStateUpdate,
  User,
  UserCreate,
  UserCreateResponse,
  UserUpdate,
  NotifPrefs,
  PushSubscriptionBody,
  Automation,
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
  let data: unknown = null;
  if (text) {
    try { data = JSON.parse(text); } catch { /* non-JSON body, leave data null */ }
  }
  if (!res.ok) {
    const msg =
      (data && typeof data === "object" && "error" in data && typeof (data as { error: unknown }).error === "string"
        ? (data as { error: string }).error
        : text || res.statusText || "Request failed");
    throw new ApiError(msg, res.status);
  }
  return data as T;
}

const json = (body: unknown) => JSON.stringify(body);

export const api = {
  // Auth
  // Either a login code (limited profiles) or username + password (admins).
  login(body: { code: string } | { username: string; password: string }) {
    return req<{ username: string }>("/login", { method: "POST", body: json(body) });
  },
  logout() {
    return req<{ status: string }>("/logout", { method: "POST" });
  },

  // Current profile (used to decide what UI to show and which sockets are
  // visible). Returns a synthetic admin when server-side auth is disabled.
  me() {
    return req<User>("/me");
  },

  // Profiles (admin only)
  listUsers() { return req<User[]>("/users"); },
  // Creating an admin (manager) user returns invite_url in addition to the
  // normal user fields — copy it before closing the modal.
  createUser(body: UserCreate) { return req<UserCreateResponse>("/users", { method: "POST", body: json(body) }); },
  updateUser(id: string, body: UserUpdate) { return req<User>(`/users/${encodeURIComponent(id)}`, { method: "PUT", body: json(body) }); },
  deleteUser(id: string) { return req<void>(`/users/${encodeURIComponent(id)}`, { method: "DELETE" }); },

  // Invite flow — both endpoints are public (no session required).
  // lookupInvite returns the username for a valid/unexpired token.
  lookupInvite(token: string) { return req<{ username: string }>(`/invite?token=${encodeURIComponent(token)}`); },
  // acceptInvite sets the password and returns a session cookie in the response.
  acceptInvite(token: string, password: string) {
    return req<{ username: string }>("/invite", { method: "POST", body: json({ token, password }) });
  },

  health() {
    return req<{ status: string; sockets: number; schedules: number; groups: number; scenes: number; timers: number; time: string }>("/health");
  },

  // Sockets
  listSockets() { return req<Socket[]>("/sockets"); },
  createSocket(body: Partial<Socket>) { return req<Socket>("/sockets", { method: "POST", body: json(body) }); },
  learnSocket(body: { protocol?: string; code?: string } = {}) {
    return req<{ code: string; protocol: string }>("/sockets/learn", { method: "POST", body: json(body) });
  },
  updateSocket(id: string, body: Partial<Socket>) { return req<Socket>(`/sockets/${encodeURIComponent(id)}`, { method: "PUT", body: json(body) }); },
  deleteSocket(id: string) { return req<void>(`/sockets/${encodeURIComponent(id)}`, { method: "DELETE" }); },
  socketOn(id: string) { return req<Socket>(`/sockets/${encodeURIComponent(id)}/on`, { method: "POST" }); },
  socketOff(id: string) { return req<Socket>(`/sockets/${encodeURIComponent(id)}/off`, { method: "POST" }); },
  socketToggle(id: string) { return req<Socket>(`/sockets/${encodeURIComponent(id)}/toggle`, { method: "POST" }); },
  socketToggleFavorite(id: string) { return req<Socket>(`/sockets/${encodeURIComponent(id)}/favorite`, { method: "POST" }); },
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
  setAllSchedules(enabled: boolean) {
    return req<{ enabled: boolean; changed: number }>(`/schedules/all/${enabled ? "enable" : "disable"}`, { method: "POST" });
  },
  updateSchedule(id: string, body: Partial<Schedule>) { return req<Schedule>(`/schedules/${encodeURIComponent(id)}`, { method: "PUT", body: json(body) }); },
  deleteSchedule(id: string) { return req<void>(`/schedules/${encodeURIComponent(id)}`, { method: "DELETE" }); },

  // Automations
  listAutomations() { return req<Automation[]>("/automations"); },
  createAutomation(body: Partial<Automation>) { return req<Automation>("/automations", { method: "POST", body: json(body) }); },
  updateAutomation(id: string, body: Partial<Automation>) { return req<Automation>(`/automations/${encodeURIComponent(id)}`, { method: "PUT", body: json(body) }); },
  deleteAutomation(id: string) { return req<void>(`/automations/${encodeURIComponent(id)}`, { method: "DELETE" }); },
  runAutomation(id: string) { return req<Automation>(`/automations/${encodeURIComponent(id)}/run`, { method: "POST" }); },

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

  // Activity
  listActivity(limit = 50) { return req<ActivityEntry[]>(`/activity?limit=${limit}`); },

  // iOS Shortcuts helper — ready-made Basic auth header for the configured creds.
  shortcutAuth() { return req<{ header: string }>("/shortcut-auth"); },

  // Timers
  listTimers() { return req<Timer[]>("/timers"); },
  createTimer(body: { target_type: TargetType; target_id: string; action: string; in_seconds?: number; fires_at?: string; note?: string }) {
    return req<Timer>("/timers", { method: "POST", body: json(body) });
  },
  deleteTimer(id: string) { return req<void>(`/timers/${encodeURIComponent(id)}`, { method: "DELETE" }); },

  // Sensors
  listSensors() { return req<Sensor[]>("/sensors"); },
  createSensor(body: Partial<Sensor>) { return req<Sensor>("/sensors", { method: "POST", body: json(body) }); },
  updateSensor(id: string, body: Partial<Sensor>) { return req<Sensor>(`/sensors/${encodeURIComponent(id)}`, { method: "PUT", body: json(body) }); },
  deleteSensor(id: string) { return req<void>(`/sensors/${encodeURIComponent(id)}`, { method: "DELETE" }); },
  sensorReadings(id: string, opts: { since_minutes?: number; limit?: number } = {}) {
    const q = new URLSearchParams();
    if (opts.since_minutes) q.set("since_minutes", String(opts.since_minutes));
    if (opts.limit) q.set("limit", String(opts.limit));
    const qs = q.toString();
    return req<SensorReading[]>(`/sensors/${encodeURIComponent(id)}/readings${qs ? `?${qs}` : ""}`);
  },
  postSensorReading(id: string, body: { value: number; time?: string }) {
    return req<SensorReading>(`/sensors/${encodeURIComponent(id)}/readings`, { method: "POST", body: json(body) });
  },
  startSensorPair(seconds = 60) {
    return req<{ active: boolean; until: string; seconds: number }>("/sensors/pair/start", {
      method: "POST",
      body: json({ seconds }),
    });
  },
  discoverSensors() {
    return req<DiscoveryState>("/sensors/discover");
  },

  // Settings
  getSettings() { return req<Settings>("/settings"); },
  updateSettings(body: Settings) { return req<Settings>("/settings", { method: "PUT", body: json(body) }); },

  // Config backup. Export hits a download endpoint directly (see Settings.svelte);
  // import posts a parsed bundle back.
  importConfig(bundle: unknown) {
    return req<{ sockets: number; schedules: number; groups: number; scenes: number; sensors: number }>(
      "/import", { method: "POST", body: json(bundle) });
  },

  // Tasmota Wi-Fi devices
  tasmotaGetState(socketId: string) {
    return req<TasmotaState>(`/tasmota/${encodeURIComponent(socketId)}`);
  },
  tasmotaSetState(socketId: string, update: TasmotaStateUpdate) {
    return req<void>(`/tasmota/${encodeURIComponent(socketId)}/state`, {
      method: "PUT",
      body: json(update),
    });
  },
  tasmotaProbe(ip: string) {
    return req<{ status: string; ip: string }>(`/tasmota/probe?ip=${encodeURIComponent(ip)}`);
  },

  // Matter devices (via the matter-bridge sidecar)
  matterTransport() {
    // Returns all configured transports — both "thread" and "wifi" can appear.
    return req<{ transports: ("thread" | "wifi")[] }>("/matter/transport");
  },
  matterListDevices() {
    return req<MatterState[]>("/matter/devices");
  },
  // Commissioning is asynchronous because the bridge can take 30–90s
  // (BLE discovery + Wi-Fi onboarding) — far longer than iOS Safari is
  // willing to keep a single fetch alive. The POST returns immediately
  // with a job id; poll matterCommissionJob until status != "pending".
  matterCommission(body: { pairing_code: string; transport?: string }) {
    return req<{ job_id: string }>("/matter/commission", { method: "POST", body: json(body) });
  },
  matterCommissionJob(jobId: string) {
    return req<{
      id: string;
      status: "pending" | "done" | "error";
      node_id?: string;
      error?: string;
      started_at: string;
      ended_at?: string;
    }>(`/matter/commission/jobs/${encodeURIComponent(jobId)}`);
  },
  matterGetState(socketId: string) {
    return req<MatterState>(`/matter/${encodeURIComponent(socketId)}`);
  },
  matterSetState(socketId: string, update: MatterStateUpdate) {
    return req<void>(`/matter/${encodeURIComponent(socketId)}/state`, {
      method: "PUT",
      body: json(update),
    });
  },

  // Push notifications
  getPushVapidKey() {
    return req<{ public_key: string }>("/push/vapid-key");
  },
  subscribePush(sub: PushSubscriptionBody) {
    return req<{ status: string }>("/push/subscribe", { method: "POST", body: json(sub) });
  },
  unsubscribePush(endpoint: string) {
    return req<{ status: string }>("/push/unsubscribe", {
      method: "DELETE",
      body: json({ endpoint }),
    });
  },
  updatePushPrefs(prefs: NotifPrefs) {
    return req<NotifPrefs>("/push/prefs", { method: "PUT", body: json(prefs) });
  },
  testPush() {
    return req<{ status: string }>("/push/test", { method: "POST" });
  },

  // MQTT — control devices and ingest sensors over a broker.
  mqttStatus() {
    return req<{ enabled: boolean; broker?: string; connected?: boolean }>("/mqtt/status");
  },
  mqttPublish(body: { topic: string; payload?: string }) {
    return req<{ status: string; topic: string }>("/mqtt/publish", {
      method: "POST",
      body: json(body),
    });
  },
};

export type Api = typeof api;
export { ApiError };
