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
        adminFabricLabel: "rf-socket-controller",
        environment: { environment: Environment.default, id: "rf-socket-controller" },
    });
    await matterServer.addCommissioningController(commissioning);
    await matterServer.start();
    console.log(`[matter-bridge] controller started — data dir: ${DATA_DIR}`);

    function key(id: NodeId | bigint): string {
        return typeof id === "bigint" ? id.toString() : (id as unknown as bigint).toString();
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
            const deadline = new Promise<boolean>((resolve) => setTimeout(() => resolve(false), 4000));
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
            state.vendor  = asString(info.vendorName);
            state.product = asString(info.productName);
            state.name    = asString(info.nodeLabel) ?? state.product;
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
            const { hue, sat } = hexToHs(update.color);
            await color.commands.moveToHueAndSaturation({
                hue, saturation: sat, transitionTime: 0, optionsMask: {}, optionsOverride: {},
            });
        }
        if (update.ct !== undefined) {
            const color = ep.getClusterClient(ColorControl.Cluster);
            if (!color) throw new Error("device does not expose ColorControl");
            await color.commands.moveToColorTemperature({
                colorTemperatureMireds: update.ct,
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
            const out: DeviceState[] = [];
            for (const id of ids) {
                try { out.push(await readState(id)); }
                catch { out.push({ id, reachable: false }); }
            }
            return out;
        },
        async commission(pairingCode: string, transport?: "wifi" | "thread"): Promise<string> {
            const opts = optionsFromPairingCode(pairingCode, transport);

            // Snapshot of commissioned nodes before the attempt so we can
            // detect any partial fabric entry left behind if commissionNode
            // throws — e.g. when the device joined Wi-Fi (Phase 1 done) but
            // the CASE session over IP failed (Phase 2 failed).
            const nodesBefore = new Set(commissioning.getCommissionedNodes().map(n => key(n)));

            let nodeId: NodeId;
            try {
                nodeId = await commissioning.commissionNode(opts);
            } catch (err) {
                // If matter.js persisted a partial node despite the failure,
                // remove it so the fabric stays clean and a retry (or
                // factory-reset + retry) starts from a known-good state.
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
                throw err;
            }
            return key(nodeId);
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
            const len  = bytes[offset++];
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
    if (process.env.MATTER_BRIDGE_WIFI_SSID?.trim())     result.push("wifi");
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
    let wifi:   ReturnType<typeof wifiNetwork>   = undefined;

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
            ...(wifi   ? { wifiNetwork:   wifi   } : {}),
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
    if      (h < 60)  { r = c; g = x; }
    else if (h < 120) { r = x; g = c; }
    else if (h < 180) { g = c; b = x; }
    else if (h < 240) { g = x; b = c; }
    else if (h < 300) { r = x; b = c; }
    else              { r = c; b = x; }
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
        if      (max === r) hue = ((g - b) / d) % 6;
        else if (max === g) hue = (b - r) / d + 2;
        else                hue = (r - g) / d + 4;
        hue = hue * 60;
        if (hue < 0) hue += 360;
    }
    const sat = max === 0 ? 0 : d / max;
    return {
        hue: Math.round((hue / 360) * 254),
        sat: Math.round(sat * 254),
    };
}
