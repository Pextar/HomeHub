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
  sonosQueueRemove(id: string, track: number) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/queue/${track}`, { method: "DELETE" });
  },
  sonosQueueClear(id: string) {
    return req<void>(`/sonos/${encodeURIComponent(id)}/queue`, { method: "DELETE" });
  },

  // Plays a streaming-service item (from Spotify search) on the group led
  // by speaker {id}; the speaker streams with its own linked account.
  sonosPlayItem(id: string, body: { service: string; uri: string; title: string }) {
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
  kefPlayItem(id: string, body: { service: string; uri: string; title: string }) {
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
  // An artist's page — top tracks and albums, behind a search result.
  spotifyArtist(uri: string) {
    return req<SpotifyArtistDetail>(`/spotify/artist?uri=${encodeURIComponent(uri)}`);
  },
  // A playlist or album's own tracks — behind a favorite that turns out to
  // be a list rather than one song.
  spotifyContext(uri: string) {
    return req<SpotifyContextDetail>(`/spotify/context?uri=${encodeURIComponent(uri)}`);
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
  mediaZonePlay(id: string, body: { provider?: string; uri: string; title?: string; kind?: string }) {
    return req<MediaPlayResult>(`/media/zones/${encodeURIComponent(id)}/play`, {
      method: "POST", body: json(body),
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
