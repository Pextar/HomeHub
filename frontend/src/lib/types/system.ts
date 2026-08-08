// App-wide odds and ends: location settings, bulk-action results, the
// route union, the activity log and toasts.

export interface Settings {
  latitude: number;
  longitude: number;
  location_name?: string;
  /** What the panel's announce strip offers before its text box. Always
   *  present on a read — the server resolves a household that has never set
   *  them to the built-in list, so there is no second place the defaults
   *  live. An empty array is a household that wants none, and is honoured. */
  announce_presets?: string[];
}

export interface BulkResult {
  updated: number;
  failures: { socket_id: string; error: string }[];
}

export type Route = "dashboard" | "rooms" | "floorplan" | "sockets" | "music" | "groups" | "scenes" | "schedules" | "sensors" | "automations" | "insights" | "activity" | "users" | "settings" | "console" | "panel";

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
