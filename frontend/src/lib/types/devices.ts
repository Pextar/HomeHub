// Physical devices: RF sockets, the smart-light bridges behind them, and
// the room/group buckets they're organised into.

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

export interface Room {
  id: string;
  name: string;
}

export interface RoomSummary extends Room {
  sockets: number;
  on: number;
}
