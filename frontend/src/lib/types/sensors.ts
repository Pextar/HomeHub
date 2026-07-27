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
