import type { ComponentProps } from "svelte";
import type Icon from "../components/Icon.svelte";
import type { Sensor } from "./types";

/**
 * What the home screen is made of, in what order, and which sensors it reads.
 *
 * Home used to be a fixed run of sections in source order, and the sensor row
 * was `sensors.slice(0, 6)` — whichever six the controller happened to list
 * first. A house with a freezer probe, three radiator sensors and one outdoor
 * thermometer got the freezer and lost the outdoor one, with no way to say
 * otherwise. This module is the answer to both: an order the user owns, a
 * hidden set, and an explicit choice of which sensors the screen is about.
 *
 * It is deliberately pure. The reactive store around it is
 * `home-layout.svelte.ts`; everything that decides *what a layout means* lives
 * here, where it can be tested without mounting a screen — and where the
 * migration rule below (what happens to a saved layout when a release adds a
 * section) is one function rather than a guess spread over a component.
 */

type IconName = ComponentProps<typeof Icon>["name"];

export type HomeSectionId =
  | "hero"
  | "nowplaying"
  | "favorites"
  | "groups"
  | "rooms"
  | "sensors"
  | "timers"
  | "devices";

export interface HomeSection {
  id: HomeSectionId;
  /** What the editor calls it, and what the section head says. */
  title: string;
  /** One line in the editor saying what the section is for. */
  blurb: string;
  icon: IconName;
  /**
   * Sections that only render at one end of the breakpoint. Both of these
   * predate the editor and are real design decisions — a phone has no room
   * for the full device grid, and a desktop replaces the room cards with it —
   * so the editor states the rule rather than pretending the switch is a
   * promise it can't keep.
   */
  only?: "phone" | "desktop";
  /** Reads a collection only an admin profile ever fetches. */
  admin?: boolean;
  /** Whether the section carries settings of its own (a chevron in the editor). */
  options?: true;
}

/**
 * The canonical order — the layout a home that has never been customised
 * gets, and the frame a saved layout is migrated against.
 */
export const HOME_SECTIONS: readonly HomeSection[] = [
  {
    id: "hero",
    title: "Whole home",
    blurb: "The master switch, with power and temperature beside it.",
    icon: "home",
  },
  {
    id: "nowplaying",
    title: "Playing now",
    blurb: "Whatever a speaker in the house is playing.",
    icon: "play",
  },
  {
    id: "favorites",
    title: "Favorites",
    blurb: "The devices you starred.",
    icon: "star",
  },
  {
    id: "groups",
    title: "Groups",
    blurb: "Every group, with a switch for all of it.",
    icon: "groups",
    admin: true,
  },
  {
    id: "rooms",
    title: "Rooms",
    blurb: "One card per room, and how much of it is on.",
    icon: "couch",
    only: "phone",
  },
  {
    id: "sensors",
    title: "Sensors",
    blurb: "The readings you want on the home screen.",
    icon: "sensor",
    admin: true,
    options: true,
  },
  {
    id: "timers",
    title: "Pending timers",
    blurb: "Anything counting down to a switch.",
    icon: "timer",
    admin: true,
  },
  {
    id: "devices",
    title: "All devices",
    blurb: "The full grid, filterable by room.",
    icon: "devices",
    only: "desktop",
  },
] as const;

const ALL_IDS: readonly HomeSectionId[] = HOME_SECTIONS.map((s) => s.id);

/** How many sensors the section shows when the user hasn't picked any. */
export const AUTO_SENSOR_COUNT = 6;

export interface HomeLayout {
  /** Every known section, in the order the screen renders them. */
  order: HomeSectionId[];
  /** The ones switched off. They keep their place in `order` so switching a
   *  section back on puts it where it was, not at the bottom. */
  hidden: HomeSectionId[];
  /** The sensors the Sensors section shows, in the order the picker lists
   *  them. `null` means "whatever there is" — the pre-editor behaviour. */
  sensors: string[] | null;
  /** The sensor the hero and the Temperature tile read. `null` averages the
   *  house, which is what the screen did before it could be told better. */
  temperature: string | null;
}

export function defaultLayout(): HomeLayout {
  return { order: [...ALL_IDS], hidden: [], sensors: null, temperature: null };
}

const isId = (v: unknown): v is HomeSectionId => ALL_IDS.includes(v as HomeSectionId);

/** Keep only strings, and only one of each. */
function strings(v: unknown): string[] {
  if (!Array.isArray(v)) return [];
  const seen = new Set<string>();
  for (const x of v) if (typeof x === "string") seen.add(x);
  return [...seen];
}

