<script lang="ts">
  import Icon from "./Icon.svelte";
  import ConfirmModal from "./ConfirmModal.svelte";
  import { route, theme, data, session } from "../lib/stores.svelte";
  import { api } from "../lib/api";
  import { openModal, modalStack } from "../lib/modal.svelte";
  import { fade } from "svelte/transition";
  import { dur, sheet } from "../lib/motion";
  import { lockBodyScroll, unlockBodyScroll } from "../lib/scroll-lock";
  import type { Route } from "../lib/types";

  async function signOut() {
    moreOpen = false;
    const ok = await openModal<boolean>(ConfirmModal, {
      title: "Sign out?",
      message:
        "You'll need to enter your username and password again to get back in.",
      confirmLabel: "Sign out",
    });
    if (!ok) return;
    try {
      await api.logout();
    } catch {
      /* ignore */
    }
    window.location.reload();
  }

  type NavItem = { route: Route; icon: any; label: string; admin?: boolean };

  // First four are surfaced as primary tabs in the mobile bottom nav.
  // The rest move into the "More" drawer on mobile, but all six show in
  // the desktop sidebar. Items marked `admin` are hidden from non-admin
  // profiles, who only get Dashboard + Devices.
  const PRIMARY_COUNT = 4;
  const allItems: NavItem[] = [
    { route: "dashboard", icon: "home", label: "Home" },
    { route: "sockets", icon: "socket", label: "Devices" },
    { route: "groups", icon: "groups", label: "Groups", admin: true },
    { route: "schedules", icon: "clock", label: "Schedules", admin: true },
    { route: "automations", icon: "automation", label: "Automations", admin: true },
    { route: "floorplan", icon: "map", label: "Floor plan", admin: true },
    { route: "sensors", icon: "sensor", label: "Sensors", admin: true },
    { route: "insights", icon: "chart", label: "Insights", admin: true },
    { route: "activity", icon: "activity", label: "Activity", admin: true },
    { route: "scenes", icon: "scenes", label: "Scenes", admin: true },
    { route: "users", icon: "user", label: "Profiles", admin: true },
    { route: "settings", icon: "settings", label: "Settings", admin: true },
  ];
  const items = $derived(allItems.filter((i) => session.isAdmin || !i.admin));
  const primary = $derived(items.slice(0, PRIMARY_COUNT));
  const overflow = $derived(items.slice(PRIMARY_COUNT));

  let moreOpen = $state(false);
  // When true, the drawer out-transitions are instant so nav-item taps
  // don't leave the backdrop visible over the incoming view.
  let skipTransition = $state(false);

  // Body scroll lock — acquired while the drawer is open so the page
  // underneath can't scroll on iOS overscroll.
  $effect(() => {
    if (moreOpen) {
      lockBodyScroll();
      return () => unlockBodyScroll();
    }
  });

  function closeDrawerInstant() {
    skipTransition = true;
    moreOpen = false;
    requestAnimationFrame(() => { skipTransition = false; });
  }

  // Auto-close the drawer whenever navigation happens.
  $effect(() => {
    // Reading route.current registers the dependency.
    route.current;
    closeDrawerInstant();
  });

  function toggleTheme() {
    theme.toggle();
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === "Escape" && moreOpen) moreOpen = false;
    if (e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return;
    // Don't hijack keys while typing or with a modal open.
    if (modalStack().length > 0) return;
    const el = e.target as HTMLElement | null;
    if (el && (el.isContentEditable ||
        ["INPUT", "TEXTAREA", "SELECT"].includes(el.tagName))) return;

    // Digit keys jump to the matching nav item (1 = first tab, …). The list
    // is already filtered to the profile's allowed routes, so non-admins can
    // only reach what they're permitted to see.
    if (/^[1-9]$/.test(e.key)) {
      const item = items[Number(e.key) - 1];
      if (item) { route.go(item.route); e.preventDefault(); }
      return;
    }
    if (e.key === "t") { toggleTheme(); e.preventDefault(); }
  }

  // True when the active route is one of the overflow items — used to
  // highlight the "More" tab so the user knows where they are.
  const moreActive = $derived(overflow.some((i) => i.route === route.current));

  // ── Drag-to-dismiss for the More drawer ─────────────────────────────
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
        moreOpen = false;
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

  const healthLabel = $derived(
    data.value.health === "ok"
      ? "Connected"
      : data.value.health === "error"
        ? "Backend offline"
        : "Connecting…",
  );
</script>

<svelte:window onkeydown={onKey} />

