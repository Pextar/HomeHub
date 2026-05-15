#!/usr/bin/env bash
# Deploy the release built by scripts/build-pi.sh to a Raspberry Pi over SSH.
#
# Usage:
#   scripts/deploy-pi.sh                         # uses claw@raspberrypi.local
#   scripts/deploy-pi.sh claw@192.168.1.42       # explicit target
#   PI_HOST=claw@rpi.local scripts/deploy-pi.sh  # via env
#
# Layout on the Pi (under the SSH user's home):
#   ~/rf-socket-controller/
#     rf-controller            (binary)
#     nexa_tx.py               (lgpio-backed Nexa 433MHz transmitter helper)
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
  "$RELEASE/frontend" \
  "$HOST:$REMOTE_DIR/"

echo "==> Seeding .env (only if missing)"
rsync -av --ignore-existing "$RELEASE/env.example" "$HOST:$REMOTE_DIR/.env"
rsync -av                   "$RELEASE/env.example" "$HOST:$REMOTE_DIR/env.example"

echo "==> Installing systemd unit"
rsync -av "$RELEASE/rf-controller.service" "$HOST:$REMOTE_DIR/rf-controller.service"
ssh "$HOST" "sudo install -m 644 '$REMOTE_DIR/rf-controller.service' /etc/systemd/system/rf-controller.service \
  && sudo systemctl daemon-reload \
  && sudo systemctl enable rf-controller \
  && sudo systemctl restart rf-controller"

echo "==> Status:"
ssh "$HOST" "systemctl --no-pager --lines=10 status rf-controller || true"

cat <<EOF

Done. The UI should be reachable at:

  http://${HOST#*@}:\$(grep -E '^PORT=' '$REMOTE_DIR/.env' | cut -d= -f2 || echo 8080)

If you have not changed AUTH_PASS yet:
  ssh $HOST 'nano ~/$REMOTE_DIR/.env && sudo systemctl restart rf-controller'
EOF
