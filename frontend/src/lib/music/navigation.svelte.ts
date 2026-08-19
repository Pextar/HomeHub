/**
 * Where the Music view is, and what "back" means from there.
 *
 * The view has two ways of going deeper and they interlock, which is why
 * this was the largest single mechanism in a 843-line component script:
 *
 *   **Screens** push over Home in a real stack — Browse opens an artist, the
 *   artist opens an album — so back means *up one level*, never all the way
 *   home. Only the top of the stack renders, and what each level was scrolled
 *   to rides on its entry.
 *
 *   **Sheets** lift from the bottom and never stack (DESIGN.md §2): a sheet
 *   that would open another swaps to it instead, and dismissing puts the
 *   first one back where it was scrolled to.
 *
 * On top of both sits one browser history entry, held for as long as the
 * view is deeper than Home and re-taken after each step back, so the phone's
 * back gesture means exactly what Escape and the back chip mean. Getting
 * that wrong strands the entry: the view leaves, the entry stays, and the
 * next back press goes nowhere.
 *
 * The three used to be tangled through the component with the player, the
 * grouping drag and six modal flows. They are separable because none of them
 * needs to know *what* a screen shows — only that one is on top of another —
 * and separating them is what lets the rules above be tested at all.
 *
 * What stays with the view: what each screen renders, and what a sheet is
 * bound to while it is up. Both come back through the hooks below.
 */

import * as sheetRun from "../sheet-run";
import type { SheetRun } from "../sheet-run";
import { lockBodyScroll, unlockBodyScroll } from "../scroll-lock";
import { settleScroll, restoreScroll, toTop } from "./scroll";

/** Home is the absence of a screen, so it isn't one of these. */
export type MusicScreen = "speakers" | "artist" | "favorite" | "browse" | "context";
export type MusicSheet = "player" | "room-edit";

export interface ScreenEntry {
  id: MusicScreen;
  /** Catalog screens: the artist / album / playlist URI they show. */
  uri?: string;
  /** Where this level was scrolled when something pushed over it. */
  scroll: number;
}

export interface MusicNavDeps {
  /** The open sheet's scroll container — a getter, since it mounts with the
   *  sheet and is gone again when it leaves. */
  sheetScrollEl: () => HTMLElement | null;
  /** The room key the player sheet is bound to, or null. Read at the moment
   *  a screen pushes, to note where to hand back to on the way out. */
  playerKey: () => string | null;
  /**
   * The stack ran out and a player had noted a room to come back to. Answer
   * false when that room is gone — regrouped, removed — and the way out
   * falls through to Home like any other missing target.
   */
  reopenPlayer: (key: string) => boolean;
  /** A popped screen gives up its scratch state. */
  onLeftScreen?: (e: ScreenEntry) => void;
  /**
   * Which sheet is showing now, after any of them closed. The view drops
   * whatever the ones that left were bound to. Not called when a sheet is
   * *raised* — nothing has stopped being shown.
   */
  onSheetsClosed?: (showing: MusicSheet | null) => void;
}

export interface MusicNav {
  /** The screen on top, or "home" when the stack is empty. */
  readonly screen: "home" | MusicScreen;
  readonly top: ScreenEntry | undefined;
  readonly sheets: SheetRun<MusicSheet>;
  readonly openSheet: MusicSheet | null;
  /** A sheet is on screen — which is not the same as one being *open*, since
   *  one rides out its dismissal after it stops being open. */
  readonly sheetUp: boolean;
  /** How many levels deep, screens and sheets together. Zero means Home with
   *  nothing over it, which is the only state that holds no history entry. */
  readonly depth: number;

  /** Push a screen over whatever is showing, closing any sheet first. */
  pushScreen(e: ScreenEntry): void;
  /** Up one level. Hands back to a noted player when the stack runs out. */
  leaveScreen(): void;
  /** Show a sheet, swapping out whatever is there and remembering where it
   *  was scrolled to. */
  swapSheet(sheet: MusicSheet): void;
  /** Dismiss the top sheet, putting back the one underneath it. */
  dropSheet(): void;
  /** Close every sheet at once. */
  hideSheet(): void;
  /** The browser's back button came back to us. */
  onPopState(): void;
}