<aside class="sidebar" aria-label="Primary">
  <div class="brand">
    <div class="mark" aria-hidden="true">
      <Icon name="bolt" size={20} />
    </div>
    <div>
      <div class="name">HomeHub</div>
      <div class="sub">Smart Home</div>
    </div>
  </div>

  <!-- Desktop: full list. Mobile: only the primary slice (the rest live in
         the More drawer). -->
  <nav class="nav nav-desktop" aria-label="Sections">
    {#each items as item, i (item.route)}
      <a
        href="#/{item.route}"
        class="nav-item"
        aria-current={route.current === item.route ? "page" : undefined}
      >
        <Icon name={item.icon} size={18} />
        <span class="nav-label">{item.label}</span>
        {#if i < 9}<kbd class="nav-key" aria-hidden="true">{i + 1}</kbd>{/if}
      </a>
    {/each}
  </nav>

  <nav class="nav nav-mobile" aria-label="Sections">
    {#each primary as item (item.route)}
      <a
        href="#/{item.route}"
        class="nav-item"
        aria-current={route.current === item.route ? "page" : undefined}
      >
        <Icon name={item.icon} size={20} />
        <span class="nav-label">{item.label}</span>
      </a>
    {/each}
    <button
      class="nav-item more-btn"
      aria-haspopup="menu"
      aria-expanded={moreOpen}
      aria-current={moreActive && !moreOpen ? "page" : undefined}
      onclick={() => (moreOpen = !moreOpen)}
    >
      <Icon name="more" size={20} />
      <span class="nav-label">More</span>
    </button>
  </nav>

  <div class="footer">
    {#if session.user?.username}
      <div class="profile" title={session.user.username}>
        <Icon name={session.user.admin ? "settings" : "socket"} size={14} />
        <span class="profile-name">{session.user.username}</span>
        {#if session.user.admin}<span class="profile-tag">Admin</span>{/if}
      </div>
    {/if}
    <button
      class="theme-toggle"
      aria-label="Toggle theme"
      onclick={toggleTheme}
    >
      <Icon name={theme.current === "dark" ? "moon" : "sun"} size={14} />
      <span>Theme</span>
    </button>
    <button class="theme-toggle" aria-label="Sign out" onclick={signOut}>
      <Icon name="logout" size={14} />
      <span>Sign out</span>
    </button>
    <div class="health" aria-live="polite">
      <span class="dot" data-state={data.value.health}></span>
      <span class="label">{healthLabel}</span>
    </div>
  </div>
</aside>

<!-- Mobile-only overflow drawer (bottom sheet). -->
{#if moreOpen}
  <div
    class="drawer-root"
    role="presentation"
    onclick={(e) => {
      if (e.target === e.currentTarget) moreOpen = false;
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
        {#each overflow as item (item.route)}
          <a
            href="#/{item.route}"
            class="drawer-item"
            role="menuitem"
            aria-current={route.current === item.route ? "page" : undefined}
            onclick={closeDrawerInstant}
          >
            <span class="drawer-icon"><Icon name={item.icon} size={20} /></span>
            <span class="drawer-label">{item.label}</span>
          </a>
        {/each}
      </div>

      <div class="drawer-section" aria-label="Settings">
        <button
          class="drawer-item"
          role="menuitem"
          onclick={() => {
            toggleTheme();
          }}
        >
          <span class="drawer-icon">
            <Icon name={theme.current === "dark" ? "sun" : "moon"} size={20} />
          </span>
          <span class="drawer-label">
            {theme.current === "dark" ? "Light theme" : "Dark theme"}
          </span>
        </button>
        <button class="drawer-item danger" role="menuitem" onclick={signOut}>
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
{/if}

<style>
  .sidebar {
    width: 240px;
    background: var(--bg-2);
    border-right: 1px solid var(--hairline);
    padding: 22px 16px;
    display: flex;
    flex-direction: column;
    gap: 4px;
    position: sticky;
    top: 0;
    height: 100vh;
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 4px 12px 22px;
    margin-bottom: var(--space-2);
  }
  .mark {
    width: 28px;
    height: 28px;
    border-radius: 8px;
    background: var(--on);
    display: grid;
    place-items: center;
    color: var(--bg);
    flex-shrink: 0;
  }
  .name {
    font-size: 15px;
    font-weight: 600;
    letter-spacing: -0.02em;
    color: var(--text);
  }
  .sub {
    font-size: 10.5px;
    font-family: var(--font-mono);
    color: var(--text-mute);
  }

  .nav {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .nav-mobile {
    display: none;
  }
  .nav-item {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 10px 12px;
    border-radius: var(--r-sm);
    color: var(--text-mute);
    font-size: 13.5px;
    font-weight: 500;
    transition:
      background 150ms ease,
      color 150ms ease;
    cursor: pointer;
    background: transparent;
    border: none;
    text-align: left;
    font-family: var(--font-sans);
    width: 100%;
  }
  .nav-item:hover {
    background: var(--card);
    color: var(--text);
  }
  .nav-item[aria-current="page"] {
    background: var(--card);
    color: var(--text);
    font-weight: 500;
  }
  .nav-item[aria-current="page"] :global(svg) {
    color: var(--on);
  }
  .nav-key {
    margin-left: auto;
    font-family: var(--font-mono);
    font-size: 10px;
    line-height: 1;
    padding: 2px 6px;
    border-radius: 6px;
    color: var(--text-mute);
    background: var(--card-3);
    opacity: 0;
    transition: opacity 150ms ease;
  }
  .sidebar:hover .nav-key { opacity: 1; }
  .nav-item[aria-current="page"] .nav-key { opacity: 1; color: var(--text-mute); }

  .footer {
    margin-top: auto;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding-top: var(--space-3);
  }
  .profile {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 12px;
    background: var(--card);
    border: 1px solid var(--hairline);
    border-radius: var(--r-md);
    color: var(--text-mute);
    font-size: 12.5px;
    min-width: 0;
  }
  .profile :global(svg) {
    color: var(--on);
    flex-shrink: 0;
  }
  .profile-name {
    font-weight: 500;
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex: 1;
    min-width: 0;
  }
  .profile-tag {
    font-size: 10px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--on);
    background: var(--on-soft);
    padding: 1px 6px;
    border-radius: var(--r-pill);
    flex-shrink: 0;
  }
  .theme-toggle {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 9px 12px;
    border: 1px solid var(--hairline);
    background: var(--card-2);
    border-radius: var(--r-sm);
    cursor: pointer;
    color: var(--text-mute);
    font-size: 13px;
    transition:
      background 150ms ease,
      color 150ms ease;
  }
  .theme-toggle:hover {
    background: var(--card-3);
    color: var(--text);
  }

  .health {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--text-mute);
    font-size: 11.5px;
    padding: 4px 12px 0;
  }
  .dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--text-dim);
    flex-shrink: 0;
    transition:
      background 200ms ease,
      box-shadow 200ms ease;
  }
  .dot[data-state="ok"] {
    background: var(--good);
    box-shadow: 0 0 0 4px var(--on-soft);
    animation: pulse-dot 2.5s ease-in-out infinite;
  }
  .dot[data-state="error"] {
    background: var(--bad);
    box-shadow: 0 0 0 3px rgba(224,138,122,0.18);
  }
  @keyframes pulse-dot {
    0%,
    100% {
      box-shadow: 0 0 0 3px rgba(156,194,138,0.18);
    }
    50% {
      box-shadow: 0 0 0 5px rgba(156,194,138,0.18);
    }
  }

  /* ---------- Mobile bottom nav — warm-dark tab bar ---------- */
  @media (max-width: 900px) {
    .sidebar {
      width: auto;
      position: fixed;
      bottom: 0;
      left: 0;
      right: 0;
      top: auto;
      height: auto;
      flex-direction: row;
      align-items: stretch;
      border-right: none;
      /* Warm-dark bar fading up into the page, matching the prototype tabbar */
      background: linear-gradient(to top, var(--bg) 55%, rgba(20,19,15,0.85));
      backdrop-filter: saturate(160%) blur(20px);
      -webkit-backdrop-filter: saturate(160%) blur(20px);
      border-top: 1px solid var(--hairline);
      box-shadow: none;
      padding: 8px 14px;
      padding-bottom: calc(8px + env(safe-area-inset-bottom));
      z-index: 100;
      gap: 0;
    }
    .brand {
      display: none;
    }
    .footer {
      display: none;
    }
    .nav-desktop {
      display: none;
    }
    .nav-mobile {
      display: flex;
      flex: 1;
      flex-direction: row;
      justify-content: space-around;
      gap: 0;
    }
    .nav-mobile .nav-item {
      flex: 1;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      gap: 4px;
      padding: 6px 10px;
      border-radius: 0;
      font-size: 10.5px;
      font-weight: 500;
      letter-spacing: 0.02em;
      text-transform: uppercase;
      color: var(--text-dim);
      text-align: center;
      width: auto;
    }
    .nav-mobile .nav-item:hover {
      background: transparent;
      color: var(--text-mute);
    }
    .nav-mobile .nav-item :global(svg) {
      width: 22px;
      height: 22px;
      /* Spring easing gives the icon a subtle pop when a tab is selected. */
      transition: transform 0.28s var(--spring);
    }
    /* Active = amber tint, no indicator line */
    .nav-mobile .nav-item[aria-current="page"] {
      background: transparent;
      color: var(--on);
      box-shadow: none;
    }
    .nav-mobile .nav-item[aria-current="page"] :global(svg) {
      color: var(--on);
      transform: scale(1.1);
    }
    /* Quick dip on press for tactile feedback. */
    .nav-mobile .nav-item:active :global(svg) {
      transform: scale(0.9);
      transition-duration: 90ms;
    }
    .nav-mobile .nav-label {
      line-height: 1;
      letter-spacing: 0.02em;
      text-transform: uppercase;
    }
  }

  /* ---------- Drawer (bottom sheet) ---------- */
  .drawer-root {
    position: fixed;
    inset: 0;
    background: rgba(10, 10, 8, 0.6);
    backdrop-filter: blur(3px);
    z-index: 120;
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
    padding: 0 var(--space-4)
      calc(var(--space-4) + 56px + env(safe-area-inset-bottom));
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
