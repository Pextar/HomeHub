# HomeHub — design brief for Claude Code

Read this in full before touching markup or CSS. Every component, page, or
state you add must obey it. If something isn't covered here, **match the
nearest existing pattern in `frontend/src/`** rather than inventing.

---

## 1. Direction in one paragraph

HomeHub is a smart-home control surface in **warm dark mode** with an
**incandescent amber accent**. It should feel like the room you're controlling
— quiet, layered, lit from within — not a generic dashboard. Surfaces are
warm near-blacks. The "ON" state lights up with amber + soft glow, mirroring
the lights themselves. Numbers are monospace; labels are sans. Restraint
over decoration.

---

## 2. Hard rules (don't bend these)

- **No emoji** anywhere. (One exception: the Kid module — `KidHome.svelte`,
  `KidLampPanel.svelte`, `KidMusic.svelte`, `KidScheduleSheet.svelte`, and
  `components/kid/`.)
- **No decorative SVG.** Icons only when functional. For missing imagery use
  the `.placeholder` striped fill with a monospace caption — never invent a
  picture.
- **No gradients** except the two sanctioned ones: the `.tile.on` warm
  gradient and the day/night timeline. No purple/blue brand gradients,
  ever.
- **No pure black.** The deepest surface is `#0a0907` (Console only). App
  background is `#14130f`.
- **No tabs inside views.** Use chip filters. There is no longer an
  exception: Music carried one — a pill subnav across its own screens — and
  it is gone (§15). A module with more than one screen uses sheets over its
  home screen, or pushes a real screen with a back chip.
- **No sheet opens another sheet.** A sheet over a sheet means two scrims,
  two swipes and an ambiguous Escape. If the thing you are opening from has
  to lead somewhere deeper, make _it_ a screen. Sheets may swap for one
  another (and put the first back on the way out) — that is one sheet, not
  two.
- **No drawers from the side.** Use bottom sheets.
- **No spinners.** Use the existing skeleton primitive.
- **No icon-only button under 44×44** hit area.
- **All numerics use `var(--font-mono)`** with `font-feature-settings: "tnum" 1`.
  Counts, watts, temps, times, percentages, IDs.
- **The tab bar is hidden on detail / form / Matter step / Console screens.**

---

## 3. Tokens — paste these verbatim

```css
:root {
  /* type */
  --font-sans:
    "Geist", -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto,
    "Helvetica Neue", Arial, sans-serif;
  --font-mono: "Geist Mono", ui-monospace, "SF Mono", Menlo, monospace;

  /* warm dark — default */
  --bg: #14130f;
  --bg-2: #1c1a15;
  --card: #1f1d17;
  --card-2: #26231c;
  --card-3: #2e2a22;
  --hairline: #2a2720;
  --border: #34302a;
  --text: #eceae4;
  --text-mute: #9c988e;
  --text-dim: #66635c;

  /* accents */
  --on: #f5bd6e; /* incandescent amber — primary */
  --on-soft: rgba(245, 189, 110, 0.14);
  --on-glow: rgba(245, 189, 110, 0.45);
  --cool: #84acc4; /* secondary */
  --cool-soft: rgba(132, 172, 196, 0.14);
  --good: #9cc28a;
  --bad: #e08a7a;
  --warn: #e8b96b;

  /* protocol badges */
  --p-rf: #f5a06e;
  --p-wifi: #9cc28a;
  --p-matter: #c4a4e0;
  --p-mqtt: #e0c47a;

  /* radii */
  --r-sm: 10px; /* nav items, small chips */
  --r-md: 14px; /* inputs, segmented controls */
  --r-lg: 22px; /* cards, tiles */
  --r-xl: 30px; /* sheets, hero buttons */
  --r-pill: 999px; /* chips, tab pill + lens */

  /* motion */
  --spring: cubic-bezier(0.34, 1.56, 0.64, 1);
}

[data-theme="light"] {
  --bg: #f5f1ea;
  --bg-2: #efeae0;
  --card: #ffffff;
  --card-2: #faf6ee;
  --card-3: #f1ebde;
  --hairline: #e6dfd0;
  --border: #dcd3bf;
  --text: #1a1813;
  --text-mute: #6b6759;
  --text-dim: #9a9485;
  --on: #c97a1f;
  --on-soft: rgba(201, 122, 31, 0.1);
  --on-glow: rgba(201, 122, 31, 0.3);
  --cool: #426c84;
  --cool-soft: rgba(66, 108, 132, 0.1);
  --good: #4e8a3d;
  --bad: #b14b3d;
}
```

---

## 4. Typography

| Role          | Family     | Size                         | Weight | Letter-spacing |
| ------------- | ---------- | ---------------------------- | ------ | -------------- |
| Display (h1)  | Geist      | 26–30 mobile · 28–40 desktop | 600    | `-0.03em`      |
| Section (h2)  | Geist      | 17                           | 600    | `-0.02em`      |
| Body          | Geist      | 14                           | 400    | `-0.005em`     |
| Label / micro | Geist Mono | 10.5–11.5, **UPPERCASE**     | 500    | `+0.08em`      |
| Numerics      | Geist Mono | any                          | 500    | `-0.01em`      |

Body line-height 1.5. Heading line-height 1.1.

---

## 5. Spacing & layout

- **Mobile screen padding:** `22px` horizontal.
- **Desktop main padding:** `28px 36px`.
- **Card internal padding:** `14–22px`. Tiles use 16.
- **Section heads:** 26px top margin, 12px bottom.
- **Grid gaps:** 10–12px between tiles, 16–20px between cards.
- **Status bar pad (mobile):** top `54px` always reserved.
- **Tab bar pad (mobile):** bottom `90px` reserved on all scroll content
  (60px bar + 30px safe area).
- **Sheets:** open from the bottom. Default height 82%, smaller (62–68%) for
  short forms. 28px top-radius, grabber + close X, sticky footer with
  primary (amber, 2fr) and optional secondary (card, 1fr).

### Desktop breakpoints

- ≥ 1280px: 4-col device grid
- ≥ 1024px: 3-col
- ≥ 768px: 2-col
- < 900px: switch to mobile shell entirely

---

## 6. Core primitives

Build these once. Everything else composes from them.

### 6.1 Tile — the workhorse

The "ON" gradient + bulb glow does most of the visual storytelling in the
product. Don't substitute a flat background-color change.

```css
.tile {
  background: var(--card);
  border: 1px solid var(--hairline);
  border-radius: var(--r-lg);
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  position: relative;
  overflow: hidden;
  transition:
    background 200ms ease,
    border-color 200ms ease;
}
.tile.on {
  background: linear-gradient(155deg, #2b2419 0%, #221d14 60%, #1d180f 100%);
  border-color: rgba(245, 189, 110, 0.18);
}
.tile.on .tile-bulb {
  background: var(--on);
  box-shadow:
    0 0 0 1px var(--on),
    0 0 24px 4px var(--on-glow);
}
```

### 6.2 Switch

Two sizes: `.sw` (44×26, list rows) and `.sw-big` (64×36, hero). Thumb uses
the spring easing. Off thumb: warm gray `#b5b1a8`. On thumb: pure white.

### 6.3 Chip

Pill, padding `7px 12px`, font 12.5. Three states:

- default — muted card
- `.active` — text-on-text (inverted)
- `.on` — amber soft + amber fg

Icon-only chips are 36×36, padding 0.

### 6.4 Rail (slider track)

- `.rail` — 6px, inline. Fill uses `--on` (or `.rail.cool > i` for cool).
- `.rail-fat` — 64px tall, embedded label + percent. Used on Light detail
  for brightness. Fill is a warm gradient `#6e4e1c → var(--on)`.

### 6.5 Protocol badge

Uppercase 10px mono label + matching tiny line icon, colored by protocol.
Never a button. Never anywhere besides device/sensor cards.

### 6.6 Status dot

6×6 round. On = amber with 4px `--on-soft` halo (`box-shadow: 0 0 0 4px ...`).

### 6.7 Placeholder

For missing imagery / not-yet-built widgets:

```css
.placeholder {
  background-image: repeating-linear-gradient(
    -45deg,
    var(--card-2) 0 8px,
    var(--card-3) 8px 16px
  );
  color: var(--text-dim);
  font-family: var(--font-mono);
  font-size: 11px;
  display: grid;
  place-items: center;
  text-align: center;
}
```

Caption format: `[ what goes here ]`, e.g. `[ floor plan SVG ]`.

### 6.8 Waveform — the "playing" motif (Music only)

A four-bar animated equaliser that marks anything **actually playing** in the
Music module. It replaces the plain status dot (§6.6) _only there_ — a dot
says "on", a waveform says "audio is moving". Bars use `--on`, animate on a
staggered 950ms loop, and collapse to a static 8px height under reduced
motion. Nowhere outside Music **and its one satellite, the Home "Playing now"
card** (§15) — that card is Music's surface on Home, so it carries the
module's motifs rather than inventing quieter ones.