export function createMusicNav(deps: MusicNavDeps): MusicNav {
  let stack = $state<ScreenEntry[]>([]);
  const screen = $derived<"home" | MusicScreen>(
    stack.length ? stack[stack.length - 1].id : "home",
  );
  const top = $derived(stack.length ? stack[stack.length - 1] : undefined);

  /** Where Home was left, so coming back lands where you were. */
  let homeScrollY = 0;

  /**
   * The room to hand back to on the way out of a screen reached from that
   * room's open player — Browse, or an artist tapped inside it. Noted from
   * whether the player was up at the moment of the push, so it takes no
   * per-caller wiring and survives going deeper.
   */
  let playerReturn: string | null = null;

  let sheets = $state<SheetRun<MusicSheet>>(sheetRun.closed());
  const openSheet = $derived(sheets.open);
  const sheetUp = $derived(sheetRun.isUp(sheets));

  /** How far each sheet was scrolled when it handed over. */
  const sheetScroll: Partial<Record<MusicSheet, number>> = {};
  function rememberSheetScroll() {
    if (sheets.open) sheetScroll[sheets.open] = deps.sheetScrollEl()?.scrollTop ?? 0;
  }

  const depth = $derived(
    (screen !== "home" ? 1 : 0) + (sheets.open ? (sheets.under ? 2 : 1) : 0),
  );

  // One history entry for the whole time the view is deeper than Home, and
  // re-taken after each step back while depth remains.
  let holdsEntry = false;
  function takeEntry() {
    if (holdsEntry) return;
    history.pushState({ musicNav: true }, "");
    holdsEntry = true;
  }
  $effect(() => {
    if (depth > 0) takeEntry();
    else if (holdsEntry) {
      holdsEntry = false;
      history.back();
    }
  });

  // The body-scroll lock keys on *whether* a sheet is up, never on which — so
  // a swap doesn't release and retake it, which on iOS would unpin and re-pin
  // the body for a frame.
  $effect(() => {
    if (!sheetUp) return;
    lockBodyScroll();
    return unlockBodyScroll;
  });

  function hideSheet() {
    if (!sheetUp) return;
    sheets = sheetRun.closeAll(sheets);
    deps.onSheetsClosed?.(null);
  }

  function dropSheet() {
    if (!sheetUp) return;
    const back = sheetRun.dismiss(sheets);
    sheets = back;
    if (back.open) settleScroll(deps.sheetScrollEl, sheetScroll[back.open] ?? 0);
    deps.onSheetsClosed?.(back.open);
  }

  function pushScreen(e: ScreenEntry) {
    if (sheets.open === "player") playerReturn = deps.playerKey();
    hideSheet();
    if (stack.length === 0) homeScrollY = window.scrollY;
    else stack[stack.length - 1].scroll = window.scrollY;
    stack = [...stack, e];
    toTop();
  }

  function leaveScreen() {
    const leaving = stack[stack.length - 1];
    if (!leaving) return;
    deps.onLeftScreen?.(leaving);
    if (stack.length > 1) {
      stack = stack.slice(0, -1);
      restoreScroll(stack[stack.length - 1].scroll);
      return;
    }
    stack = [];
    if (playerReturn) {
      const back = playerReturn;
      playerReturn = null;
      if (deps.reopenPlayer(back)) return;
      // The room disappeared in the meantime — fall through to Home like any
      // other missing target.
    }
    restoreScroll(homeScrollY);
  }

  return {
    get screen() {
      return screen;
    },
    get top() {
      return top;
    },
    get sheets() {
      return sheets;
    },
    get openSheet() {
      return openSheet;
    },
    get sheetUp() {
      return sheetUp;
    },
    get depth() {
      return depth;
    },

    pushScreen,
    leaveScreen,
    dropSheet,
    hideSheet,

    swapSheet(sheet) {
      rememberSheetScroll();
      sheets = sheetRun.swapTo(sheets, sheet);
      // The sheet arriving opens at its own top; the one it replaced keeps
      // the offset just remembered, for when it comes back.
      sheetScroll[sheet] = 0;
    },

    onPopState() {
      if (depth === 0) return; // not our entry — a real route change
      holdsEntry = false; // the browser has consumed it
      if (sheetUp) dropSheet();
      else if (screen !== "home") leaveScreen();
      // Take another one if the view is still deeper than Home. This has to
      // happen here rather than in the effect above: stepping artist → browse
      // leaves `depth` at 1, so the effect has nothing to re-run on, and
      // `holdsEntry` is deliberately not reactive (an effect that both reads
      // and writes it would loop). Without this the entry was consumed and
      // never replaced, and the *next* back press left the view instead of
      // climbing the level it was aimed at.
      if (depth > 0) takeEntry();
    },
  };
}
