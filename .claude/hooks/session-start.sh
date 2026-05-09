#!/bin/bash
# SessionStart hook for the RF Socket Controller repo.
#
# Prepares the web sandbox so Claude can immediately run / test / lint
# both halves of the project:
#
#   1. Backend  — `cd backend && go mod tidy` to populate go.sum.
#   2. Frontend — `cd frontend && npm install && npm run build` so
#      frontend/dist/ exists for the Go server to serve.
#
# The hook is idempotent and short-circuits work that's already done.
# It only runs in Claude Code on the web (CLAUDE_CODE_REMOTE=true).
#
# Network failures (registry blocked, proxy unreachable) are reported as
# warnings — the session must still start in air-gapped sandboxes.

set -uo pipefail

if [ "${CLAUDE_CODE_REMOTE:-}" != "true" ]; then
    exit 0
fi

cd "$CLAUDE_PROJECT_DIR"

echo ">> SessionStart: preparing rf-socket-controller workspace"

# ---------- Backend (Go) ----------
if [ -f backend/go.mod ]; then
    if [ ! -f backend/go.sum ]; then
        echo ">> backend: go mod tidy (no go.sum yet)"
        if ! (cd backend && go mod tidy); then
            echo ">> WARNING: go mod tidy failed (network restricted?)"
            echo ">>          Run it manually once network is available."
        fi
    else
        echo ">> backend: go.sum present, skipping tidy"
    fi
fi

# ---------- Frontend (npm + Vite + Svelte) ----------
if [ -f frontend/package.json ]; then
    cd frontend

    # Reuse node_modules across sessions when the lockfile hasn't moved.
    LOCK_HASH=""
    if [ -f package-lock.json ]; then
        LOCK_HASH=$(sha256sum package-lock.json | awk '{print $1}')
    elif [ -f package.json ]; then
        LOCK_HASH=$(sha256sum package.json | awk '{print $1}')
    fi

    INSTALL_STAMP="node_modules/.session-start-hook.lock-hash"
    NEEDS_INSTALL=1
    if [ -d node_modules ] && [ -f "$INSTALL_STAMP" ] && \
       [ "$(cat "$INSTALL_STAMP" 2>/dev/null)" = "$LOCK_HASH" ]; then
        NEEDS_INSTALL=0
    fi

    INSTALL_OK=1
    if [ "$NEEDS_INSTALL" = "1" ]; then
        echo ">> frontend: npm install (lockfile changed or first run)"
        # `npm install` (not `npm ci`) is friendlier to caching when no
        # lockfile is checked in.
        if npm install --no-audit --no-fund --loglevel=error; then
            mkdir -p node_modules
            printf "%s" "$LOCK_HASH" > "$INSTALL_STAMP"
        else
            echo ">> WARNING: npm install failed (registry blocked?)"
            echo ">>          Run \`npm install\` manually once network is available."
            INSTALL_OK=0
        fi
    else
        echo ">> frontend: dependencies up-to-date, skipping install"
    fi

    # Always (re)build so frontend/dist/ exists when possible. Vite's
    # incremental cache makes subsequent builds fast.
    if [ "$INSTALL_OK" = "1" ] && [ -d node_modules ]; then
        if [ ! -f dist/index.html ] || [ src -nt dist ] 2>/dev/null; then
            echo ">> frontend: npm run build"
            if ! npm run build --silent; then
                echo ">> WARNING: frontend build failed (see output above)."
            fi
        else
            echo ">> frontend: dist/ up-to-date, skipping build"
        fi
    else
        echo ">> frontend: skipping build (deps not installed)"
    fi

    cd "$CLAUDE_PROJECT_DIR"
fi

echo ">> SessionStart: workspace ready"