```css
.wave {
  display: flex;
  align-items: flex-end;
  gap: 2.5px;
  height: 13px;
}
.wave i {
  width: 2.5px;
  border-radius: 1px;
  background: var(--on);
  height: 4px;
  animation: wv 950ms ease-in-out infinite;
}
.wave i:nth-child(1) {
  animation-delay: 0s;
}
.wave i:nth-child(2) {
  animation-delay: 0.15s;
}
.wave i:nth-child(3) {
  animation-delay: 0.3s;
}
.wave i:nth-child(4) {
  animation-delay: 0.1s;
}
@keyframes wv {
  0%,
  100% {
    height: 3px;
  }
  50% {
    height: 13px;
  }
}
@media (prefers-reduced-motion: reduce) {
  .wave i {
    animation: none;
    height: 8px;
  }
}
```

---

## 7. Shells

### Mobile

```
┌──────────────────────────────┐
│  54px status bar pad         │
├──────────────────────────────┤
│  22px padding                │
│  content scrolls             │
│  padding-bottom: 90px        │
├──────────────────────────────┤
│  glass tab pill + safe area  │
└──────────────────────────────┘
```

Tab bar items, in order: **Home · Rooms · Music · Scenes · Schedule ·
Settings**. Max 6. The bar is a **floating glass pill** (`.tabbar` >
`.tabdock`), detached 26px from the bottom edge with 14px side inset: warm
translucent fill (`rgba(36,33,26,.52)`), `backdrop-filter: blur(26px)
saturate(1.7)`, 1px white-10% edge + inset specular highlight. **Icon-only,
22px** — every item carries an `aria-label`. The active item is a solid amber
`.tab-lens` capsule behind the icon (the icon flips to the ink-on-amber
`--primary-fg`; `#3a2400` in the mock); the lens is absolutely positioned
from the active index and **slides** between slots (440ms `--spring`), so
only one amber shape is ever on screen. Item hit area stays 44px min-height.

The pill floats over content rather than sitting on an opaque bar, so its
frame is `pointer-events: none` and the band it occupies — pill height plus
the float gap, which grows with the safe area — is published as
`--nav-clear`. Content padding and anything fixed above the dock (toasts,
the assistant FAB, the Music mini-player and selection bar) offsets by that
token instead of a hardcoded bar height. It is `0` on desktop.

The **assistant FAB** shares that band, bottom-right — but it yields it.
**A view that docks a full-width bar there claims the corner** (`bottomBar`
store, claimed from an `$effect` and released by its cleanup) and the FAB
stands down for as long as the bar is up, the same way Music's dock stands
down behind the card it would duplicate. Two controls must never stack in
one corner, and of the two the transport is the one you reached for.

A bar in that band still keeps its trailing control clear of the button by
reserving **`--fab-clear`**, never a literal 64px, because the FAB has three
ways to be absent: the claim above, the Settings › Interface switch that
hides it per device, and desktop, where the rail entry and ⌘K are the
launchers. All three zero the token, so the bar gets its own edge back
instead of dodging a button that isn't there; animate the padding so it
glides in as the FAB scales away. A missing FAB must never cost the feature,
so **the assistant also lives in the mobile More sheet**, permanently — it
has no route, so that sheet is its only fixed home.

In this app the six slots are the four primary nav entries plus **More**,
which opens the overflow sheet; the lens sits on More whenever the active
route lives in that sheet. The rail and the dock are the same component
(`components/Sidebar.svelte`), so the mock's `.tabbar` / `.tabdock` are
`.sidebar` / `.nav-mobile` below 900px; `.tab-lens` keeps its name.

Detail / form screens hide the tab bar and gain a back chevron in a 36×36
icon chip top-left. Title centers; right side gets the action chip (Edit,
More, Done).

### Desktop

```
┌─────────┬────────────────────────────────────────┐
│         │  topbar: h1 left · action buttons right│
│ NavRail │ ─────────────────────────────────────  │
│  240px  │  content                               │
│         │  padding: 28px 36px                    │
│         │                                        │
└─────────┴────────────────────────────────────────┘
```

NavRail items: 240px wide, `padding: 22px 16px`. Each item: 10×12 padding,
`--r-sm` radius, icon left (18px). Active item has `--card` background,
`--on`-tinted icon.

**Transient surfaces on desktop are popovers, not modals.** Notifications,
add-device, command palette — popovers anchored to their trigger.

---

## 8. Iconography

- Use the established icon set (`icons.jsx` → ported to `Icon.svelte`).
  Every icon is a single line/shape path on a 24×24 viewBox, stroke-based
  (no fills), `stroke-linecap: round`, `stroke-linejoin: round`,
  `stroke-width: 1.6` default.
- Standard size 18px in UI, 16px in chips, 22px in tab bar.
- Color is `currentColor` always — never hardcode an icon color.
- If you need a new icon, add it as one terse path in the same style.
  **No multi-path icons. No filled icons. No gradient icons.**

---

## 9. Motion

| Event           | Duration             | Easing                                  |
| --------------- | -------------------- | --------------------------------------- |
| Press           | 80ms                 | ease — scale to 0.97 (squish, not move) |
| Switch thumb    | 220ms                | `var(--spring)`                         |
| Hover (desktop) | 120ms                | ease — translateY(-1px)                 |
| View transition | 240ms in / 140ms out | cubic-out — fly-in y:10, fade-out       |
| Sheet open      | 280ms                | cubic-out from bottom; backdrop 200ms   |
| Reduced motion  | 0.001ms              | all of the above collapse               |

Hover lift is **`@media (hover: hover)` only.** Don't apply on touch.

**Haptics are part of motion.** Anything that flips state under a finger —
a toggle, a switch, a scene run, an armed pull-to-refresh — calls
`haptic()` from `lib/utils.ts` in the handler itself. It has two backends
because the platforms don't agree: `navigator.vibrate` on Android, and on
iOS (which has no Vibration API at all) a hidden `<input type="checkbox"
switch>` clicked through its label, which is the one way to reach the
Taptic engine from the web. Both need a real user gesture, so never call
it from a timer, an effect, or a network callback — call it on the tap,
before the round trip.

---

## 10. State patterns

- **Empty state:** mid-card (not full-screen). Dashed border, dim icon
  (28–32px in `--text-dim`), one-line title, optional one-line subtitle,
  single CTA in `.chip.on` style.
- **Loading:** the existing skeleton primitive — shimmer over a muted card.
  Don't use spinners.
- **Confirmation:** centered card, 340px max width. `--bad`-soft icon
  badge, two-line copy, button row: Cancel left (`.chip`) + danger-fill
  right (`background: var(--bad); color: #fff`).
- **Toast:** floats above tab bar, 16px from bottom edges. 280ms slide-up.
  Icon dot left, message, optional action right. Tone via icon color:
  `info → --cool`, `warn → --warn`, `error → --bad`, `success → --good`.

  **A toast is never a confirmation.** There is no `toasts.success` and no
  `toasts.info` — the store doesn't expose them. An action that worked is
  shown by the thing that changed: the sheet closes, the card goes, the tile
  lights, the player names the track, a form's Save greys out or says
  "Saved". Announcing it again in a card the user has to dismiss is noise,
  and it buried the one toast that mattered under sixty that didn't.

  Two things still earn one. **Failures** (`error`, `warn`), because nothing
  on screen can show that nothing happened. And **a control with a
  deadline** (`show` with an `action`) — Undo after the master switch,
  Refresh after an update lands. Those aren't announcements; the toast is
  the only place the affordance exists.

  When an action asks the hardware a question — pair, probe, send a test
  signal — the answer goes **inline, under the button that asked it**, and
  stays there. It has to survive the walk over to the socket.

---

## 11. Decision flow when adding a new surface

Use this to keep new screens consistent with the rest of the app:

```
Is it a list of things?
 ├─ Yes → card-grouped list pattern (see SettingsScreen / DevicesScreen).
 │       44–60px row, 36px-wide icon left, content middle, switch
 │       OR chevron right. Section heads in mono uppercase 11px.
 │
 └─ No  → Is it a single thing's detail page?
          ├─ Yes → top: 36×36 back chip + centered title block (name +
          │        subline) + 36×36 action chip. No tab bar. Hero card
          │        with the primary control, then secondary cards below.
          │
          └─ No  → Is it a form?
                   ├─ Yes → SHEET, not a screen. 82% default height.
                   │        Sticky footer: amber primary (2fr) + optional
                   │        card secondary (1fr).
                   │
                   └─ No  → Ask before building. Anything outside
                            list/detail/form needs design review.
```

---

## 12. Anti-patterns — reject these on sight

- Tabs nested inside a view → use chip filters
- A module that reshapes the global tab bar to its own destinations → the
  app-level nav is fixed; open module screens as sheets over its home screen,
  or push a screen with a back chip
