import { api } from "./api";
import type { Socket, Schedule, Group, Scene, Timer, RoomSummary, ToastSpec, Route, ActivityEntry, Sensor, Settings } from "./types";

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
      const [sockets, schedules, groups, scenes, timers, rooms, activity, sensors, settings] = await Promise.all([
        api.listSockets(),
        api.listSchedules(),
        api.listGroups(),
        api.listScenes(),
        api.listTimers(),
        api.listRooms(),
        api.listActivity(20),
        api.listSensors(),
        api.getSettings(),
      ]);
      data.sockets = sockets ?? [];
      data.schedules = schedules ?? [];
      data.groups = groups ?? [];
      data.scenes = scenes ?? [];
      data.timers = timers ?? [];
      data.rooms = rooms ?? [];
      data.activity = activity ?? [];
      data.sensors = sensors ?? [];
      data.settings = settings ?? { latitude: 0, longitude: 0 };
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

  return {
    get value() { return data; },
    refresh,
    pingHealth,
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
  const valid: Route[] = ["dashboard", "sockets", "groups", "scenes", "schedules", "sensors", "settings"];
  const current = $state<{ route: Route }>({ route: parse() });

  function parse(): Route {
    const m = window.location.hash.match(/^#\/([\w-]+)/);
    const r = (m?.[1] ?? "dashboard") as Route;
    return valid.includes(r) ? r : "dashboard";
  }

  window.addEventListener("hashchange", () => { current.route = parse(); });
  if (!window.location.hash) window.location.hash = "#/dashboard";

  return {
    get current() { return current.route; },
    go(r: Route) { window.location.hash = `#/${r}`; },
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

export const data = createDataStore();
export const toasts = createToastStore();
export const route = createRouteStore();
export const theme = createThemeStore();
