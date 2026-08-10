// Everything that makes a device act on its own: schedules, scenes,
// timers and the automation rule engine.

export type TargetType = "socket" | "group" | "room" | "scene";
// "set" changes a smart light's brightness/colour only, without an on/off
// change — automation actions only (schedules/timers reject it).
export type SocketAction = "on" | "off" | "toggle" | "set";
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

export interface SceneAction {
  socket_id: string;
  action: SceneActionKind;
  level?: number; // 1-100, smart lights only
  color?: string; // "RRGGBB", smart lights only
}

/**
 * What a scene or an automation does to the house's *audio*, beside what it
 * does to the sockets.
 *
 * The three verbs are the ones that need nothing but a room. A scene
 * expresses a moment — "we're watching a film", "everyone's out" — and
 * "start playing this record" needs a catalog item to name, which is what
 * the music timers are for. `room` is the media layer's own key
 * ("sonos:<id>" / "kef:<id>" / "zone:<id>"), the same vocabulary the timers,
 * the play shelves and the listening tallies use.
 */
export type MusicActionKind = "pause" | "resume" | "volume";

export interface MusicAction {
  room: string;
  action: MusicActionKind;
  /** 0-100; required by "volume" and ignored by the other two. */
  volume?: number;
}

// One time-phased stage within a scene.
// delay_minutes=0 means "run immediately on activation".
// The same socket can appear in multiple steps with different settings.
export interface SceneStep {
  delay_minutes: number;
  actions: SceneAction[];
  /** One row per room, at most. Absent on scenes written before this. */
  music?: MusicAction[];
}

/** Accent preset keys for a scene tile; each maps to a design token. */
export type SceneAccent = "amber" | "cool" | "violet" | "orange" | "green" | "gold";

export interface Scene {
  id: string;
  name: string;
  room?: string;
  /** Optional icon name from the shared icon set, shown on the tile. */
  icon?: string;
  /** Optional accent preset; empty = auto hue derived from the name. */
  color?: SceneAccent | "";
  steps: SceneStep[];
  /** @deprecated legacy field; migrated to steps on the server */
  actions?: SceneAction[];
  last_activated_at?: string;
  activate_count?: number;
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
  type: "device" | "time_range" | "time_before" | "time_after";
  // device
  socket_id?: string;
  state?: "on" | "off";
  // time_range / time_before / time_after
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

// ── Rule editor drafts ──────────────────────────────────────────────
// Edited in place by components/RuleEditor.svelte (the shared
// WHEN / ONLY-IF / THEN editor). The owning modal builds the actual
// Automation API payload from a draft itself.

export interface RuleActionDraft {
  target_type: TargetType;
  target_id: string;
  action: string;
  level: number;
  color: string;
  /** Present only on a group/room "on" action authored per-lamp: instead of
   *  one uniform fan-out, the action compiles to one socket action per member,
   *  each with its own state/level/colour. Keyed by socket id. Absent = the
   *  uniform (whole group/room) behaviour. */
  perLamp?: Record<string, { state: "on" | "off" | "ignore"; level: number; color: string }>;
}

export interface RuleDraft {
  trigType: AutomationTriggerType;
  trigTimeMode: string;
  trigTime: string;
  trigSolarOffset: number;
  trigDays: number[];
  trigSensorId: string;
  trigOp: "above" | "below";
  trigValue: number;
  trigSocketId: string;
  trigToState: "on" | "off";
  conditions: AutomationCondition[];
  actions: RuleActionDraft[];
  /** The rule's music rows, edited in place. Same shape as what is saved —
   *  unlike the socket actions, there is nothing to compile. */
  music: MusicAction[];
}

// One independent trigger → optional conditions → actions rule. An automation
// holds one or more, firing each on its own trigger.
export interface AutomationRule {
  trigger: AutomationTrigger;
  conditions?: AutomationCondition[];
  actions: AutomationAction[];
  /** Same shape and same reasons as SceneStep.music. A rule may carry only
   *  music — quieting the house is something to do. */
  music?: MusicAction[];
}

export interface Automation {
  id: string;
  name: string;
  enabled: boolean;
  rules: AutomationRule[];
  last_fired_at?: string;
  run_count?: number;
  /** Set when this automation was created as a rule inside a scene wizard. */
  scene_id?: string;
  /** Server-computed HH:MM for each rule's solar time trigger (sunrise/sunset +
   *  offset), index-aligned to rules. An entry is empty for non-solar triggers
   *  or when location is not configured. */
  effective_trigger_times?: (string | null)[];
}