- A sheet whose rows open another sheet → make the opener a screen (§15)
- Side drawer → use sheet
- Spinner → use skeleton
- Brand gradient (purple/blue/teal) → warm-only palette
- Pure black surface → `--bg` is the floor (Console is the only exception)
- Emoji outside the Kid module (KidHome / KidLampPanel / KidMusic /
  KidScheduleSheet / components/kid/)
- Icon-only button smaller than 44×44 hit area
- Numbers in sans → must be mono
- Tab bar visible on detail/form/Matter step/Console screens
- Modal on desktop where a popover would do
- A new color invented inline → if it's not in the token list above, use
  the closest token. If nothing fits, **stop and ask.**

---

## 13. Sanity checklist before opening a PR

- [ ] Every "ON" state uses `.tile.on` (gradient + bulb glow), not a flat color
- [ ] Every number is in `var(--font-mono)` with `tnum` enabled
- [ ] Every section header is 17px / 600, left-padded 22px (mobile) or 0 (desktop)
- [ ] Every list row matches the 44–60px / 36-icon / chevron-right pattern
- [ ] Tab bar is hidden on detail / form / Matter step screens
- [ ] Tab bar is the floating glass pill; active state is the sliding
      `.tab-lens`, never an amber icon colour
- [ ] Anything sitting above the tab dock offsets by `--nav-clear`, not a
      literal bar height; anything reserving room for the assistant FAB uses
      `--fab-clear`
- [ ] Notification indicator is exactly 7×7 amber (`--on`)
- [ ] No emoji outside the Kid module (KidHome / KidLampPanel / KidMusic /
      KidScheduleSheet / components/kid/)
