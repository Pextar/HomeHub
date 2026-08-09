/**
 * Theme: the stored preference ("dark" / "light" / "auto") and the dark-or-light
 * value everything actually renders from.
 *
 * "Auto" is the interesting one. Reading `prefers-color-scheme` once at load is
 * not enough, and neither is the media query's `change` event on its own: an
 * installed PWA is frozen while it sits behind the home screen, so a device that
 * flips to dark at sunset never delivers that event — the app comes back looking
 * exactly as it did hours ago, and stays that way until it is killed and
 * relaunched. So the OS value is also re-read on every resume (visible again,
 * page shown from the back/forward cache, window refocused). Each of those is a
 * cheap idempotent re-read that no-ops when nothing moved.
 */

import { BAR_COLOR, type Resolved } from "./theme-colors";

export type ThemeMode = "dark" | "light" | "auto";
export type { Resolved };

const STORAGE_KEY = "theme";

export interface ThemeStore {
  /** The resolved dark/light value — what CSS and icons key off. */
  readonly current: Resolved;
  /** The stored preference: "dark", "light", or "auto". */
  readonly mode: ThemeMode;
  /** What the system is asking for right now — what "Auto" would pick. */
  readonly system: Resolved;
  setMode(mode: ThemeMode): void;
  cycle(): void;
  /** Re-read the OS preference and adopt it if we're on auto. Wired to the
   *  media query and to every resume; exposed for tests. */
  sync(): void;
  /** Drops the listeners. Only tests need this — the app's store lives as long
   *  as the document does. */
  destroy(): void;
}

export function createThemeStore(): ThemeStore {
  const media = window.matchMedia("(prefers-color-scheme: light)");

  function systemNow(): Resolved {
    return media.matches ? "light" : "dark";
  }
  function resolve(mode: ThemeMode): Resolved {
    return mode === "auto" ? systemNow() : mode;
  }
  function initialMode(): ThemeMode {
    try {
      const saved = localStorage.getItem(STORAGE_KEY);
      if (saved === "dark" || saved === "light" || saved === "auto") return saved;
    } catch {
      // private browsing — fall through to the default
    }
    return "auto";
  }

  const startMode = initialMode();
  const t = $state<{ mode: ThemeMode; resolved: Resolved; system: Resolved }>({
    mode: startMode,
    resolved: resolve(startMode),
    system: systemNow(),
  });

  function apply() {
    document.documentElement.dataset.theme = t.resolved;
    // The status bar / address bar is the one surface CSS can't reach, and
    // on an installed PWA it is half the screen's edge. Left alone it stayed
    // the dark value forever, which is what made light — and so auto — look
    // like it hadn't taken.
    document
      .querySelector('meta[name="theme-color"]')
      ?.setAttribute("content", BAR_COLOR[t.resolved]);
  }
  apply();

  function setMode(mode: ThemeMode) {
    t.mode = mode;
    t.resolved = resolve(mode);
    try {
      localStorage.setItem(STORAGE_KEY, mode);
    } catch {
      // private browsing — the choice holds for this session only
    }
    apply();
  }

  function sync() {
    const system = systemNow();
    const resolved = resolve(t.mode);
    if (system === t.system && resolved === t.resolved) return;
    t.system = system;
    t.resolved = resolved;
    apply();
  }

  // `addListener` is the Safari < 14 spelling, and an iPhone that old is
  // exactly the device most likely to be left running this on a shelf.
  if (typeof media.addEventListener === "function") media.addEventListener("change", sync);
  else media.addListener?.(sync);

  // The resume path. A frozen PWA misses the `change` event entirely, so
  // coming back to the foreground has to be treated as "the OS may have moved
  // under us". `pageshow` covers a back/forward-cache restore, which likewise
  // resumes a document without re-running any of this.
  const onVisible = () => {
    if (document.visibilityState === "visible") sync();
  };
  document.addEventListener("visibilitychange", onVisible);
  window.addEventListener("pageshow", sync);
  window.addEventListener("focus", sync);

  return {
    get current() { return t.resolved; },
    get mode() { return t.mode; },
    get system() { return t.system; },
    setMode,
    /** The rail's one-tap shortcut. It steps through all three modes rather
     *  than flipping dark/light, because a binary toggle silently threw away
     *  an "Auto" the user had chosen and gave no way back to it from here. */
    cycle() {
      setMode(t.mode === "dark" ? "light" : t.mode === "light" ? "auto" : "dark");
    },
    sync,
    destroy() {
      if (typeof media.removeEventListener === "function") media.removeEventListener("change", sync);
      else media.removeListener?.(sync);
      document.removeEventListener("visibilitychange", onVisible);
      window.removeEventListener("pageshow", sync);
      window.removeEventListener("focus", sync);
    },
  };
}

export const theme = createThemeStore();
