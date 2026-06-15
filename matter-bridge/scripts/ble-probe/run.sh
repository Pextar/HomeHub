#!/usr/bin/env bash
#
# BLE-in-netns probe — run this ON THE PI (the appliance running the bridge).
#
# Answers the one unknown blocking the macvlan/eth0 isolation plan: does the
# Bluetooth HCI adapter stay reachable when matter-bridge runs in its own
# network namespace? It runs the probe under three conditions and prints a
# verdict telling you (and Claude) exactly which architecture is viable.
#
#   A) isolated netns + NET_ADMIN/NET_RAW   <- the real deploy condition
#   B) isolated netns + --privileged        <- diagnostic: caps vs netns?
#   C) host netns (--net=host)              <- sanity: does BLE work at all here?
#
# Usage:   ./run.sh
#   MATTER_BRIDGE_HCI_ID=0   adapter index (default 0)
#   PROBE_SCAN_MS=15000      scan window per run (ms)
#
set -uo pipefail

HCI_ID="${MATTER_BRIDGE_HCI_ID:-0}"
SCAN_MS="${PROBE_SCAN_MS:-15000}"
IMG="ble-netns-probe"
DIR="$(cd "$(dirname "$0")" && pwd)"

if ! command -v docker >/dev/null 2>&1; then
    echo "docker not found on PATH — run this on the Pi/appliance." >&2
    exit 1
fi

echo "==> Building probe image (first build compiles noble; ~2-3 min on a Pi)"
docker build -t "$IMG" "$DIR" || { echo "image build failed"; exit 1; }

run() {
    local label="$1"; shift
    echo
    echo "=================================================================="
    echo "  RUN $label"
    echo "=================================================================="
    docker run --rm \
        -e MATTER_BRIDGE_HCI_ID="$HCI_ID" \
        -e PROBE_SCAN_MS="$SCAN_MS" \
        "$@" "$IMG"
    local rc=$?
    echo ">>> RUN $label exit code: $rc"
    return $rc
}

# --- A: the real deploy condition -------------------------------------------
if run "A (isolated netns + NET_ADMIN/NET_RAW — real deploy condition)" \
        --network bridge --cap-add=NET_ADMIN --cap-add=NET_RAW; then
    cat <<'EOF'

╔════════════════════════════════════════════════════════════════════╗
║ VERDICT: PASS — BLE works in an isolated netns with minimal caps.   ║
║ macvlan/eth0 isolation is SAFE. Tell Claude to build the deploy.    ║
╚════════════════════════════════════════════════════════════════════╝
EOF
    exit 0
fi

# --- B: does privileged change anything? ------------------------------------
if run "B (isolated netns + --privileged — diagnostic)" \
        --network bridge --privileged; then
    cat <<'EOF'

╔════════════════════════════════════════════════════════════════════╗
║ VERDICT: PARTIAL — works only with --privileged, not plain caps.    ║
║ Isolation is viable but the container needs --privileged (or a      ║
║ specific extra cap). Paste this output to Claude.                   ║
╚════════════════════════════════════════════════════════════════════╝
EOF
    exit 0
fi

# --- C: sanity — does BLE work at all from a container here? -----------------
if run "C (host netns --net=host — sanity)" \
        --network host --cap-add=NET_ADMIN --cap-add=NET_RAW; then
    cat <<'EOF'

╔════════════════════════════════════════════════════════════════════╗
║ VERDICT: NETNS BLOCKS BLE — works on host net, fails when isolated. ║
║ AF_BLUETOOTH is netns-scoped on this kernel. We must NOT put the    ║
║ commissioning process in a separate netns as-is. Different          ║
║ architecture needed (split BLE from mDNS, or pass the BT controller ║
║ into the namespace). Paste this output to Claude.                   ║
╚════════════════════════════════════════════════════════════════════╝
EOF
    exit 1
fi

cat <<EOF

╔════════════════════════════════════════════════════════════════════╗
║ VERDICT: BLE FAILED EVEN ON HOST NET. The adapter isn't usable from ║
║ a container at all here. Check: hci${HCI_ID} is up (hciconfig -a),       ║
║ bluetoothd state, kernel BT modules. Paste this output to Claude.   ║
╚════════════════════════════════════════════════════════════════════╝
EOF
exit 1