- [ ] No new colors invented — only tokens from §3
- [ ] Reduced-motion media query collapses your animations to 0.001ms
- [ ] Hit areas ≥ 44×44 on touch
- [ ] Light theme verified (toggle via `[data-theme="light"]` on `<html>`)
- [ ] Nothing announces its own success — no `toasts.success` / `toasts.info`
      (they don't exist); confirmation is the UI that changed
- [ ] Theme reads `theme.current` (resolved dark/light), never `theme.mode`
      — which can be `"auto"`, and follows the OS live

---

## 14. File map (where new code goes)

```
frontend/src/
├── app.css                  ← tokens from §3 live here
├── App.svelte               ← router; don't change view-transition wiring
├── components/
│   ├── Tile.svelte          ← §6.1
│   ├── Switch.svelte        ← §6.2 (sw + sw-big variants)
│   ├── Chip.svelte          ← §6.3
│   ├── Rail.svelte          ← §6.4 (rail + rail-fat variants)
│   ├── ProtocolBadge.svelte ← §6.5
│   ├── Sheet.svelte         ← bottom-sheet host
│   ├── TabBar.svelte        ← mobile shell
│   ├── NavRail.svelte       ← desktop shell
│   ├── Icon.svelte          ← single <Icon name="..."> wrapping the path map
│   └── music/               ← the Music module's own pieces (see below)
├── lib/music/               ← the Music module's state (see below)
├── views/                   ← one .svelte per top-level surface
└── modals/                  ← sheets and confirms; one per flow
```

When adding a brand-new view, place it in `views/`, register the route in
`App.svelte`, and add an entry to the NavRail (desktop) and/or TabBar
(mobile) if it's top-level. Sub-screens don't get nav entries.

**A module with more surfaces than one file can hold gets a folder.** Music is
the first: its screens, sheets and player sections live in
`components/music/`, and its state — the three bridges, the room model above
them, the focus, search and its history, the grouping gesture — in
`lib/music/`. `views/Music.svelte` stays the single entry in `views/` and is
the shell: the topbar, the navigation of §15, the polling, and which surface
is up.

- **The catalog has two shared shapes and no third.** Every list of songs is
  `components/music/TrackList.svelte` and every browsable card is
  `MediaCard.svelte`; the four catalog screens (`SearchScreen`,
  `ArtistScreen`, `ContextScreen`, and `FavoriteBrowseScreen` over the last of
  those) compose them rather than growing their own row. A fifth surface that
  lists tracks adds a prop there, not a fourth copy of the row.
- **The room model is the module's spine** (`lib/music/rooms.svelte.ts`). It
  is the only file that knows a Sonos group from a KEF speaker from a HomeHub
  zone; every component above it takes a `Room` and asks it what it can do
  (§15.1). Adding a fourth make means adding a branch there and nowhere else.
  That is the test for whether a new piece of Music state belongs in a
  component: if it needs to know the make, it doesn't.

Two rules keep that from becoming a dumping ground:

- **A folder, not a prefix.** `components/` is documented above as the shared
  primitives; twenty `MusicSomething.svelte` files interleaved with them would
  make that list unreadable. Anything genuinely shared beyond the module
  graduates _out_ of the folder — the §6.8 waveform did the opposite journey
  and became `components/music/Waveform.svelte` because three files had grown
  their own copy of it.
- **State factories, never singletons.** A module-level store outlives the
  view and strands whatever it was holding; every store in `lib/music/` is a
  `create*()` the view instantiates, and none of them own an `$effect` —
  effects belong to the component whose lifetime they should share.

---

## 15. Music module (Sonos + KEF + Spotify)

The Music view (`views/Music.svelte`) is the one place with a live-audio
character. It reuses the shared primitives but layers a few module-specific
patterns on top. Keep these consistent if you extend it.

### 15.1 The one noun: a room

**Music speaks about rooms, and nothing else.** Underneath there are three
genuinely different objects — a Sonos group, a lone KEF speaker, and a
HomeHub zone that can span both — and the bridges that drive them stay
separate for good reasons (§15.6). But the module used to make the _user_
carry that split: the same Sonos One appeared as a "zone" chip, as a group
card, and as a draggable puck on three different surfaces, each with its own
way in and its own transport. That was the module's central fault, and
`lib/music/rooms.svelte.ts` is the fix.

- **Exactly one room owns a speaker.** Precedence is zone → Sonos group →
  lone speaker: the arrangement someone deliberately built beats the
  household's own grouping, which beats a speaker standing alone. One sound
  is listed once, under one name, in one place. If a speaker ever shows up on
  two cards, that is a bug in the room model, not a labelling choice.
- **Capabilities are carried, never inferred from the make.** A room says
  whether it can seek, skip, queue, take play modes or pick an input, and
  surfaces render only what it says yes to. A Sonos room playing radio can't
  seek; a KEF has no queue; a room HomeHub is streaming to has no skip. **A
  control that would be refused is worse than a control that isn't there** —
  the oldest rule in this section and the one everything else here follows
  from.
- **Live values are methods, not fields.** `rooms.list` is rebuilt when the
  topology changes; a progress bar ticking once a second must not rebuild it,
  or every card in the grid churns. Ask the model (`rooms.progress(r)`,
  `rooms.isPlaying(r)`), don't snapshot into the list.
- **Nothing in a component branches on a make.** If you find yourself writing
  `if (kind === "kef")` in a `.svelte` file, the capability you want is
  missing from `Room` — add it there.

### 15.2 Look

- **Music stays amber.** Music is a peer view in the nav, not a separate app,
  so it uses the same incandescent accent as everything else. A
  module-specific accent was tried and rejected: recolouring one top-level
  view invites every other view to claim its own hue, and the waveform
  already does the differentiating work. **Don't reintroduce a Music-only
  palette.**
- **Playing surface.** A hero, room card or mini-player that is playing uses
  the sanctioned `.tile.on` warm gradient (`var(--tile-on-gradient)` +
  `var(--tile-on-border)`) — the same "ON" look as a lit device. No separate
  music gradient exists or should be invented.
- **Waveform, not dot.** Anything playing shows the §6.8 waveform where a
  status dot would otherwise sit. Idle uses the `speaker` icon. This animated
  motif, not colour, is what marks Music as its own module.
- **Focus is a ring; playing is a fill.** "You are looking at this" and "this
  is making noise" are different claims and can both be true of one card, so
  they get different treatments and never share one.

### 15.3 Three surfaces, and what each is for

**Home** answers _where_. **Browse** answers _what_. **Speakers** answers
_which device_. Nothing appears on two of them.

- **Home = one hero over a grid of rooms.** The hero (`NowHero`) shows the
  focused room at full size and carries its whole transport — art, title,
  scrubber or read-only rail, prev/play/next, shuffle and repeat where they
  exist, and a volume fader. The grid (`RoomCard`) is every room in the
  house, same card shape whatever the make. That is the entire screen, plus a
  row through to Speakers.
  - **Tap focuses, tap again opens.** One target, two gestures, and the
    second is only reachable once the first has told you what you're about to
    open. A grid where a stray tap launched a full sheet is what this
    replaced.
  - **The grid's order is stable.** Not "playing first": a card that jumps
    across the grid the moment you press play is a card you can't drag onto
    anything.
  - **The pager dots appear only when more than one room is playing.** With
    one, the grid below is already the whole answer, and a single dot is a
    control that can't do anything.
- **Browse = everything you can start.** Favorites, recent searches and
  Spotify's catalog on one screen (`SearchScreen.svelte`), with **one** "Play
  on" picker at the top of it. The favorites shelf used to sit on Home under
  its own copy of that picker, which made the landing screen half _where_ and
  half _what_ with room for neither.
- **Speakers = the devices.** Names, addresses, tone, the status light. It is
  a screen and not a sheet because its rows open a speaker's settings one
  level further, and a sheet must never open another sheet.

### 15.4 Grouping: one gesture, two mechanisms

- **You drag a room onto a room, on the screen where the rooms are.**
  Grouping used to live one sheet deep in a grid of small tiles that were a
  dimmer second copy of the room list above them. There is one list now and
  the gesture happens on it (`lib/music/room-drag.svelte.ts`).
- **The user never picks the mechanism.** Two Sonos rooms group natively,
  because that is what the household is for and it is sample-locked. Anything
  with a KEF or an existing HomeHub room in it becomes a HomeHub room,
  because that is the only thing that can span makes. The merged card says
  which happened; the gesture doesn't ask.
- **The whole card moves.** What was dragged was a room, so every speaker in
  it goes. (Contrast the old puck grid, where only the dragged _speaker_
  moved — correct when the object was a speaker, wrong now that it's a room.)
- **The target absorbs the source.** The card you dropped onto is the one
  that survives, and keeps its name. Merging two built rooms retires the
  emptied one, because a second name over the same speakers is exactly the
  duplication this model exists to prevent.
- **On touch the lift is a hold, not a move.** A finger that moves straight
  away is scrolling the page, so a card only lifts after ~260 ms of staying
  put; a mouse lifts on an 8px move. Once lifted, the page's scroll is
  refused for the rest of the gesture — `touch-action` can't be changed
  mid-gesture, so it takes a non-passive `touchmove` handler. Holding at an
  edge auto-scrolls, and the card under a stationary pointer is re-checked
  every frame, since the ghost is pinned to the viewport while the list moves.
- **The keyboard gets the same gesture.** Drag is the whole affordance, which
  would otherwise leave grouping with no keyboard path at all. A focused card
  answers **G** to pick a room up, **Tab** to move, **Enter** to drop it in,
  **Escape** to put it back — candidates ringed the way a drop target is, and
  every step announced through a live region, since a drag has no running
  commentary of its own. Stated once as a footnote under the grid, never as
  chrome on every card.
- **Splitting is in the player, not on the card.** A card is a target for a
  drop; hanging an "Ungroup" control on it would put a second meaning on the
  thing you're aiming at. Native grouping undoes from the player's own
  control; a room the user named is edited or deleted through the editor.
- **`New room` exists for the times the gesture doesn't fit** — naming one
  first, or picking speakers that aren't next to each other in the grid.

### 15.5 One player, one destination, one dock

- **One player sheet for any room** (`Player.svelte`). There were three —
  Sonos, KEF and zone — each a near-copy that drifted from the others, and
  which one you got depended on a distinction the user never asked to care
  about. It now renders by capability: the queue pane for a room that has a
  queue, a scrubber where there's something to seek into, skips except on the
  stream route, play modes for a Sonos coordinator, the input chips for a
  KEF, the route note for a room HomeHub assembled, and one fader per speaker
  plus a room-wide one when there's more than one.
- **Focus _is_ the destination.** They used to be two ideas — a chip row
  picked where music went, and a separate tap opened a player — so you could
  read one room while queueing to another. One piece of state now
  (`destination.svelte.ts`, over the room list): the hero shows it, the
  player opens it, Browse plays to it.
- **"Play on" is a button with a menu, not a row of chips.** A radio row
  listing every room in the house sat on top of every browsing screen, so the
  busiest part of the screen was the part you almost never change, and six
  rooms wrapped it to three lines before a single result showed. A house with
  one room gets a plain line: a menu with one item can't do anything.
- **The dock is a fallback, never a duplicate.** It carries the same track
  and transport as the hero, so it stands down while the hero is on screen
  and appears the moment it scrolls away — which is always, on every screen
  that isn't Home. It **survives a pause**, since that is where whatever was
  last playing stays one tap from playing again, and paused it drops the
  `.tile.on` surface for a plain card: nothing is moving, so nothing should
  say it is. While it is up the assistant FAB stands down (§7) so the
  transport gets the whole bar.
- **Transport is optimistic.** A play/pause tap flips every icon, waveform
  and surface immediately and the local state wins until the poll agrees —
  or for 6s, so a command a speaker quietly ignored can't leave a button
  lying about its state forever. Rolled back if the call fails.
- **"Play similar" is HomeHub's preference, not the speaker's.** A Sonos group
  goes quiet when its queue runs out; the setting that keeps a room going —
  topping the queue up with tracks like the ones it has been playing, through
  `AddToQueue` so nothing interrupts what is already going — is the hub's own
  (`api/sonos_autoplay.go`), held per coordinator in memory. It renders as one
  more play-mode chip beside Crossfade because that is what it reads as, and
  three things follow from it being ours rather than the speaker's: it is
  **off again after a restart**, it needs a Spotify account to find anything
  to add, and it exists only for a Sonos coordinator — there is no queue to
  top up anywhere else. Name it for the choice it settles: the room either
  continues with its queue or continues with something like it. **One word
  across surfaces** — the chip is `Play similar` in the player and on the
  panel, never `Autoplay` in one place and something else in the other.
- **Progress rides on the playing surface.** A 2px hairline along a card's
  bottom edge (`ProgressLine`), extrapolated between polls by `clock.beat`
  so it creeps rather than stepping. Zero duration renders nothing at all
  rather than a made-up position.

### 15.6 Screens, sheets and getting back

- **Home is the only fixed screen.** Everything else pushes over it with a
  back chip (§11 detail shape); the player and the room editor lift from the
  bottom. Nothing rides below Music's header but content.
- **A sheet must never open another sheet, and sheets swap rather than
  stack.** Editing a room from its own player replaces the player and puts it
  back on the way out — one scrim, one Escape, one thing to swipe away, and
  no lost place either. What may be remembered is one level only; three
  levels of "back" is a navigation stack, and a navigation stack is what
  screens are for. The rule lives in `lib/sheet-run.ts` as a tested state
  machine rather than as loose flags in a view, and the body-scroll lock keys
  on _whether_ a sheet is up and never on which — a swap that released and
  retook it would unpin and re-pin the body on iOS for a frame.
- **Back means "up one", not "leave the module".** Music holds one history
  entry the whole time it is deeper than Home and re-takes it after each step
  back, so an Android back gesture climbs the same ladder Escape and the back
  chip do.
- **Coming back means coming back to where you were.** Both screens and
  sheets remember their scroll offset and restore it once the returning
  content has _laid out_ — one tick is too early, the content is still sizing
  and the offset gets clamped to whatever fits at that instant. Reaching
  Browse from an open player also remembers the _room_: leaving Browse, or
  anything opened from it, reopens that player rather than dropping to Home.
- **The global tab bar never changes shape.** Music is one destination among
  the app's nav entries; nothing in the module replaces or reshapes it.

### 15.7 Keyboard

The player covers the whole screen, so while it is open it answers the
transport keys a music app is expected to answer to — and **only the ones
this room can actually take**, which is why the hint line under the transport
is built from the room's capabilities rather than written out flat. Space
plays, arrows seek or skip (shift to skip when they seek), up/down are
volume, `m` mutes, `s`/`r` are shuffle and repeat, `q` is the queue.
Everything else stays scoped: only Escape and `/` work from the view at
large, so the module never swallows keys the rest of the app might want.

### 15.8 The player sheet's shape

- **Full player = bottom sheet, art-led** (`MusicSheet`, `--r-xl` top
  corners, a grabber, glass top bar). The art is the biggest, most obviously
  tappable thing on the hero and it opens the biggest surface.
- **The sheet drags down like every other sheet.** The top bar always drags;
  the body drags only from a scroll position of zero.
- **It unfolds out of whatever opened it** — the dock, a room card, the hero —
  rather than arriving over it (`grow` in `lib/motion.ts`): the panel travels
  from that frame to its own, its window opens by `clip-path` as it goes, and
  the art the two surfaces share flies between the two sizes. One player at two
  sizes, not two players. Reached any other way — a back gesture, the keyboard,
  reduced motion — it is the plain slide.
  **The moving window is a hard budget, and it is spent.** An animating clip
  only stays on the compositor while nothing inside it has to be redrawn, so
  the three things that would are handled by the transition itself rather than
  left in the CSS: the frosted head stands down, the content is faded by an
  animation rather than a per-frame style write, and **the ambient light waits
  for the window** — the blurred art is the single most expensive thing that
  can sit inside a moving clip, so the player opens on its own surface and the
  room lights a beat later, as the window settles. Anything new inside this
  sheet that blurs, filters or writes style per frame belongs on the same list.
  Two corollaries worth stating because both were bugs: a corner that is
  divided by a scale must be **solved per frame, never interpolated** — a
  linear radius against an eased box swells to three times its size halfway
  across — and the window must open a little **past** the panel's own frame, or
  the panel's shadow arrives in the one frame the clip is dropped.
- **The scrubber is a real control where the source allows it** and a
  read-only rail where it isn't (`TrackRail`). A source reporting no duration
  gets one honest line instead of a fabricated position.
- **The art answers a swipe on touch** where the room can skip — half-speed
  follow, firing past ~60px, and vertical always loses to the sheet drag.
- **The queue is a second pane inside the same sheet**, reached from an "Up
  next" row that names the actual next track. Not a segmented control; §2 has
  no exception left to lean on. The header's left button becomes a back
  chevron and Escape still leaves the player outright.
- **Queueing never interrupts.** Tapping a result plays it now; "Play next"
  and "Add to queue" live behind the row's overflow, and only for a Sonos
  room, since the queue is a Sonos group's.
- **The search box behaves like a search box.** Typing debounces (400ms),
  Enter runs it now, a clear X appears once there is something to clear,
  ArrowDown hands the caret to the first result, and arriving puts the caret
  in the box on `(pointer: fine)` only — auto-focus on a phone throws the
  software keyboard over the results.

### 15.9 The catalog: search, artist, album, playlist

Browse used to answer a search with four flat lists of name-and-art rows, so
choosing between two albums by the same band meant tapping one to find out.
The catalog surfaces now say what the service actually told us — and they are
a **stack**, because browsing a catalog is a drill-down and back has to mean
_up one_.

- **A tap opens; only a song plays.** An artist opens their page, an album or
  playlist opens its listing, a track plays on the focused room. A container
  also carries an explicit `Play album` / `Play playlist`, so "look inside"
  and "start it" are different targets rather than one ambiguous one. There is
  no exception to widen here: **no speaker takes an artist URI** (see
  `sonos.SpotifyItem`), so `Play <artist>` starts their top track and the line
  under it says which — a button that names what it will do beats one that
  silently picks.
- **The screens are a real stack** (`stack` in `views/Music.svelte`), not a
  single `screen` flag. Browse → artist → album is three levels; the back
  chip, Escape and the Android gesture all climb one, and every level restores
  the scroll offset it was left at (§15.6). Only the top level is mounted.
- **A detail is fetched once per URI and kept for the session.** An artist's
  page doesn't change while you're looking at an album from it, so coming back
  is instant instead of replaying a skeleton.
- **Every row and card says what distinguishes it.** A track carries its album
  and its length, an album its year and track count, an artist their following,
  a playlist its owner and size. All of it is optional on the wire: absent
  means the service didn't answer, and the field is dropped rather than
  invented — the same discipline as a speaker's capabilities.
- **One top result, at full size.** A search for a name is usually a search for
  one thing; it gets a card with its own stats and a stated destination ("See
  top tracks & albums"), above the per-kind shelves.
- **Songs are a list, everything else is a grid.** `TrackList` is the one
  row shape (numbered where the order is the information — an album's sides,
  an artist's most-played) and `MediaCard` the one card. A grid, not a
  carousel: a rail hid half the matches behind a horizontal swipe on the
  screen where they were hardest to reach.
- **The kind filter counts.** `Albums 7` is a decision; `Albums` is a guess.
- **What the page has already said, the rows don't repeat.** An album's own
  tracks carry neither its cover nor its artist (a featured artist still
  differs, so that survives), and below 560px the trailing play mark goes —
  it is a hover affordance, and it was being paid for by truncating the title
  and artist you choose a song by.
- **Sections degrade one at a time.** The artist page's top tracks,
  discography and "Fans also like" are three independent reads; Spotify
  retired related-artists for apps created after Nov 2024, so a refusal costs
  that shelf and nothing else. A shelf with no answer is absent, never empty.
- **A favorite that is a list is the same page as any other list**
  (`FavoriteBrowseScreen` renders `ContextScreen`). The only difference is that
  playing it _whole_ goes out through the Sonos household, so that button —
  and only that button — is disabled off a Sonos room, with the reason said
  under it; an individual track inside it is a plain Spotify item any room
  can take.
- **Playlist descriptions arrive as HTML** and are rendered as text. Spotify
  puts links in them; interpolating that would be an injection.

- **Home shows what's playing, and only that.** The dashboard's "Playing now"
  section (`components/NowPlaying.svelte`) is the only piece of Music that
  lives outside the Music view. It is a _glance_ surface, so it is
  deliberately smaller than the module it points at: one card per playing
  group — art, the §6.8 waveform over the zone name, track, artist/album,
  play/pause (skips appear from 430px up) — on the `.tile.on` playing
  surface. Everything else about a group (scrubber, volume, queue, play
  modes) stays in the full player; tapping the card goes there. The one thing
  it carries beyond playback is the shared live-status chip in its section
  head: how current this card is qualifies everything on it, so that belongs
  wherever the card is, not only in the view it points at. It owns its
  own refresh — on the `music` SSE topic, with a slower backstop poll behind
  it — because Sonos state doesn't live in the shared data store,
  and it renders **nothing at all** when there are no speakers, when the
  bridge is unreachable, or for a non-admin profile — a home without Sonos
  must not see a dead section, and a failed poll never raises a toast. Only
  a transport action the user actually took does.
- **Stay honest about the backend.** The local Sonos bridge exposes
  transport, volume, mute, join/leave, favorites, seek, play modes
  (shuffle × repeat × crossfade), the group queue (browse, jump, add,
  play-next, remove, clear), and — per speaker — bass, treble, loudness, the
  home-theatre EQ block (night mode, speech enhancement, sub on/off + level,
  surround), the status LED, the touch-control lock, the group sleep timer,
  the serial/firmware/MAC block and the device's own product image.
  It does **not** expose line-in or TV sources, music-library browsing beyond
  favorites, stereo balance, renaming a speaker's Sonos-side zone name, or
  creating and breaking stereo pairs — so there is no UI for any of those.
  Don't add UI for capabilities the bridge can't back; wire the endpoint
  first. Three shapes worth copying when you do:
  - Settings that belong to a _group_ rather than a speaker ride on the
    coordinator's `group_state` in the status poll, fetched in a second
    pass once the topology is known. Asking every follower would multiply
    the poll for no new information. The sleep timer is group-scoped the same
    way, and a follower's pane names the zone that owns it instead of
    offering the control.
  - A control whose speaker-side call can be refused (seek on a stream)
    renders as a label explaining why, never as a dead control.
  - **Configuration is read on demand, never polled.** A speaker's bass does
    not change on its own, so the settings snapshot (eleven SOAP calls, run in
    four parallel branches) is fetched when the settings pane opens and
    nowhere else.
    Only _state_ — what is playing, how loud, grouped with what — belongs in
    the status poll. Adding a setting to that poll would cost every open tab
    eleven extra calls per speaker every five seconds to watch nothing happen.
- **KEF is a second bridge, not a second Sonos.** `internal/kef` speaks the
  local HTTP API on KEF's wireless speakers (LS50 Wireless II, LSX II, LS60).
  It sits _beside_ the Sonos bridge, and the shared layer above them
  (`internal/media`, docs/MEDIA-PROTOCOL.md) deliberately does not flatten
  them: it describes each speaker by _capability_ and lets a route engine pick
  a path, rather than inventing groups KEF doesn't have or dropping the ones
  Sonos does. A Sonos household is zones that group and share a queue; a KEF
  speaker is one standalone stereo pair with an input selector; the protocol
  says so in both cases and neither has to pretend.
  Consequences for the UI, all of them the "stay honest about the backend"
  rule applied:
  - **A KEF can now be in a zone with a Sonos — and the UI must say how.**
    This section used to read "a KEF speaker never plays together with
    anything", and that was true of what the vendors offer: Sonos grouping is
    Sonos-only, and Spotify Connect reaches one device at a time. It is no
    longer true of HomeHub, which can hold the single Spotify session itself,
    decode once and serve the audio to both — the `stream` route.

    What must not follow is presenting the two kinds of zone as the same
    thing, because they are not. A native Sonos group is sample-locked and
    full quality. A streamed mixed zone lands its speakers a few hundred
    milliseconds apart, shows on Sonos as a stream rather than as a track,
    and takes over the account's active Spotify device. So a zone whose route
    is `stream` **names that in the UI**, using the backend's own `reason`
    string — which already names the speaker responsible — rather than a
    generic badge. The backend reports `route` and `sync` on every zone read
    precisely so the UI never has to guess which case it is looking at.

    The corollary is the rule that keeps this from being a regression: **a
    Sonos-only zone must never show stream affordances**, because it never
    takes that route. `native` and `group` are chosen ahead of `stream`
    whenever they fit, so an all-Sonos zone behaves exactly as it did before
    the protocol existed. If a Sonos-only zone ever renders as buffered, that
    is a bug in route selection, not a labelling choice.

    Where that lands in the module:
    - **The two mechanisms are one gesture, and the UI picks** (§15.4). This
      used to be two sections in a sheet — named cross-vendor zones above, a
      Sonos puck grid below — on the grounds that merging them would claim a
      KEF can be dragged into a Sonos group. The claim was right; the remedy
      was wrong. Making the user choose the mechanism made them learn a
      vendor boundary to do a domestic thing. So the drop decides:
      Sonos-native where both sides can take it, a HomeHub room where they
      can't, and the card the rooms merged into names which either way.
    - **The route note is the backend's sentence, never the UI's guess**
      (`ZoneRoute`). `stream` gets a mono `HomeHub stream · buffered` tag
      plus the `reason`; `exact` sync gets one quiet `In sync` line; a zone
      of one gets nothing, because there is nothing to keep in step. A zone
      nothing can serve shows its `problem` in `--bad` — those strings name
      which speaker blocked which route, which is the actionable part, and a
      generic "unavailable" would throw that away.
    - **The route removes controls from the shared player; it doesn't get its
      own.** A room HomeHub is streaming to reports no skip and no seek, so
      `Player.svelte` renders neither, and a line says track changes come
      from Spotify instead. That is the same mechanism a Sonos radio stream
      and a KEF speaker already used; the route is one more input to it, not
      a reason for a fourth sheet.
    - **One sound is listed once**, which the room model now guarantees
      structurally rather than by filtering vendor lists at each call site
      (§15.1). A HomeHub room claims its speakers whether or not it is
      playing, so nothing appears twice even while idle — the old rule only
      held while a zone was live.
    - **Favorites stay Sonos-only and say so.** A favorite is a household
      list entry, so any room that isn't a Sonos group can't take one — not
      just a KEF. Browse says what it needs instead of offering a rail of
      dead cards.
    - **One Spotify session, said before the tap.** The `stream` and `connect`
      routes both hold the account's single active session, so starting one
      stops the other. The player also offers **Stop**, distinct from pause,
      because that is what hands the session back.
    - **The editor is a form, so §11 gives it the sheet shape** — and it
      _swaps_ with whatever raised it, so editing a room from its own player
      is still one sheet, one scrim, one Escape. It claims nothing about
      routes: predicting one would be a second copy of the route engine in
      the UI, drifting from the real one. The player says what the room will
      actually do.

  - **A KEF room is a room like any other.** Same card, same hero, same
    player sheet — the sheet simply drops what the speaker hasn't got: no
    queue, no play modes, no scrubber (KEF's API has no seek, so the position
    line is read-only, and an input reporting no duration gets no line at
    all). It adds the input selector, which is the one question a KEF raises
    that nothing else does.

    This section used to argue there shouldn't be a KEF player at all — "an
    art-led sheet with a volume slider in it" — so a KEF room's chip pushed
    the Speakers screen instead, from a row of chips where every neighbour
    lifted a player. Two rooms that looked identical, one tap apart, going to
    completely different places: the module's worst seam by a distance. The
    premise was also wrong; KEF reports art, title, artist, album, position
    and duration, and answers play/pause/next/previous, volume and mute. What
    it lacks is two sections, not a player.

  - **Its settings live on its Speakers screen, reached from the player.**
    `views/KEFSpeakerDetail.svelte` still carries transport, because it is
    also reachable directly from Speakers and would otherwise be a settings
    pane you cannot play from. The player's action chip is the way _out_ to
    it — the sheet stands down first, so a screen can push without a sheet
    ever opening one. The Sonos pane still omits transport, because its
    player already has it; do not read this as licence to put volume back
    there.
  - **The input selector is the "play this" control.** There is no queue to
    point somewhere, so switching to the optical input _is_ the "play the TV"
    action. It renders as chips (§2), and every model shows the same list —
    there is no "what inputs do you have" call, so a model without USB simply
    refuses it rather than the UI hiding it.
  - **Starting music goes out through Spotify, and only that one thing does.**
    The speaker's API can play, pause and skip but has nothing that _takes_
    content: no queue, no URI, no favorites. So a search result reaches a KEF
    speaker through Spotify Connect — `internal/api/kef_spotify.go` asks the
    Web API to point playback at the speaker, which then streams it itself.
    Everything else about a KEF speaker stays on the LAN; this is the single
    exception, and it must not become a habit. What follows from it:
    - It needs the **user's** Spotify account (Premium, plus the two player
      scopes), not the speaker's. A login made before HomeHub asked for those
      scopes searches fine and cannot play, so `spotify/status` reports
      `playback` and Browse says "reconnect" **before** the tap rather than
      surfacing a 403 after it.
    - The backend **wakes the speaker onto Wi-Fi first**: a speaker in standby
      or on its optical input is not a Connect device at all, and "device not
      found" for a speaker sitting right there is not an answer.
    - **Which Connect device a speaker is** gets matched on its name, and the
      `Spotify` card on its screen is where that is corrected when the names
      differ — one row naming where music will come from, one chip row of the
      account's visible devices, and a way back to name matching. The card is
      absent when there is no Spotify account to ask: setting Spotify up has
      one home, on Browse, and a second one here would be duplication.
    - **Transport stays local.** Once something is playing, pause and skip go
      to the speaker over its own API — the cloud is only how playback is
      _started_, never how it is controlled.
  - **Everything else follows the pointer rule.** The settings snapshot has a
    field per DSP path and `undefined` means "this model didn't answer for
    it" — an LSX II has no subwoofer output, so the whole Subwoofer card is
    absent for one. Same discipline as Sonos' `capabilities`, different
    mechanism.
  - **It shares the room grid, the hero, the player, the picker, the dock and
    the dashboard's glance card.** A playing KEF speaker is a room on the same
    `.tile.on` surface with the same §6.8 waveform, opening the same sheet —
    a peer, not a lesser citizen. On Speakers it is its own list under a `KEF`
    heading, not interleaved with the Sonos one: the rows' sub-lines mean
    different things and the screens they open answer different questions.
  - **The `Live` chip stays Sonos-only.** It reports whether GENA
    subscriptions are working. KEF has no change notifications to subscribe
    to — the KEF Connect app polls too — so a chip saying "Polling" about
    them would be reporting a fault that doesn't exist. The backend polls the
    speakers once for the whole process, caches, and publishes on the same
    `music` SSE topic when something actually changes; five open tabs cost
    the speaker what one does.
  - **Adding a speaker is one sheet for both bridges**
    (`modals/SpeakerModal.svelte`), with a chip row for the brand. A separate
    "Add Sonos" / "Add KEF" button would put the least interesting decision
    first. Discovery differs underneath — Sonos expands its own topology, KEF
    narrows an SSDP sweep and then asks each responder whether it speaks the
    KEF API — but both end at the same picker, and typing an address is an
    equal path in both, not a fallback for a failed scan.

- **State is pushed, and the poll is only a backstop.** The backend
  subscribes to each speaker's UPnP change notifications (GENA — see
  `internal/sonos/monitor.go`) and caches what they report, so pressing play
  on the speaker itself reaches the screen in well under a second. Two
  consequences for anything built on top:
  - **Never assume the push is there.** Subscriptions need a speaker that
    can reach HomeHub back over plain HTTP, which not every network allows.
    When they can't be established the same status endpoint reads every
    speaker synchronously instead — exactly what it did before any of this
    existed — and reports `live: false`. Both music surfaces pick their
    polling interval from that flag (20s/45s pushed, 5s/10s not), and a new
    surface must do the same rather than hardcoding a fast poll or trusting
    the events blindly.
  - **Music changes are their own SSE topic.** Speaker state is far
    chattier than the store — a volume drag is a dozen notifications — so it
    is published as `music`, not `changed`. Subscribing a view to the wrong
    one makes every open tab refetch every socket and sensor in the house
    each time somebody turns the kitchen up. Use `onLive(topic, fn)` from
    `lib/live.ts`, which shares one connection across the whole app.
  - **Push has a face, and it is not a red light.** Subscriptions used to be
    invisible in both directions: working, they were silent; failing, the app
    was simply slower with nothing on screen admitting why. Three surfaces fix
    that, and the split between them is deliberate — a **status chip** (`Live`
    / `Polling`, amber `.chip.on` only when live) because the answer qualifies
    everything under it; a **`Live updates` row on Speakers**, the §11 list-row
    shape, because Speakers is where the devices are managed and a chip nobody
    notices is not discoverable; and the **sheet they all open**
    (`modals/SonosEventsModal.svelte`), which is the only place with room to
    explain.
    The chip is one component — `components/LiveStatusChip.svelte` — carried
    by **both** the Music topbar and Home's "Playing now" head, and it must
    stay one: the same word has to mean the same thing and lead to the same
    place from either surface, which two hand-rolled copies would not survive.
    It holds back until the first poll lands, because "Polling" before an
    answer is a guess, not a report; below 380px it drops its label for the
    icon alone and squares up to 44×44, keeping its `aria-label` either way.
    The sheet answers in a fixed order — is push working, which
    speaker isn't, what would fix it — and it is written to reassure, not to
    alarm: polling is the _old_ behaviour, not a fault, so its copy says what
    the user actually loses (a few seconds) and states plainly that nothing is
    broken. The address speakers must reach appears **only when it's
    actionable**; when everything is live it is trivia. It carries the one
    control that can change the outcome — **Try again**, hitting
    `POST /api/sonos/events/retry`, which releases the watchers from a backoff
    up to five minutes long — because a diagnostic screen that can only
    describe a problem sends the user to a terminal.
    Read `GET /api/sonos/events` for the per-speaker detail; it reports the
    monitor's own bookkeeping and never touches the network, so polling it
    while the sheet is open is free.

---

## 16. Panel — the kiosk surface

The Panel (`views/Panel.svelte`, pieces in `components/panel/`) is the
always-on display: an old iPad on a wall or shelf, logged in once, living at
`/#/panel`. It is a **sibling of the app shell, not a view inside it** —
`App.svelte` renders it chromeless the same way it renders `KidHome`: no
sidebar, no tab dock, no assistant FAB, no pull-to-refresh. It is reached
from the sidebar's last entry and the PWA's "Panel" shortcut, and left
through one quiet Exit chip, top-right.

**Home and music are one surface with two depths.** The panel is the fused
Home+Music screen — home control on the left and centre, the player on the
right, both always visible. The second depth is the panel's **own music
screen** (`#/panel?music=1`, one tap in through the player's art and
meta): the featured room's player riding on the right, and a work area on
the left switched by chip between three panes — **Search** (the catalog,
with the room's recent searches and the account's playlists while the box
is empty), **Queue** (the featured Sonos group's,
with jump / remove / two-tap clear), **Rooms** (every room, with
Sonos-native grouping: join the featured room, split one, or step a
single speaker out). Its back chip returns to the dashboard depth, and
idling there sleeps home to the ambient face like any other panel idle.
Both depths read one shared speaker store (`lib/panel-music.svelte.ts`) —
one poll, one featured source, one now-playing line. The full Music view
stays for the jobs a wall can't do (cross-vendor HomeHub rooms, speaker
settings, EQ, Spotify setup), reached via Exit. Three behaviours keep the
pair coherent:

- **Sticky home.** Entering the panel marks the device panel-homed
  (`panel-home` in localStorage, next to the theme — it describes this
  screen, never the household). While the mark stands, the dashboard route
  renders the panel in its place, so an iPad that reboots or relaunches
  the PWA lands home again. Exit lifts the mark and returns to the normal
  app. Kid profiles never enter the panel, so the mark can't strand one.
- **Idle auto-return.** While panel-homed and on any other route, the
  shared idle clock (same `lib/panel.ts` numbers the panel sleeps by)
  walks the device back to `#/panel?idle=1` — arriving on the ambient
  face, not waking the UI. Open modals are dismissed first: a kiosk must
  never strand a sheet over its home screen.
- **The ambient face carries music.** While something plays, the idle face
  adds one line under the status — art thumbnail, track, "Artist · Album ·
  Zone" (`PanelNowPlaying`, fed by the same binding that sizes the music
  column). The clock stays the subject; playback is the footnote. Nothing
  playing, nothing shown.

