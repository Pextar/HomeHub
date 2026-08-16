<script lang="ts">
  import Icon from "./Icon.svelte";
  import ConfirmModal from "./ConfirmModal.svelte";
  import SidebarDrawer from "./SidebarDrawer.svelte";
  import { route, theme, data, session, sidebar, assistant } from "../lib/stores.svelte";
  import { api } from "../lib/api";
  import { openModal, modalStack } from "../lib/modal.svelte";
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
    { route: "music", icon: "musicNotes", label: "Music", admin: true },
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
    // A kiosk surface for wall/tablet displays rather than a management
    // screen — it sits last so the phone's primary tabs are unaffected.
    { route: "panel", icon: "monitor", label: "Panel" },
  ];
  const items = $derived(allItems.filter((i) => session.isAdmin || !i.admin));
  const primary = $derived(items.slice(0, PRIMARY_COUNT));
  const overflow = $derived(items.slice(PRIMARY_COUNT));

  // Live count shown beside a drawer row — the menu answers "what's in
  // there" before you tap. Empty/unknown sections get no badge at all.
  function badgeFor(r: Route): string | null {
    const d = data.value;
    switch (r) {
      case "automations": {
        const n = d.automations.length + d.schedules.length;
        return n ? String(n) : null;
      }
      case "rooms":   return d.rooms.length ? String(d.rooms.length) : null;
      case "sensors": return d.sensors.length ? String(d.sensors.length) : null;
      case "scenes":  return d.scenes.length ? String(d.scenes.length) : null;
      default:        return null;
    }
  }

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
    theme.cycle();
  }

  // Dark / Light / Auto each get their own face and word, so the shortcut
  // says which of the three it just landed on instead of only which way it
  // flipped. Auto wears the device, not a time of day — it is the setting
  // that defers to the phone.
  const themeIcon = $derived(
    theme.mode === "auto" ? "monitor" : theme.mode === "dark" ? "moon" : "sun",
  );
  const themeLabel = $derived(
    theme.mode === "auto" ? `Auto theme · ${theme.system}` : theme.mode === "dark" ? "Dark theme" : "Light theme",
  );

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
    // Cmd/Ctrl-K is handled globally by AssistantLauncher (opens the assistant).
  }

  // True when the active route is one of the overflow items — used to
  // highlight the "More" tab so the user knows where they are.
  const moreActive = $derived(overflow.some((i) => i.route === route.current));

  // ── Mobile tab dock lens (DESIGN.md §7) ─────────────────────────────
  // The active slot is marked by a single amber capsule that slides
  // between slots, so its position is derived from the active index
  // rather than painted onto the item. Slots are equal-width flex
  // children, so index maths lands exactly on a slot without measuring:
  // the dock's inner track is the full width minus its 10px side padding.
  const slotCount = $derived(primary.length + 1); // + the More button
  const activeSlot = $derived.by(() => {
    const i = primary.findIndex((it) => it.route === route.current);
    if (i >= 0) return i;
    return moreActive ? primary.length : -1;
  });
  const slotWidth = $derived(`calc((100% - 20px) / ${slotCount})`);

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
    {#if session.isAdmin}
      <button class="nav-item assistant-launch" data-label="Assistant" onclick={() => assistant.show()}>
        <Icon name="assistant" size={18} />
        <span class="nav-label">Assistant</span>
        <kbd class="nav-key" aria-hidden="true">⌘K</kbd>
      </button>
    {/if}
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
    <!-- Sliding amber lens behind the active icon. Hidden outright when
         no slot matches, so a stray route never leaves it parked on the
         wrong item. -->
    <i
      class="tab-lens"
      class:hidden={activeSlot < 0}
      aria-hidden="true"
      style:left={`calc(${Math.max(activeSlot, 0)} * ${slotWidth} + 10px)`}
      style:width={slotWidth}
    ></i>
    {#each primary as item, i (item.route)}
      <a
        href="#/{item.route}"
        class="nav-item"
        class:lit={activeSlot === i}
        aria-label={item.label}
        aria-current={route.current === item.route ? "page" : undefined}
      >
        <span class="nav-icon"><Icon name={item.icon} size={22} /></span>
        <span class="nav-label">{item.label}</span>
      </a>
    {/each}
    <button
      class="nav-item more-btn"
      class:lit={activeSlot === primary.length}
      aria-label="More"
      aria-haspopup="menu"
      aria-expanded={moreOpen}
      aria-current={moreActive && !moreOpen ? "page" : undefined}
      onclick={() => (moreOpen = !moreOpen)}
    >
      <span class="nav-icon"><Icon name="more" size={22} /></span>
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
          <button class="profile-btn" aria-label="Theme: {themeLabel}" title={themeLabel} onclick={toggleTheme}>
            <Icon name={themeIcon} size={13} />
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
  <SidebarDrawer
    {overflow}
    {badgeFor}
    {themeIcon}
    {themeLabel}
    {healthLabel}
    {skipTransition}
    onToggleTheme={toggleTheme}
    onSignOut={signOut}
    onClose={() => (moreOpen = false)}
  />
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
  .sidebar:hover .nav-key,
  .nav-item:focus-visible .nav-key { opacity: 1; }
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
    z-index: var(--z-menu);
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

  /* ---------- Mobile bottom nav — floating glass dock ---------- */
  /* On mobile the sidebar becomes the transparent frame around a
     detached glass pill (DESIGN.md §7) — the desktop collapse feature
     doesn't apply here, so reset everything. */
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
      background: none;
      box-shadow: none;
      /* The frame itself is only a positioner — it must not swallow taps
         on the content showing either side of the pill. */
      pointer-events: none;
      padding: 0 14px var(--tabdock-inset);
      z-index: var(--z-dock);
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
    /* The dock: warm translucent glass with a specular top edge. */
    .nav-mobile {
      position: relative;
      display: flex;
      flex: 1;
      flex-direction: row;
      gap: 0;
      padding: 9px 10px;
      border-radius: var(--r-pill);
      background: var(--dock-fill);
      backdrop-filter: blur(26px) saturate(1.7);
      -webkit-backdrop-filter: blur(26px) saturate(1.7);
      border: 1px solid var(--dock-edge);
      box-shadow: var(--dock-shadow);
      pointer-events: auto;
    }
    .nav-mobile .nav-item {
      flex: 1;
      /* Above the lens, so the icon reads against the amber. */
      position: relative;
      z-index: var(--z-sticky);
      align-items: center;
      justify-content: center;
      gap: 0;
      min-height: 44px;
      padding: 0;
      border-radius: var(--r-pill);
      color: var(--text-dim);
      width: auto;
      transition: color 200ms ease, transform 90ms ease;
    }
    .nav-mobile .nav-item:hover {
      background: transparent;
      color: var(--text-dim);
    }
    .nav-mobile .nav-item:active {
      transform: scale(0.97);
    }
    .nav-mobile .nav-icon {
      display: grid;
      place-items: center;
    }
    .nav-mobile .nav-item :global(svg) {
      /* The rail tints the active icon amber; here the amber is the lens
         behind it, so the icon has to follow the item's own colour. */
      color: inherit;
      transition: transform 320ms var(--spring);
    }
    /* Active state is the lens behind the icon — the icon itself only
       flips to the ink-on-amber token and lifts a hair. */
    .nav-mobile .nav-item.lit {
      background: transparent;
      color: var(--primary-fg);
      box-shadow: none;
    }
    .nav-mobile .nav-item.lit :global(svg) {
      transform: translateY(-1px) scale(1.08);
    }
    /* Icon-only bar: the label stays in the DOM for the desktop rail but
       is dropped here, so each item carries an aria-label instead. */
    .nav-mobile .nav-label {
      display: none;
    }
    .tab-lens {
      position: absolute;
      top: 11px;
      bottom: 11px;
      border-radius: var(--r-pill);
      background: var(--on);
      box-shadow:
        0 0 20px 2px var(--on-glow),
        inset 0 1px 0 rgba(255, 255, 255, 0.45);
      transition:
        left 440ms var(--spring),
        width 440ms var(--spring),
        opacity 200ms ease;
    }
    .tab-lens.hidden {
      opacity: 0;
    }
    @media (prefers-reduced-motion: reduce) {
      .tab-lens,
      .nav-mobile .nav-item,
      .nav-mobile .nav-item :global(svg) {
        transition-duration: 0.001ms;
      }
    }
  }

</style>
