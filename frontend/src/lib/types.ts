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

/** Per-user push notification preferences. Categories default to true on first subscribe. */
export interface NotifPrefs {
  sensor_alerts: boolean;
  state_changes: boolean;
  schedule_fired: boolean;
  device_offline: boolean;
  // Quiet hours suppress everything except sensor alerts between
  // quiet_start and quiet_end (local time, may wrap past midnight).
  quiet_hours?: boolean;
  quiet_start?: string; // "HH:MM"
  quiet_end?: string;   // "HH:MM"
  // Devices opted out of notifications while their category stays enabled.
  muted_socket_ids?: string[];
  muted_sensor_ids?: string[];
}

// A login profile. Non-admins only see/control the sockets in socket_ids;
// admins ignore that list and have full access.
//
// Roles:
//   - owner=true, admin=true  → the one bootstrapped system owner
//   - owner=false, admin=true → manager (full access, added via invite link)
//   - admin=false             → limited profile (login code, specific devices)
export interface User {
  id: string;
  username: string;
  admin: boolean;
  /** True for the one bootstrapped owner — cannot be deleted or demoted. */
  owner?: boolean;
  /** True while the invite link hasn't been accepted yet (no password set). */
  pending_invite?: boolean;
  // A limited profile rendered with the playful, oversized kid layout.
  kid: boolean;
  // Limited profiles sign in with this generated code instead of a password;
  // empty/absent for admins.
  login_code?: string;
  socket_ids: string[];
  created_at: string;
  notif_prefs?: NotifPrefs;
}

// New admin users get an invite link — no password is set at creation time.
// Limited profiles (admin: false) get a code generated server-side.
export interface UserCreate {
  username: string;
  admin: boolean;
  kid?: boolean;
  socket_ids: string[];
}

// Response from POST /api/users when creating an admin (manager) profile.
// invite_url is only present in this one response; store it before closing.
export interface UserCreateResponse extends User {
  invite_url?: string;
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

/** Shape expected by POST /api/push/subscribe */
export interface PushSubscriptionBody {
  endpoint: string;
  keys: { p256dh: string; auth: string };
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
  level?: number; // 1-100, smart lights only
  color?: string; // "RRGGBB", smart lights only
}

// One time-phased stage within a scene.
// delay_minutes=0 means "run immediately on activation".
// The same socket can appear in multiple steps with different settings.
export interface SceneStep {
  delay_minutes: number;
  actions: SceneAction[];
}

export interface Scene {
  id: string;
  name: string;
  steps: SceneStep[];
  /** @deprecated legacy field; migrated to steps on the server */
  actions?: SceneAction[];
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

export type AutomationTriggerType = "time" | "sensor" | "device";

export interface AutomationTrigger {
  type: AutomationTriggerType;
  // time
  time_mode?: "fixed" | "sunrise" | "sunset";
  time?: string;
  solar_offset_minutes?: number;
  days?: number[];
  // sensor
  sensor_id?: string;
  op?: "above" | "below";
  value?: number;
  // device
  socket_id?: string;
  to_state?: "on" | "off";
}

export interface AutomationCondition {
  type: "device" | "time_range";
  // device
  socket_id?: string;
  state?: "on" | "off";
  // time_range
  after?: string;
  before?: string;
}

export interface AutomationAction {
  target_type: TargetType;
  target_id: string;
  action: SocketAction | "activate";
  level?: number;  // 1-100, smart lights only
  color?: string;  // "RRGGBB", smart lights only
}

export interface Automation {
  id: string;
  name: string;
  enabled: boolean;
  trigger: AutomationTrigger;
  conditions?: AutomationCondition[];
  actions: AutomationAction[];
  last_fired_at?: string;
  run_count?: number;
}

export interface BulkResult {
  updated: number;
  failures: { socket_id: string; error: string }[];
}

export type Route = "dashboard" | "floorplan" | "sockets" | "groups" | "scenes" | "schedules" | "sensors" | "automations" | "insights" | "activity" | "users" | "settings";

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
