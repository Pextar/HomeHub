// Shared model for the smart-light control surface.
//
// Matter and Tasmota expose the same four knobs (on, brightness, colour,
// colour temperature) under different field names and different endpoints.
// Everything here is vendor-neutral: the bridge-specific modals adapt their
// own state shape to `LightSnapshot`/`LightUpdate` and let
// components/SmartLightControl.svelte render it.
//
// Colour temperature is in mireds throughout — the unit the Matter
// ColorTemperature cluster uses, and what Tasmota's CT field carries.
// kelvin = 1_000_000 / mired.

/** A device's current state, with unsupported channels left undefined. */
export interface LightSnapshot {
  on: boolean;
  /** Brightness 1-100. Undefined when the device isn't dimmable. */
  level?: number;
  /** "RRGGBB", no leading #. Undefined when the device isn't colour-capable. */
  color?: string;
  /** Mireds, 153-500. Undefined when the device isn't tunable-white. */
  ct?: number;
}

/** A partial write. Only the fields present are sent to the device. */
export interface LightUpdate {
  on?: boolean;
  level?: number;
  color?: string;
  ct?: number;
}

/** One-tap colour+brightness combination offered under "Scenes". */
export interface LightPreset {
  key: string;
  label: string;
  kind: "white" | "color";
  level: number;
  /** Mireds; set on "white" presets. */
  ct?: number;
  /** "RRGGBB"; set on "color" presets. */
  color?: string;
}

// Tuned for typical tunable-white bulbs. A preset is only offered when the
// device supports the channel it drives, so a colour-only bulb shows just
// the "color" entries and vice versa.
export const LIGHT_PRESETS: LightPreset[] = [
  { key: "read", label: "Reading", kind: "white", level: 100, ct: 250 },
  { key: "concentrate", label: "Concentrate", kind: "white", level: 100, ct: 180 },
  { key: "relax", label: "Relax", kind: "white", level: 40, ct: 400 },
  { key: "night", label: "Night", kind: "white", level: 12, ct: 454 },
  { key: "warm", label: "Warm", kind: "white", level: 80, ct: 370 },
  { key: "daylight", label: "Daylight", kind: "white", level: 100, ct: 200 },
  { key: "sunset", label: "Sunset", kind: "color", level: 70, color: "FF6A3D" },
  { key: "forest", label: "Forest", kind: "color", level: 60, color: "3DBF6A" },
  { key: "ocean", label: "Ocean", kind: "color", level: 70, color: "3DAFFF" },
  { key: "lavender", label: "Lavender", kind: "color", level: 60, color: "B47CFF" },
  { key: "rose", label: "Rose", kind: "color", level: 60, color: "FF6FA3" },
];

// Below ~18% the preview disc would read as plain black and the user would
// lose the hue they picked, so brightness scaling bottoms out there.
const MIN_PREVIEW_SCALE = 0.18;

const CT_MIN = 153; // ≈6500K, cool
const CT_MAX = 500; // ≈2000K, warm
const CT_COOL_RGB = [206, 233, 255];
const CT_WARM_RGB = [255, 184, 107];

/** Approximate sRGB for a mired value, as the eye reads a white bulb. */
export function ctToRgb(mireds: number): [number, number, number] {
  const t = Math.max(0, Math.min(1, (mireds - CT_MIN) / (CT_MAX - CT_MIN)));
  return [0, 1, 2].map((i) =>
    Math.round(CT_COOL_RGB[i] + (CT_WARM_RGB[i] - CT_COOL_RGB[i]) * t),
  ) as [number, number, number];
}

/** Compose an "RRGGBB"/"#RRGGBB" colour over black, scaled by brightness. */
export function tintForLevel(hex: string, level: number): string {
  const h = hex.replace(/^#/, "");
  const rgb: [number, number, number] = [
    parseInt(h.slice(0, 2), 16),
    parseInt(h.slice(2, 4), 16),
    parseInt(h.slice(4, 6), 16),
  ];
  return scaledCss(rgb, level);
}

/** The same, for a colour temperature rather than a picked colour. */
export function ctToCss(mireds: number, level: number): string {
  return scaledCss(ctToRgb(mireds), level);
}

function scaledCss([r, g, b]: [number, number, number], level: number): string {
  const k = Math.max(MIN_PREVIEW_SCALE, level / 100);
  return `rgb(${Math.round(r * k)}, ${Math.round(g * k)}, ${Math.round(b * k)})`;
}

/** Mireds as a rounded kelvin label, e.g. "2700K". */
export function kelvinLabel(mireds: number): string {
  return `${Math.round(1_000_000 / mireds / 50) * 50}K`;
}
