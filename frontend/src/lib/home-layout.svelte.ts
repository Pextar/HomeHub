import {
  defaultLayout,
  normalizeLayout,
  reorder,
  type HomeLayout,
  type HomeSectionId,
} from "./home-layout";

/**
 * The home screen's arrangement, as this device remembers it.
 *
 * **Device-local on purpose**, in localStorage beside the theme and the floor
 * plan's positions rather than in the controller's settings. The wall panel,
 * the phone in a pocket and the tablet on the kitchen counter are looked at
 * for different reasons and want different screens, and one person tidying
 * their phone must not rearrange everyone else's home (the same rule
 * `uiPrefs` states for the assistant button). A layout that follows an account
 * across devices is a different feature and needs the backend; this is not it.
 *
 * Every write goes through here so the save is never forgotten, and every read
 * of storage is guarded — private browsing throws on both ends.
 */

const KEY = "home.layout.v1";

export function createHomeLayoutStore() {
  const s = $state<{ layout: HomeLayout }>({ layout: load() });

  function load(): HomeLayout {
    try {
      const raw = localStorage.getItem(KEY);
      return normalizeLayout(raw ? JSON.parse(raw) : null);
    } catch {
      // Private browsing, or a half-written value from a crashed tab.
      return defaultLayout();
    }
  }

  function write(next: HomeLayout) {
    s.layout = next;
    try {
      localStorage.setItem(KEY, JSON.stringify(next));
    } catch {
      /* private browsing — the layout still applies for this session */
    }
  }

  return {
    get layout() {
      return s.layout;
    },
    get order() {
      return s.layout.order;
    },
    get sensors() {
      return s.layout.sensors;
    },
    get temperature() {
      return s.layout.temperature;
    },

    isHidden: (id: HomeSectionId) => s.layout.hidden.includes(id),

    setHidden(id: HomeSectionId, hidden: boolean) {
      const has = s.layout.hidden.includes(id);
      if (has === hidden) return;
      write({
        ...s.layout,
        hidden: hidden ? [...s.layout.hidden, id] : s.layout.hidden.filter((x) => x !== id),
      });
    },

    /** Put `id` at index `to` within the whole order. */
    move(id: HomeSectionId, to: number) {
      write({ ...s.layout, order: reorder(s.layout.order, id, to) });
    },

    /** `null` restores the automatic pick. */
    setSensors(ids: string[] | null) {
      write({ ...s.layout, sensors: ids === null ? null : [...ids] });
    },

    /** `null` goes back to averaging the house. */
    setTemperature(id: string | null) {
      write({ ...s.layout, temperature: id });
    },

    reset() {
      write(defaultLayout());
    },
  };
}

export const homeLayout = createHomeLayoutStore();
