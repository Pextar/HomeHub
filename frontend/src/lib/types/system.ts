// App-wide odds and ends: location settings, bulk-action results, the
// route union, the activity log and toasts.

export interface Settings {
  latitude: number;
  longitude: number;
  location_name?: string;
}

export interface BulkResult {
  updated: number;
  failures: { socket_id: string; error: string }[];
}

export type Route = "dashboard" | "rooms" | "floorplan" | "sockets" | "music" | "groups" | "scenes" | "schedules" | "sensors" | "automations" | "insights" | "activity" | "users" | "settings" | "console";

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