The reference hardware is an iPad Air 2 — 1024×768, Safari 15, an A8X that
drops frames on blur and stacked shadows. Every rule below is that
constraint made visible.

- **One screen per depth, no chrome.** Landscape grid, three zones:
  clock/status hero (left, 300px), room lights (centre, flexible), music
  (right, 352px — present only when speakers exist; the grid is sized from
  the `speakers-seen` memory so the column doesn't pop in after the first
  poll). Nothing scrolls: each zone owns its overflow internally. Portrait
  is a stacked, scrollable fallback — supported, not designed-for. The
  music depth keeps the same shape: search and its results on the left
  (the list owns its scroll), the featured player on the right.
- **The job is glance + tap.** Room tiles toggle the room; the music card
  transports and sets volume; the master button does all-on/all-off. The
  music depth adds the player's daily jobs — search, the queue, Sonos
  grouping — as panes on the one kiosk surface, still with no sheets, no
  modals, no forms: destructive taps arm for a second tap instead of a
  confirm dialog. Anything that _configures_ (speakers, cross-vendor
  rooms, Spotify) sends the user to the full app, not to a panel
  sub-screen.
- **The resting state is the ambient face.** After 120s untouched (45s
  between 22:00–06:00) the UI fades to clock + date + one status line
  ("3 of 8 lights on · 21° inside"), dimmed to 45% at night. Any touch
  wakes it. The face drifts a few pixels per minute (transform-only) so no
  pixel holds one value for hours. This is the nightstand face, the
  burn-in guard and the backlight saver in one.
