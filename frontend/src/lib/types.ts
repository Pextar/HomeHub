export interface Socket {
  id: string;
  name: string;
  code: string;
  protocol: string;
  state: boolean;
  room: string;
}

export type TargetType = "socket" | "group" | "scene";
export type SocketAction = "on" | "off" | "toggle";
export type SceneActionKind = "on" | "off";

export interface Schedule {
  id: string;
  socket_id?: string;
  target_type?: TargetType;
  target_id?: string;
  action: SocketAction | "activate";
  time: string;
  days: number[];
  enabled: boolean;
  random_offset_minutes?: number;
  last_fired_at?: string;
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

export type Route = "dashboard" | "sockets" | "groups" | "scenes" | "schedules";

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
