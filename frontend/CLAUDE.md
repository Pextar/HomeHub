# HomeHub frontend — Svelte conventions

Scoped guide for `frontend/`. See the root `CLAUDE.md` for project-wide
layout and workflow, and `backend/CLAUDE.md` for the Go side.

## ⚠️ Before touching any file in here

**Read `DESIGN.md` (repo root) in full before editing any `.svelte`,
`.css`, or `.ts` file in `frontend/src/`.** It is the single source of
truth for every visual decision. When something isn't explicitly
covered there, match the nearest existing pattern in `frontend/src/`
rather than inventing.

## Build & dev

```bash
cd frontend && npm run build   # production build (also used as type-check)
cd frontend && npm run dev     # dev server
```

The session startup hook builds the frontend automatically; if `dist/`
is already up-to-date it's skipped.

## Tests

Two layers, both under `npm run test`:

- **`lib/`** — stores and pure functions. A store built from runes needs
  an effect root, which `src/test-runes.svelte.ts` provides.
- **components** — mounted into jsdom with `@testing-library/svelte` and
  queried by role or label, so a test fails for the same reason a person
  would notice. See `src/test-setup.ts` for the pattern and the two jsdom
  gaps it patches (`matchMedia`, `Element.animate`).

Runes do not work inside a `.test.ts` file — those aren't run through the
Svelte compiler. Use a plain object for props a component mutates.

Worth a component test: anything a parent's stylesheet used to reach into
(a layout mode that became a prop), anything with an armed or optimistic
state, and any control §15 says must be *absent* rather than disabled.

## The panel store's roles

`PanelMusicStore` is ninety-odd members and no component wants more than a
fifth of them. It is declared as roles — `PanelRooms`, `PanelTransport`,
`PanelQueue`, `PanelGrouping`, … — and the store is their sum.

A component names the roles it uses:

```ts
let { music }: { music: PanelRooms & PanelQueue } = $props();
```

Structural typing does the rest: the real store satisfies any subset, the
parent still passes `{music}`, and nothing changes at runtime. What changes
is that reaching for something the component did not declare stops
compiling — and a test can hand it a small object instead of a whole store.

A component that passes `{music}` on to children needs their roles too, so
containers end up naming most of them; that is the honest reading, not a
reason to widen a leaf. `PanelBrowseRooms` takes the whole store for exactly
this reason.

## Layout

```
frontend/src/
├── app.css        ← global tokens (§3 of DESIGN.md lives here)
├── App.svelte     ← router; don't change view-transition wiring
├── lib/
│   ├── types/     ← interfaces, one file per domain; index.ts re-exports
│   ├── api/       ← typed fetch wrappers, one slice per domain
│   ├── music/     ← the music layer (below) — stores and pure rules
│   ├── panel-music/        ← the wall's own concerns
│   ├── panel-music.svelte.ts ← the panel store; its roles are in
│   │                           panel-music/types.ts
│   ├── stores.svelte.ts
│   └── utils.ts
├── components/    ← shared primitives (Modal, Icon, Switch, …)
│   ├── music/     ← the Music view's parts
│   ├── panel/     ← the wall's parts
│   └── kid/       ← the kid module's parts (DESIGN.md §17)
├── modals/        ← one Svelte file per sheet/dialog flow
└── views/         ← one Svelte file per top-level screen
```

`types.ts`, `api.ts` and the big components were single files once. When one
grows past what a reader can hold, split it by domain and leave the old name
re-exporting, rather than letting it keep growing.

## The music layer (`lib/music/`)

