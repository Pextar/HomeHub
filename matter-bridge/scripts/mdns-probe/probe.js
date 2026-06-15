"use strict";
//
// mDNS discovery probe — does a JS mDNS client bound to :5353 actually RECEIVE
// _matter._tcp records in THIS network namespace?
//
// This is the operational-discovery half of Matter (no Bluetooth involved), so
// it can run both on the host (contended :5353) and in an isolated macvlan netns
// (private :5353). Comparing the two answers the question both remaining
// architectures depend on: "does a clean :5353 fix discovery?"
//
// It uses `multicast-dns` — a pure-JS mDNS client that binds :5353 with
// SO_REUSEADDR and joins 224.0.0.251, the same shape as matter.js's own mDNS.
//
//   responses received + >=1 _matter record  -> mDNS RX works here. exit 0.
//   no _matter records (or no responses)      -> mDNS RX broken here. exit 2.

const SCAN_MS = Number(process.env.PROBE_SCAN_MS || 12000);

let mdns;
try {
    mdns = require("multicast-dns")({ reuseAddr: true, loopback: true });
} catch (err) {
    console.log("[probe] FAIL: could not start multicast-dns:", err.message);
    process.exit(3);
}

const responders = new Set();     // source IPs that answered anything
const matterInstances = new Map(); // instance name -> { host, addrs:Set, port }
let anyResponse = false;

mdns.on("error", (e) => console.log("[probe] socket error:", e.message));

mdns.on("response", (res, rinfo) => {
    anyResponse = true;
    if (rinfo && rinfo.address) responders.add(rinfo.address);

    for (const a of [...(res.answers || []), ...(res.additionals || [])]) {
        const name = a.name || "";
        if (a.type === "PTR" && name === "_matter._tcp.local") {
            const inst = a.data; // e.g. <fabric>-<node>._matter._tcp.local
            if (!matterInstances.has(inst)) matterInstances.set(inst, { host: null, addrs: new Set(), port: null });
        }
        if (a.type === "SRV" && name.endsWith("._matter._tcp.local")) {
            const e = matterInstances.get(name) || { host: null, addrs: new Set(), port: null };
            e.host = a.data && a.data.target;
            e.port = a.data && a.data.port;
            matterInstances.set(name, e);
        }
        if (a.type === "A" || a.type === "AAAA") {
            // attach address to any instance whose host matches this record name
            for (const e of matterInstances.values()) {
                if (e.host && e.host === name) e.addrs.add(a.data);
            }
        }
    }
});

function ask() {
    mdns.query([
        { name: "_services._dns-sd._udp.local", type: "PTR" },
        { name: "_matter._tcp.local", type: "PTR" },
    ]);
}

console.log(`[probe] querying _matter._tcp for ${SCAN_MS} ms ...`);
ask();
const ticker = setInterval(ask, 2000);

setTimeout(() => {
    clearInterval(ticker);
    try { mdns.destroy(); } catch { /* ignore */ }

    console.log("");
    console.log("==================== mDNS PROBE SUMMARY ====================");
    console.log(`responses received : ${anyResponse ? "YES" : "NO"}`);
    console.log(`distinct responders: ${responders.size}  [${[...responders].join(", ")}]`);
    console.log(`_matter instances  : ${matterInstances.size}`);
    for (const [inst, e] of matterInstances) {
        const short = inst.replace("._matter._tcp.local", "");
        console.log(`   - ${short}  host=${e.host || "?"}  port=${e.port || "?"}  addrs=[${[...e.addrs].join(", ")}]`);
    }
    console.log("===========================================================");

    if (matterInstances.size > 0) {
        console.log("PROBE PASS: this namespace receives _matter._tcp records.");
        process.exit(0);
    } else {
        console.log(`PROBE FAIL: no _matter._tcp records received here (${anyResponse ? "got other responses" : "got NO responses at all"}).`);
        process.exit(2);
    }
}, SCAN_MS);
