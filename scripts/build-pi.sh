#!/usr/bin/env bash
# Build a release for the Raspberry Pi.
#
# Output: dist/release/{rf-controller, frontend/dist/, rf-controller.service, env.example}
#
# Defaults to 64-bit Pi (Pi 3/4/5 running 64-bit Pi OS / Ubuntu).
# For 32-bit Pi OS or Pi Zero/1/2:
#   GOARCH=arm GOARM=7 scripts/build-pi.sh
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
RELEASE="$ROOT/dist/release"

GOARCH="${GOARCH:-arm64}"
GOARM_VAR=()
if [ "$GOARCH" = "arm" ]; then
  GOARM_VAR=("GOARM=${GOARM:-7}")
fi

echo "==> Cleaning $RELEASE"
rm -rf "$RELEASE"
mkdir -p "$RELEASE/frontend"

echo "==> Building frontend"
(
  cd "$ROOT/frontend"
  if [ ! -d node_modules ]; then npm install; fi
  npm run build
)
cp -R "$ROOT/frontend/dist" "$RELEASE/frontend/dist"

echo "==> Cross-compiling backend (GOOS=linux GOARCH=$GOARCH${GOARM:+ GOARM=$GOARM})"
(
  cd "$ROOT/backend"
  env CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" ${GOARM_VAR[@]+"${GOARM_VAR[@]}"} \
    go build -trimpath -ldflags='-s -w' -o "$RELEASE/rf-controller" .
)

# Matter bridge is platform-independent (JS) — package the sources and let
# the Pi run `npm install --omit=dev && npm run build`. We skip the build
# here because matter.js pulls in native bindings (BLE) that must be
# built on the Pi itself.
if [ "${SKIP_MATTER_BRIDGE:-}" != "1" ]; then
  echo "==> Packaging matter-bridge sources"
  mkdir -p "$RELEASE/matter-bridge"
  cp -R "$ROOT/matter-bridge/src"          "$RELEASE/matter-bridge/src"
  cp    "$ROOT/matter-bridge/package.json" "$RELEASE/matter-bridge/"
  cp    "$ROOT/matter-bridge/tsconfig.json" "$RELEASE/matter-bridge/"
  cp    "$ROOT/matter-bridge/README.md"    "$RELEASE/matter-bridge/"
  cp    "$ROOT/deploy/matter-bridge.service" "$RELEASE/"
fi

cp "$ROOT/deploy/rf-controller.service" "$RELEASE/"
cp "$ROOT/deploy/env.example"            "$RELEASE/"
cp "$ROOT/scripts/nexa_tx.py"            "$RELEASE/"

echo "==> Release ready:"
ls -lh "$RELEASE"
echo
echo "Next: scripts/deploy-pi.sh [user@host]"
