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

# Frontend
cd frontend && npm run build   # production build (also used as type-check)
cd frontend && npm run dev     # dev server
```

The session startup hook builds the frontend automatically; if `dist/`
is already up-to-date it's skipped.
