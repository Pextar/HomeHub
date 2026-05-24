import { api } from "./api";
import type { Socket, Schedule, Group, Scene, Timer, RoomSummary, ToastSpec, Route, ActivityEntry, Sensor, Settings, User } from "./types";

// Reactive global state. Svelte 5 runes ($state) make any property mutation
// trigger downstream reactivity in components that read these values.
//
// Each "store" is just an object exposing $state-backed properties.
function createDataStore() {
  const data = $state({
    sockets: [] as Socket[],
    schedules: [] as Schedule[],
    groups: [] as Group[],
    scenes: [] as Scene[],
    timers: [] as Timer[],
    rooms: [] as RoomSummary[],
    activity: [] as ActivityEntry[],
    sensors: [] as Sensor[],
    settings: { latitude: 0, longitude: 0 } as Settings,
    loaded: false,
    health: "unknown" as "ok" | "error" | "unknown",
  });

  async function refresh() {
    try {
      // Sockets and rooms are visible to every profile (filtered server-side
      // to the caller's allowed set). The rest are admin-only — fetching them
      // as a non-admin would 401/403, so we skip them entirely.
      const [sockets, rooms] = await Promise.all([api.listSockets(), api.listRooms()]);
      data.sockets = sockets ?? [];
      data.rooms = rooms ?? [];

      if (session.isAdmin) {
        const [schedules, groups, scenes, timers, activity, sensors, settings] = await Promise.all([
          api.listSchedules(),
          api.listGroups(),
          api.listScenes(),
          api.listTimers(),
          api.listActivity(50),
          api.listSensors(),
          api.getSettings(),
        ]);
        data.schedules = schedules ?? [];
        data.groups = groups ?? [];
        data.scenes = scenes ?? [];
        data.timers = timers ?? [];
        data.activity = activity ?? [];
        data.sensors = sensors ?? [];
        data.settings = settings ?? { latitude: 0, longitude: 0 };
      }
      data.loaded = true;
    } catch (e) {
      toasts.error("Failed to load data", (e as Error).message);
    }
  }

  async function pingHealth() {
    try {
      await api.health();
      data.health = "ok";
    } catch {
      data.health = "error";
    }
  }

  // Merge a single updated socket (returned by an action endpoint) into the
  // store in place, avoiding a full refresh of every collection.
  function applySocket(updated: Socket) {
    const i = data.sockets.findIndex(s => s.id === updated.id);
    if (i >= 0) data.sockets[i] = updated;
  }

  return {
    get value() { return data; },
    refresh,
    pingHealth,
    applySocket,
    socketById: (id: string) => data.sockets.find(s => s.id === id),
    groupById:  (id: string) => data.groups.find(g => g.id === id),
    sceneById:  (id: string) => data.scenes.find(s => s.id === id),
  };
}

function createToastStore() {
  const items = $state<ToastSpec[]>([]);
  let nextId = 1;

  function show(spec: Omit<ToastSpec, "id">) {
    const id = nextId++;
    const t: ToastSpec = { id, ...spec };
    items.push(t);
    const timeout = spec.timeoutMs ?? (spec.tone === "error" ? 5000 : 3500);
    if (timeout > 0) setTimeout(() => dismiss(id), timeout);
  }

  function dismiss(id: number) {
    const idx = items.findIndex(t => t.id === id);
    if (idx >= 0) items.splice(idx, 1);
  }

  return {
    get items() { return items; },
    dismiss,
    show,
    info:    (title: string, message?: string) => show({ title, message, tone: "info" }),
    success: (title: string, message?: string) => show({ title, message, tone: "success" }),
    warn:    (title: string, message?: string) => show({ title, message, tone: "warn" }),
    error:   (title: string, message?: string) => show({ title, message, tone: "error" }),
  };
}

function createRouteStore() {
  const valid: Route[] = ["dashboard", "floorplan", "sockets", "groups", "scenes", "schedules", "sensors", "insights", "activity", "users", "settings"];
  const current = $state<{ route: Route; query: Record<string, string> }>({ route: parse(), query: parseQuery() });

  function parse(): Route {
    const m = window.location.hash.match(/^#\/([\w-]+)/);
    const r = (m?.[1] ?? "dashboard") as Route;
    return valid.includes(r) ? r : "dashboard";
  }
  function parseQuery(): Record<string, string> {
    const i = window.location.hash.indexOf("?");
    if (i < 0) return {};
    return Object.fromEntries(new URLSearchParams(window.location.hash.slice(i + 1)));
  }

  window.addEventListener("hashchange", () => {
    current.route = parse();
    current.query = parseQuery();
  });
  if (!window.location.hash) window.location.hash = "#/dashboard";

  return {
    get current() { return current.route; },
    get query() { return current.query; },
    go(r: Route, params?: Record<string, string>) {
      const q = params && Object.keys(params).length ? "?" + new URLSearchParams(params).toString() : "";
      window.location.hash = `#/${r}${q}`;
    },
  };
}

function createThemeStore() {
  const t = $state<{ theme: "dark" | "light" }>({ theme: initial() });

  function initial(): "dark" | "light" {
    const saved = localStorage.getItem("theme");
    if (saved === "dark" || saved === "light") return saved;
    return window.matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark";
  }
  function apply() {
    document.documentElement.dataset.theme = t.theme;
  }
  apply();

  return {
    get current() { return t.theme; },
    toggle() {
      t.theme = t.theme === "dark" ? "light" : "dark";
      localStorage.setItem("theme", t.theme);
      apply();
    },
  };
}

// Current login profile. Loaded once after auth; drives which sockets are
// visible and whether admin-only UI (Groups, Scenes, Settings, …) shows.
function createSessionStore() {
  const s = $state<{ user: User | null; loaded: boolean }>({ user: null, loaded: false });

  async function load() {
    try {
      s.user = await api.me();
    } catch {
      s.user = null;
    }
    s.loaded = true;
  }

  return {
    get user() { return s.user; },
    get loaded() { return s.loaded; },
    // Default to admin when we couldn't load a profile (e.g. server-side
    // auth is off), so the app stays fully usable rather than locking down.
    get isAdmin() { return s.user?.admin ?? true; },
    load,
  };
}

export const session = createSessionStore();
export const data = createDataStore();
export const toasts = createToastStore();
export const route = createRouteStore();
export const theme = createThemeStore();