- **Type and targets are distance-scaled, not phone-scaled.** Hero clock
  76px; ambient clock `clamp(104px, 20vw, 168px)`; tile names 19px; track
  title 21px; transport buttons 64px with an 80px centre; room tiles share
  the zone's height down to a 120px floor, then scroll internally.
- **Old-GPU motion budget.** No backdrop-filter anywhere. The §6.1 glow
  lives only on small badges (56px icon chips), never on whole tiles.
  Animations are opacity/transform only, ≤200ms, and the ambient fade is
  the one 600ms exception. The app-shell view transition does not run
  here.
- **Music is the panel's second satellite** (after Home's "Playing now",
  §6.8) and carries the waveform by the same licence. One source is
  featured — the user's chip pick, else whatever is playing — with Sonos
  groups and KEF speakers as equal sources. Transport stays honest per
  bridge (§15): KEF gets play/pause but no skips, standby renders as a
  label, and a source that can't be reached is absent rather than dead.
  Art and meta are the tap-through to the music depth; transport,
  volume and the source chips stay on both depths — the player answers
  on the wall, the library lives one tap in.
- **The player card is one component with two widths** (§16 keeps them
  in `components/panel/`). Both depths get the scrubber — seek on a
  Sonos track, a read-only rail elsewhere, an honest no-position line
  where there is no duration — and a Sonos coordinator's play-mode
  chips (shuffle, repeat, crossfade). The depth's card adds what the
  dashboard's glance surface hasn't room for: one fader per speaker
  under the room-wide one when a group has more than one, the KEF
  input selector, and the Up-next row that names the actual next track
  and opens the queue pane (§15.8's door, same sentence). The mode row
  carries §15.5's **Play similar** with the rest, since "what happens
  when this song is the last one" is exactly the question a wall gets
  asked; the depth's card adds one line naming which way it will go,
  because that choice shows itself only once the queue has run out.
- **On the music depth, a song plays; an artist opens.** The wall keeps
  the flat gesture where it's about starting sound: a song found by
  search plays, an album or playlist plays whole — the player names what
  started and on which room, the featured source being the destination
  the player column's chips name. The search answers with §15.9's **top
  result** first — the one thing the search was almost certainly after,
  as the same row at full size — then the per-kind shelves. An artist row
  opens the artist's page
  instead: the app's own §15.9 catalog screens ride in the depth's work
  area — the most-played tracks, the discography shelves, "Fans also
  like" — fetched once per URI and kept for the session. From there a
  record or a related artist goes one level deeper still, the back chip
  climbs one level rather than leaving the depth, and Escape walks the
  same ladder. Queueing without interrupting lives behind the row's
  overflow ("Play next", "Add to queue"), only for a Sonos destination,
  exactly like §15.8's rule — and the queue pane is where it lands.
  **Recent searches live on the wall too**, keyed by the featured room
  with the app's own key format, so the wall and the phone share one
  per-room history — a chip re-runs one, an × forgets one, Clear forgets
  the room's list. A search is remembered when it is submitted (§15.8's
  Enter) _and_ when a result is acted on, because the wall's flow is
  type → tap with no Enter in between. While the box is empty the pane
  idles on those recents and the account's playlists — the playlists as
  an art grid (§15.9's rule: everything but songs is a grid), a tap
  still playing one whole. **The box knows when the software keyboard
  is up** (measured off `visualViewport`, so a docked, floating or split
  keyboard all read true, and no keyboard reads as none): the depth
  re-floors to just above it and the results go dense — single-line
  rows, no shelf labels, no kind chips, the top-result card folded back
  into the shelves — because typing should show a handful of matches
  where one card used to fit. The keyboard is part of the flow: Enter
  dismisses it, and so does a tap on any result, so the rich layout
  returns for the choosing. The Sonos favorites shelf stays
  in the full view: recents answer the wall's idle moment better than a
  static list that never changes.
  Spotify setup stays in the full view: an unconnected depth says so and
  points there, because configuration is the full app's job.
- **Grouping on the wall is Sonos-native and tap-based.** The app's
  drag would be imprecise at arm's length, so the Rooms pane names the
  action instead: every other Sonos room gets "Join {featured}", the
  featured group gets a two-tap "Split", and each non-lead member gets
  an × to step out. Cross-vendor HomeHub rooms are never created here —
  the route engine, the Spotify-session trade-offs and the persistent
  naming all belong to the full view — and the pane says so in one
  honest line rather than offering half of it.
- **Rooms, or devices.** With no rooms defined, the centre falls back to a
  device grid; an empty control surface is never acceptable. Unassigned
  devices appear only there — the room grid is rooms, not clutter.
- **Everything else is stock.** Same tokens, same `.tile.on` gradient on
  room tiles / master / playing card, same mono numerals, same runAction
  silence-on-success. The panel should read as the app, quieter — not a
  redesign of it.

---

## 17. The kid surface — lamps, schedules, and now music

The kid module is the one place the quiet rules lift: emoji are the
language, targets run bigger than 44px, weights run 800, and anything
destructive is a **two-tap arm** (the button turns pink and asks "really?"
for three seconds) rather than a confirm modal. It lives in
`views/KidHome.svelte` (lamps + schedules), `views/KidLampPanel.svelte`
(the colour playground), `modals/KidScheduleSheet.svelte`, and
`views/KidMusic.svelte` with `components/kid/` (the music player). It is
rendered chromeless for `kid` profiles, exactly like the panel is for the
wall.

**Kid music is the panel's feature set, spoken kid, over Sonos only.** A
kid profile may browse and control the household's Sonos groups — the
backend gate is `requireAdminOrKid`, and discovery, speaker management,
settings, KEF and the media layer stay admin-only, because configuration
is the full app's job. Everything the wall's music depth does, the kid
player does: room pick, transport, seek rail, play modes, per-speaker
faders, queue, search with recents, artist/album drill-down, and
play-together grouping.

**Spotify is per-account: a kid searches as the kid.** `internal/spotify`
holds one household account (the "" key — the Music view, KEF Connect and
autoplay ride on it) plus one account per kid profile, all sharing the
developer app's client ID (which stays admin-set, and changing it wipes
every account). A kid links their own account from the search pane itself —
"Connect Spotify", the same PKCE flow, landing back in the kid app — so the
account's own settings (a Spotify Kids account's explicit filter) apply to
what the kid can find, and no grown-up's listening history leaks into the
kid's results. Only when no client ID exists at all does the pane say "ask
a grown-up". `spotifyAccount()` in `internal/api/spotify.go` is the split:
kid caller → their account, everyone else → the household's.

