export interface Socket {
  id: string;
  name: string;
  code: string;
  protocol: string;
  state: boolean;
  room: string;
  favorite?: boolean;
  emoji?: string;      // shown big in kid mode
  readonly?: boolean;  // sensor / monitoring device — no on/off commands
}

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

export interface Room {
  id: string;
  name: string;
}

export interface RoomSummary extends Room {
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
}

// One independent trigger → optional conditions → actions rule. An automation
// holds one or more, firing each on its own trigger.
export interface AutomationRule {
  trigger: AutomationTrigger;
  conditions?: AutomationCondition[];
  actions: AutomationAction[];
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

export interface BulkResult {
  updated: number;
  failures: { socket_id: string; error: string }[];
}

export type Route = "dashboard" | "rooms" | "floorplan" | "sockets" | "music" | "groups" | "scenes" | "schedules" | "sensors" | "automations" | "insights" | "activity" | "users" | "settings" | "console";

// ---- Sonos (local UPnP control) ----

export interface SonosSpeaker {
  id: string;
  name: string;
  ip: string;
  uuid: string;
  room?: string;
  model?: string;
}

export interface SonosTrack {
  title?: string;
  artist?: string;
  album?: string;
  /** Absolute URL or an /api/sonos/{id}/art proxy path — usable as <img src> directly. */
  art_uri?: string;
}

export interface SonosState {
  transport_state: string; // PLAYING | PAUSED_PLAYBACK | STOPPED | TRANSITIONING
  playing: boolean;
  volume: number; // 0-100
  muted: boolean;
  track?: SonosTrack;
  position?: string; // H:MM:SS
  duration?: string; // H:MM:SS, empty for live streams
  /** 1-based position in the group queue; absent on radio / line-in. */
  queue_track?: number;
}

export type SonosRepeat = "off" | "all" | "one";

/**
 * Playback settings that belong to a zone group rather than to one speaker.
 * Only present on a coordinator's view — followers never carry it.
 */
export interface SonosGroupState {
  shuffle: boolean;
  repeat: SonosRepeat;
  crossfade: boolean;
  queue_length: number;
  /** True when the group is playing from its queue rather than a stream. */
  from_queue: boolean;
}

/** One track in a group queue. */
export interface SonosQueueItem {
  track: number; // 1-based
  title?: string;
  artist?: string;
  album?: string;
  art_uri?: string;
  duration?: string; // H:MM:SS
}

/** A registered speaker plus its live state (state omitted when unreachable). */
export interface SonosSpeakerView extends SonosSpeaker {
  reachable: boolean;
  state?: SonosState;
  group_state?: SonosGroupState;
}

/** One live zone group, expressed in registered speaker ids. */
export interface SonosGroupView {
  coordinator_id: string;
  member_ids: string[];
  /** Zone names grouped on the Sonos side but not registered in HomeHub. */
  unregistered?: string[];
}

export interface SonosStatus {
  speakers: SonosSpeakerView[];
  groups: SonosGroupView[];
  /**
   * True when the backend is subscribed to the speakers' own change
   * notifications, so this state arrived without anyone asking for it.
   * False means it was read on demand — the bridge couldn't subscribe —
   * and the caller should keep polling at the old rate.
   */
  live?: boolean;
}

/**
 * One speaker's push status. Timestamps are RFC3339 strings and absent when
 * the thing they describe hasn't happened — no event yet, no renewal due.
 */
export interface SonosEventSpeaker {
  id: string;
  name: string;
  ip: string;
  /** Whether this speaker currently holds event subscriptions. */
  subscribed: boolean;
  reachable: boolean;
  /** Subscribed service keys: transport, rendering, topology. */
  services?: string[];
  /** The URL this speaker was told to post its notifications to. */
  callback?: string;
  /** Notifications accepted from it since the hub started. */
  events: number;
  last_event?: string;
  renew_at?: string;
  /** Why the last subscription attempt failed; absent once one succeeds. */
  error?: string;
}

/**
 * How the push subsystem is doing, per speaker. Read by the "Live updates"
 * sheet — the surface that explains why the app is or isn't getting speaker
 * state pushed to it, and what to do about it.
 */
export interface SonosEventHealth {
  /** At least one speaker is subscribed — matches SonosStatus.live. */
  live: boolean;
  /** The subscription supervisor is up. False is a hub problem, not a network one. */
  running: boolean;
  subscribed: number;
  total: number;
  /** The address speakers are told to reach the hub on. */
  callback?: string;
  /** Why that address couldn't be worked out at all. */
  callback_error?: string;
  speakers: SonosEventSpeaker[];
}

/**
 * Which of the model-dependent controls a speaker actually answered for.
 * There is no "what can you do" action on Sonos, so the backend works this
 * out by asking and seeing what faults — render only what is true here,
 * because a control that would be refused is worse than one that isn't there.
 */
export interface SonosCapabilities {
  bass: boolean;
  treble: boolean;
  loudness: boolean;
  night_mode: boolean;
  /** Sonos' name for what its app calls speech enhancement. */
  dialog_level: boolean;
  sub: boolean;
  surround: boolean;
}

/** Read-only identity block — serial, firmware, MAC. Support detail only. */
export interface SonosZoneInfo {
  serial_number?: string;
  software_version?: string;
  hardware_version?: string;
  mac_address?: string;
  /** The speaker's own room name, which can differ from HomeHub's label for it. */
  zone_name?: string;
}

/**
 * One speaker's settings snapshot. Every adjustable field is optional:
 * absent means "this model doesn't have it", which is a different statement
 * from a zero value — check `capabilities` rather than truthiness.
 */
export interface SonosSettings {
  capabilities: SonosCapabilities;
  bass?: number;      // -10…10
  treble?: number;    // -10…10
  loudness?: boolean;
  night_mode?: boolean;
  dialog_level?: boolean;
  sub_enabled?: boolean;
  sub_gain?: number;  // -15…15
  surround?: boolean;
  /** The status light on the speaker's face. */
  led?: boolean;
  /** True when the speaker's touch controls are locked. */
  button_lock?: boolean;
  /** Whole minutes left on the group sleep timer; 0 when none is set. */
  sleep_minutes: number;
  info: SonosZoneInfo;
  model_number?: string;
  display_name?: string;
  /** The speaker publishes a picture of itself — otherwise use the placeholder. */
  has_image: boolean;
}

/**
 * The writable half of a settings snapshot. Send one field per interaction:
 * the backend applies a patch in a fixed order and stops at the first
 * refusal, so a single field keeps "what did the speaker refuse" unambiguous.
 */
export interface SonosSettingsPatch {
  bass?: number;
  treble?: number;
  loudness?: boolean;
  night_mode?: boolean;
  dialog_level?: boolean;
  sub_enabled?: boolean;
  sub_gain?: number;
  surround?: boolean;
  led?: boolean;
  button_lock?: boolean;
  /** Group-scoped — send to a coordinator. 0 cancels a running timer. */
  sleep_minutes?: number;
}

/** A discovered (not necessarily registered) speaker on the LAN. */
export interface SonosCandidate {
  ip: string;
  uuid: string;
  room: string;
  model: string;
  registered: boolean;
}

/** One "My Sonos" favorite. uri/metadata round-trip to the play endpoint untouched. */
export interface SonosFavorite {
  id: string;
  title: string;
  art_uri?: string;
  uri: string;
  metadata?: string;
  service?: string;
}

// ---- KEF (local HTTP control) ----
//
// A second speaker bridge beside Sonos, not folded into it: a KEF speaker is
// one standalone stereo pair with an input selector, where Sonos is zones
// that group and share a queue. There is no grouping, no queue and no
// favorites here because the speaker's API has none — see internal/kef.

export interface KEFSpeaker {
  id: string;
  name: string;
  ip: string;
  /** Normalised MAC — the stable device id, since KEF has no RINCON. */
  mac: string;
  room?: string;
  model?: string;
  /**
   * Which Spotify Connect device this speaker is, when the name it registers
   * with Spotify isn't the name here. Optional: normally the names match and
   * the backend resolves it on its own. Starting music on a KEF speaker goes
   * through Connect — its local API can play and pause but has nothing to be
   * handed — so this is the pairing that makes a search result playable.
   */
  spotify_device_id?: string;
  spotify_device_name?: string;
}

/** Physical inputs. Not every model has every one — read `source` to see. */
export type KEFSource = "wifi" | "bluetooth" | "tv" | "optic" | "coaxial" | "analog" | "usb";

/** How long the speaker waits in silence before powering itself down. */
export type KEFStandbyMode = "standby_none" | "standby_20mins" | "standby_60mins";

export interface KEFTrack {
  title?: string;
  artist?: string;
  album?: string;
  /** Usually an absolute service URL; a speaker-relative path is proxied. */
  art_uri?: string;
}

export interface KEFState {
  /** The speaker's own word: playing | paused | stopped. */
  status: string;
  playing: boolean;
  source: string;
  /** False when the speaker is in standby — awake enough to answer, not to play. */
  powered_on: boolean;
  volume: number; // 0-100
  muted: boolean;
  track?: KEFTrack;
  /** Milliseconds. Duration is 0 for live streams and the physical inputs. */
  position_ms?: number;
  duration_ms?: number;
}

/** A registered speaker plus its live state (state omitted when unreachable). */
export interface KEFSpeakerView extends KEFSpeaker {
  reachable: boolean;
  state?: KEFState;
  /** Unix ms the reading was taken — extrapolate position from this, not from now. */
  read_at?: number;
}

export interface KEFStatus {
  speakers: KEFSpeakerView[];
  /**
   * True when this came from the backend's own poller rather than from a
   * read performed to answer this request. KEF speakers have no change
   * notifications to subscribe to, so unlike Sonos there is no "live" claim
   * to make — the backend polls once for everyone and pushes `music` on a
   * real change.
   */
  warm?: boolean;
}

/** A discovered (not necessarily registered) speaker on the LAN. */
export interface KEFCandidate {
  ip: string;
  mac: string;
  name: string;
  model: string;
  registered: boolean;
}

/** Read-only identity block. Support detail only. */
export interface KEFInfo {
  name?: string;
  model?: string;
  mac?: string;
  firmware?: string;
  release?: string;
}

/**
 * One speaker's settings snapshot. Every adjustable field is optional, and
 * absent means "this model doesn't have it" — a different statement from a
 * zero value, so check for `undefined` rather than truthiness. An LSX II has
 * no subwoofer output, so none of the sub fields come back for one.
 *
 * Placement and treble trim are in tenths of a dB, which is how the speaker
 * stores them: -60 is -6.0 dB.
 */
export interface KEFSettings {
  info: KEFInfo;
  bass_extension?: "less" | "standard" | "extra";
  desk_mode?: boolean;
  desk_gain?: number;   // -60…0
  wall_mode?: boolean;
  wall_gain?: number;   // -60…0
  treble?: number;      // -20…20
  phase_correction?: boolean;
  high_pass_mode?: boolean;
  high_pass_freq?: number; // 50…120 Hz
  subwoofer_out?: boolean;
  sub_lp_freq?: number;    // 40…250 Hz
  sub_gain?: number;       // -10…10 dB
  sub_phase?: "phase0" | "phase180";
  standby_mode?: KEFStandbyMode;
  max_volume?: number;  // 0…100
  volume_limit?: boolean;
}

/**
 * The writable half of a settings snapshot. Send one field per interaction:
 * the backend reports the first refusal, so a single field keeps "what did
 * the speaker refuse" unambiguous.
 */
export interface KEFSettingsPatch {
  bass_extension?: "less" | "standard" | "extra";
  desk_mode?: boolean;
  desk_gain?: number;
  wall_mode?: boolean;
  wall_gain?: number;
  treble?: number;
  phase_correction?: boolean;
  high_pass_mode?: boolean;
  high_pass_freq?: number;
  subwoofer_out?: boolean;
  sub_lp_freq?: number;
  sub_gain?: number;
  sub_phase?: "phase0" | "phase180";
  standby_mode?: KEFStandbyMode;
  max_volume?: number;
  volume_limit?: boolean;
}

// ---- Spotify search (plays back through the speakers' linked account) ----

export interface SpotifyStatus {
  configured: boolean; // client ID set
  connected: boolean;  // OAuth completed
  display_name?: string;
  /** Exact redirect URI to register in the Spotify developer dashboard. */
  redirect_uri: string;
  /**
   * True when HomeHub is served over plain HTTP: the redirect URI is a
   * parked loopback address, and the user finishes the login by pasting
   * the address they land on (see api.spotifyExchange). Over HTTPS the
   * callback completes automatically.
   */
  manual: boolean;
  /**
   * Whether this login may start playback (KEF speakers go through Spotify
   * Connect). False on a login granted before HomeHub asked for the player
   * scopes: search still works, playing doesn't, and only reconnecting
   * fixes it — so it is worth saying before the tap rather than after.
   */
  playback: boolean;
}

/** One Spotify Connect endpoint the connected account can play to. */
export interface SpotifyDevice {
  id: string;
  name: string;
  type: string; // Speaker | Computer | Smartphone | …
  active: boolean;
  /** Spotify accepts no commands for these at all (some car/TV integrations). */
  restricted: boolean;
  volume?: number;
}

/** The Connect pairing for one KEF speaker, plus what else is on offer. */
export interface KEFSpotifyView {
  pinned_id?: string;
  pinned_name?: string;
  /** What a play would use right now; absent when nothing matches. */
  device?: SpotifyDevice;
  /** Why `device` is absent, in words worth showing. */
  reason?: string;
  devices: SpotifyDevice[];
}

export interface SpotifyItem {
  kind: "track" | "album" | "playlist";
  /** Canonical Spotify URI (spotify:track:…) — what the play endpoint takes. */
  uri: string;
  name: string;
  sub?: string;     // artist / owner line
  art_url?: string; // https CDN image
}

export interface SpotifyResults {
  tracks: SpotifyItem[];
  albums: SpotifyItem[];
  playlists: SpotifyItem[];
}

// ---- Assistant (local LLM) ----

export interface AssistantStatus {
  enabled: boolean;
  model?: string;
  reachable?: boolean;
  last_error?: string;
}

// One tool the assistant ran during a turn, shown as a small card in the thread.
export interface AssistantToolCall {
  name: string;
  result: string;
}

// A pending bulk/destructive action awaiting the user's confirmation. The
// signed token is opaque — it round-trips to the backend untouched.
export interface AssistantConfirmation {
  token: string;
  summary: string;
  affected?: string[];
  tool: string;
}

export type AssistantRole = "user" | "assistant";

export interface AssistantMessage {
  role: AssistantRole;
  content: string;
  tools?: AssistantToolCall[];
  // true while the assistant bubble is still waiting on its first token.
  pending?: boolean;
  // set when this assistant turn ended in an error, so it can render as such.
  error?: boolean;
}

// Events streamed from /api/assistant/chat and /confirm.
export type AssistantStreamEvent =
  | { type: "token"; text: string }
  | { type: "tool"; name: string; result: string }
  | { type: "confirmation"; confirmation: AssistantConfirmation }
  | { type: "error"; message: string }
  | { type: "done" };

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

// ── Media protocol ───────────────────────────────────────────────────────
// The vendor-neutral layer: speakers and services addressed uniformly, plus
// zones — sets of speakers that play together regardless of make. The Sonos
// and KEF types above stay: the per-speaker detail views need vendor
// specifics that don't generalise. See docs/MEDIA-PROTOCOL.md.

/** Which bridge is behind an endpoint. */
export type MediaVendor = "sonos" | "kef";

/**
 * One thing a speaker can do. These are the strings the backend emits, and
 * the UI keys off them rather than off the vendor — that is what lets a new
 * bridge appear without every component learning about it.
 */
export type MediaCapability =
  | "transport"
  | "volume"
  | "seek"
  | "queue"
  | "group"
  | "play_uri"
  | "native_service"
  | "connect"
  | "wake";

/** A speaker's identity and what it can do, without touching the device. */
export interface MediaEndpoint {
  id: string;
  name: string;
  room?: string;
  vendor: MediaVendor;
  model?: string;
  capabilities: MediaCapability[];
  /** Bridge-qualified id ("sonos:abc") — what a zone stores as a member. */
  member: string;
}

/** Normalised transport state, common across both bridges. */
export type MediaPlayState = "playing" | "paused" | "stopped" | "transitioning";

export interface MediaTrack {
  title?: string;
  artist?: string;
  album?: string;
  art_uri?: string;
}

export interface MediaNowPlaying {
  state: MediaPlayState;
  volume: number;
  muted: boolean;
  track?: MediaTrack;
  position_ms?: number;
  duration_ms?: number;
  /** When the reading was taken — extrapolate position from this, not now. */
  at: string;
}

/**
 * How content reaches a zone. Ordered best-first by the backend, and the
 * ordering is a guarantee: `stream` is only ever chosen when nothing else
 * can serve the whole zone, so a Sonos-only zone never lands on it.
 */
export type MediaRoute = "native" | "connect" | "group" | "stream";

/**
 * How well a route keeps speakers together. `buffered` is the honest label
 * for the stream route — a few hundred milliseconds apart, stable once
 * running, and not the sample-locked sync a native group gets.
 */
export type MediaSync = "exact" | "single" | "buffered";

/** Whether something is usable right now, and if not what to do about it. */
export interface MediaAvailability {
  ok: boolean;
  configured: boolean;
  reason?: string;
}

export interface MediaProvider {
  id: string;
  name: string;
  availability: MediaAvailability;
  routes: MediaRoute[];
  /**
   * Whether the cross-vendor route works. A separate question from
   * `availability` — a service can search perfectly and be unable to stream,
   * and the two need different prompts.
   */
  streaming: MediaAvailability;
}

/** One member of a zone, with whatever live state it reported. */
export interface MediaZoneSpeaker extends MediaEndpoint {
  state?: MediaNowPlaying;
  /** Set when the speaker behind this member no longer exists. */
  missing?: boolean;
}

export interface MediaZone {
  id: string;
  name: string;
  /** Bridge-qualified speaker ids, in the order the user arranged them. */
  members: string[];
  room?: string;
  speakers: MediaZoneSpeaker[];
  /** What a play would use right now; absent when nothing can serve it. */
  route?: MediaRoute;
  sync?: MediaSync;
  /** Why that route, in words fit to show. Always set when route is. */
  reason?: string;
  /** Why nothing can serve this zone. Set instead of route/reason. */
  problem?: string;
}

export type MediaItemKind = "track" | "album" | "playlist" | "artist" | "station";

export interface MediaItem {
  provider: string;
  kind: MediaItemKind;
  uri: string;
  title: string;
  subtitle?: string;
  art_uri?: string;
}

export interface MediaResults {
  tracks: MediaItem[];
  albums: MediaItem[];
  playlists: MediaItem[];
  artists: MediaItem[];
}

/** What a play answered: which route it took, and why. */
export interface MediaPlayResult {
  route: MediaRoute;
  sync: MediaSync;
  reason: string;
  /** Present only on the stream route. */
  stream_url?: string;
  speakers: string[];
}

/** One route that couldn't serve a zone, and the reason naming the speaker. */
export interface MediaRouteBlock {
  route: MediaRoute;
  reason: string;
}

export interface MediaZoneRoutes {
  speakers: string[];
  route?: MediaRoute;
  sync?: MediaSync;
  reason?: string;
  problem?: string;
  /** Every rejected route, in preference order. Present with `problem`. */
  blocked?: MediaRouteBlock[];
}
