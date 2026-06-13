<script lang="ts">
  import Icon from "./Icon.svelte";
  import ConfirmModal from "./ConfirmModal.svelte";
  import { route, theme, data, session, sidebar } from "../lib/stores.svelte";
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
    { route: "assistant", icon: "assistant", label: "Assistant", admin: true },
    { route: "groups", icon: "groups", label: "Groups", admin: true },
    { route: "automations", icon: "automation", label: "Automations", admin: true },
    { route: "rooms", icon: "couch", label: "Rooms", admin: true },
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
    if (e.key === "[") { sidebar.toggle(); e.preventDefault(); }
    if ((e.metaKey || e.ctrlKey) && e.key === "k") { route.go("sockets"); e.preventDefault(); }
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

<aside class="sidebar" class:collapsed={sidebar.collapsed} aria-label="Primary">
  <div class="brand">
    <!-- In collapsed mode the chevron button disappears and this mark
         becomes the sole expand trigger, so make it a real button always. -->
    <button
      class="mark"
      onclick={() => sidebar.toggle()}
      aria-label={sidebar.collapsed ? "Expand sidebar" : "Collapse sidebar"}
    >
      <Icon name="bolt" size={20} />
    </button>
    <div class="brand-text">
      <div class="name">HomeHub</div>
      <div class="sub">Smart Home</div>
    </div>
    <!-- Flows as the last flex item; transitions to width:0 in collapsed
         mode so it takes no space and never overlaps the mark. -->
    <button
      class="collapse-btn"
      tabindex={sidebar.collapsed ? -1 : 0}
      aria-label="Collapse sidebar"
      onclick={() => sidebar.toggle()}
    >
      <Icon name="chevronLeft" size={16} />
    </button>
  </div>

  <!-- Desktop: full list. Mobile: only the primary slice (the rest live in
         the More drawer). -->
  <nav class="nav nav-desktop" aria-label="Sections">
    {#each items as item, i (item.route)}
      <a
        href="#/{item.route}"
        class="nav-item"
        data-label={item.label}
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
        <span class="nav-icon"><Icon name={item.icon} size={20} /></span>
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
      <span class="nav-icon"><Icon name="more" size={20} /></span>
      <span class="nav-label">More</span>
    </button>
  </nav>

  <div class="footer">
    {#if session.user?.username}
      <div class="profile-card" title={session.user.username}>
        <div class="profile-avatar">{session.user.username.trim().charAt(0).toUpperCase() || "?"}</div>
        <div class="profile-info">
          <span class="profile-name">{session.user.username}</span>
          <span class="profile-role">{session.user.admin ? "Admin" : "Limited"}</span>
        </div>
        <div class="profile-btns">
          <button class="profile-btn" aria-label="Toggle theme" onclick={toggleTheme}>
            <Icon name={theme.current === "dark" ? "moon" : "sun"} size={13} />
          </button>
          <button class="profile-btn" aria-label="Sign out" onclick={signOut}>
            <Icon name="logout" size={13} />
          </button>
        </div>
      </div>
    {/if}
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
  /* ============================================================
     Desktop sidebar — expanded (240px) and collapsed (64px)
     ============================================================ */

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
    /* Drives the flex layout in App.svelte — as width transitions the
       main content area expands/contracts to fill the remaining space. */
    transition: width 280ms cubic-bezier(0.4, 0, 0.2, 1);
    /* overflow:visible (default) so the tooltip ::after can escape
       the sidebar's boundary in collapsed mode. */
  }
  .sidebar.collapsed {
    /* 64px = sidebar padding (16×2) + 18px icon + 14px breathing room */
    width: 64px;
  }

  /* ── Brand ──────────────────────────────────────────────── */
  .brand {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 4px 0 22px;       /* horizontal handled by sidebar padding */
    margin-bottom: var(--space-2);
    /* Transition gap (removes space between mark and invisible elements)
       and padding-left (slides mark to centre when collapsed). */
    transition: gap 280ms cubic-bezier(0.4, 0, 0.2, 1),
                padding-left 280ms cubic-bezier(0.4, 0, 0.2, 1);
  }
  /* Centre the mark icon in the 32 px inner rail:
     inner = 64px sidebar − 16px×2 padding = 32px
     mark = 28px  →  offset = (32−28)/2 = 2px                 */
  .sidebar.collapsed .brand {
    gap: 0;
    padding-left: 2px;
  }

  /* Mark: button-reset + amber square.  Acts as collapse trigger in
     expanded mode and expand trigger once the chevron has hidden. */
  .mark {
    width: 28px;
    height: 28px;
    border-radius: 8px;
    background: var(--on);
    display: grid;
    place-items: center;
    color: var(--bg);
    flex-shrink: 0;
    /* button reset */
    border: none;
    padding: 0;
    cursor: pointer;
    font: inherit;
    transition: box-shadow 150ms ease, opacity 150ms ease;
  }
  .mark:hover {
    box-shadow: 0 0 0 3px var(--on-soft);
  }
  .mark:active {
    opacity: 0.8;
  }

  .brand-text {
    flex: 1;
    min-width: 0;
    overflow: hidden;          /* keeps text from bleeding during transition */
  }
  .name {
    font-size: 15px;
    font-weight: 600;
    letter-spacing: -0.02em;
    color: var(--text);
    white-space: nowrap;
    transition: opacity 180ms ease;
  }
  .sub {
    font-size: 10.5px;
    font-family: var(--font-mono);
    color: var(--text-mute);
    white-space: nowrap;
    transition: opacity 180ms ease;
  }
  .sidebar.collapsed .name,
  .sidebar.collapsed .sub { opacity: 0; }

  /* Chevron button — flows as the last flex item (margin-left:auto pushes
     it to the right edge in expanded mode).  Collapses to width:0 when
     the sidebar is collapsed so it takes NO layout space and can never
     overlap the mark icon. */
  .collapse-btn {
    margin-left: auto;
    min-width: 0;              /* lets width transition below zero */
    width: 28px;
    height: 28px;
    overflow: hidden;
    display: grid;
    place-items: center;
    border: 1px solid var(--hairline);
    background: transparent;
    border-radius: var(--r-sm);
    cursor: pointer;
    color: var(--text-mute);
    flex-shrink: 0;
    transition:
      width    280ms cubic-bezier(0.4, 0, 0.2, 1),
      opacity  200ms ease,
      background 150ms ease,
      color    150ms ease;
  }
  .collapse-btn:hover {
    background: var(--card-3);
    color: var(--text);
  }
  /* In collapsed mode: shrink to nothing — zero width, zero opacity */
  .sidebar.collapsed .collapse-btn {
    width: 0;
    opacity: 0;
    pointer-events: none;
  }

  /* ── Nav ────────────────────────────────────────────────── */
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
      color 150ms ease,
      padding 280ms cubic-bezier(0.4, 0, 0.2, 1),
      gap 280ms cubic-bezier(0.4, 0, 0.2, 1);
    cursor: pointer;
    background: transparent;
    border: none;
    text-align: left;
    font-family: var(--font-sans);
    width: 100%;
    /* Needed for the collapsed tooltip ::after */
    position: relative;
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

  /* Label: fade out and collapse width so the icon centres in the rail */
  .nav-label {
    overflow: hidden;
    white-space: nowrap;
    /* max-width collapses the label's contribution to the flex row,
       letting justify-content:center pull the icon to the middle. */
    max-width: 180px;
    transition: max-width 280ms cubic-bezier(0.4, 0, 0.2, 1), opacity 200ms ease;
  }
  .sidebar.collapsed .nav-label {
    max-width: 0;
    opacity: 0;
  }

  /* Keyboard shortcut badges */
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
    flex-shrink: 0;
  }
  .sidebar:hover .nav-key { opacity: 1; }
  .nav-item[aria-current="page"] .nav-key { opacity: 1; color: var(--text-mute); }
  .sidebar.collapsed .nav-key { display: none; }

  /* Collapse nav item padding → centers the icon in the 32px inner rail */
  .sidebar.collapsed .nav-desktop .nav-item {
    padding-left: 0;
    padding-right: 0;
    gap: 0;
    justify-content: center;
  }

  /* Tooltip — appears to the right of the sidebar in collapsed mode.
     Uses content: attr(data-label) so no extra markup needed.
     left: calc(100% + 20px) puts it 4px past the sidebar's right border
     (inner width 32px + sidebar right padding 16px + gap 4px = 52px from
     the nav item's left edge, 16+52=68px from sidebar left vs 64px wide). */
  .sidebar.collapsed .nav-desktop .nav-item::after {
    content: attr(data-label);
    position: absolute;
    left: calc(100% + 20px);
    top: 50%;
    transform: translateY(-50%);
    padding: 5px 10px;
    background: var(--card-3);
    border: 1px solid var(--hairline);
    border-radius: var(--r-sm);
    color: var(--text);
    font-size: 13px;
    font-weight: 500;
    white-space: nowrap;
    box-shadow: var(--shadow-md);
    opacity: 0;
    transition: opacity 120ms ease;
    pointer-events: none;
    z-index: 200;
  }
  .sidebar.collapsed .nav-desktop .nav-item:hover::after {
    opacity: 1;
  }

  /* ── Footer ─────────────────────────────────────────────── */
  .footer {
    margin-top: auto;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding-top: var(--space-3);
  }

  /* Profile card — avatar + name/role + action icon buttons */
  .profile-card {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 12px;
    background: var(--card);
    border: 1px solid var(--hairline);
    border-radius: var(--r-md);
    min-width: 0;
    transition:
      padding 280ms cubic-bezier(0.4, 0, 0.2, 1),
      gap     280ms cubic-bezier(0.4, 0, 0.2, 1);
  }
  .sidebar.collapsed .profile-card {
    justify-content: center;
    padding: 10px 0;
    gap: 0;
  }

  .profile-avatar {
    width: 30px;
    height: 30px;
    border-radius: 50%;
    background: var(--card-3);
    display: grid;
    place-items: center;
    font-family: var(--font-mono);
    font-weight: 600;
    font-size: 12px;
    color: var(--on);
    flex-shrink: 0;
  }

  .profile-info {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 1px;
    overflow: hidden;
    max-width: 140px;
    transition: max-width 280ms cubic-bezier(0.4, 0, 0.2, 1), opacity 200ms ease;
  }
  .sidebar.collapsed .profile-info {
    max-width: 0;
    opacity: 0;
  }
  .profile-name {
    font-size: 12.5px;
    font-weight: 500;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .profile-role {
    font-size: 10.5px;
    color: var(--text-mute);
    white-space: nowrap;
  }

  .profile-btns {
    display: flex;
    gap: 2px;
    flex-shrink: 0;
    overflow: hidden;
    max-width: 60px;
    transition: max-width 280ms cubic-bezier(0.4, 0, 0.2, 1), opacity 200ms ease;
  }
  .sidebar.collapsed .profile-btns {
    max-width: 0;
    opacity: 0;
  }
  .profile-btn {
    width: 28px;
    height: 28px;
    display: grid;
    place-items: center;
    background: transparent;
    border: none;
    border-radius: var(--r-sm);
    color: var(--text-mute);
    cursor: pointer;
    transition: background 150ms ease, color 150ms ease;
  }
  .profile-btn:hover {
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
    transition: padding 280ms cubic-bezier(0.4, 0, 0.2, 1);
  }
  .sidebar.collapsed .health {
    justify-content: center;
    padding-left: 0;
    padding-right: 0;
  }
  .health .label {
    overflow: hidden;
    white-space: nowrap;
    max-width: 120px;
    transition: max-width 280ms cubic-bezier(0.4, 0, 0.2, 1), opacity 200ms ease;
  }
  .sidebar.collapsed .health .label {
    max-width: 0;
    opacity: 0;
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
  /* On mobile the sidebar becomes a fixed bottom bar — the desktop
     collapse feature doesn't apply here. Reset everything. */
  @media (max-width: 900px) {
    .collapse-btn { display: none; }
    .sidebar {
      /* Reset collapse-specific overrides */
      width: auto !important;
      transition: none !important;
      position: fixed;
      bottom: 0;
      left: 0;
      right: 0;
      top: auto;
      height: auto;
      flex-direction: row;
      align-items: stretch;
      border-right: none;
      /* Use the theme-aware --bg-bar token so this gradient renders correctly
         in both dark and light modes (the old hardcoded rgba was near-black,
         which looked broken on the warm cream light background in Safari). */
      background: linear-gradient(to top, var(--bg) 60%, var(--bg-bar));
      backdrop-filter: saturate(180%) blur(28px);
      -webkit-backdrop-filter: saturate(180%) blur(28px);
      border-top: 1px solid var(--hairline);
      /* Subtle upward shadow gives the bar depth and separates it from content */
      box-shadow: 0 -8px 24px rgba(0, 0, 0, 0.07);
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
      /* --text-mute is more legible than --text-dim at this tiny size,
         especially on the warm-cream light background */
      color: var(--text-mute);
      text-align: center;
      width: auto;
      position: relative;
    }
    .nav-mobile .nav-item:hover {
      background: transparent;
      color: var(--text-mute);
    }
    .nav-mobile .nav-item :global(svg) {
      width: 22px;
      height: 22px;
      /* Spring easing for the scale pop; ease for the amber glow fade */
      transition: transform 0.28s var(--spring), filter 0.22s ease;
    }
    /* Active = amber text color */
    .nav-mobile .nav-item[aria-current="page"] {
      background: transparent;
      color: var(--on);
      box-shadow: none;
    }
    /* Lamp fixture: amber bar at the very top with a two-layer glow —
       first layer hugs the bar itself, second fans downward onto the icon */
    .nav-mobile .nav-item[aria-current="page"]::before {
      content: '';
      position: absolute;
      top: 0;
      left: 50%;
      transform: translateX(-50%);
      width: 28px;
      height: 3px;
      border-radius: 0 0 var(--r-pill) var(--r-pill);
      background: var(--on);
      box-shadow:
        0 0 10px 4px var(--on-glow),
        0 22px 36px 6px var(--on-glow);
    }
    /* Icon bubble — the lit surface beneath the lamp; softer glow
       so it reads as receiving light rather than emitting it */
    .nav-mobile .nav-icon {
      width: 52px;
      height: 30px;
      display: flex;
      align-items: center;
      justify-content: center;
      border-radius: var(--r-pill);
      transition: background 0.22s ease, box-shadow 0.22s ease;
    }
    .nav-mobile .nav-item[aria-current="page"] .nav-icon {
      background: var(--on-soft);
      box-shadow: 0 0 0 1px var(--on-soft), 0 0 14px 3px var(--on-glow);
    }
    .nav-mobile .nav-item[aria-current="page"] :global(svg) {
      color: var(--on);
      transform: scale(1.1);
      filter: drop-shadow(0 0 8px var(--on-glow));
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
      /* Reset desktop collapse styles that bleed in via .nav-label */
      max-width: none !important;
      opacity: 1 !important;
      overflow: visible;
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
