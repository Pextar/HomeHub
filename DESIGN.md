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
  `KidLampPanel.svelte`, and `KidScheduleSheet.svelte`.)
- **No decorative SVG.** Icons only when functional. For missing imagery use
  the `.placeholder` striped fill with a monospace caption — never invent a
  picture.
- **No gradients** except the two sanctioned ones: the `.tile.on` warm
  gradient and the day/night timeline. No purple/blue brand gradients,
  ever.
- **No pure black.** The deepest surface is `#0a0907` (Console only). App
  background is `#14130f`.
- **No tabs inside views.** Use chip filters. *One sanctioned exception:*
  the Music subnav (§15) — a pill segmented control switching between a
  module's own screens. It is nav, not filtering, and it never reshapes the
  global tab bar. Don't generalise it to other views without design review.
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
  --font-sans: "Geist", -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
  --font-mono: "Geist Mono", ui-monospace, "SF Mono", Menlo, monospace;

  /* warm dark — default */
  --bg:        #14130f;
  --bg-2:      #1c1a15;
  --card:      #1f1d17;
  --card-2:    #26231c;
  --card-3:    #2e2a22;
  --hairline:  #2a2720;
  --border:    #34302a;
  --text:      #eceae4;
  --text-mute: #9c988e;
  --text-dim:  #66635c;

  /* accents */
  --on:        #f5bd6e;            /* incandescent amber — primary */
  --on-soft:   rgba(245,189,110,0.14);
  --on-glow:   rgba(245,189,110,0.45);
  --cool:      #84acc4;            /* secondary */
  --cool-soft: rgba(132,172,196,0.14);
  --good:      #9cc28a;
  --bad:       #e08a7a;
  --warn:      #e8b96b;

  /* protocol badges */
  --p-rf:      #f5a06e;
  --p-wifi:    #9cc28a;
  --p-matter:  #c4a4e0;
  --p-mqtt:    #e0c47a;

  /* radii */
  --r-sm: 10px;     /* nav items, small chips */
  --r-md: 14px;     /* inputs, segmented controls */
  --r-lg: 22px;     /* cards, tiles */
  --r-xl: 30px;     /* sheets, hero buttons */
  --r-pill: 999px;   /* chips, tab pill + lens */

  /* motion */
  --spring: cubic-bezier(0.34, 1.56, 0.64, 1);
}

[data-theme="light"] {
  --bg: #f5f1ea;  --bg-2: #efeae0;
  --card: #ffffff; --card-2: #faf6ee; --card-3: #f1ebde;
  --hairline: #e6dfd0; --border: #dcd3bf;
  --text: #1a1813; --text-mute: #6b6759; --text-dim: #9a9485;
  --on: #c97a1f; --on-soft: rgba(201,122,31,0.10); --on-glow: rgba(201,122,31,0.30);
  --cool: #426c84; --cool-soft: rgba(66,108,132,0.10);
  --good: #4e8a3d; --bad: #b14b3d;
}
```

---

## 4. Typography

| Role             | Family     | Size                          | Weight | Letter-spacing |
|------------------|------------|-------------------------------|--------|----------------|
| Display (h1)     | Geist      | 26–30 mobile · 28–40 desktop  | 600    | `-0.03em`      |
| Section (h2)     | Geist      | 17                            | 600    | `-0.02em`      |
| Body             | Geist      | 14                            | 400    | `-0.005em`     |
| Label / micro    | Geist Mono | 10.5–11.5, **UPPERCASE**      | 500    | `+0.08em`      |
| Numerics         | Geist Mono | any                           | 500    | `-0.01em`      |

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
  display: flex; flex-direction: column; gap: 12px;
  position: relative; overflow: hidden;
  transition: background 200ms ease, border-color 200ms ease;
}
.tile.on {
  background: linear-gradient(155deg, #2b2419 0%, #221d14 60%, #1d180f 100%);
  border-color: rgba(245,189,110,0.18);
}
.tile.on .tile-bulb {
  background: var(--on);
  box-shadow: 0 0 0 1px var(--on), 0 0 24px 4px var(--on-glow);
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
  background-image: repeating-linear-gradient(-45deg,
    var(--card-2) 0 8px, var(--card-3) 8px 16px);
  color: var(--text-dim);
  font-family: var(--font-mono);
  font-size: 11px;
  display: grid; place-items: center; text-align: center;
}
```

