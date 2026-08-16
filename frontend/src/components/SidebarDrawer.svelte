<script lang="ts">
  /**
   * The "More" drawer: the bottom sheet behind the tab bar's last slot,
   * holding the routes that did not fit and the actions that are not routes
   * at all — the assistant, the theme, signing out.
   *
   * Mobile-only, which is why it is a sheet rather than a side drawer (§2).
   * It is drag-to-dismiss from two entry points, and that is the reason it
   * is worth a file of its own: the handle row always drags, while the
   * surface itself only starts dragging once the gesture clears an intent
   * threshold, so a tap on a drawer item still registers as a click rather
   * than as a one-pixel drag.
   */
  import Icon from "./Icon.svelte";
  import { fade } from "svelte/transition";
  import { dur, sheet } from "../lib/motion";
  import { route, data, session, assistant } from "../lib/stores.svelte";
  import type { Route } from "../lib/types";

  /** The nav rows the sidebar builds — same shape both navs draw from. */
  type NavItem = { route: Route; icon: any; label: string; admin?: boolean };

  let {
    overflow,
    badgeFor,
    themeIcon,
    themeLabel,
    healthLabel,
    skipTransition = false,
    onToggleTheme,
    onSignOut,
    onClose,
  }: {
    /** The routes the tab bar had no slot for. */
    overflow: NavItem[];
    /** The count a row wears, already formatted; null for no badge. */
    badgeFor: (r: Route) => string | null;
    themeIcon: any;
    themeLabel: string;
    healthLabel: string;
    /** Close without animating — the sheet is going because the app is
     *  navigating, and a sheet sliding out over a new screen reads as the
     *  new screen arriving badly. */
    skipTransition?: boolean;
    onToggleTheme: () => void;
    onSignOut: () => void;
    onClose: () => void;
  } = $props();

  function closeDrawerInstant() {
    onClose();
  }


  // Two entry points:
  //   - The handle row: always drags (touch-action: none).
  //   - The drawer surface itself: drags only after the gesture clears an
  //     intent threshold, so taps on drawer items still register as clicks.
  let drawerDragY = $state(0);
  let drawerDragging = $state(false);
  let drawerDismissing = $state(false);
  let drawerPending = false;
  let drawerDragStartY = 0;
  let drawerDragStartX = 0;

  function startDrawerDrag(e: PointerEvent, target: HTMLElement) {
    drawerDragging = true;
    drawerDragStartY = e.clientY;
    drawerDragStartX = e.clientX;
    drawerDragY = 0;
    try { target.setPointerCapture(e.pointerId); } catch { /* not capturable */ }
  }

  // Handle row — always drags.
  function onHandlePointerDown(e: PointerEvent) {
    if (drawerDismissing) return;
    startDrawerDrag(e, e.currentTarget as HTMLElement);
    e.preventDefault();
  }
  function onHandlePointerMove(e: PointerEvent) {
    if (!drawerDragging) return;
    drawerDragY = Math.max(0, e.clientY - drawerDragStartY);
  }
  function onHandlePointerUp() { finishDrawerDrag(); }
  function onHandlePointerCancel() { cancelDrawerDrag(); }

  // Drawer surface — drags after intent threshold; otherwise allows clicks.
  function onSurfacePointerDown(e: PointerEvent) {
    if (drawerDismissing) return;
    if (e.pointerType === "mouse") return; // surface drag is touch only
    drawerPending = true;
    drawerDragStartY = e.clientY;
    drawerDragStartX = e.clientX;
  }
  function onSurfacePointerMove(e: PointerEvent) {
    if (drawerDragging) {
      drawerDragY = Math.max(0, e.clientY - drawerDragStartY);
      e.preventDefault();
      return;
    }
    if (!drawerPending) return;
    const dy = e.clientY - drawerDragStartY;
    const dx = e.clientX - drawerDragStartX;
    if (dy > 8 && dy > Math.abs(dx)) {
      drawerPending = false;
      startDrawerDrag(e, e.currentTarget as HTMLElement);
      drawerDragY = dy;
      e.preventDefault();
    } else if (dy < -4 || Math.abs(dx) > 12) {
      drawerPending = false;
    }
  }
  function onSurfacePointerUp() {
    drawerPending = false;
    if (drawerDragging) finishDrawerDrag();
  }
  function onSurfacePointerCancel() {
    drawerPending = false;
    if (drawerDragging) cancelDrawerDrag();
  }

  function finishDrawerDrag() {
    if (!drawerDragging) return;
    drawerDragging = false;
    if (drawerDragY > 90) {
      drawerDismissing = true;
      drawerDragY = 600;
      setTimeout(() => {
        onClose();
        drawerDragY = 0;
        drawerDismissing = false;
      }, 220);
    } else {
      requestAnimationFrame(() => { drawerDragY = 0; });
    }
  }
  function cancelDrawerDrag() {
    if (!drawerDragging) return;
    drawerDragging = false;
    requestAnimationFrame(() => { drawerDragY = 0; });
  }
