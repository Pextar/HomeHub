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

## Layout

```
frontend/src/
├── app.css        ← global tokens (§3 of DESIGN.md lives here)
├── App.svelte     ← router; don't change view-transition wiring
├── lib/
│   ├── types.ts   ← all TypeScript interfaces
│   ├── api.ts     ← typed fetch wrappers
│   ├── stores.svelte.ts
│   └── utils.ts
├── components/    ← shared primitives (Modal, Icon, Switch, …)
├── modals/        ← one Svelte file per sheet/dialog flow
└── views/         ← one Svelte file per top-level screen
```

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
