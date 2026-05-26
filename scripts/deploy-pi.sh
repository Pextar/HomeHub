#!/usr/bin/env bash
# Deploy the release built by scripts/build-pi.sh to a Raspberry Pi over SSH.
#
# Usage:
#   scripts/deploy-pi.sh                         # uses claw@raspberrypi.local
#   scripts/deploy-pi.sh claw@192.168.1.42       # explicit target
#   PI_HOST=claw@rpi.local scripts/deploy-pi.sh  # via env
#
# Also install a local MQTT broker (Mosquitto) on the Pi:
#   SETUP_MOSQUITTO=1 scripts/deploy-pi.sh                              # anonymous, trusted LAN
#   SETUP_MOSQUITTO=1 MQTT_USERNAME=ctrl MQTT_PASSWORD=secret \
#     scripts/deploy-pi.sh                                             # with auth (recommended)
#
# Layout on the Pi (under the SSH user's home):
#   ~/rf-socket-controller/
#     rf-controller            (binary)
#     nexa_tx.py               (lgpio-backed Nexa 433MHz transmitter helper)
#     ft007th_rx.py            (lgpio-backed FT007TH 433MHz receiver helper)
#     frontend/dist/           (built UI)
#     data/                    (runtime state, never overwritten)
#     .env                     (seeded once from env.example, never overwritten)
#     rf-controller.service    (systemd unit, copied to /etc/systemd/system/)
#
# The systemd unit uses User=claw and /home/claw/... — if your SSH user is not
# "claw", edit deploy/rf-controller.service before deploying.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
RELEASE="$ROOT/dist/release"

HOST="${1:-${PI_HOST:-claw@raspberrypi.local}}"
REMOTE_DIR="${PI_REMOTE_DIR:-rf-socket-controller}"

if [ ! -x "$RELEASE/rf-controller" ]; then
  echo "release missing — run scripts/build-pi.sh first" >&2
  exit 1
fi

echo "==> Target: $HOST:~/$REMOTE_DIR"
ssh "$HOST" "mkdir -p '$REMOTE_DIR' '$REMOTE_DIR/data'"

echo "==> Syncing binary + transmitter helper + frontend"
rsync -av --delete \
  --exclude='data/' \
  --exclude='.env' \
  "$RELEASE/rf-controller" \
  "$RELEASE/nexa_tx.py" \
  "$RELEASE/ft007th_rx.py" \
  "$RELEASE/frontend" \
  "$HOST:$REMOTE_DIR/"

if [ -d "$RELEASE/matter-bridge" ]; then
  echo "==> Syncing matter-bridge sources"
  rsync -av --delete \
    --exclude='data/' \
    --exclude='node_modules/' \
    --exclude='dist/' \
    "$RELEASE/matter-bridge" \
    "$HOST:$REMOTE_DIR/"

  echo "==> Installing matter-bridge deps + building on the Pi"
  # node + npm must be installed on the Pi (apt install nodejs npm).
  ssh "$HOST" "cd '$REMOTE_DIR/matter-bridge' && \
    mkdir -p data && \
    npm install && \
    npm run build"

  echo "==> Installing matter-bridge systemd unit"
  rsync -av "$RELEASE/matter-bridge.service" "$HOST:$REMOTE_DIR/matter-bridge.service"
  ssh "$HOST" "sudo install -m 644 '$REMOTE_DIR/matter-bridge.service' /etc/systemd/system/matter-bridge.service \
    && sudo systemctl daemon-reload \
    && sudo systemctl enable matter-bridge \
    && sudo systemctl restart matter-bridge"
fi

echo "==> Seeding .env (only if missing)"
rsync -av --ignore-existing "$RELEASE/env.example" "$HOST:$REMOTE_DIR/.env"
rsync -av                   "$RELEASE/env.example" "$HOST:$REMOTE_DIR/env.example"

# Optional: install + configure Mosquitto on the Pi so the controller is also
# the MQTT broker. Opt-in (SETUP_MOSQUITTO=1) so deploys that already point at
# an external broker aren't disturbed. Pass MQTT_USERNAME/MQTT_PASSWORD to
# require auth (recommended); omit for anonymous access on a trusted LAN.
if [ "${SETUP_MOSQUITTO:-}" = "1" ]; then
  echo "==> Setting up local Mosquitto broker"
  rsync -av "$RELEASE/setup-mosquitto.sh" "$RELEASE/mosquitto.conf" "$HOST:$REMOTE_DIR/"
  ssh "$HOST" "chmod +x '$REMOTE_DIR/setup-mosquitto.sh' && \
    MQTT_USERNAME='${MQTT_USERNAME:-}' MQTT_PASSWORD='${MQTT_PASSWORD:-}' \
    ENV_FILE='$REMOTE_DIR/.env' '$REMOTE_DIR/setup-mosquitto.sh'"
fi

echo "==> Installing systemd unit"
rsync -av "$RELEASE/rf-controller.service" "$HOST:$REMOTE_DIR/rf-controller.service"
ssh "$HOST" "sudo install -m 644 '$REMOTE_DIR/rf-controller.service' /etc/systemd/system/rf-controller.service \
  && sudo systemctl daemon-reload \
  && sudo systemctl enable rf-controller \
  && sudo systemctl restart rf-controller"

echo "==> Status:"
ssh "$HOST" "systemctl --no-pager --lines=10 status rf-controller || true"

HTTP_PORT=$(ssh "$HOST" "grep -E '^PORT=' '$REMOTE_DIR/.env' 2>/dev/null | cut -d= -f2" || echo 8080)
HTTPS_PORT=$(ssh "$HOST" "grep -E '^HTTPS_PORT=' '$REMOTE_DIR/.env' 2>/dev/null | cut -d= -f2" || true)

cat <<EOF

Done. The UI should be reachable at:

  http://${HOST#*@}:${HTTP_PORT:-8080}
${HTTPS_PORT:+  https://${HOST#*@}:$HTTPS_PORT  (self-signed cert — accept the browser warning once)
}
If you have not changed AUTH_PASS yet:
  ssh $HOST 'nano ~/$REMOTE_DIR/.env && sudo systemctl restart rf-controller'

To enable HTTPS on an existing install (needed for QR scanning on a phone):
  ssh $HOST "echo 'HTTPS_PORT=8443' >> ~/$REMOTE_DIR/.env && sudo systemctl restart rf-controller"
EOF
