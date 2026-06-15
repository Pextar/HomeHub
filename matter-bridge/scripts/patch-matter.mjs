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
