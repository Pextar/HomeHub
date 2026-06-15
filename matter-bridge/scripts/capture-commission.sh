#!/usr/bin/env bash
#
# Capture ONE Thread commissioning attempt across all three layers, timestamped
# into a single log, so we can see exactly where the operational-discovery race
# is lost:
#
#   - matter-bridge journal (what matter.js discovers / tries / times out on)
#   - OTBR SRP server hosts  (does the new device register, when, with what addr)
#
# Usage (on the Pi):
#   ./capture-commission.sh            # captures for 180s
#   DUR=240 ./capture-commission.sh    # longer window
#
# Then: trigger the Thread commission from the UI right after it says GO.
#
set -u
DUR="${DUR:-180}"
OUT="/tmp/commission-capture-$(date +%H%M%S).log"
OTBR_PID="$(pgrep -x otbr-agent | head -1)"
SOCK="/proc/${OTBR_PID}/root/run/openthread-wpan0.sock"

echo "Writing -> $OUT  (for ${DUR}s)"
echo "Priming sudo..."; sudo -v

# Layer 1: matter-bridge log (background)
sudo journalctl -u matter-bridge -f -o short-precise --no-hostname >>"$OUT" 2>&1 &
JPID=$!

# Layer 2: SRP host table, polled (background)
(
  end=$((SECONDS + DUR))
  while [ "$SECONDS" -lt "$end" ]; do
    printf '\n===SRP %s===\n' "$(date +%T.%3N)" >>"$OUT"
    printf 'srp server host\n' | sudo nc -U -q2 "$SOCK" 2>/dev/null \
      | grep -E 'service.arpa|deleted:|addresses:' >>"$OUT"
    sleep 3
  done
) &
LPID=$!

echo "============================================================"
echo "  GO — trigger the Thread commission from the UI now."
echo "  Capturing for ${DUR}s..."
echo "============================================================"
sleep "$DUR"

sudo kill "$JPID" "$LPID" 2>/dev/null
sudo pkill -P "$JPID" 2>/dev/null
echo
echo "Done -> $OUT"
echo "Quick look at the decisive moments:"
echo "  grep -nE 'commissioning step|operational|[Dd]iscover|Reconnect|handler error|===SRP|service.arpa|deleted:|addresses:' $OUT"