</script>

  <div
    class="drawer-root"
    role="presentation"
    onclick={(e) => {
      if (e.target === e.currentTarget) onClose();
    }}
    in:fade={{ duration: dur(180) }}
    out:fade={skipTransition ? { duration: 0 } : { duration: dur(200) }}
  >
    <div
      class="drawer"
      class:dragging={drawerDragging}
      role="menu"
      tabindex="-1"
      aria-label="More options"
      style:transform={drawerDragY > 0 ? `translateY(${drawerDragY}px)` : ''}
      style:opacity={drawerDragY > 0 ? Math.max(0.4, 1 - drawerDragY / 300) : undefined}
      style:transition={drawerDragging ? 'none' : drawerDragY > 0 ? 'transform 0.22s ease-in, opacity 0.22s ease-in' : 'transform 0.3s cubic-bezier(0.34, 1.56, 0.64, 1)'}
      in:sheet={{ duration: 340, mode: "slide" }}
      out:sheet={{ instant: skipTransition || drawerDismissing, duration: 260, mode: "slide" }}
      onpointerdown={onSurfacePointerDown}
      onpointermove={onSurfacePointerMove}
      onpointerup={onSurfacePointerUp}
      onpointercancel={onSurfacePointerCancel}
    >
      <div class="drawer-handle-zone"
        role="presentation"
        onpointerdown={onHandlePointerDown}
        onpointermove={onHandlePointerMove}
        onpointerup={onHandlePointerUp}
        onpointercancel={onHandlePointerCancel}>
        <div class="drawer-handle" aria-hidden="true"></div>
      </div>

      <div class="drawer-section" aria-label="Sections">
        <!-- The assistant isn't a route, so it has no tab-bar slot. It lives
             here as well as on the FAB — that button is optional (Settings),
             and the feature must not go with it. -->
        {#if session.isAdmin}
          <button
            class="drawer-item"
            role="menuitem"
            onclick={() => { closeDrawerInstant(); assistant.show(); }}
          >
            <span class="drawer-icon"><Icon name="assistant" size={20} /></span>
            <span class="drawer-label">Assistant</span>
          </button>
        {/if}
        {#each overflow as item (item.route)}
          {@const badge = badgeFor(item.route)}
          <a
            href="#/{item.route}"
            class="drawer-item"
            role="menuitem"
            aria-current={route.current === item.route ? "page" : undefined}
            onclick={closeDrawerInstant}
          >
            <span class="drawer-icon"><Icon name={item.icon} size={20} /></span>
            <span class="drawer-label">{item.label}</span>
            {#if badge}<span class="drawer-badge mono">{badge}</span>{/if}
          </a>
        {/each}
      </div>

      <div class="drawer-section" aria-label="Settings">
        <button
          class="drawer-item"
          role="menuitem"
          onclick={() => {
            onToggleTheme();
          }}
        >
          <span class="drawer-icon">
            <Icon name={themeIcon} size={20} />
          </span>
          <span class="drawer-label">{themeLabel}</span>
        </button>
        <button class="drawer-item danger" role="menuitem" onclick={onSignOut}>
          <span class="drawer-icon"><Icon name="logout" size={20} /></span>
          <span class="drawer-label">Sign out</span>
        </button>
      </div>

      <div class="drawer-health" aria-live="polite">
        <span class="dot" data-state={data.value.health}></span>
        <span>{healthLabel}</span>
      </div>
    </div>
  </div>

<style>
  /* ---------- Drawer (bottom sheet) ---------- */
  .drawer-root {
    position: fixed;
    inset: 0;
    background: rgba(10, 10, 8, 0.6);
    backdrop-filter: blur(3px);
    z-index: var(--z-scrim);
    display: flex;
    align-items: flex-end;
    justify-content: center;
    /* Don't let any overscroll bubble out to the page underneath. */
    overscroll-behavior: contain;
  }
  :global([data-theme="light"]) .drawer-root {
    background: rgba(40, 34, 24, 0.35);
  }
  .drawer {
    width: 100%;
    background: var(--card);
    backdrop-filter: saturate(180%) blur(24px);
    -webkit-backdrop-filter: saturate(180%) blur(24px);
    border-top: 1px solid var(--hairline);
    border-top-left-radius: var(--r-xl);
    border-top-right-radius: var(--r-xl);
    padding: 0 var(--space-4) calc(var(--space-4) + var(--nav-clear));
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    box-shadow: var(--shadow-lg);
    /* The drawer surface itself gets pointer events so a downward swipe
       anywhere on it can dismiss; touch-action: pan-y keeps native
       horizontal gestures (back-swipe) out of our way. */
    touch-action: pan-y;
    will-change: transform;
  }
  .drawer.dragging { cursor: grabbing; }
  .drawer-handle-zone {
    /* Generous tap area around the pill — the whole strip is grabbable. */
    width: 100%;
    padding: var(--space-3) 0 var(--space-2);
    display: flex;
    justify-content: center;
    align-items: center;
    touch-action: none;
    cursor: grab;
  }
  .drawer-handle-zone:active { cursor: grabbing; }
  .drawer-handle {
    width: 40px;
    height: 5px;
    border-radius: var(--r-pill);
    background: var(--card-3);
    pointer-events: none;
  }
  .drawer-section {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: var(--space-1) 0;
  }
  .drawer-section + .drawer-section {
    border-top: 1px solid var(--hairline);
    padding-top: var(--space-2);
    margin-top: var(--space-1);
  }
  .drawer-item {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: 14px var(--space-3);
    border-radius: var(--r-md);
    color: var(--text);
    background: transparent;
    border: none;
    cursor: pointer;
    font: inherit;
    text-align: left;
    width: 100%;
    transition:
      background 150ms ease,
      color 150ms ease;
  }
  .drawer-item:hover {
    background: var(--card-2);
  }
  .drawer-item:active {
    background: var(--card-3);
  }
  .drawer-item[aria-current="page"] {
    background: var(--card-2);
    color: var(--on);
    font-weight: 600;
  }
  .drawer-item[aria-current="page"] :global(svg) {
    color: var(--on);
  }
  .drawer-item.danger {
    color: var(--bad);
  }
  .drawer-icon {
    width: 28px;
    display: grid;
    place-items: center;
    color: var(--text-mute);
  }
  .drawer-item[aria-current="page"] .drawer-icon,
  .drawer-item.danger .drawer-icon {
    color: inherit;
  }
  .drawer-label {
    font-size: 15px;
    flex: 1;
    min-width: 0;
  }
  .drawer-badge {
    font-size: 11px;
    color: var(--text-mute);
    background: var(--card-2);
    border: 1px solid var(--hairline);
    padding: 2px 8px;
    border-radius: var(--r-pill);
    flex-shrink: 0;
  }
  .drawer-item[aria-current="page"] .drawer-badge {
    color: var(--on);
    border-color: var(--tile-on-border);
    background: var(--on-soft);
  }
  .drawer-health {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--text-mute);
    font-size: 12px;
    padding: var(--space-2) var(--space-3) 0;
    border-top: 1px solid var(--hairline);
    margin-top: var(--space-1);
  }


  /* Hide the drawer entirely on desktop — it's a mobile-only affordance. */
  @media (min-width: 901px) {
    .drawer-root {
      display: none;
    }
  }
</style>