Four surfaces play music — the Music view, the wall's depth (`components/
panel/`), the kid module (`components/kid/`) and the device detail pages —
and each of them has, at various times, written its own copy of the same
rule. Every copy so far has drifted before anyone noticed, because a rule
written twice is a rule nobody owns.

So a rule that more than one surface needs lives here, and the surfaces
supply what genuinely differs as hooks:

| module | the one rule it owns |
|---|---|
| `catalog.ts` | what a row, card or top result says about an item; how a result set is shelved (`searchSections`); the kind vocabulary (`SEARCH_KINDS`) |
| `catalog-cache.svelte.ts` | reading an artist or record page once per URI |
| `catalog-stack.svelte.ts` | the ladder over those pages — push, pop, don't re-push the top |
| `fader.svelte.ts` | who owns a volume slider's value, the finger or the device |
| `volume.ts` | the clamp, and the mid-drag send throttle behind `fader` |
| `keyboard.svelte.ts` | how much of the screen the software keyboard has taken |
| `navigation.svelte.ts` | where the Music view is: its screen stack, its sheet run, and the one history entry over both |
| `device-sheets.ts` | the equipment sheets, and what has to be re-read when one changes something |
| `format.ts` | counts, durations, hours |

Before writing a rule about *what music means* inside a `.svelte` file, look
here for it. If it isn't here and a second surface will need it, put it here
with tests — these modules are plain enough to test directly, which is the
main reason they're worth extracting.

**Not everything shared should be.** The Music view's search screen doesn't
use `searchSections`, and its screen router doesn't use `catalog-stack`:
both are genuinely different from the wall's, and forcing them through a
shared shape would cost more than the copy it saves. Say so in a comment
when you decide that, so the next reader doesn't "finish the job".

## The wall's store (`lib/panel-music/`)

`panel-music.svelte.ts` keeps what is genuinely about *the panel* — the
sources, the featured room, polling, the transport — and each feature under
it has its own file. Every one takes the same two things: getters for what
moves under it (`featured`, `sources`) and the store's guarded `run`, so one
action disables the same control as any other.

`announce` · `grouping` · `history` · `queue` · `saved` · `sources` ·
`starting` · `timers` — plus `types.ts`, which holds the store's roles and
the `PanelRunner` they all take.

## Logic that isn't music

The same rule applies outside the music layer: a rule with no markup in it
belongs in `lib/`, not in a component's `<script module>`.

| module | what it owns |
|---|---|
| `rules.ts` | what an automation rule means — the target vocabulary, and `compileAction`, which translates the shape a rule is *authored* in into the shape it is *run* in |
| `console-language.ts` | how the console understands a name: resolution order, sentence parsing, and what Tab offers |

Both of these lived in a component and were imported *from* a `.svelte` file
by other components, which is the tell. Both had no test until they moved,
and both turned out to have a bug in them.

## Splitting a big file

Two different problems wear the same symptom, and they want opposite cures.

**A big script** is a component doing several jobs. Extract each into
`lib/` as a factory taking the bits it genuinely needs — usually getters,
since what it watches moves under it — and hand back what differs as hooks.
The win isn't the line count: a rule that only ever ran inside a mounted
component has no test, and every one extracted so far has turned out to have
a bug in it.

**A big stylesheet** is a component drawing several things. Extract each as
a component that takes its own CSS with it. Two guards make this safe:

- **`npm run check` names every selector left behind.** Nothing dead comes
  along, and nothing live gets stranded — trust the warnings, they're exact.
- **Scoped CSS doesn't reach across a component boundary**, in either
  direction. A rule that keys off an ancestor's state class (`.kb-open .x`,
  `.full .x`) stays with the ancestor, and shared chrome has to become a
  component of its own rather than a class two files both style. Where the
  parent's state genuinely has to reach a child, pass it as a prop.

When two blocks look like candidates for separate components, check whether
they share a stylesheet first. Two things styled alike are usually one thing
with a parameter — the room's fader and a speaker's are `KidVolumeRow`, and
an artist's page and a record's are `KidCatalogPage`. Splitting them would
have handed each a copy of the chrome, which is where the two versions start
disagreeing.

## Conventions

- **Svelte 5 runes** (`$state`, `$derived`, `$effect`, `$props`).
  No legacy reactive `$:` declarations.
- Component CSS is **scoped**. Global utility classes live in `app.css`.
- Always use **CSS variables from the token set** in `DESIGN.md §3`.
  Never hardcode a colour, radius, or shadow.
- **Semantic HTML + ARIA**: `aria-invalid` on invalid inputs,
  `aria-label` on icon-only buttons, `role="menu"` on overflow menus.
- **Touch targets**: ≥ 44×44 px on `@media (pointer: coarse)`.
- **iOS zoom prevention**: inputs must have `font-size: 16px` minimum
  on `@media (pointer: coarse)` (or `max-width: 600px`).
- **Numbers** (counts, %, temps, times, IDs): always `var(--font-mono)`
  with class `mono` or `font-feature-settings: "tnum" 1`.

## Key design rules (from DESIGN.md §2)

- No emoji outside the Kid module (`KidHome.svelte`, `KidLampPanel.svelte`,
  `KidScheduleSheet.svelte`).
- No gradients except `.tile.on` and the day/night timeline.
- No pure black; deepest surface is `--bg` (`#14130f`).
- No tabs inside views — use chip filters.
- No side drawers — use bottom sheets.
- No sheet opens another sheet — make the opener a screen (sheets may swap).
- No spinners — use the skeleton primitive.
- All numerics in `var(--font-mono)`.
- Icon-only buttons must have a ≥ 44×44 hit area on touch.

## Quick sanity checklist (from DESIGN.md §13)

- [ ] "ON" state uses `.tile.on` gradient + bulb glow, not a flat colour
- [ ] Every number uses `var(--font-mono)` with `tnum` enabled
- [ ] No new colours invented — only tokens from DESIGN.md §3
- [ ] Hit areas ≥ 44×44 on touch (`pointer: coarse`)
- [ ] `font-size: 16px` on mobile inputs (prevents iOS auto-zoom)
- [ ] Light theme verified (`[data-theme="light"]` on `<html>`)
- [ ] Reduced-motion query collapses animations to `0.001ms`
