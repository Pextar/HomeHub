"use strict";
//
// BLE-in-netns probe.
//
// The matter-bridge commissioning hop needs BLE (noble -> hci0). When the
// bridge is moved into its own network namespace (macvlan on eth0) to escape
// the host's :5353 mDNS contention, the open question is whether the Bluetooth
// HCI adapter is still reachable from that namespace — on many kernels
// AF_BLUETOOTH is netns-scoped, which would silently break commissioning.
//
// This probe uses the SAME noble fork the bridge uses
// (@stoprocent/noble, pulled in by @project-chip/matter-node-ble.js) and does
// the one thing that matters: bring the adapter to `poweredOn` and scan.
//
//   - Reaches `poweredOn`         -> HCI is reachable here. exit 0 (PASS).
//   - `unsupported`/`unauthorized`-> adapter not accessible.  exit 2 (FAIL).
//   - never powers on in time     -> HCI not reachable here.  exit 2 (FAIL).
//
// Discovering advertisements is a bonus (proves end-to-end RX), but powering on
// is the real signal: it's exactly what BleNode does before commissioning.

const hciId = process.env.MATTER_BRIDGE_HCI_ID;
if (hciId !== undefined && hciId !== "") {
    // @stoprocent/noble selects the adapter via this env var.
    process.env.NOBLE_HCI_DEVICE_ID = hciId;
}

const SCAN_MS = Number(process.env.PROBE_SCAN_MS || 15000);

let noble;
try {
    noble = require("@stoprocent/noble");
} catch (err) {
    console.log("[probe] FAIL: could not load @stoprocent/noble:", err.message);
    process.exit(3);
}

const devices = new Map(); // id -> { rssi, name }
let poweredOn = false;
let done = false;

// Hard safety cap so the container always exits.
const hardCap = setTimeout(() => finish("hard timeout"), SCAN_MS + 10000);

noble.on("stateChange", async (state) => {
    console.log(`[probe] adapter state -> ${state}`);
    if (state === "poweredOn") {
        poweredOn = true;
        console.log("[probe] HCI reachable from this namespace — scanning for advertisements...");
        try {
            await noble.startScanningAsync([], true); // allowDuplicates
        } catch (e) {
            console.log("[probe] startScanning error:", e.message);
        }
        setTimeout(() => finish("scan window elapsed"), SCAN_MS);
    } else if (state === "unsupported" || state === "unauthorized") {
        console.log(`[probe] FAIL: adapter not accessible from this netns (state=${state})`);
        finish(`state=${state}`);
    }
    // "poweredOff" / "unknown" / "resetting": wait — hardCap will fire if it
    // never transitions to poweredOn.
});

noble.on("discover", (p) => {
    const id = p.address && p.address !== "" && p.address !== "unknown" ? p.address : p.id;
    if (!devices.has(id)) {
        const name = (p.advertisement && p.advertisement.localName) || "";
        devices.set(id, { rssi: p.rssi, name });
        console.log(`[probe] discovered ${id}  rssi=${p.rssi}  name=${name}`);
    }
});

async function finish(reason) {
    if (done) return;
    done = true;
    clearTimeout(hardCap);
    try { await noble.stopScanningAsync(); } catch { /* ignore */ }

    console.log("");
    console.log("==================== PROBE SUMMARY ====================");
    console.log(`reason         : ${reason}`);
    console.log(`adapter        : hci${hciId ?? "0"}`);
    console.log(`powered on     : ${poweredOn ? "YES" : "NO"}`);
    console.log(`devices seen   : ${devices.size}`);
    console.log("======================================================");

    if (poweredOn) {
        console.log("PROBE PASS: HCI is reachable and the adapter powered on in this namespace.");
        if (devices.size === 0) {
            console.log("(note: no BLE advertisements seen — fine if nothing is advertising nearby;");
            console.log(" power-on is the signal that matters for commissioning.)");
        }
        process.exit(0);
    } else {
        console.log("PROBE FAIL: adapter never powered on — HCI is NOT reachable from this namespace.");
        process.exit(2);
    }
}
