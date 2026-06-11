// Matter bridge sidecar.
//
// A standalone Node.js process the Go backend talks to over loopback HTTP.
// It owns a matter.js CommissioningController and persists its fabric data
// under MATTER_BRIDGE_DATA (default ./data) so commissioned devices survive
// restarts. The Go side never speaks the Matter wire protocol directly —
// it only sees this clean little JSON API.
//
// Endpoints:
//   GET    /health
//   GET    /devices                       — list commissioned nodes (cached state)
//   POST   /commission   { pairing_code } — commission a new device, returns { node_id }
//   GET    /devices/:id                   — fetch live state from the device
//   PUT    /devices/:id/state             — apply { on?, level?, color?, ct? }
//   DELETE /devices/:id                   — decommission and forget
//
// Pairing codes are the 11- or 21-digit manual code or the "MT:..." QR
// payload printed on the device / box.
import http from "node:http";
import { URL } from "node:url";
import { startController, MatterController, availableTransports } from "./controller.js";

const PORT = Number(process.env.MATTER_BRIDGE_PORT || 8765);
const HOST = process.env.MATTER_BRIDGE_HOST || "127.0.0.1";

async function main() {
    const controller = await startController();

    const server = http.createServer((req, res) => {
        handle(req, res, controller).catch((err) => {
            console.error("[matter-bridge] handler error:", err);
            // If the handler already started writing, writeHead would throw;
            // just drop the connection in that case.
            if (res.headersSent) { res.destroy(); return; }
            writeJson(res, 500, { error: (err as Error).message });
        });
    });

    server.listen(PORT, HOST, () => {
        console.log(`[matter-bridge] listening on http://${HOST}:${PORT}`);
    });

    const shutdown = async (sig: string) => {
        console.log(`[matter-bridge] ${sig} — shutting down`);
        server.close();
        try { await controller.close(); } catch (e) { console.error(e); }
        process.exit(0);
    };
    process.on("SIGINT", () => void shutdown("SIGINT"));
    process.on("SIGTERM", () => void shutdown("SIGTERM"));
}

async function handle(req: http.IncomingMessage, res: http.ServerResponse, controller: MatterController) {
    const url = new URL(req.url || "/", `http://${req.headers.host || "localhost"}`);
    const path = url.pathname;
    const method = req.method || "GET";

    if (path === "/health" && method === "GET") {
        writeJson(res, 200, {
            status: "ok",
            devices: controller.listIds().length,
            // Array of configured transports — both "thread" and "wifi" can
            // appear simultaneously; the commission wizard lets the user pick.
            transports: availableTransports(),
        });
        return;
    }

    if (path === "/devices" && method === "GET") {
        writeJson(res, 200, await controller.list());
        return;
    }

    if (path === "/commission" && method === "POST") {
        const body = await readJson<{ pairing_code?: string; transport?: string }>(req);
        const code = (body?.pairing_code || "").trim();
        if (!code) { writeJson(res, 400, { error: "pairing_code is required" }); return; }
        const transport = body?.transport === "thread" ? "thread"
                        : body?.transport === "wifi"   ? "wifi"
                        : undefined;
        const nodeId = await controller.commission(code, transport);
        writeJson(res, 200, { node_id: nodeId });
        return;
    }

    const deviceMatch = path.match(/^\/devices\/([^/]+)(\/state)?$/);
    if (deviceMatch) {
        const id = decodeURIComponent(deviceMatch[1]);
        const isState = !!deviceMatch[2];
        if (!/^\d+$/.test(id)) {
            // Node ids are decimal strings; anything else would throw in
            // BigInt() and surface as a misleading 500.
            writeJson(res, 400, { error: `invalid node id ${JSON.stringify(id)}` });
            return;
        }
        if (method === "GET" && !isState) {
            const state = await controller.getState(id);
            writeJson(res, 200, state);
            return;
        }
        if (method === "PUT" && isState) {
            const update = await readJson<Record<string, unknown>>(req);
            await controller.setState(id, update || {});
            res.writeHead(204).end();
            return;
        }
        if (method === "DELETE" && !isState) {
            await controller.remove(id);
            res.writeHead(204).end();
            return;
        }
    }

    writeJson(res, 404, { error: "not found" });
}

function writeJson(res: http.ServerResponse, status: number, body: unknown) {
    const data = JSON.stringify(body);
    res.writeHead(status, { "Content-Type": "application/json", "Content-Length": Buffer.byteLength(data) });
    res.end(data);
}

async function readJson<T>(req: http.IncomingMessage): Promise<T | null> {
    const chunks: Buffer[] = [];
    for await (const c of req) chunks.push(c as Buffer);
    if (!chunks.length) return null;
    const text = Buffer.concat(chunks).toString("utf8");
    if (!text.trim()) return null;
    return JSON.parse(text) as T;
}

main().catch((err) => {
    console.error("[matter-bridge] fatal:", err);
    process.exit(1);
});