/**
 * A saved order is a snapshot of the sections that existed when it was saved,
 * so every release that adds one has to decide where it lands for people who
 * already customised their home. Appending to the end is the easy answer and
 * the wrong one: a section designed to sit under the hero would arrive below
 * the device grid, and the only signal the user gets that a feature shipped is
 * a strip at the bottom of a long page.
 *
 * So a missing section lands **where the canonical order puts it** — directly
 * after the nearest section above it that the user still has. Their ordering
 * decisions are all kept; ours fills the gaps.
 */
function withNewSections(stored: HomeSectionId[]): HomeSectionId[] {
  const out = stored.slice();
  HOME_SECTIONS.forEach((meta, canonical) => {
    if (out.includes(meta.id)) return;
    let at = 0;
    for (let i = canonical - 1; i >= 0; i--) {
      const j = out.indexOf(HOME_SECTIONS[i].id);
      if (j >= 0) {
        at = j + 1;
        break;
      }
    }
    out.splice(at, 0, meta.id);
  });
  return out;
}

/**
 * Read whatever was in storage into a layout that is safe to render: unknown
 * ids dropped (a section removed in a release), duplicates collapsed, missing
 * ones inserted, and anything of the wrong shape replaced by its default.
 * Never throws — a corrupted preference must not cost the user their home
 * screen.
 */
export function normalizeLayout(raw: unknown): HomeLayout {
  const d = defaultLayout();
  if (!raw || typeof raw !== "object") return d;
  const o = raw as Record<string, unknown>;

  const order = withNewSections(strings(o.order).filter(isId));
  const hidden = strings(o.hidden).filter(isId);
  // `null` is a real value here (automatic), so only an array turns it into a
  // choice — a missing key means the user never made one.
  const sensors = Array.isArray(o.sensors) ? strings(o.sensors) : null;
  const temperature = typeof o.temperature === "string" ? o.temperature : null;

  return { order, hidden, sensors, temperature };
}

/** Move `id` to index `to`, clamped into the list. Pure — returns a new array. */
export function reorder(
  order: readonly HomeSectionId[],
  id: HomeSectionId,
  to: number,
): HomeSectionId[] {
  const from = order.indexOf(id);
  if (from < 0) return [...order];
  const out = order.slice();
  out.splice(from, 1);
  out.splice(Math.max(0, Math.min(out.length, to)), 0, id);
  return out;
}

/**
 * The sections in layout order, with the ones this profile can't see removed.
 * A non-admin never fetches groups, sensors or timers (see `data.refresh`), so
 * offering to arrange them would be offering a control that can't do anything.
 */
export function sectionsFor(layout: HomeLayout, isAdmin: boolean): HomeSection[] {
  const byId = new Map(HOME_SECTIONS.map((s) => [s.id, s]));
  return layout.order
    .map((id) => byId.get(id))
    .filter((s): s is HomeSection => !!s && (isAdmin || !s.admin));
}

export const isHidden = (layout: HomeLayout, id: HomeSectionId): boolean =>
  layout.hidden.includes(id);

/**
 * The sensors the home screen shows.
 *
 * A chosen list keeps the order it was saved in, and quietly loses sensors
 * that have since been deleted — the alternative is a card that says "—"
 * forever for a device that no longer exists. It is *not* rewritten on load,
 * so unplugging a sensor for a week and putting it back doesn't cost its
 * place on the screen.
 */
export function homeSensors(all: readonly Sensor[], layout: HomeLayout): Sensor[] {
  if (layout.sensors === null) return all.slice(0, AUTO_SENSOR_COUNT);
  const byId = new Map(all.map((s) => [s.id, s]));
  return layout.sensors.map((id) => byId.get(id)).filter((s): s is Sensor => !!s);
}

/**
 * What the hero and the Temperature tile say.
 *
 * The old reading was the mean of every temperature sensor in the house
 * labelled "inside", which in a house with an outdoor sensor was a number that
 * described nowhere. A named sensor answers it properly; where none is named
 * the average is still the best guess available, but it now says what it is.
 */
export function homeTemperature(
  all: readonly Sensor[],
  layout: HomeLayout,
): { value: number; label: string; named: boolean } | null {
  const temps = all.filter((s) => s.kind === "temperature" && s.last_value != null);
  if (temps.length === 0) return null;

  const picked = layout.temperature ? temps.find((s) => s.id === layout.temperature) : undefined;
  // One sensor is its own answer: there is nothing to average and nothing to
  // choose, so the house reads that thermometer whether or not anyone said so.
  const only = temps.length === 1 ? temps[0] : undefined;
  const one = picked ?? only;
  if (one) return { value: one.last_value as number, label: one.name, named: true };

  const sum = temps.reduce((acc, s) => acc + (s.last_value as number), 0);
  return {
    value: sum / temps.length,
    label: `Average of ${temps.length}`,
    named: false,
  };
}
