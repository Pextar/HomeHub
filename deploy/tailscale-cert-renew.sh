#!/usr/bin/env bash
# deploy/tailscale-cert-renew.sh
#
# Renews the Tailscale-issued TLS cert for this Pi and reloads Caddy.
# Placed in /etc/cron.weekly/ by tailscale-https-setup.sh.
#
# 'tailscale cert' is a no-op when the cert is still fresh (>30 days left),
# so running weekly is safe and costs nothing when no renewal is needed.
#
# Usage:
#   sudo deploy/tailscale-cert-renew.sh
#   # or via cron (installed automatically by tailscale-https-setup.sh):
#   # /etc/cron.weekly/tailscale-cert-renew

set -euo pipefail

info() { printf '\033[1;34m→\033[0m %s\n' "$*"; }
ok()   { printf '\033[1;32m✓\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

[[ $EUID -eq 0 ]] || die "run as root: sudo $0"

# ── resolve hostname ──────────────────────────────────────────────────────────

TS_STATUS=$(tailscale status --json 2>/dev/null) \
    || die "tailscale status failed — is Tailscale running?"

HOSTNAME=$(python3 - <<'PY'
import json, sys
d = json.loads(sys.stdin.read())
name = d.get("Self", {}).get("DNSName", "")
if not name:
    sys.exit(1)
print(name.rstrip("."))
PY
<<< "$TS_STATUS") || die "Could not determine Tailscale hostname."

CERT_DIR=/etc/tailscale/certs
CRT_FILE="${CERT_DIR}/${HOSTNAME}.crt"
KEY_FILE="${CERT_DIR}/${HOSTNAME}.key"

[[ -d "$CERT_DIR" ]] || die "Cert directory $CERT_DIR not found — run deploy/tailscale-https-setup.sh first."

# ── renew ─────────────────────────────────────────────────────────────────────

info "Renewing cert for $HOSTNAME (no-op if still fresh)…"
tailscale cert \
    --cert-file "$CRT_FILE" \
    --key-file  "$KEY_FILE" \
    "$HOSTNAME"
ok "Cert up to date"

# Re-apply key permissions in case tailscale cert reset them.
if getent group caddy &>/dev/null; then
    chown root:caddy "$KEY_FILE"
    chmod 640        "$KEY_FILE"
fi

# ── reload Caddy ──────────────────────────────────────────────────────────────

info "Reloading Caddy…"
systemctl reload caddy
ok "Caddy reloaded — cert is live"
