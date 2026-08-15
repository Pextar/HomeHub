# HomeHub — project guide for Claude Code

This file covers project-wide layout only. Conventions live in
scoped guides that load automatically when you're working in that
subtree:

- **`backend/CLAUDE.md`** — Go conventions, package map, staged-flow
  and locking rules. Read before touching anything under `backend/`.
- **`frontend/CLAUDE.md`** — Svelte conventions, design rules, and the
  `DESIGN.md` gate. Read before touching anything under `frontend/`.

## Project layout

```
homehub/
├── DESIGN.md              ← design system (frontend/CLAUDE.md points here)
├── CLAUDE.md               ← this file
├── backend/CLAUDE.md       ← Go conventions + package map
├── frontend/CLAUDE.md      ← Svelte conventions + design rules
├── design/                 ← reference assets: mockup JSX, spec HTML,
│   ├── handoff-spec.html   │  design styles, screenshots
│   ├── styles.css
│   ├── screenshots/
│   └── *.jsx                ← design canvas prototypes
├── backend/                ← Go (net/http, gorilla/mux)
│   └── internal/            ← see backend/CLAUDE.md for the package map
└── frontend/                ← Svelte 5 + Vite
    └── src/                  ← see frontend/CLAUDE.md for the layout
```

## Development workflow

```bash
# Backend
cd backend && go build ./...
cd backend && go test ./...
cd backend && golangci-lint run   # CI runs this and it fails the build

# Frontend
cd frontend && npm run check   # svelte-check — the real type-check
cd frontend && npm run lint
cd frontend && npm run test
cd frontend && npm run build   # production build
cd frontend && npm run dev     # dev server
```

**`npm run build` is not a type-check.** Vite strips types without reading
them, so a type error inside a `.svelte` file builds cleanly and fails CI.
`npm run check` is the one that catches it — run that before pushing, along
with `golangci-lint run`, which enforces checks `go vet` does not.

The session startup hook builds the frontend automatically; if `dist/`
is already up-to-date it's skipped.
