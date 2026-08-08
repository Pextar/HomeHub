import type {
  Socket,
  Schedule,
  Group,
  Scene,
  Timer,
  Room,
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
  AssistantStatus,
  AssistantStreamEvent,
  AssistantMessage,
  AssistantConfirmation,
  SonosStatus,
  SonosSpeaker,
  SonosCandidate,
  SonosEventHealth,
  SonosSettings,
  SonosSettingsPatch,
  SonosFavorite,
  SonosQueueItem,
  SonosRepeat,
  KEFStatus,
  KEFSpeaker,
  KEFCandidate,
  KEFSettings,
  KEFSettingsPatch,
  KEFSource,
  KEFSpotifyView,
  SpotifyStatus,
  SpotifyItem,
  SpotifyListening,
  SpotifyResults,
  SpotifyArtistDetail,
  SpotifyContextDetail,
  MediaEndpoint,
  MediaProvider,
  MediaResults,
  MediaZone,
  MediaZoneRoutes,
  MediaPlayResult,
  MediaHistory,
  MediaTopPlays,
  Listening,
  MusicTimer,
  MusicTimerView,
  MusicSleepResult,
  SpotifyLibrary,
  AnnounceStatus,
  AnnounceResult,
} from "./types";

/**
 * The body every "play this" endpoint takes.
 *
 * Only service/uri/title reach the speaker. The rest is carried so the room's
 * history has something worth drawing — a shelf tile needs a picture and a
 * second line, and asking the catalog for them again later would mean a
 * service round-trip to redraw a row we already had in hand.
 */
