/**
 * Patches to matter.js node_modules that fix Thread-over-concurrent-commissioning bugs.
 *
 * Run automatically via the "postinstall" npm hook after every `npm ci`.
 * Each patch is idempotent — safe to apply multiple times.
 *
 * Patch 1 — PeerSet.js: BtpFlowError falls back to mDNS instead of aborting.
 *   Root cause: during Thread commissioning, the device closes BLE right after
 *   ConnectNetwork succeeds.  matter.js then tries the last-known operational
 *   address (still the BLE/PASE address) inside #reconnectKnownAddress.  That
 *   call throws BtpFlowError ("BTP session is not active") which is NOT caught
 *   as a NoResponseTimeoutError, so it propagates and aborts commissioning at
 *   exactly 30s instead of waiting 120s for the Thread mDNS record to appear.
 *
 * Patch 2 — ControllerCommissioningFlow.js: minimum 120s failsafe arming.
 *   Many Thread devices report failSafeExpiryLengthSeconds = 60, making the
 *   periodic re-arm timer fire every 30s.  Raising the minimum to 120s means
 *   the timer only fires once at 60s — less noise, and fewer opportunities for
 *   the re-arm to race with BLE closing.
 */

import { readFileSync, writeFileSync, existsSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(fileURLToPath(import.meta.url), "../../node_modules");

function patch(relPath, description, from, to) {
  const absPath = resolve(root, relPath);
  if (!existsSync(absPath)) {
    console.warn(
      `[patch-matter] SKIP ${description} — file not found: ${relPath}`,
    );
    return;
  }
  let src = readFileSync(absPath, "utf8");
  if (src.includes(to)) {
    console.log(`[patch-matter] already applied: ${description}`);
    return;
  }
  if (!src.includes(from)) {
    console.warn(
      `[patch-matter] SKIP ${description} — expected text not found (version mismatch?)`,
    );
    return;
  }
  writeFileSync(absPath, src.replace(from, to), "utf8");
  console.log(`[patch-matter] applied: ${description}`);
}

// Patch 1: treat BtpFlowError the same as NoResponseTimeoutError so that
// #reconnectKnownAddress falls back to mDNS discovery instead of aborting.
patch(
  "@matter/protocol/dist/esm/peer/PeerSet.js",
  "PeerSet: BtpFlowError falls back to mDNS in #reconnectKnownAddress",
  `if (error instanceof NoResponseTimeoutError) {`,
  `if (error instanceof NoResponseTimeoutError || error.constructor?.name === "BtpFlowError") {`,
);

// Patch 2: raise minimum failsafe to 120s so the re-arm timer fires at most
// once per 60s rather than every 30s when the device reports a 60s failsafe.
patch(
  "@matter/protocol/dist/esm/peer/ControllerCommissioningFlow.js",
  "CommissioningFlow: minimum 120s failsafe arming interval",
  `this.#failSafeTimeMs = basicCommissioningInfo.failSafeExpiryLengthSeconds * 1e3;`,
  `this.#failSafeTimeMs = Math.max(basicCommissioningInfo.failSafeExpiryLengthSeconds, 120) * 1e3;`,
);

// Patch 3: do not start the re-arm failsafe timer during the reconnect step.
//
// After connectNetwork the device switches to Thread/Wi-Fi and stops
// responding to BLE commands. The re-arm timer fires at failSafeTimeMs/2
// (60s with Patch 2) and calls armFailSafe over BLE. That command never gets
// a response and times out after 35 s, invalidating the BLE interaction
// client. The invalidation propagates and aborts the commissioning before the
// 120s mDNS discovery window closes — exactly the failure we see in logs as
// "Error while re-arming failsafe ... Expected response data missing within
// timeout of 35000ms" followed immediately by "handler error".
//
// Skipping the re-arm is safe: the failsafe was just set to 120s at step 12.3
// (the last arm before connectNetwork), matching the mDNS discovery window.
// If mDNS finds the device in time, CommissioningComplete is sent and the
// failsafe is disarmed. If not, the device rolls back regardless.
patch(
  "@matter/protocol/dist/esm/peer/ControllerCommissioningFlow.js",
  "CommissioningFlow: disable re-arm timer during reconnect step",
  `    if (isConcurrentFlow) {
      reArmFailsafeInterval.start();
    }`,
  `    // Patched: re-arm timer disabled — see scripts/patch-matter.mjs Patch 3.`,
);

// Patch 4: arm the device commissioning failsafe for at least 180s.
//
// Root cause (observed on a Thread commission that reached the reconnect step):
// after ConnectNetwork the device must attach to the Thread mesh and register
// SRP before the controller can discover it over CASE-over-IP. A real device
// took ~80s to register SRP. But stock 0.12.6 arms the failsafe with the
// device's reported failSafeExpiryLengthSeconds (60s) and NO floor, so the
// device's 60s failsafe expired first, rolled the commissioning back, and
// tombstoned the SRP record it had just registered. matter.js then queried for
// a record that no longer existed and timed out after its 120s discovery window
// ("operational device cannot be found on the network").
//
// Arming >=180s keeps the device alive past the 120s discovery window so a slow
// Thread attach/SRP registration is still found. Bounded by the device's own
// maxCumulativeFailsafeSeconds so we never request more than it allows (the
// device rejects an over-long failsafe). Note Patch 2 already raises the
// internal #failSafeTimeMs floor; this patch fixes the value actually SENT in
// the armFailSafe command.
patch(
  "@matter/protocol/dist/esm/peer/ControllerCommissioningFlow.js",
  "CommissioningFlow: arm failsafe >=180s so slow Thread SRP registration isn't rolled back",
  `expiryLengthSeconds: this.#collectedCommissioningData.basicCommissioningInfo?.failSafeExpiryLengthSeconds`,
  `expiryLengthSeconds: Math.min(Math.max(this.#collectedCommissioningData.basicCommissioningInfo?.failSafeExpiryLengthSeconds ?? 60, 180), this.#collectedCommissioningData.basicCommissioningInfo?.maxCumulativeFailsafeSeconds ?? 900)`,
);

// Patch 5: extend the operational (CASE-over-IP) discovery window from 120s to
// 170s during the post-connectNetwork reconnect step.
//
// Root cause (observed live on a successful Thread join): after ConnectNetwork
// the device attaches to the Thread mesh and registers SRP, but the OTBR
// advertising proxy can take >120s to mirror that SRP record into LAN mDNS
// (the `_matter._tcp` operational record). Stock 0.12.6 hardcodes
// `timeoutSeconds: 120` for this discovery, so matter.js gives up ~120s in and
// throws "operational device cannot be found on the network" — even though the
// record appears on the wire seconds later (confirmed via avahi-browse on eth0).
//
// Patch 4 already keeps the DEVICE alive for 180s (armFailSafe >=180). This
// patch widens matter.js's own discovery window to 170s so it is still looking
// when the slow advertising-proxy record finally appears, while staying under
// the 180s device failsafe (leaving ~10s for the CASE handshake to complete
// before the device would roll back).
patch(
  "@matter/protocol/dist/esm/peer/ControllerCommissioner.js",
  "ControllerCommissioner: extend operational discovery window 120s -> 170s",
  `timeoutSeconds: 120,`,
  `timeoutSeconds: 170,`,
);

// Patch 6: cap the mDNS re-query backoff at 15s (was 1 hour).
//
// Root cause (observed live, and the reason Patch 5 alone wasn't enough):
// MdnsScanner doubles its DNS-SD query interval each round — 1.5, 3, 6, 12, 24,
// 48, 96, 192s … capped only at 60*60 (1 hour). So during a commissioning
// discovery window the LAST active query goes out at ~T+94s and the next would
// not be until ~T+190s. But the OTBR advertising proxy mirrors a freshly-joined
// Thread device's SRP record into LAN mDNS only at ~T+128s — i.e. AFTER
// matter.js has gone silent. The record is provably on the wire (avahi-browse
// sees it on eth0) yet matter.js never re-queries in time, so discovery times
// out even with Patch 5's longer window (extra time spent only passively
// listening, and the openthread mDNS proxy answers queries rather than sending
// gratuitous announcements matter.js could catch passively).
//
// Capping the interval at 15s makes matter.js keep actively querying (…24, then
// every 15s) for the whole window, so it re-queries within ~15s of the record
// appearing and completes CASE. Slightly chattier mDNS while a discovery is
// pending — negligible on a home LAN, and it also speeds up normal operational
// reconnects.
patch(
  "@matter/protocol/dist/esm/mdns/MdnsScanner.js",
  "MdnsScanner: cap re-query backoff at 15s so late Thread SRP records are still queried",
  `      nextAnnounceInterval,
      60 * 60`,
  `      nextAnnounceInterval,
      15`,
);
