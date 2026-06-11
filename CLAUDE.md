# HomeHub — project guide for Claude Code

## ⚠️ Before touching any frontend file

**Read `DESIGN.md` in full before editing any `.svelte`, `.css`, or `.ts`
file in `frontend/src/`.** It is the single source of truth for every
visual decision. When something isn't explicitly covered there, match
the nearest existing pattern in `frontend/src/` rather than inventing.

---

## Project layout

```
rf-socket-controller/
├── DESIGN.md              ← design system (read first, always)
├── CLAUDE.md              ← this file
├── design/                ← reference assets: mockup JSX, spec HTML,
│   ├── handoff-spec.html  │  design styles, screenshots
│   ├── styles.css
│   ├── screenshots/
│   └── *.jsx              ← design canvas prototypes
├── backend/               ← Go (net/http, gorilla/mux)
│   └── internal/
│       ├── api/           ← HTTP handlers
│       ├── store/         ← state, persistence, validation, actions
│       ├── scheduler/     ← schedule + automation engine (5-sec tick)
│       ├── rf/            ← 433 MHz transmitter
│       ├── tasmota/       ← Wi-Fi smart-light bridge
│       └── matter/        ← Matter/Thread bridge
└── frontend/              ← Svelte 5 + Vite
    └── src/
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

---

## Development workflow

```bash
# Backend
cd backend && go build ./...
cd backend && go test ./...

# Frontend
cd frontend && npm run build   # production build (also used as type-check)
cd frontend && npm run dev     # dev server
```

The session startup hook builds the frontend automatically; if `dist/`
is already up-to-date it's skipped.

---

## Backend conventions

- **All state lives in `store.Store`**; callers acquire `Mu` (RWMutex)
  for multi-step operations. Methods annotated "Caller must hold Mu"
  do not lock themselves.
- **`ValidateX` functions** normalise and check; they are always called
  before persisting. Never skip them.
- **`Save()`** writes every JSON file atomically. Call it after any
  mutation; callers hold the lock when calling it.
- **`CascadeDeleteSocket`** must be kept in sync with any new field that
  references a socket ID.
- Scheduler ticks every 5 s; automation engine runs inside the same
  tick. Both use the staged flow below.
- **Multi-socket fan-out** (bulk, group, room, scene, scheduler,
  automations) uses the staged flow in `store/staged.go`:
  `StageAction`/`StageSocketSend` under `Mu` → `SendStaged` off-lock →
  `ApplyStaged` under `Mu`, then `Save()` and `FlushLights()`. Device I/O
  must never run while `Mu` is held. Single-socket toggles use
  `ApplyState`, which transmits synchronously so the HTTP response can
  report the device error directly.
- All transmissions go through `store.Transmit` — never `RF.Send`
  directly. It serializes 433 MHz sends (`txMu`) so concurrent
  transmissions can't overlap on air.
- Smart-light bridge calls (Tasmota, Matter) are always deferred to
  `FlushLights()` so they never block the store lock.

---

## Frontend conventions

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

---

## Key design rules (from DESIGN.md §2)

- No emoji outside `KidHome.svelte`.
- No gradients except `.tile.on` and the day/night timeline.
- No pure black; deepest surface is `--bg` (`#14130f`).
- No tabs inside views — use chip filters.
- No side drawers — use bottom sheets.
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