export interface PlayItemBody {
  service: string;
  uri: string;
  title: string;
  kind?: string;
  sub?: string;
  art_uri?: string;
}

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
  createRoom(body: { name: string }) { return req<Room>("/rooms", { method: "POST", body: json(body) }); },
  updateRoom(id: string, body: { name: string }) { return req<Room>(`/rooms/${encodeURIComponent(id)}`, { method: "PUT", body: json(body) }); },
  deleteRoom(id: string) { return req<void>(`/rooms/${encodeURIComponent(id)}`, { method: "DELETE" }); },
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
  runAutomationRule(id: string, ruleIdx: number) { return req<Automation>(`/automations/${encodeURIComponent(id)}/rules/${ruleIdx}/run`, { method: "POST" }); },

  // Groups
  listGroups() { return req<Group[]>("/groups"); },
  createGroup(body: Partial<Group>) { return req<Group>("/groups", { method: "POST", body: json(body) }); },
  updateGroup(id: string, body: Partial<Group>) { return req<Group>(`/groups/${encodeURIComponent(id)}`, { method: "PUT", body: json(body) }); },
  deleteGroup(id: string) { return req<void>(`/groups/${encodeURIComponent(id)}`, { method: "DELETE" }); },
  groupAction(id: string, action: SocketAction) {
    return req<BulkResult & { group: string }>(`/groups/${encodeURIComponent(id)}/${action}`, { method: "POST" });
  },

  // Scenes
  listScenes() { return req<Scene[]>("/scenes"); },
  createScene(body: Partial<Scene>) { return req<Scene>("/scenes", { method: "POST", body: json(body) }); },
  updateScene(id: string, body: Partial<Scene>) { return req<Scene>(`/scenes/${encodeURIComponent(id)}`, { method: "PUT", body: json(body) }); },
  deleteScene(id: string) { return req<void>(`/scenes/${encodeURIComponent(id)}`, { method: "DELETE" }); },
  activateScene(id: string) {
    return req<BulkResult & { scene: string }>(`/scenes/${encodeURIComponent(id)}/activate`, { method: "POST" });
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
  startSensorPair(seconds = 300) {
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

  // Sonos speakers (local UPnP control)
  sonosStatus() { return req<SonosStatus>("/sonos/status"); },
  sonosDiscover() { return req<SonosCandidate[]>("/sonos/discover"); },
  sonosEventHealth() { return req<SonosEventHealth>("/sonos/events"); },
  // Asks every watcher to resubscribe now instead of at its own backoff. The
  // work is asynchronous — re-read the health endpoint to see the outcome.
  sonosEventRetry() { return req<{ ok: boolean }>("/sonos/events/retry", { method: "POST" }); },
  sonosCreateSpeaker(body: { ip: string; name?: string; room?: string }) {
    return req<SonosSpeaker>("/sonos/speakers", { method: "POST", body: json(body) });
  },
  sonosUpdateSpeaker(id: string, body: { ip?: string; name?: string; room?: string }) {
    return req<SonosSpeaker>(`/sonos/speakers/${encodeURIComponent(id)}`, { method: "PUT", body: json(body) });
  },
  sonosDeleteSpeaker(id: string) {
    return req<void>(`/sonos/speakers/${encodeURIComponent(id)}`, { method: "DELETE" });
  },
  // Device settings — read on demand, not part of the status poll. The
  // response says which of the model-dependent controls this speaker has.
  sonosSettings(id: string) {
    return req<SonosSettings>(`/sonos/${encodeURIComponent(id)}/settings`);
  },
  // Send one field per interaction: the backend applies a patch in order and
  // stops at the first refusal, so a single field keeps the error unambiguous.
  sonosUpdateSettings(id: string, patch: SonosSettingsPatch) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/settings`, { method: "PUT", body: json(patch) });
  },
  /**
   * A picture of this speaker model, proxied from the speaker's own device
   * description. 404s when the speaker publishes none — render the striped
   * placeholder then, never a stand-in for another model.
   */
  sonosImageURL(id: string) {
    return `${BASE}/sonos/${encodeURIComponent(id)}/image`;
  },
  // Transport actions go to the group coordinator.
  sonosPlay(id: string) { return req<void>(`/sonos/${encodeURIComponent(id)}/play`, { method: "POST" }); },
  sonosPause(id: string) { return req<void>(`/sonos/${encodeURIComponent(id)}/pause`, { method: "POST" }); },
  sonosNext(id: string) { return req<void>(`/sonos/${encodeURIComponent(id)}/next`, { method: "POST" }); },
  sonosPrevious(id: string) { return req<void>(`/sonos/${encodeURIComponent(id)}/previous`, { method: "POST" }); },
  sonosSetVolume(id: string, level: number, group = false) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/volume`, { method: "PUT", body: json({ level, group }) });
  },
  sonosSetMute(id: string, muted: boolean) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/mute`, { method: "PUT", body: json({ muted }) });
  },
  sonosJoin(id: string, targetId: string) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/join`, { method: "POST", body: json({ target_id: targetId }) });
  },
  sonosLeave(id: string) { return req<void>(`/sonos/${encodeURIComponent(id)}/leave`, { method: "POST" }); },
  // Regroup a household in one ordered request: `join` land on {id}, then
  // `leave` step out into groups of their own. The order is the feature —
  // "take the music with me" is join-then-leave, because the destination has
  // to be handed the queue and the stream while the old room is still
  // coordinating. Looping over the two calls above from a browser keeps that
  // order only as long as the page does; here the run *is* the request, so a
  // panel that is navigated away from mid-gesture can't leave a household
  // half moved.
  sonosGroup(id: string, body: { join?: string[]; leave?: string[] }) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/group`, { method: "POST", body: json(body) });
  },
  sonosFavorites(id: string) { return req<SonosFavorite[]>(`/sonos/${encodeURIComponent(id)}/favorites`); },
  sonosPlayFavorite(id: string, fav: SonosFavorite) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/favorites/play`, { method: "POST", body: json(fav) });
  },
  // Transport extras — all group-level, so {id} must be the coordinator.
  sonosSeek(id: string, position: string) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/seek`, { method: "PUT", body: json({ position }) });
  },
  // Jumps to a 1-based queue position, switching the group back to its
  // queue first if it was parked on a stream.
  sonosSeekTrack(id: string, track: number) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/seek`, { method: "PUT", body: json({ track }) });
  },
  // Shuffle and repeat go together: Sonos stores them as one value.
  sonosSetPlayMode(id: string, shuffle: boolean, repeat: SonosRepeat) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/playmode`, { method: "PUT", body: json({ shuffle, repeat }) });
  },
  sonosSetCrossfade(id: string, enabled: boolean) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/crossfade`, { method: "PUT", body: json({ enabled }) });
  },
  // "Continue play similar" — see DESIGN.md's autoplay note. Group-level,
  // like every other transport extra: {id} must be the coordinator.
  sonosSetAutoplay(id: string, enabled: boolean) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/autoplay`, { method: "PUT", body: json({ enabled }) });
  },

  // Group queue. Adding never disturbs what is playing — pass next: true to
  // drop the item in after the current track instead of at the end.
  sonosQueue(id: string) { return req<SonosQueueItem[]>(`/sonos/${encodeURIComponent(id)}/queue`); },
  sonosQueueAdd(
    id: string,
    body: { service?: string; uri: string; title?: string; metadata?: string; next?: boolean },
  ) {
    return req<{ track: number; length: number }>(`/sonos/${encodeURIComponent(id)}/queue`, {
      method: "POST",
      body: json(body),
    });
  },
  // A whole run in one request. Reach for this over a loop of sonosQueueAdd
  // whenever there is more than one item: "more like this" is eight tracks,
  // and as eight requests from a wall panel — a 2015 iPad on household Wi-Fi
  // — it is eight round trips, each carrying its own position read, sent
  // backwards so that Sonos resolves each "next" into the right slot. Here
  // the order of the array is the order they land in.
  sonosQueueAddMany(
    id: string,
    items: { service?: string; uri: string; title?: string; metadata?: string }[],
    next = false,
  ) {
    return req<{ track: number; length: number; added: number }>(
      `/sonos/${encodeURIComponent(id)}/queue`,
      { method: "POST", body: json({ items, next }) },
    );
  },
  sonosQueueRemove(id: string, track: number) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/queue/${track}`, { method: "DELETE" });
  },
  sonosQueueClear(id: string) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/queue`, { method: "DELETE" });
  },
  // Moves a queued track to another position. Both numbers are read off the
  // queue as it looks now; the backend converts to the insertion point Sonos
  // actually wants.
  sonosQueueMove(id: string, track: number, to: number) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/queue/${track}`, {
      method: "PUT",
      body: json({ to }),
    });
  },

  // Plays a streaming-service item (from Spotify search) on the group led
  // by speaker {id}; the speaker streams with its own linked account.
  sonosPlayItem(id: string, body: PlayItemBody) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/play-item`, { method: "POST", body: json(body) });
  },

  // KEF speakers (local HTTP control). No grouping, queue or favorites —
  // the speaker's API has none; the input selector is what picks a source.
  kefStatus() { return req<KEFStatus>("/kef/status"); },
  kefDiscover() { return req<KEFCandidate[]>("/kef/discover"); },
  kefCreateSpeaker(body: { ip: string; name?: string; room?: string }) {
    return req<KEFSpeaker>("/kef/speakers", { method: "POST", body: json(body) });
  },
  kefUpdateSpeaker(id: string, body: { ip?: string; name?: string; room?: string }) {
    return req<KEFSpeaker>(`/kef/speakers/${encodeURIComponent(id)}`, { method: "PUT", body: json(body) });
  },
  kefDeleteSpeaker(id: string) {
    return req<void>(`/kef/speakers/${encodeURIComponent(id)}`, { method: "DELETE" });
  },
  kefPlay(id: string) { return req<void>(`/kef/${encodeURIComponent(id)}/play`, { method: "POST" }); },
  kefPause(id: string) { return req<void>(`/kef/${encodeURIComponent(id)}/pause`, { method: "POST" }); },
  kefNext(id: string) { return req<void>(`/kef/${encodeURIComponent(id)}/next`, { method: "POST" }); },
  kefPrevious(id: string) { return req<void>(`/kef/${encodeURIComponent(id)}/previous`, { method: "POST" }); },
  kefSetVolume(id: string, level: number) {
    return req<void>(`/kef/${encodeURIComponent(id)}/volume`, { method: "PUT", body: json({ level }) });
  },
  kefSetMute(id: string, muted: boolean) {
    return req<void>(`/kef/${encodeURIComponent(id)}/mute`, { method: "PUT", body: json({ muted }) });
  },
  // Switching the input is how you pick what plays: there is no queue to
  // point somewhere, so selecting "optic" *is* the "play the TV" action.
  kefSetSource(id: string, source: KEFSource) {
    return req<void>(`/kef/${encodeURIComponent(id)}/source`, { method: "PUT", body: json({ source }) });
  },
  kefSetPower(id: string, on: boolean) {
    return req<void>(`/kef/${encodeURIComponent(id)}/power`, { method: "PUT", body: json({ on }) });
  },
  // Device settings — read on demand, not part of the status poll. Fields
  // the model doesn't have are simply absent from the response.
  kefSettings(id: string) {
    return req<KEFSettings>(`/kef/${encodeURIComponent(id)}/settings`);
  },
  // One field per interaction, so "what did the speaker refuse" stays clear.
  kefUpdateSettings(id: string, patch: KEFSettingsPatch) {
    return req<void>(`/kef/${encodeURIComponent(id)}/settings`, { method: "PUT", body: json(patch) });
  },
  // Starts a Spotify item on a KEF speaker. Same body as sonosPlayItem, a
  // different road underneath: the speaker's own API can't be handed content,
  // so this asks Spotify to point Connect playback at it. The backend wakes
  // the speaker onto Wi-Fi first. A 409 means something the user can fix —
  // reconnect Spotify, or pick which Connect device this speaker is.
  kefPlayItem(id: string, body: PlayItemBody) {
    return req<void>(`/kef/${encodeURIComponent(id)}/play-item`, { method: "POST", body: json(body) });
  },
  // The Connect pairing for one speaker, plus the account's visible devices.
  kefSpotifyDevices(id: string) {
    return req<KEFSpotifyView>(`/kef/${encodeURIComponent(id)}/spotify`);
  },
  // Pin which Connect device a speaker is; an empty id goes back to matching
  // on the speaker's name.
  kefSetSpotifyDevice(id: string, device_id: string, device_name = "") {
    return req<KEFSpeaker>(`/kef/${encodeURIComponent(id)}/spotify`, {
      method: "PUT",
      body: json({ device_id, device_name }),
    });
  },

  // Spotify search/browse (user's own account via PKCE — configured in the Music view)
  spotifyStatus() { return req<SpotifyStatus>("/spotify/status"); },
  spotifySetConfig(clientId: string) {
    return req<void>("/spotify/config", { method: "PUT", body: json({ client_id: clientId }) });
  },
  spotifyLoginURL() { return req<{ url: string }>("/spotify/login"); },
  // Manual-flow finish: pass the full address the browser landed on after consent.
  spotifyExchange(url: string) {
    return req<void>("/spotify/exchange", { method: "POST", body: json({ url }) });
  },
  spotifyDisconnect() { return req<void>("/spotify/disconnect", { method: "POST" }); },
  // `kind` narrows to one of tracks/albums/playlists/artists and `offset`
  // pages into it — what a shelf's "Show more" needs, since Spotify caps a
  // search at ten results per kind. `signal` lets a superseded search be
  // called off rather than left to finish and be discarded.
  spotifySearch(
    q: string,
    limit = 8,
    opts: { kind?: string; offset?: number; signal?: AbortSignal } = {},
  ) {
    const p = new URLSearchParams({ q, limit: String(limit) });
    if (opts.kind) p.set("kind", opts.kind);
    if (opts.offset) p.set("offset", String(opts.offset));
    return req<SpotifyResults>(`/spotify/search?${p}`, { signal: opts.signal });
  },
  spotifyMyPlaylists() { return req<SpotifyItem[]>("/spotify/playlists"); },
  // What the account has been playing, for the idle shelves. 409 means the
  // login predates the listening scopes — a reconnect, not a fault.
  spotifyListening() { return req<SpotifyListening>("/spotify/listening"); },
  // Songs to continue with, seeded from an artist name — the same engine
  // "play similar" uses when a queue runs dry, asked for on purpose. By
  // name because that is what a speaker reports about what it is playing.
  spotifySimilar(artist: string, limit = 8) {
    return req<SpotifyItem[]>(
      `/spotify/similar?artist=${encodeURIComponent(artist)}&limit=${limit}`,
    );
  },
  // An artist's page — top tracks and albums, behind a search result.
  spotifyArtist(uri: string) {
    return req<SpotifyArtistDetail>(`/spotify/artist?uri=${encodeURIComponent(uri)}`);
  },
  // A playlist or album's own tracks — behind a favorite that turns out to
  // be a list rather than one song.
  // Whether a track is in the account's library, and putting it there. The
  // read needs no scope the login didn't already have; the write does, which
  // is why status.library exists — hide the control rather than offer a tap
  // that will be refused.
  spotifySaved(uri: string) {
    return req<{ saved: boolean }>(`/spotify/saved?uri=${encodeURIComponent(uri)}`);
  },
  spotifySetSaved(uri: string, saved: boolean) {
    return req<void>("/spotify/saved", { method: "PUT", body: json({ uri, saved }) });
  },
  spotifyContext(uri: string) {
    return req<SpotifyContextDetail>(`/spotify/context?uri=${encodeURIComponent(uri)}`);
  },
  // The rest of the collection — saved albums, kept playlists, the artists
  // this account actually plays — read as three shelves in one round trip.
  // A shelf whose scope was refused comes back empty rather than failing the
  // request; only all three failing is the account's problem rather than one
  // permission's.
  spotifyLibrary(limit = 20) {
    return req<SpotifyLibrary>(`/spotify/library?limit=${limit}`);
  },
  // What came out lately. The one shelf that is about the catalog rather
  // than about this household, and so the only answer available on an
  // evening when nobody wants to hear anything they already know.
  spotifyNewReleases(limit = 20) {
    return req<SpotifyItem[]>(`/spotify/new-releases?limit=${limit}`);
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

  // Assistant (local LLM). Status is a plain request; chat/confirm stream and
  // are handled by streamAssistantChat / streamAssistantConfirm below.
  assistantStatus() {
    return req<AssistantStatus>("/assistant/status");
  },

  // ── Media protocol ─────────────────────────────────────────────────────
  // Speakers and services addressed uniformly, plus zones — sets of speakers
  // that play together regardless of make. The sonos*/kef* calls above stay:
  // they carry vendor specifics the detail views need.
  // See docs/MEDIA-PROTOCOL.md.
  mediaEndpoints() { return req<MediaEndpoint[]>("/media/endpoints"); },
  mediaProviders() { return req<MediaProvider[]>("/media/providers"); },
  mediaSearch(q: string, opts?: { provider?: string; limit?: number }) {
    const p = new URLSearchParams({ q });
    if (opts?.provider) p.set("provider", opts.provider);
    if (opts?.limit) p.set("limit", String(opts.limit));
    return req<MediaResults>(`/media/search?${p}`);
  },
  mediaZones() { return req<MediaZone[]>("/media/zones"); },
  mediaCreateZone(body: { name: string; members: string[]; room?: string }) {
    return req<MediaZone>("/media/zones", { method: "POST", body: json(body) });
  },
  mediaUpdateZone(id: string, body: { name?: string; members?: string[]; room?: string }) {
    return req<MediaZone>(`/media/zones/${encodeURIComponent(id)}`, { method: "PUT", body: json(body) });
  },
  mediaDeleteZone(id: string) {
    return req<void>(`/media/zones/${encodeURIComponent(id)}`, { method: "DELETE" });
  },
  // What this zone could do, and for anything it can't, which speaker blocked
  // it. Read before playing so the UI can explain a limitation rather than
  // letting the user discover it as a failure.
  mediaZoneRoutes(id: string, provider?: string) {
    const p = provider ? `?provider=${encodeURIComponent(provider)}` : "";
    return req<MediaZoneRoutes>(`/media/zones/${encodeURIComponent(id)}/routes${p}`);
  },
  // Starts content on a zone. The response says which route was chosen and
  // why — a streamed zone genuinely differs from a natively grouped one, and
  // the UI is expected to say so rather than present them as equivalent.
  // A 409 means something the user can fix: connect an account, wake a
  // speaker, install librespot, or pick different speakers.
  mediaZonePlay(
    id: string,
    body: { provider?: string; uri: string; title?: string; kind?: string; sub?: string; art_uri?: string },
  ) {
    return req<MediaPlayResult>(`/media/zones/${encodeURIComponent(id)}/play`, {
      method: "POST", body: json(body),
    });
  },
  // What a room has been asked to play, newest first. A room with no history
  // of its own answers with the household's, flagged as such — the shelf says
  // "Played here" for one and "Played recently" for the other, because a wall
  // must never imply a room played something it didn't.
  mediaHistory(room: string, limit = 12) {
    return req<MediaHistory>(
      `/media/history?room=${encodeURIComponent(room)}&limit=${limit}`,
    );
  },
  // One room stops remembering one thing; without a uri it forgets the lot.
  // The shelves are *ranked*, so a record started by mistake doesn't sink —
  // it competes for the first shelf the wall offers, and every accidental
  // replay pushes it up. Per room, never household-wide: the same record is
  // the kids' room's favourite and the living room's mistake.
  mediaForgetPlay(room: string, uri?: string) {
    const p = new URLSearchParams({ room });
    if (uri) p.set("uri", uri);
    return req<void>(`/media/history?${p}`, { method: "DELETE" });
  },
  // What a room keeps coming back to, rather than what it happened to play
  // last. `hour` takes a local hour or "now", and ranks by what this room
  // plays at that hour — the difference between offering the kitchen its
  // breakfast radio at eight and offering it last night's dinner record. The
  // answer says which of the two it gave (`by_hour`).
  mediaTopPlays(room: string, opts: { limit?: number; hour?: number | "now" } = {}) {
    const p = new URLSearchParams({ room, limit: String(opts.limit ?? 8) });
    if (opts.hour !== undefined) p.set("hour", String(opts.hour));
    return req<MediaTopPlays>(`/media/history/top?${p}`);
  },
  // What the household listens to, summed over every room: who does the
  // listening, which artists it keeps coming back to, and when in the day it
  // is loud. Deliberately not per-room — the per-room questions are already
  // answered above, and this is the one picture none of them can give.
  mediaInsights(limit = 8) {
    return req<Listening>(`/media/insights?limit=${limit}`);
  },

  // ── Music timers ───────────────────────────────────────────────────────
  // Music that starts and stops on its own: the half the socket scheduler
  // could never reach. A wake-up is arranged in advance and described in
  // full, so it is ordinary CRUD; a sleep timer is set by someone already in
  // bed and is "forty minutes, this room", so it has a call of its own that
  // does the arithmetic.
  musicTimers() { return req<MusicTimerView[]>("/media/timers"); },
  musicCreateTimer(body: Omit<MusicTimer, "id">) {
    return req<MusicTimer>("/media/timers", { method: "POST", body: json(body) });
  },
  // The body replaces the timer wholesale: the two schedules are mutually
  // exclusive, so a partial update would have to define what clearing each
  // of them looks like.
  musicUpdateTimer(id: string, body: Omit<MusicTimer, "id">) {
    return req<MusicTimer>(`/media/timers/${encodeURIComponent(id)}`, {
      method: "PUT", body: json(body),
    });
  },
  musicDeleteTimer(id: string) {
    return req<void>(`/media/timers/${encodeURIComponent(id)}`, { method: "DELETE" });
  },
  // "Quiet in forty minutes." `minutes` is when the room goes silent and the
  // fade is the tail of that wait rather than time added to it, so the room
  // is quiet at forty and not at forty-eight. Setting one twice replaces it.
  // The engine puts the volume back afterwards — a room faded to two and
  // paused is inaudible the next morning.
  musicSleep(body: { room: string; minutes: number; fade_minutes?: number; volume?: number }) {
    return req<MusicSleepResult>("/media/timers/sleep", { method: "POST", body: json(body) });
  },
  // Stop a ramp in flight without deleting anything — "I'm still up". The
  // room keeps whatever volume it started the fade at.
  musicCancelFade(room: string) {
    return req<{ cancelled: boolean }>("/media/timers/fade/cancel", {
      method: "POST", body: json({ room }),
    });
  },

  // Calling the house. The status read is what decides whether the control is
  // drawn at all and whether it offers words or only a chime.
  announceStatus() { return req<AnnounceStatus>("/announce"); },
  announce(text: string, rooms?: string[]) {
    return req<AnnounceResult>("/announce", {
      method: "POST",
      body: json(rooms?.length ? { text, rooms } : { text }),
    });
  },

  mediaZoneResume(id: string) {
    return req<void>(`/media/zones/${encodeURIComponent(id)}/resume`, { method: "POST" });
  },
  mediaZonePause(id: string) {
    return req<void>(`/media/zones/${encodeURIComponent(id)}/pause`, { method: "POST" });
  },
  mediaZoneNext(id: string) {
    return req<void>(`/media/zones/${encodeURIComponent(id)}/next`, { method: "POST" });
  },
  mediaZonePrevious(id: string) {
    return req<void>(`/media/zones/${encodeURIComponent(id)}/previous`, { method: "POST" });
  },
  // Stop, unlike pause, also releases a stream session — so librespot stops
  // holding the account's Spotify device.
  mediaZoneStop(id: string) {
    return req<void>(`/media/zones/${encodeURIComponent(id)}/stop`, { method: "POST" });
  },
  mediaZoneVolume(id: string, level: number) {
    return req<void>(`/media/zones/${encodeURIComponent(id)}/volume`, { method: "PUT", body: json({ level }) });
  },
  mediaZoneMute(id: string, muted: boolean) {
    return req<void>(`/media/zones/${encodeURIComponent(id)}/mute`, { method: "PUT", body: json({ muted }) });
  },

};

export type Api = typeof api;
export { ApiError };

// ---- Assistant streaming (SSE over POST) ----

// The wire shape sent to the backend: only role + content (system prompt and
// tool results are reconstructed server-side).
type ChatHistory = { role: "user" | "assistant"; content: string }[];

function historyOf(messages: AssistantMessage[]): ChatHistory {
  return messages
    .filter((m) => m.content.trim() !== "")
    .map((m) => ({ role: m.role, content: m.content }));
}

// Parse one SSE frame ("event: X\ndata: <json>") into a typed event. Returns
// null for frames we don't recognise (e.g. keepalive comments).
function parseFrame(frame: string): AssistantStreamEvent | null {
  let event = "";
  const dataLines: string[] = [];
  for (const line of frame.split("\n")) {
    if (line.startsWith("event:")) event = line.slice(6).trim();
    else if (line.startsWith("data:")) dataLines.push(line.slice(5).trim());
  }
  if (!event) return null;
  let data: unknown = null;
  if (dataLines.length) {
    try { data = JSON.parse(dataLines.join("\n")); } catch { return null; }
  }
  switch (event) {
    case "token":
      return { type: "token", text: String(data ?? "") };
    case "tool": {
      const d = data as { name?: string; result?: string };
      return { type: "tool", name: d?.name ?? "", result: d?.result ?? "" };
    }
    case "confirmation":
      return { type: "confirmation", confirmation: data as AssistantConfirmation };
    case "error":
      return { type: "error", message: String(data ?? "error") };
    case "done":
      return { type: "done" };
    default:
      return null;
  }
}

// streamPost opens a streaming POST and dispatches each parsed SSE event to
// onEvent until the body closes or the AbortSignal fires.
async function streamPost(
  path: string,
  body: unknown,
  onEvent: (e: AssistantStreamEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  const res = await fetch(BASE + path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
    signal,
  });
  if (!res.ok || !res.body) {
    const text = await res.text().catch(() => "");
    let msg = res.statusText || "assistant request failed";
    try {
      const j = JSON.parse(text);
      if (j && typeof j.error === "string") msg = j.error;
    } catch { /* keep statusText */ }
    throw new ApiError(msg, res.status);
  }
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buf = "";
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buf += decoder.decode(value, { stream: true });
    let idx: number;
    // Frames are separated by a blank line.
    while ((idx = buf.indexOf("\n\n")) >= 0) {
      const frame = buf.slice(0, idx);
      buf = buf.slice(idx + 2);
      const ev = parseFrame(frame);
      if (ev) onEvent(ev);
    }
  }
}

// streamAssistantChat sends the conversation and streams the assistant's reply.
export function streamAssistantChat(
  messages: AssistantMessage[],
  onEvent: (e: AssistantStreamEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  return streamPost("/assistant/chat", { messages: historyOf(messages) }, onEvent, signal);
}

// streamAssistantConfirm executes a previously-paused action and streams the
// model's closing summary.
export function streamAssistantConfirm(
  token: string,
  messages: AssistantMessage[],
  onEvent: (e: AssistantStreamEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  return streamPost("/assistant/confirm", { token, messages: historyOf(messages) }, onEvent, signal);
}
