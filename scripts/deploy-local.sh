#!/usr/bin/env bash
# Install the release built by scripts/build-pi.sh onto *this* machine.
#
# Companion to scripts/deploy-pi.sh, which does the same job remotely over
# SSH from a dev machine. This variant is for when the script is already
# running on the Pi itself — e.g. a self-hosted GitHub Actions runner
# installed on the Pi, driven by .github/workflows/deploy.yml on every push
# to master.
#
# Usage:
#   scripts/deploy-local.sh                       # installs to ~/homehub
#   PI_REMOTE_DIR=/opt/homehub scripts/deploy-local.sh
#
# Installing/restarting the systemd units requires passwordless sudo for
# `systemctl` and `install` for whichever user runs this (see
# docs/INSTALL.md's CI/CD section for the sudoers snippet).
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
RELEASE="$ROOT/dist/release"
TARGET_DIR="${PI_REMOTE_DIR:-$HOME/homehub}"

if [ ! -x "$RELEASE/homehub" ]; then
  echo "release missing — run scripts/build-pi.sh first" >&2
  exit 1
fi

echo "==> Target: $TARGET_DIR"
mkdir -p "$TARGET_DIR" "$TARGET_DIR/data"

echo "==> Installing binary + transmitter helper + frontend"
rsync -a --delete \
  --exclude='data/' \
  --exclude='.env' \
  "$RELEASE/homehub" \
  "$RELEASE/nexa_tx.py" \
  "$RELEASE/ft007th_rx.py" \
  "$RELEASE/frontend" \
  "$TARGET_DIR/"

if [ -d "$RELEASE/matter-bridge" ]; then
  echo "==> Installing matter-bridge sources"
  rsync -a --delete \
    --exclude='data/' \
    --exclude='node_modules/' \
    --exclude='dist/' \
    "$RELEASE/matter-bridge" \
    "$TARGET_DIR/"

  echo "==> Installing matter-bridge deps + building"
  (cd "$TARGET_DIR/matter-bridge" && mkdir -p data && npm install && npm run build)

  echo "==> Installing matter-bridge systemd unit"
  sudo install -m 644 "$RELEASE/matter-bridge.service" /etc/systemd/system/matter-bridge.service
  sudo systemctl daemon-reload
  sudo systemctl enable matter-bridge
  sudo systemctl restart matter-bridge
fi

echo "==> Seeding .env (only if missing)"
if [ ! -f "$TARGET_DIR/.env" ]; then
  cp "$RELEASE/env.example" "$TARGET_DIR/.env"
fi
cp "$RELEASE/env.example" "$TARGET_DIR/env.example"

echo "==> Installing systemd unit"
sudo install -m 644 "$RELEASE/homehub.service" /etc/systemd/system/homehub.service
sudo systemctl daemon-reload
sudo systemctl enable homehub
sudo systemctl restart homehub

echo "==> Status:"
systemctl --no-pager --lines=10 status homehub || true
