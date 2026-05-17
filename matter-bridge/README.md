# Matter Bridge

A small Node.js sidecar that lets the Go backend control Matter-over-Wi-Fi
devices (lights, plugs, etc.) without speaking the Matter protocol itself.

It wraps [matter.js](https://github.com/project-chip/matter.js)'s
`CommissioningController` and exposes a minimal JSON HTTP API on
`127.0.0.1:8765` — the Go backend's `internal/matter` client calls it.

## Why a sidecar?

matter.js is the most mature open Matter implementation, but it's Node-only.
Rather than re-implement the spec in Go, we run a small Node process next to
the Go binary and proxy commands over loopback. The Pi has Wi-Fi and BLE
built in, which is all Matter needs over the IP transport — no Zigbee or
Thread radios required (Thread devices would need a separate border router).

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

## HTTP API

| Method | Path                  | Body / Notes                                       |
|--------|-----------------------|----------------------------------------------------|
| GET    | `/health`             | `{ status, devices }`                              |
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

The bridge uses BLE to discover the device, brings it onto the Wi-Fi
fabric, and assigns a stable node id. From then on it talks over IP only.

## Hardware

- Raspberry Pi with Wi-Fi and Bluetooth (Pi 3/4/5/Zero 2 all work).
- The user running the bridge needs Bluetooth access (`bluetoothctl` works
  → the bridge can commission). On Raspberry Pi OS this typically means
  adding the user to the `bluetooth` group.
- For Thread-only Matter devices (e.g. some Eve / Nanoleaf sensors) you'd
  also need a Thread border router — out of scope here.

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
