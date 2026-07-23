// Thin wrapper around matter.js's CommissioningController.
//
// Responsibilities:
//   - Persist fabric & node data under MATTER_BRIDGE_DATA so commissioned
//     devices survive restarts.
//   - Reconnect to all known nodes on startup.
//   - Translate the bridge HTTP API into matter.js cluster calls
//     (OnOff, LevelControl, ColorControl).
//
// State model exposed to the Go side is intentionally narrow — it mirrors
// the Tasmota state shape already used by the frontend so a single
// "smart light" modal can drive either backend.
//
// We use the stable legacy `@project-chip/matter.js` controller surface
// (CommissioningController + PairedNode + Endpoint.getClusterClient). The
// `@matter/*` packages provide the underlying types/protocol/storage.
import "@project-chip/matter-node.js";
import {
    CommissioningController,
    NodeCommissioningOptions,
    MatterServer,
} from "@project-chip/matter.js";
import { BleNode } from "@project-chip/matter-node-ble.js";
import { Environment, Logger, LogLevel, singleton, StorageManager } from "@matter/general";
import { Ble } from "@matter/protocol";
import { NodeId, VendorId } from "@matter/types";
import {
    ManualPairingCodeCodec,
    QrPairingCodeCodec,
} from "@matter/types";
import { OnOff, LevelControl, ColorControl } from "@matter/types/clusters";
import { StorageBackendDisk } from "@matter/nodejs";
import path from "node:path";
import fs from "node:fs";

const DATA_DIR = process.env.MATTER_BRIDGE_DATA || path.resolve(process.cwd(), "data");

export interface DeviceState {
    id: string;             // node id as a decimal string
    name?: string;
    vendor?: string;
    product?: string;
    reachable: boolean;
    on?: boolean;
    level?: number;         // 0..100
    color?: string;         // RRGGBB hex
    ct?: number;            // 153..500 mired
}

export interface MatterController {
    listIds(): string[];
    list(): Promise<DeviceState[]>;
    // transport: "wifi" | "thread" — selects which credentials to use.
    // Omit (or pass undefined) to auto-detect from what's configured.
    commission(pairingCode: string, transport?: "wifi" | "thread"): Promise<string>;
    getState(nodeId: string): Promise<DeviceState>;
    setState(nodeId: string, update: Partial<DeviceState>): Promise<void>;
    remove(nodeId: string): Promise<void>;
    close(): Promise<void>;
}

