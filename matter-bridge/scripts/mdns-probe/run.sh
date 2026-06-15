#!/usr/bin/env bash
#
# mDNS discovery probe — run this ON THE PI.
#
# Answers the question both remaining architectures depend on: does giving
# matter's mDNS a CLEAN, private :5353 actually let it discover _matter._tcp
# devices? It runs the same probe twice:
#
#   HOST     : --net=host           -> contended :5353 (what the bridge has today)
#   ISOLATED : macvlan on eth0       -> private :5353, still on the LAN
#
# If ISOLATED finds _matter records and HOST finds few/none, contention is the
# cause and a clean-mDNS architecture (separate host, or python-matter-server's
# avahi delegation) will fix it. If ISOLATED ALSO finds nothing, the problem is
# elsewhere (OTBR advertising / OMR routing / multi-BR) and we look there.
#
# Uses your ONLINE Wi-Fi Matter bulbs as the stable target (the Thread device
# keeps rolling back, so it's not reliably advertising).
#
# Env: IFACE=eth0  PROBE_SCAN_MS=12000  PROBE_IP=<free addr on eth0 subnet>
#
set -uo pipefail

IFACE="${IFACE:-eth0}"
SCAN_MS="${PROBE_SCAN_MS:-12000}"
IMG="mdns-netns-probe"
NET="mdns-probe-net"
DIR="$(cd "$(dirname "$0")" && pwd)"

command -v docker >/dev/null 2>&1 || { echo "docker not found — run on the Pi." >&2; exit 1; }

echo "==> Building probe image (pure JS, fast)"
docker build -t "$IMG" "$DIR" || { echo "build failed"; exit 1; }

# ---- RUN 1: host netns (today's contended :5353) ---------------------------
echo
echo "=================================================================="
echo "  RUN HOST (--net=host — contended :5353, today's condition)"
echo "=================================================================="
docker run --rm --network host -e PROBE_SCAN_MS="$SCAN_MS" "$IMG"
host_rc=$?
echo ">>> HOST exit code: $host_rc"

# ---- RUN 2: isolated macvlan netns (private :5353, still on LAN) ------------
SUBNET="$(ip -o -f inet addr show "$IFACE" 2>/dev/null | awk '{print $4}' | head -1)"
GW="$(ip route 2>/dev/null | awk -v i="$IFACE" '/^default/ && $0 ~ i {print $3; exit}')"

echo
if [ -z "$SUBNET" ] || [ -z "$GW" ]; then
    echo "!! Could not auto-detect subnet/gateway for $IFACE (subnet='$SUBNET' gw='$GW')."
    echo "   Skipping the isolated run. Set IFACE correctly and re-run."
    iso_rc="skipped"
else
    # Convert the /prefix host address to a network address for docker macvlan.
    NETADDR="$(python3 - "$SUBNET" <<'PY' 2>/dev/null || echo "")
import ipaddress,sys
print(ipaddress.ip_interface(sys.argv[1]).network)
PY
)"
    NETADDR="${NETADDR:-$SUBNET}"
    echo "==> Creating macvlan network on $IFACE  subnet=$NETADDR gw=$GW"
    docker network rm "$NET" >/dev/null 2>&1 || true
    if docker network create -d macvlan --subnet="$NETADDR" --gateway="$GW" -o parent="$IFACE" "$NET" >/dev/null; then
        IPARG=()
        [ -n "${PROBE_IP:-}" ] && IPARG=(--ip "$PROBE_IP")
        echo "=================================================================="
        echo "  RUN ISOLATED (macvlan on $IFACE — private :5353, on the LAN)"
        echo "=================================================================="
        docker run --rm --network "$NET" "${IPARG[@]}" -e PROBE_SCAN_MS="$SCAN_MS" "$IMG"
        iso_rc=$?
        echo ">>> ISOLATED exit code: $iso_rc"
        docker network rm "$NET" >/dev/null 2>&1 || true
    else
        echo "!! macvlan network create failed (a stray DHCP collision on auto-IP can cause this)."
        echo "   Retry with a known-free address, e.g.:  PROBE_IP=192.168.68.250 ./run.sh"
        iso_rc="error"
    fi
fi

# ---- VERDICT ---------------------------------------------------------------
echo
echo "╔════════════════════════════════════════════════════════════════════╗"
printf "║ HOST run exit=%-4s   ISOLATED run exit=%-8s                      ║\n" "$host_rc" "$iso_rc"
echo "╠════════════════════════════════════════════════════════════════════╣"
if [ "$host_rc" != "0" ] && [ "$iso_rc" = "0" ]; then
    echo "║ CONTENTION CONFIRMED: clean :5353 finds devices, contended doesn't. ║"
    echo "║ -> separate-host OR python-matter-server WILL fix it. Pick one.     ║"
elif [ "$host_rc" = "0" ] && [ "$iso_rc" = "0" ]; then
    echo "║ Both find devices: mDNS RECEPTION isn't the blocker. The failure is ║"
    echo "║ elsewhere (OTBR advert / OMR routing / device rollback). Rethink.   ║"
elif [ "$iso_rc" = "0" ] && [ "$host_rc" = "0" ]; then
    echo "║ inconclusive — see per-run summaries above.                        ║"
else
    echo "║ Neither found _matter records: look at OTBR advertising / OMR /     ║"
    echo "║ multi-BR, not the bridge. Paste both summaries to Claude.           ║"
fi
echo "╚════════════════════════════════════════════════════════════════════╝"
echo "Paste BOTH run summaries (the _matter instances lists) back to Claude."