Caption format: `[ what goes here ]`, e.g. `[ floor plan SVG ]`.

### 6.8 Waveform — the "playing" motif (Music only)

A four-bar animated equaliser that marks anything **actually playing** in the
Music module. It replaces the plain status dot (§6.6) *only there* — a dot
says "on", a waveform says "audio is moving". Bars use `--on`, animate on a
staggered 950ms loop, and collapse to a static 8px height under reduced
motion. Nowhere outside Music **and its one satellite, the Home "Playing now"
card** (§15) — that card is Music's surface on Home, so it carries the
module's motifs rather than inventing quieter ones.

```css
.wave { display: flex; align-items: flex-end; gap: 2.5px; height: 13px; }
.wave i {
  width: 2.5px; border-radius: 1px; background: var(--on); height: 4px;
  animation: wv 950ms ease-in-out infinite;
}
.wave i:nth-child(1) { animation-delay: 0s; }
.wave i:nth-child(2) { animation-delay: 0.15s; }
.wave i:nth-child(3) { animation-delay: 0.3s; }
.wave i:nth-child(4) { animation-delay: 0.1s; }
@keyframes wv { 0%, 100% { height: 3px; } 50% { height: 13px; } }
@media (prefers-reduced-motion: reduce) { .wave i { animation: none; height: 8px; } }
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

The **assistant FAB** shares that band, bottom-right. A bar that runs the
full width there (the Music dock, the grouping bar) keeps its trailing
control clear of it by reserving **`--fab-clear`**, never a literal 64px —
because the FAB is optional: Settings › Interface switches it off per device,
which zeroes the token and gives those bars their edge back. It is also `0`
on desktop, where the rail entry and ⌘K are the launchers. A hidden FAB must
never cost the feature, so **the assistant also lives in the mobile More
sheet**, permanently — it has no route, so that sheet is its only fixed home.

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

| Event             | Duration | Easing                                    |
|-------------------|----------|-------------------------------------------|
| Press             | 80ms     | ease — scale to 0.97 (squish, not move)   |
| Switch thumb      | 220ms    | `var(--spring)`                           |
| Hover (desktop)   | 120ms    | ease — translateY(-1px)                   |
| View transition   | 240ms in / 140ms out | cubic-out — fly-in y:10, fade-out |
| Sheet open        | 280ms    | cubic-out from bottom; backdrop 200ms     |
| Reduced motion    | 0.001ms  | all of the above collapse                 |

Hover lift is **`@media (hover: hover)` only.** Don't apply on touch.

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

- Tabs nested inside a view → use chip filters (except Music's subnav, §15)
- A module that reshapes the global tab bar to its own destinations → the
  app-level nav is fixed; put module screens in a subnav instead
- Side drawer → use sheet
- Spinner → use skeleton
- Brand gradient (purple/blue/teal) → warm-only palette
- Pure black surface → `--bg` is the floor (Console is the only exception)
- Emoji outside the Kid module (KidHome / KidLampPanel / KidScheduleSheet)
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
- [ ] No emoji outside the Kid module (KidHome / KidLampPanel / KidScheduleSheet)
- [ ] No new colors invented — only tokens from §3
- [ ] Reduced-motion media query collapses your animations to 0.001ms
- [ ] Hit areas ≥ 44×44 on touch
- [ ] Light theme verified (toggle via `[data-theme="light"]` on `<html>`)

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
│   └── Icon.svelte          ← single <Icon name="..."> wrapping the path map
├── views/                   ← one .svelte per top-level surface
└── modals/                  ← sheets and confirms; one per flow
```

