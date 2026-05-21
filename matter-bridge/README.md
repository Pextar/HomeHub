# Matter Bridge

A small Node.js sidecar that lets the Go backend control Matter devices —
both **Matter over Wi-Fi** (lights, plugs) and **Matter over Thread**
(sensors, some Eve / Aqara / Nanoleaf devices) — without speaking the
Matter protocol itself.

It wraps [matter.js](https://github.com/project-chip/matter.js)'s
`CommissioningController` and exposes a minimal JSON HTTP API on
`127.0.0.1:8765` — the Go backend's `internal/matter` client calls it.

## Why a sidecar?

matter.js is the most mature open Matter implementation, but it's Node-only.
Rather than re-implement the spec in Go, we run a small Node process next to
the Go binary and proxy commands over loopback. The Pi needs Wi-Fi and BLE
for Wi-Fi Matter devices. For Thread devices a **Thread Border Router**
(Apple TV 4K, HomePod mini, or an OpenThread Border Router) bridges the
Thread mesh to the IP network — the Pi itself doesn't need a Thread radio.

## Quick start

```sh
cd matter-bridge
npm install
npm run build
npm start
```

Set `MATTER_BRIDGE_PORT` to change the port (default `8765`) and
`MATTER_BRIDGE_DATA` to change where the fabric/credentials are stored
(default `./data`). The data directory must be writable and persisted
across restarts — losing it means every device has to be re-commissioned.

**Network credentials are required for commissioning.** Set exactly one of:

| Device type | Env var(s) |
|-------------|-----------|
| **Matter over Wi-Fi** (bulbs, plugs) | `MATTER_BRIDGE_WIFI_SSID` + `MATTER_BRIDGE_WIFI_PASS` |
| **Matter over Thread** (sensors, some Eve / Nanoleaf) | `MATTER_BRIDGE_THREAD_DATASET` (hex TLV) — the Thread network name is auto-parsed from the TLV; set `MATTER_BRIDGE_THREAD_NETWORK_NAME` to override |

`MATTER_BRIDGE_THREAD_DATASET` takes priority when both are set. See
[`docs/MATTER.md`](../docs/MATTER.md) for how to get the Thread Operational
Dataset from an Apple TV 4K, HomePod mini, or OTBR.

## HTTP API

| Method | Path                  | Body / Notes                                       |
|--------|-----------------------|----------------------------------------------------|
| GET    | `/health`             | `{ status, devices, transport }`                   |
| GET    | `/devices`            | List of `DeviceState`                              |
| POST   | `/commission`         | `{ pairing_code }` (manual or `MT:` QR payload)    |
| GET    | `/devices/:id`        | Live `DeviceState` (queries the device)            |
| PUT    | `/devices/:id/state`  | Partial update: `{ on?, level?, color?, ct? }`     |
| DELETE | `/devices/:id`        | Decommission and forget                            |

`DeviceState`:

```ts
{
  id: string,         // Matter node id, decimal
  name?: string,
  vendor?: string,
  product?: string,
  reachable: boolean,
  on?: boolean,
  level?: number,     // 0..100
  color?: string,     // RRGGBB hex
  ct?: number,        // 153..500 mired (warm..cool)
}
```

## Pairing

Most Matter bulbs come with both an 11-digit manual code (printed on the
device) and a QR code starting with `MT:`. Either works:

```sh
curl -X POST -H 'Content-Type: application/json' \
  -d '{"pairing_code":"3496-112-0001"}' \
  http://127.0.0.1:8765/commission
```

The bridge uses BLE to discover the device, brings it onto the Wi-Fi or
Thread fabric (whichever credential is configured), and assigns a stable
node id. From then on it talks over IP only (via the Border Router for
Thread devices).

## Hardware

- Raspberry Pi with Wi-Fi and Bluetooth (Pi 3/4/5/Zero 2 all work).
- The user running the bridge needs Bluetooth access (`bluetoothctl` works
  → the bridge can commission). On Raspberry Pi OS this means adding the
  user to the `bluetooth` group.
- For Thread Matter devices: a **Thread Border Router** on the same LAN —
  Apple TV 4K (3rd gen 2022+), HomePod mini, or any OpenThread Border
  Router. The Pi does **not** need a Thread radio.

## Notes

The bridge uses the stable legacy [`@project-chip/matter.js`](https://www.npmjs.com/package/@project-chip/matter.js)
controller surface (`CommissioningController` + `PairedNode`). The newer
`@matter/*` packages provide the underlying types, protocol and storage.

If matter.js's API changes between releases, pin the package versions in
`package.json` or adapt the cluster calls in `src/controller.ts`.

### BLE prerequisites on Linux

The `@matter/nodejs-ble` package uses
[noble](https://github.com/abandonware/noble) under the hood. Native
build deps are needed on first install:

```sh
sudo apt install -y build-essential libudev-dev libcap2-bin
sudo setcap cap_net_raw+eip $(eval readlink -f $(which node))
```

The last line lets Node access raw BLE sockets without root.
