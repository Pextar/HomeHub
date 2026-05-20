export interface Socket {
  id: string;
  name: string;
  code: string;
  protocol: string;
  state: boolean;
  room: string;
  favorite?: boolean;
  emoji?: string; // shown big in kid mode
}

export type TargetType = "socket" | "group" | "scene";
export type SocketAction = "on" | "off" | "toggle";
export type SceneActionKind = "on" | "off";

// "fixed" fires at the wall-clock `time`. "sunrise"/"sunset" fire at the
// sun event plus `solar_offset_minutes` (negative = before, positive = after).
export type ScheduleTimeMode = "fixed" | "sunrise" | "sunset";

export interface Schedule {
  id: string;
  socket_id?: string;
  target_type?: TargetType;
  target_id?: string;
  action: SocketAction | "activate";
  time_mode?: ScheduleTimeMode;
  time: string;
  solar_offset_minutes?: number;
  days: number[];
  enabled: boolean;
  random_offset_minutes?: number;
  last_fired_at?: string;
  effective_time?: string;
}

export interface Settings {
  latitude: number;
  longitude: number;
  location_name?: string;
}

// A login profile. Non-admins only see/control the sockets in socket_ids;
// admins ignore that list and have full access.
export interface User {
  id: string;
  username: string;
  admin: boolean;
  // A limited profile rendered with the playful, oversized kid layout.
  kid: boolean;
  // Limited profiles sign in with this generated code instead of a password;
  // empty/absent for admins.
  login_code?: string;
  socket_ids: string[];
  created_at: string;
}

// Admins need a password; limited profiles (admin: false) get a code
// generated server-side, so password is omitted for them.
export interface UserCreate {
  username: string;
  password?: string;
  admin: boolean;
  kid?: boolean;
  socket_ids: string[];
}

// All fields optional — only the ones present are changed. An empty/omitted
// password leaves the existing one untouched. Set regenerate_code to issue a
// fresh login code for a limited profile.
export interface UserUpdate {
  username?: string;
  password?: string;
  admin?: boolean;
  kid?: boolean;
  socket_ids?: string[];
  regenerate_code?: boolean;
}

// Tasmota Wi-Fi device state. Fields are undefined when the device doesn't
// support that capability (e.g. a plain plug has no dimmer or color).
export interface TasmotaState {
  on: boolean;
  dimmer?: number;  // 1-100
  color?: string;   // RRGGBB hex
  ct?: number;      // 153-500 mired (500 = warm, 153 = cool)
}

export interface TasmotaStateUpdate {
  on?: boolean;
  dimmer?: number;
  color?: string;
  ct?: number;
}

// Matter device state (mirrors the matter-bridge sidecar's DeviceState).
// Fields are undefined when the device doesn't expose that capability.
export interface MatterState {
  id: string;
  name?: string;
  vendor?: string;
  product?: string;
  reachable: boolean;
  on?: boolean;
  level?: number;   // 0..100
  color?: string;   // RRGGBB hex
  ct?: number;      // 153..500 mired
}

export interface MatterStateUpdate {
  on?: boolean;
  level?: number;
  color?: string;
  ct?: number;
}

export interface Group {
  id: string;
  name: string;
  socket_ids: string[];
}

export interface SceneAction {
  socket_id: string;
  action: SceneActionKind;
}

export interface Scene {
  id: string;
  name: string;
  actions: SceneAction[];
}

export interface Timer {
  id: string;
  target_type: TargetType;
  target_id: string;
  action: SocketAction | "activate";
  fires_at: string;
  created_at: string;
  note?: string;
}

export interface RoomSummary {
  name: string;
  sockets: number;
  on: number;
}

export interface BulkResult {
  updated: number;
  failures: { socket_id: string; error: string }[];
}

export type Route = "dashboard" | "floorplan" | "sockets" | "groups" | "scenes" | "schedules" | "sensors" | "users" | "settings";

export type SensorKind = "temperature" | "humidity" | "motion" | "light" | "power" | "custom";

export interface Sensor {
  id: string;
  name: string;
  kind: SensorKind;
  unit: string;
  code: string;
  protocol: string;
  field?: string;
  room?: string;
  alert_min?: number;
  alert_max?: number;
  last_value?: number;
  last_reading_at?: string;
}

export interface SensorReading {
  time: string;
  value: number;
}

export interface DiscoveryCandidate {
  protocol: string;
  code: string;
  fields: Record<string, number>;
  count: number;
  first_seen: string;
  last_seen: string;
}

export interface DiscoveryState {
  active: boolean;
  until: string;
  candidates: DiscoveryCandidate[];
}

export interface ActivityEntry {
  id: number;
  time: string;
  kind: "socket" | "group" | "scene" | "room" | "bulk";
  source: "manual" | "schedule" | "timer";
  action: string;
  label: string;
  status: "ok" | "error";
  error?: string;
}

export interface ToastSpec {
  id: number;
  title: string;
  message?: string;
  tone: "info" | "success" | "warn" | "error";
  timeoutMs?: number;
  action?: { label: string; onClick: () => void };
}