export async function startController(): Promise<MatterController> {
    const levelName = (process.env.MATTER_LOG_LEVEL || "INFO").toUpperCase();
    const level = (LogLevel as unknown as Record<string, LogLevel>)[levelName];
    if (level !== undefined) Logger.defaultLogLevel = level;

    fs.mkdirSync(DATA_DIR, { recursive: true });

    // BLE is needed for the initial commissioning hop. On systems without
    // Bluetooth (e.g. dev laptops) we skip registration so the rest of the
    // controller still comes up; only commissioning will then fail.
    try {
        const hciId = process.env.MATTER_BRIDGE_HCI_ID
            ? Number(process.env.MATTER_BRIDGE_HCI_ID)
            : undefined;
        Ble.get = singleton(() => new BleNode(hciId !== undefined ? { hciId } : undefined));
        console.log("[matter-bridge] BLE central registered");
    } catch (err) {
        console.warn("[matter-bridge] BLE not available — commissioning will be unavailable:", err);
    }

    const storage = new StorageManager(new StorageBackendDisk(DATA_DIR));
    await storage.initialize();

    const matterServer = new MatterServer(storage);

    const commissioning = new CommissioningController({
        autoConnect: false,             // we connect lazily on first use
        adminFabricLabel: "homehub",
        environment: { environment: Environment.default, id: "homehub" },
    });
    await matterServer.addCommissioningController(commissioning);
    await matterServer.start();
    console.log(`[matter-bridge] controller started — data dir: ${DATA_DIR}`);

    function key(id: NodeId | bigint): string {
        return (id as bigint).toString();
    }

    async function getPaired(id: string) {
        const node = await commissioning.getNode(NodeId(BigInt(id)));
        if (!node.isConnected) {
            // PairedNode.connect() is non-blocking; wait for initial remote
            // sync — but cap it. If the device is offline, this event will
            // never fire and we'd hang forever, eating the Go-side HTTP
            // timeout and breaking the UI with a "Load failed" error.
            node.connect();
            const ready = new Promise<boolean>((resolve) => {
                Promise.resolve(node.events.initializedFromRemote)
                    .then(() => resolve(true))
                    .catch(() => resolve(false));
            });
            // Thread devices can take up to ~20 s for the initial CASE session
            // after SRP records propagate through the border router.  Wi-Fi
            // devices are typically ready in 2–5 s.  We cap at 20 s so a
            // genuinely offline device doesn't eat the full Go-side HTTP timeout.
            const deadline = new Promise<boolean>((resolve) => setTimeout(() => resolve(false), 20_000));
            await Promise.race([ready, deadline]);
        }
        return node;
    }

    function pickPrimaryEndpoint(node: Awaited<ReturnType<typeof getPaired>>) {
        const devices = node.getDevices();
        if (!devices || devices.length === 0) return undefined;
        // Prefer an endpoint with an OnOff cluster (lights, plugs).
        for (const d of devices) {
            const ep = d as any;
            if (typeof ep.getClusterClient === "function" && ep.getClusterClient(OnOff.Cluster)) {
                return ep;
            }
        }
        return devices[0] as any;
    }

    async function readState(id: string): Promise<DeviceState> {
        const state: DeviceState = { id, reachable: false };
        const node = await getPaired(id).catch(() => null);
        if (!node) return state;
        state.reachable = node.isConnected;
        const info = node.basicInformation;
        if (info) {
            state.vendor = asString(info.vendorName);
            state.product = asString(info.productName);
            state.name = asString(info.nodeLabel) ?? state.product;
        }
        const ep = pickPrimaryEndpoint(node);
        if (!ep) return state;

        const onOff = ep.getClusterClient(OnOff.Cluster);
        if (onOff) state.on = await safeRead<boolean>(() => onOff.attributes.onOff.get());

        const level = ep.getClusterClient(LevelControl.Cluster);
        if (level) {
            const raw = await safeRead<number>(() => level.attributes.currentLevel.get());
            if (raw != null) state.level = Math.round((raw / 254) * 100);
        }

        const color = ep.getClusterClient(ColorControl.Cluster);
        if (color) {
            const ct = await safeRead<number>(() => color.attributes.colorTemperatureMireds.get());
            if (ct != null) state.ct = ct;
            const hue = await safeRead<number>(() => color.attributes.currentHue.get());
            const sat = await safeRead<number>(() => color.attributes.currentSaturation.get());
            if (hue != null && sat != null) state.color = hsToHex(hue, sat);
        }
        return state;
    }

    async function actOn(id: string, update: Partial<DeviceState>): Promise<void> {
        const node = await getPaired(id);
        const ep = pickPrimaryEndpoint(node);
        if (!ep) throw new Error(`node ${id} has no controllable endpoint`);

        if (update.on !== undefined) {
            const onOff = ep.getClusterClient(OnOff.Cluster);
            if (!onOff) throw new Error("device does not expose OnOff");
            if (update.on) await onOff.commands.on(); else await onOff.commands.off();
        }
        if (update.level !== undefined) {
            const level = ep.getClusterClient(LevelControl.Cluster);
            if (!level) throw new Error("device does not expose LevelControl");
            const lvl = Math.max(0, Math.min(254, Math.round((update.level / 100) * 254)));
            await level.commands.moveToLevelWithOnOff({
                level: lvl, transitionTime: 0, optionsMask: {}, optionsOverride: {},
            });
        }
        if (update.color !== undefined) {
            const color = ep.getClusterClient(ColorControl.Cluster);
            if (!color) throw new Error("device does not expose ColorControl");
            if (!/^#?[0-9a-fA-F]{6}$/.test(update.color)) {
                // Reject early — a malformed hex would otherwise turn into
                // NaN hue/saturation in the cluster command.
                throw new Error(`invalid color ${JSON.stringify(update.color)} — want RRGGBB hex`);
            }
            const { hue, sat } = hexToHs(update.color);
            await color.commands.moveToHueAndSaturation({
                hue, saturation: sat, transitionTime: 0, optionsMask: {}, optionsOverride: {},
            });
        }
        if (update.ct !== undefined) {
            const color = ep.getClusterClient(ColorControl.Cluster);
            if (!color) throw new Error("device does not expose ColorControl");
            // Clamp to the mired range the rest of the stack uses (Tasmota
            // convention, also the bounds most bulbs accept).
            const ct = Math.max(153, Math.min(500, Math.round(update.ct)));
            await color.commands.moveToColorTemperature({
                colorTemperatureMireds: ct,
                transitionTime: 0, optionsMask: {}, optionsOverride: {},
            });
        }
    }

    return {
        listIds() {
            return commissioning.getCommissionedNodes().map((n) => key(n));
        },
        async list() {
            const ids = commissioning.getCommissionedNodes().map((n) => key(n));
            // Bounded concurrency: each offline device can burn several
            // safeRead timeouts, so a sequential sweep over a handful of
            // unreachable nodes would take tens of seconds.
            const CONCURRENCY = 4;
            const out: DeviceState[] = new Array(ids.length);
            let next = 0;
            async function worker() {
                while (next < ids.length) {
                    const i = next++;
                    try { out[i] = await readState(ids[i]); }
                    catch { out[i] = { id: ids[i], reachable: false }; }
                }
            }
            await Promise.all(Array.from({ length: Math.min(CONCURRENCY, ids.length) }, worker));
            return out;
        },
        async commission(pairingCode: string, transport?: "wifi" | "thread"): Promise<string> {
            const opts = optionsFromPairingCode(pairingCode, transport);

            // BLE connections are flaky on Linux/Raspberry Pi; one automatic
            // retry is safe because a BLE error means no Matter protocol was
            // exchanged — the device has not received network credentials and no
            // fabric entry was written on either side.
            //
            // BlueZ/Noble on the Pi throws several distinct phrasings for what is
            // really the same class of transient GATT-connect failure. All are safe
            // to retry: a failed BLE connect means no Matter protocol was exchanged,
            // so the device received no credentials and no fabric entry exists.
            //
            // We split BLE failures into two classes with DIFFERENT remedies:
            //
            //   TRANSIENT — a fresh HCI connect stalled or returned a failure
            //   status. One in-process retry sometimes catches.
            //     "Timeout while connecting to peripheral …"  — HCI connect stalled.
            //     "Error while connecting to peripheral …"    — HCI LE Create
            //                                                   Connection returned a
            //                                                   failure status.
            //
            //   PERMANENT — Noble's per-peripheral state machine is corrupted from a
            //   prior failed attempt and refuses ALL further connects until the
            //   process restarts. In-process retries just waste ~40 s each, so we exit
            //   IMMEDIATELY and let systemd restart us with a clean Noble instance
            //   (the first attempt after a restart is the one that reliably catches).
            //     "Can not connect to peripheral … unexpected state 'error'" (or "error")
            //       (Single or double quotes depending on Noble version.)
            const BLE_TRANSIENT_RE =
                /(?:timeout|error) while connecting to peripheral/i;
            const BLE_PERMANENT_RE =
                /can ?not connect to peripheral|unexpected state\s*["']?error["']?/i;

            // matter.js throws this once BLE + network provisioning have already
            // succeeded but the post-commissioning operational (CASE-over-IP)
            // reconnect can't find the device's _matter._tcp record via mDNS.
            // For Thread this almost always means the device joined the fabric
            // but its operational SRP record never propagated to the controller
            // — not a transient fault, so we surface an actionable message
            // instead of the opaque library text. (Substring from
            // @matter/protocol ControllerDiscovery.discoverOperationalDevice.)
            const OPERATIONAL_DISCOVERY_RE =
                /operational device cannot be found on the network/i;
            const MAX_BLE_RETRIES = 2; // up to 3 attempts total; BLE on Linux is flaky

            // Snapshot commissioned nodes before any attempt so orphan cleanup
            // works correctly across retries.
            const nodesBefore = new Set(commissioning.getCommissionedNodes().map(n => key(n)));

            for (let attempt = 0; attempt <= MAX_BLE_RETRIES; attempt++) {
                if (attempt > 0) {
                    console.log(`[matter-bridge] retrying commission (attempt ${attempt + 1}) after BLE timeout…`);
                    await new Promise<void>(r => setTimeout(r, 6000));
                }
                try {
                    // Pass false so matter.js doesn't open a *second* operational
                    // connection + subscription right after commissioning — we manage
                    // operational connections lazily via getPaired() on first control.
                    // This flag does NOT affect CommissioningComplete: that is sent by
                    // the commissioning flow itself (step 16) before commissionNode
                    // returns, once CASE over the operational (Thread) network succeeds.
                    const nodeId = await commissioning.commissionNode(opts, false);
                    return key(nodeId);
                } catch (err) {
                    const msg = (err as Error).message ?? "";
                    const isBlePermanent = BLE_PERMANENT_RE.test(msg);
                    const isBleTransient = BLE_TRANSIENT_RE.test(msg);
                    const isBleFailure = isBlePermanent || isBleTransient;

                    // Clean up any partial fabric entries before retrying or surfacing the error.
                    const orphaned = commissioning.getCommissionedNodes()
                        .map(n => key(n))
                        .filter(id => !nodesBefore.has(id));
                    if (orphaned.length > 0) {
                        console.warn(
                            `[matter-bridge] commission failed — removing ${orphaned.length} orphaned node(s): ${orphaned.join(", ")}`,
                        );
                        for (const id of orphaned) {
                            try {
                                await commissioning.removeNode(NodeId(BigInt(id)));
                            } catch (removeErr) {
                                console.error(`[matter-bridge] could not remove orphaned node ${id}:`, removeErr);
                            }
                        }
                    }

                    // A transient fault (fresh HCI connect stalled) can catch on
                    // one in-process retry. A permanent fault (Noble state
                    // corrupted) never will — skip straight to the restart path.
                    if (isBleTransient && !isBlePermanent && attempt < MAX_BLE_RETRIES) {
                        console.warn(`[matter-bridge] transient BLE error on attempt ${attempt + 1} — retrying`);
                        continue;
                    }

                    // Either Noble's per-peripheral state machine is permanently
                    // corrupted, or a transient fault has exhausted its retries.
                    // Both require a fresh process: exit now so systemd restarts the
                    // bridge with a clean Noble instance — the first attempt after a
                    // restart is the one that reliably catches. We exit IMMEDIATELY on
                    // a permanent fault rather than burning ~40 s per pointless retry.
                    if (isBleFailure) {
                        console.error(
                            isBlePermanent
                                ? `[matter-bridge] BLE peripheral state corrupted (${msg}) — ` +
                                  `exiting immediately for a clean Noble restart`
                                : `[matter-bridge] transient BLE error persists after ${attempt + 1} attempt(s) — ` +
                                  `exiting so systemd can restart with a clean BLE state`,
                        );
                        setTimeout(() => process.exit(1), 100);
                        throw new Error(
                            "Bluetooth commissioning failed — the Pi's BLE adapter dropped the connection. " +
                            "The bridge is restarting with a clean Bluetooth state; wait a few seconds and " +
                            "trigger commissioning again (the first attempt after a restart is the reliable one). " +
                            "If this keeps happening, use a USB BLE dongle (set MATTER_BRIDGE_HCI_ID) and stop " +
                            "bluetoothd (`sudo systemctl disable --now bluetooth`). " +
                            `(underlying error: ${msg})`,
                        );
                    }

                    // BLE + network provisioning worked, but the device's
                    // operational record never appeared so CASE-over-IP couldn't
                    // be established. Retrying won't help — point the operator at
                    // the real culprits. (Most common: a re-paired Thread device
                    // that kept stale credentials, so it reports "already
                    // connected", matter.js skips ConnectNetwork, and the device
                    // never re-registers SRP. A clean factory reset fixes it.)
                    if (OPERATIONAL_DISCOVERY_RE.test(msg)) {
                        throw new Error(
                            "Device joined the fabric but its operational record never appeared on the network " +
                            "(CASE-over-IP discovery timed out). For Thread devices this usually means: " +
                            "(1) the device kept stale Thread credentials from a previous pairing — factory-reset " +
                            "it fully (pairing mode alone is not enough) so it re-attaches and re-registers SRP; " +
                            "(2) the Thread Border Router or its SRP server is down — verify `ot-ctl state` and " +
                            "`ot-ctl srp server state`; or (3) the OTBR's active dataset no longer matches " +
                            "MATTER_BRIDGE_THREAD_DATASET. " +
                            `(underlying error: ${msg})`,
                        );
                    }

                    throw err;
                }
            }
            // Unreachable; satisfies TypeScript.
            throw new Error("commission: exhausted retry loop");
        },
        async getState(nodeId: string) { return readState(nodeId); },
        async setState(nodeId: string, update: Partial<DeviceState>) { await actOn(nodeId, update); },
        async remove(nodeId: string) {
            await commissioning.removeNode(NodeId(BigInt(nodeId)));
        },
        async close() {
            await matterServer.close();
            await storage.close();
        },
    };
}

function wifiNetwork(): { wifiSsid: string; wifiCredentials: string } | undefined {
    const ssid = process.env.MATTER_BRIDGE_WIFI_SSID;
    const pass = process.env.MATTER_BRIDGE_WIFI_PASS ?? "";
    if (!ssid) return undefined;
    return { wifiSsid: ssid, wifiCredentials: pass };
}

function threadNetwork(): { networkName: string; operationalDataset: string } | undefined {
    const dataset = process.env.MATTER_BRIDGE_THREAD_DATASET?.trim();
    if (!dataset) return undefined;

    // matter.js needs the Thread network name to verify the commissionee can
    // see our Thread network. It's already encoded in the Operational Dataset
    // TLV (type 0x03 = Network Name); we parse it automatically.
    // Set MATTER_BRIDGE_THREAD_NETWORK_NAME to override if parsing fails.
    const name = process.env.MATTER_BRIDGE_THREAD_NETWORK_NAME?.trim()
        || parseThreadNetworkName(dataset);

    if (!name) {
        throw new Error(
            "Could not determine Thread network name from MATTER_BRIDGE_THREAD_DATASET. " +
            "Set MATTER_BRIDGE_THREAD_NETWORK_NAME explicitly.",
        );
    }
    return { networkName: name, operationalDataset: dataset };
}

// Extracts the Network Name (TLV type 0x03) from a hex-encoded Thread
// Operational Dataset. Returns an empty string if not found or on parse error.
function parseThreadNetworkName(hexDataset: string): string {
    try {
        const bytes = Buffer.from(hexDataset.replace(/\s/g, ""), "hex");
        let offset = 0;
        while (offset + 2 <= bytes.length) {
            const type = bytes[offset++];
            const len = bytes[offset++];
            if (offset + len > bytes.length) break;
            if (type === 0x03) {              // Network Name
                return bytes.subarray(offset, offset + len).toString("utf8");
            }
            offset += len;
        }
    } catch { /* ignore */ }
    return "";
}

// Returns all network transports that have credentials configured.
// Both "thread" and "wifi" can be returned simultaneously — the caller
// (commission wizard) picks which one to use for each device.
export function availableTransports(): ("thread" | "wifi")[] {
    const result: ("thread" | "wifi")[] = [];
    if (process.env.MATTER_BRIDGE_THREAD_DATASET?.trim()) result.push("thread");
    if (process.env.MATTER_BRIDGE_WIFI_SSID?.trim()) result.push("wifi");
    return result;
}

function optionsFromPairingCode(code: string, transport?: "wifi" | "thread"): NodeCommissioningOptions {
    const trimmed = code.trim();
    let discriminator: number | undefined;
    let shortDiscriminator: number | undefined;
    let passcode: number;

    if (trimmed.toUpperCase().startsWith("MT:")) {
        const decoded = QrPairingCodeCodec.decode(trimmed)[0];
        if (!decoded) throw new Error("could not decode QR pairing code");
        discriminator = decoded.discriminator;
        passcode = decoded.passcode;
    } else {
        const decoded = ManualPairingCodeCodec.decode(trimmed.replace(/[^0-9]/g, ""));
        passcode = decoded.passcode;
        discriminator = decoded.discriminator;
        shortDiscriminator = decoded.shortDiscriminator;
    }

    // Resolve network credentials.
    // If transport is explicitly specified, use exactly that — error if not configured.
    // If unspecified (legacy / direct bridge call), fall back to Thread-first auto-detect.
    let thread: ReturnType<typeof threadNetwork> = undefined;
    let wifi: ReturnType<typeof wifiNetwork> = undefined;

    if (transport === "thread") {
        thread = threadNetwork();
        if (!thread) throw new Error(
            "Transport \"thread\" requested but MATTER_BRIDGE_THREAD_DATASET is not set.",
        );
    } else if (transport === "wifi") {
        wifi = wifiNetwork();
        if (!wifi) throw new Error(
            "Transport \"wifi\" requested but MATTER_BRIDGE_WIFI_SSID is not set.",
        );
    } else {
        // Auto: Thread takes priority when both are configured.
        thread = threadNetwork();
        if (!thread) wifi = wifiNetwork();
    }

    if (thread) {
        console.log("[matter-bridge] commissioning with Thread network (MATTER_BRIDGE_THREAD_DATASET)");
    } else if (wifi) {
        console.log(`[matter-bridge] commissioning with Wi-Fi SSID: ${process.env.MATTER_BRIDGE_WIFI_SSID}`);
    } else {
        console.warn("[matter-bridge] neither MATTER_BRIDGE_THREAD_DATASET nor MATTER_BRIDGE_WIFI_SSID is set — device will not receive network credentials and commissioning will stall at the network provisioning step");
    }

    return {
        commissioning: {
            regulatoryCountryCode: "XX",
            ...(thread ? { threadNetwork: thread } : {}),
            ...(wifi ? { wifiNetwork: wifi } : {}),
            // No finalizeCommissioning override: we let matter.js run the full
            // commissioning flow, including operational discovery (mDNS), CASE
            // reconnect, and CommissioningComplete (step 16). This requires the
            // Thread Border Router to publish the device's operational service via
            // mDNS on the infrastructure interface — i.e. OTBR must run with the
            // correct BACKBONE_INTERFACE/INFRA_IF_NAME and an enabled SRP server.
            // Without CommissioningComplete the device's fail-safe expires and it
            // rolls back, so this step is what actually finishes onboarding.
        },
        discovery: {
            identifierData: discriminator !== undefined
                ? { longDiscriminator: discriminator }
                : { shortDiscriminator: shortDiscriminator! },
            // Explicitly request both BLE and mDNS so the controller
            // registers 2 scanners instead of defaulting to mDNS only.
            discoveryCapabilities: { ble: true, onIpNetwork: true },
        },
        passcode,
    };
}

function asString(v: unknown): string | undefined {
    if (v == null) return undefined;
    if (typeof v === "string") return v || undefined;
    return String(v) || undefined;
}

async function safeRead<T>(read: () => Promise<T> | T): Promise<T | undefined> {
    // Each attribute read goes over the wire — bound it so one stuck
    // cluster can't make the whole /devices/:id GET miss its HTTP deadline.
    try {
        const v = await Promise.race([
            Promise.resolve(read()),
            new Promise<undefined>((resolve) => setTimeout(() => resolve(undefined), 2500)),
        ]);
        return v as T;
    } catch {
        return undefined;
    }
}

// --- Color conversion helpers ---
// Matter ColorControl uses 0..254 for both hue and saturation.

function hsToHex(hue254: number, sat254: number): string {
    const h = (hue254 / 254) * 360;
    const s = sat254 / 254;
    const v = 1;
    const c = v * s;
    const x = c * (1 - Math.abs(((h / 60) % 2) - 1));
    const m = v - c;
    let r = 0, g = 0, b = 0;
    if (h < 60) { r = c; g = x; }
    else if (h < 120) { r = x; g = c; }
    else if (h < 180) { g = c; b = x; }
    else if (h < 240) { g = x; b = c; }
    else if (h < 300) { r = x; b = c; }
    else { r = c; b = x; }
    const to = (n: number) => Math.round((n + m) * 255).toString(16).padStart(2, "0");
    return (to(r) + to(g) + to(b)).toUpperCase();
}

function hexToHs(hex: string): { hue: number; sat: number } {
    const h = hex.replace(/^#/, "");
    const r = parseInt(h.slice(0, 2), 16) / 255;
    const g = parseInt(h.slice(2, 4), 16) / 255;
    const b = parseInt(h.slice(4, 6), 16) / 255;
    const max = Math.max(r, g, b), min = Math.min(r, g, b);
    const d = max - min;
    let hue = 0;
    if (d !== 0) {
        if (max === r) hue = ((g - b) / d) % 6;
        else if (max === g) hue = (b - r) / d + 2;
        else hue = (r - g) / d + 4;
        hue = hue * 60;
        if (hue < 0) hue += 360;
    }
    const sat = max === 0 ? 0 : d / max;
    return {
        hue: Math.round((hue / 360) * 254),
        sat: Math.round(sat * 254),
    };
}