- **Same brain, same search, same memory.** The player drives its own
  `createPanelMusic({ sonosOnly: true })` — the KEF poll never fires and no
  KEF source can appear — and the search reuses `createSpotify` and
  `createSearchHistory` keyed `sonos:{coordinator}`, so a search run on the
  kid's tablet lands in the same per-room recents as one from the wall or a
  phone. New music state belongs in those factories, not in a kid copy.
- **One screen, chip-switched panes.** The featured room's player rides on
  top; below it three big chips — 🔎 Find, 🎶 Up next, 🔊 Rooms — swap the
  pane. The panes never unmount, so a search halfway typed survives a peek
  at the queue. The chips never wrap: they size to their words and scroll
  sideways on a narrow phone.
- **It is a phone surface and nothing else, so the fold is the budget.**
  The player, the chips and the top of the pane under them have to fit one
  screen, or a kid opens the module and cannot see the thing they came for.
  This is the constraint every layout decision here answers to, and it was
  lost once already: a player carrying every control the wall's does ran
  past the bottom of an iPhone on its own, pushing search off-screen behind
  a scroll the kid had no reason to expect.
  - **The player card holds what a finger comes back to, and nothing
    else** — art, title, the seek rail, prev/play/next, one volume fader.
    Play modes and the per-speaker faders are set-and-forget and together
    they are half a phone screen, so they fold behind one `🎛️ More
    controls` row. Nothing was dropped: it is one tap away and it stays
    open for the session.
  - **The room is named in the header, not in a chip row.** A radio row of
    every room in the house sat above the player, so the top of the module
    was the part that almost never changes — the same fault §15.5 settled
    for the app, and it gets the same answer. The header states the room;
    tapping it goes to the Rooms pane, which is the one place that owns
    rooms and is already where they are joined together. A house with one
    room gets a plain line, since a control that can't do anything isn't
    one.
  - **The pane chips pin.** They are the module's whole navigation and a
    results list is longer than a phone, so they stick to the top of the
    scrollport on a glass band, and a tap on the chip you are already on
    scrolls that pane back to its own top. They stand down while the
    software keyboard is up — the pane you want is the pane you are in —
    and the search box takes the pinned band instead, because while you are
    typing that is the one thing you must not lose.
  - **No Up-next row in the player.** The pinned chip beside it reads
    `🎶 Up next 12` from anywhere on the screen, which is the row's whole
    job; the queue pane says the rest, and drops its own heading for the
    same reason.
  - **A row's length is not worth its title.** Below 480px the track
    duration goes from the search and queue rows — it was being paid for by
    truncating the song and artist a kid picks by, the trade §15.9 already
    settled for the app's rows.
