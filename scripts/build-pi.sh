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
  cp -R "$ROOT/matter-bridge/scripts"     "$RELEASE/matter-bridge/scripts"
  cp    "$ROOT/matter-bridge/package.json" "$RELEASE/matter-bridge/"
  cp    "$ROOT/matter-bridge/tsconfig.json" "$RELEASE/matter-bridge/"
  cp    "$ROOT/matter-bridge/README.md"    "$RELEASE/matter-bridge/"
  cp    "$ROOT/deploy/matter-bridge.service" "$RELEASE/"
fi

cp "$ROOT/deploy/rf-controller.service" "$RELEASE/"
cp "$ROOT/deploy/env.example"            "$RELEASE/"
cp "$ROOT/scripts/nexa_tx.py"            "$RELEASE/"
cp "$ROOT/scripts/ft007th_rx.py"         "$RELEASE/"
# Optional local MQTT broker (Mosquitto). deploy-pi.sh runs the setup script
# on the Pi when SETUP_MOSQUITTO=1; both files live side by side in the
# release so the script can find its config.
cp "$ROOT/scripts/setup-mosquitto.sh"    "$RELEASE/"
cp "$ROOT/deploy/mosquitto.conf"         "$RELEASE/"
# Optional local LLM assistant (Ollama). deploy-pi.sh runs setup-ollama.sh on
# the Pi when SETUP_OLLAMA=1; the script is self-contained (it writes its own
# systemd unit).
cp "$ROOT/scripts/setup-ollama.sh"       "$RELEASE/"

echo "==> Release ready:"
ls -lh "$RELEASE"
echo
echo "Next: scripts/deploy-pi.sh [user@host]"