When adding a brand-new view, place it in `views/`, register the route in
`App.svelte`, and add an entry to the NavRail (desktop) and/or TabBar
(mobile) if it's top-level. Sub-screens don't get nav entries.

---

## 15. Music module (Sonos + Spotify)

The Music view (`views/Music.svelte`) is the one place with a live-audio
character. It reuses the shared primitives but layers a few module-specific
patterns on top. Keep these consistent if you extend it.

- **Music stays amber.** Music is a peer view in the nav, not a separate
  app, so it uses the same incandescent accent as everything else. A
  module-specific accent was tried and rejected: recolouring one top-level
  view invites every other view to claim its own hue, and the waveform
  already does the differentiating work. **Don't reintroduce a Music-only
  palette.**
- **Playing surface.** A group card, room puck, or the mini-player that is
  playing uses the sanctioned `.tile.on` warm gradient
  (`var(--tile-on-gradient)` + `var(--tile-on-border)`) — the same "ON" look
  as a lit device. No separate music gradient exists or should be invented.
- **Waveform, not dot.** Anything playing shows the §6.8 waveform where a
  status dot would otherwise sit — in group cards, room pucks, and the
  mini-player. Idle uses the `speaker` icon. This animated motif, not
  colour, is what marks Music as its own module.
- **Three screens behind a subnav.** Music has its own Home / Rooms /
  Search screens, switched by a sticky pill segmented control at the top of
  the view (`<Segmented full accent>`, `position: sticky`). This is the §2
  exception. Two rules make it work:
  - **The global tab bar never changes shape.** Music is one destination
    among the app's nav entries; entering it must not swap the app-level
    bar for module-specific tabs. The subnav lives *inside* the view,
    above the fold — never stacked on the tab bar.
  - **Subnav is navigation, not filtering.** Kind filters inside Search
    (Songs / Albums / Playlists) remain chip filters, per §2.
  - **It sticks as a band, not as a floating pill.** `top: 0`, bled over the
    shell's page padding (36px desktop, `--space-4` mobile) so nothing
    scrolls through a gap above it or past its sides, on the same glass as
    the player sheet's top bar: `--bg-bar`, `backdrop-filter`, and a fading
    bottom edge. A pill that stuck 8px down over an unbled `--bg` strip left
    cards visibly sliding through the gutters around it.

  Screen contents: **Home** = Playing now + Favorites + room chips
  (each opens that room's player; "Manage" jumps to Rooms). **Rooms** =
  the grouping puck grid + unreachable speakers. **Search** = Spotify.
  The mini-player and the full-player sheet persist across all three.
- **"Playing now" means playing.** The Home screen's section lists only
  groups that are actually playing; when none are, it collapses to a single
  quiet row (speaker icon, "Nothing playing", the reachable count, and a way
  onward) instead of one dead card per zone. Idle zones stay one tap away in
  the room chips below, which is where "open a room" belongs.
- **One visible destination.** Favorites and Search share a single "Play on"
  row — chips when there is more than one group, the group's name when there
  is only one, never nothing. Opening a room's player sets it too: the room
  you are looking at is the room the next favorite should land in. It
  defaults to whatever is already playing. Starting playback also **confirms
  in words** (`Playing · <track> · <room>`), because a tap has no visible
  effect until the next 5s poll lands.
- **Docked mini-player.** When something is playing, a compact bar sticks to
  the bottom of the view (`position: sticky`, cleared above the mobile tab
  bar and safe area): art, track, waveform, transport. Tapping it — or any
  "Playing now" card — expands the **full player**. It **survives a pause**:
  "Playing now" means playing and lets go of the zone, so the dock is where
  a paused zone stays one tap from playing again. Paused, it drops the
  `.tile.on` surface for a plain card and swaps the waveform for the idle
  speaker icon — nothing is moving, so nothing should say it is.
- **Transport is optimistic.** A play/pause tap flips every icon, waveform
  and card in the view immediately and holds that state until the poll
  agrees (or 6s passes, or the call fails and it rolls back). A five-second
  wait for a button to answer reads as a dropped tap.
- **Progress rides on the playing surface.** Cards that carry a transport —
  the "Playing now" cards and the dock — show a 2px hairline of progress
  along their bottom edge, extrapolated between polls like the player's
  scrubber. Sources that report no duration get no line rather than a
  made-up one, the same honesty the scrubber owes them.
- **The dock is a fallback, never a duplicate.** It carries the same track
  and the same transport as the Home screen's "Playing now" card — both
  gain prev/next from 430px up and drop them below it, so neither is ever
  the richer control — and so it stands down while that card is on screen
  and appears the moment the card scrolls away (`IntersectionObserver`, bottom inset discounting the band the
  dock and tab bar occupy). On Rooms and Search no such card exists, so the
  dock is simply always there. Reaching for the transport must never mean
  choosing between two identical controls stacked on one screen.
- **Full player = bottom sheet, art-led.** A bottom sheet on mobile (`--r-xl`
  top radius, `transition:sheet`, scrim, body-scroll-lock), a centered dialog
  ≥ 601px. Rendered inline (not the modal stack) so it stays live against the
  5s poll. Album art is the largest element on the screen
  (`min(340px, 78vw)`) and carries the bulb glow underneath
  (`box-shadow` in `--on-glow`) — the same light a lit device gives off.
  Below it, in order: title/artist, scrubber, transport, extras, volume.
  It carries the full §5 dismiss kit — **grabber, collapse chevron, close X,
  Escape, backdrop click, and drag-down** — because it is the only surface in
  the app that covers the nav; a user must never feel stuck in it. Covering
  the nav is literal: the scrim/sheet sit at `z-index: 125/126`, above the
  mobile bar (100) and the nav drawer (120), below the modal stack (150) so a
  confirm still lands on top.
- **The player answers the input the device actually has.** On a phone the
  art — the biggest target on the screen — **swipes sideways to change
  track**: it follows the finger at half speed, and past ~60px of travel it
  fires prev/next. Vertical always loses to the sheet's own drag/scroll, so
  the two gestures never fight. On a machine with a keyboard the sheet takes
  the transport keys instead (space play, ←/→ seek, shift ←/→ track, ↑/↓
  volume, m mute, q queue, s/r play modes, Escape out), advertised in one
  mono line under the transport that only renders on `(hover: hover) and
  (pointer: fine)`. A range input under the caret keeps its own arrow keys.
  Outside the player only Escape and "/" (jump to Search) are claimed — a
  module must not swallow keys the rest of the app might want.
- **The player drags down like every other sheet.** Same gesture as
  `components/Modal.svelte`, and it must stay that way: the top bar always
  drags (`touch-action: none`), the scroll body drags only from
  `scrollTop === 0` and only after a clear downward pull, so the queue still
  scrolls; past 90px the sheet rides the throw out instead of snapping back
  and replaying its own exit. Phones only — from 601px the dialog's transform
  carries its centering, so a drag offset would knock it off-centre.
- **The sticky top bar is glass, not a slab.** The grabber and header travel
  together as one `position: sticky` unit — a long queue must never scroll
  the way out off the screen — over a translucent, `backdrop-filter`-blurred
  band whose bottom ~22px fades out (`mask-image`). Art and rows dissolve as
  they pass underneath. An opaque bar was tried first and read as a floating
  block cutting the content in half.
- **The scrubber is a real control where the source allows it.** It is an
  `<input type="range">`, not a decorative bar, so it drags, takes arrow
  keys, and inherits the volume sliders' coarse-pointer sizing. Between
  polls the player extrapolates the position locally so it advances once a
  second rather than jumping every five. Sources that report no duration —
  radio, line-in, TV — get an honest `live stream` label instead, because
  the speaker would refuse a seek there.
- **Shuffle and repeat flank the transport**; they are `.t-mode` circles
  that take `--on-soft` + `--on` when engaged, deliberately quieter than the
  amber play button. Repeat cycles off → all → one on tap and reports its
  next state in `aria-label`. Crossfade is a plain `.chip.on` in the extras
  row, not a switch — it is a preference, not a device state. All three are
  group-level, so they only render when the coordinator reported a
  `group_state`; a follower's view never carries one.
- **The queue is a second pane inside the same sheet**, reached from an
  "Up next" row that names the actual next track. Not a segmented control:
  §2's Music exception covers the view's subnav only. The header's left
  button becomes a back chevron, the close X stays put, and Escape still
  leaves the player outright rather than stepping back a level. Rows show a
  mono track number, replaced by the §6.8 waveform on the one playing; that
  row takes `--tile-on-gradient`. Tapping a row jumps to it, the trailing X
  removes it, and "Clear" is destructive enough to need a confirm.
- **Queueing never interrupts.** Tapping a search result or favorite plays
  it now; "Play next" and "Add to queue" live behind the row's overflow
  (`role="menu"`), and favorites carry a corner `+` on the art. Each
  confirms with a toast naming the position it landed in — queueing onto a
  group playing radio is legal but silent, so the feedback has to be
  explicit. An open row menu takes focus and walks with the arrow keys, so
  queueing from the keyboard doesn't mean tabbing through every result.
- **The search box behaves like a search box.** Typing debounces (400ms) but
  Enter runs the query immediately, a clear X appears once there is
  something to clear (Escape does the same from inside the field), and
  arriving on the Search screen puts the caret in the box — on `(pointer:
  fine)` only, since auto-focus on a phone throws the software keyboard over
  the results.
- **Rooms grouping is a puck grid, not a list.** Each reachable speaker is a
  puck carrying **two intents on two targets**: the body opens that room's
  player, the corner circle selects it for grouping (amber ring + filled
  check when selected, 44px hit area on touch). The body used to select,
  which left the Rooms screen with no way through to playback at all —
  opening a room is the common intent, grouping is the deliberate one.
  Selecting 2+ raises a floating "Group" bar, which carries its own way out
  (a clear X, and Escape) — leaving selection mode must never mean
  un-tapping every circle one at a time. Existing multi-speaker zones sit
  inside a dashed enclosure (`--tile-on-border`) with an "Ungroup"
  affordance.
- **Home shows what's playing, and only that.** The dashboard's "Playing now"
  section (`components/NowPlaying.svelte`) is the only piece of Music that
  lives outside the Music view. It is a *glance* surface, so it is
  deliberately smaller than the module it points at: one card per playing
  group — art, the §6.8 waveform over the zone name, track, artist/album,
  play/pause (skips appear from 430px up) — on the `.tile.on` playing
  surface. Everything else about a group (scrubber, volume, queue, play
  modes) stays in the full player; tapping the card goes there. It owns its
  own slower poll because Sonos state doesn't live in the shared data store,
  and it renders **nothing at all** when there are no speakers, when the
  bridge is unreachable, or for a non-admin profile — a home without Sonos
  must not see a dead section, and a failed poll never raises a toast. Only
  a transport action the user actually took does.
- **Stay honest about the backend.** The local Sonos bridge exposes
  transport, volume, mute, join/leave, favorites, seek, play modes
  (shuffle × repeat × crossfade) and the group queue (browse, jump, add,
  play-next, remove, clear). It does **not** expose sleep timers, EQ /
  bass / treble, line-in or TV sources, or music-library browsing beyond
  favorites — so there is no UI for any of those. Don't add UI for
  capabilities the bridge can't back; wire the endpoint first. Two shapes
  worth copying when you do:
  - Settings that belong to a *group* rather than a speaker ride on the
    coordinator's `group_state` in the status poll, fetched in a second
    pass once the topology is known. Asking every follower would multiply
    the poll for no new information.
  - A control whose speaker-side call can be refused (seek on a stream)
    renders as a label explaining why, never as a dead control.

---

## 16. When in doubt

1. Open `index.html` in the design project — it's the source of truth.
2. Pick the nearest existing screen and copy its skeleton.
3. If you're inventing a token, color, or shape that isn't in this doc,
   **stop and ask** instead of guessing.