- **The mini bar is the phone's dock, and §15.5's rule is its rule.** Once
  the _transport_ has scrolled away — the buttons the bar copies, not the
  card around them — a pill docks at the bottom: art, track, play/pause,
  next, with the same "fallback, never a duplicate" discipline as the app's
  dock (its text taps back to the top of the screen, where Back is too). It
  hides while the software keyboard is up rather than sit behind it. A song
  tapped deep in the results gets its "starting" feedback twice: the row's
  cover breathes until the poll lands, and the mini bar answers with the
  new track.
- **The playing halo breathes on opacity, never on a box-shadow.** The
  player card is the size of a phone screen, and animating its own shadow
  repaints all of it every frame for the whole time music plays. The glow
  lives on a pseudo-element whose opacity animates instead. Anything else
  in this module that wants to pulse follows the same rule.
- **The words stay the module's words.** The mode chips keep §15.5's one
  word across surfaces — Shuffle, Repeat, Crossfade, Play similar — with an
  emoji in front, not a rename. Numbers stay mono. Copy that only a kid
  reads ("Really split up?", "A mystery song") is allowed to be playful;
  errors keep "Oops!".
- **A song plays, a container plays whole, an artist opens.** Same gesture
  map as the wall — but the drill-down pages are kid-native (hero art, a big
  Play button that names what it will start, popular songs, album grids,
  more-like-them), not the app's §15.9 screens, which speak the app's quiet
  language. Back climbs one level. Queueing without interrupting is a row's
  big ＋ → "Play next" / "Add to the end", and it says 🎉, because a kid
  can't watch a count change a pane away.
- **Grouping is the star system.** The featured room wears a ⭐; every
  other card's one button is "🤝 Join {star}"; the star's card lists its
  speakers with a ✕ to step one out and a two-tap "Split up" for all.
  Cross-vendor rooms are never created here — with Sonos the only make a
  kid can reach, there is nothing to explain. **The two things a card does
  are stacked, never side by side**: the name row moves the ⭐ and says
  "Tap to play here", and Join sits full width under it — sharing a row
  with the name truncated another room's name to "🤝 Join Living Ro…" on
  every phone, on the one button whose whole content is that name.
- **The seek rail and faders are one primitive** (`components/kid/
KidSlider.svelte`) — a real range input over a painted track and fill,
  `onInput` for the live drag, `onChange` for the authoritative send, the
  same contract as the Music module's Slider. A source with no duration
  gets the honest line ("📻 Live radio"), never a fabricated position, and
  a room where nothing has played yet gets neither — it offers "🔎 Find
  something to play" instead, since the empty rail was the answer to a
  question nobody had asked yet.
  **The kid's volume faders cap at 50** (`VOL_MAX` in `KidMusic.svelte`) —
  loud enough to enjoy, not enough to upset the house; a speaker already
  louder reads its real number and can only be turned down.
- **The copy is older-kid, not toddler.** The reader is about eleven: real
  music words everywhere (queue, shuffle, repeat, crossfade), no baby talk
  ("Unknown song", not "a mystery song"; "No matches", not a 🙈), and the
  two-tap "really?" stays — it's about accidents, not age.
- **The software keyboard is part of the flow**, same as the wall: measured
  off `visualViewport`, the results go dense while it's up, and Enter or a
  tap on a result dismisses it. The search input is 18px, so iOS never
  zooms. It is measured **once, by the view**, not by the pane that owns
  the box — three things answer to it (the dense results, the pane chips
  standing down, the mini bar hiding rather than sitting behind the keys),
  and three copies of the same listener would drift.

---

## 18. When in doubt

1. Open `index.html` in the design project — it's the source of truth.
2. Pick the nearest existing screen and copy its skeleton.
3. If you're inventing a token, color, or shape that isn't in this doc,
   **stop and ask** instead of guessing.
