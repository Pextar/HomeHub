# Matter Support

The controller can drive any Matter-over-Wi-Fi device (smart bulbs, plugs,
switches) alongside the existing 433MHz RF sockets and Tasmota devices.
Thread-only devices need a separate Thread border router and aren't
covered here.

## How it works

```
Browser ── HTTP ──> Go backend ── HTTP (loopback) ──> matter-bridge ── Wi-Fi/BLE ──> device
                       │
                       └── 433MHz RF, Tasmota (unchanged)
```

`matter-bridge/` is a small Node.js process that owns the matter.js
library and the on-the-wire conversation. The Go backend never speaks
the Matter protocol directly — it just calls a tiny loopback JSON API.

## Setup on the Pi

The deploy script (`scripts/deploy-pi.sh`) handles this automatically; the
manual steps if you need them:

```sh
# 1. Install Node.js + npm (one-time)
sudo apt install -y nodejs npm

# 2. Build the bridge
cd ~/rf-socket-controller/matter-bridge
npm install --omit=dev
npx tsc -p tsconfig.json

# 3. Install + start the systemd unit
sudo cp ~/rf-socket-controller/matter-bridge.service /etc/systemd/system/
sudo systemctl enable --now matter-bridge

# 4. The Go backend reads MATTER_BRIDGE_URL from .env; the default
#    (http://127.0.0.1:8765) Just Works when both run on the same Pi.
sudo systemctl restart rf-controller
```

The Pi's Bluetooth adapter is used for BLE commissioning. Make sure the
service user is in the `bluetooth` group:

```sh
sudo usermod -aG bluetooth claw
```

## Commissioning a device

1. Plug in the device. Most Matter bulbs flash on first boot to indicate
   they're in pairing mode.
2. In the web UI tap **Add socket**, pick **Matter (Wi-Fi)**, paste the
   11-digit manual pairing code (or the `MT:` QR-code payload) printed on
   the device, and press **Commission device**.
3. The bridge discovers the device via BLE, brings it onto your Wi-Fi
   network, and assigns it a stable Matter node id. Save the socket and
   it'll appear alongside your RF sockets.

Commissioning takes 30–60 seconds.

## Compatible devices (no extra hardware)

Anything with the official **Matter** logo that connects over Wi-Fi:

- Linkind / AiDot Matter smart bulbs
- Philips Hue bulbs (via the Hue Bridge with Matter support)
- IKEA TRÅDFRI bulbs (via the DIRIGERA hub)
- Nanoleaf Essentials Matter (Wi-Fi variants)
- Various Matter smart plugs (TP-Link Tapo, Aqara, Eve Energy WiFi…)

Thread-only devices (some Eve and Aqara sensors, certain Nanoleaf panels)
need a separate Thread border router and won't be discovered by this bridge.

## Disabling Matter

Set `MATTER_BRIDGE_URL=disabled` in `.env`. The Go backend will skip the
Matter codepath entirely and the **Matter (Wi-Fi)** option in the socket
editor will fail-soft (the bridge call returns 503).

## HTTP API (proxied through the Go backend)

| Method | Path                              | Notes                                       |
|--------|-----------------------------------|---------------------------------------------|
| GET    | `/api/matter/devices`             | List every node the bridge knows about       |
| POST   | `/api/matter/commission`          | `{ pairing_code }`, returns `{ node_id }`    |
| GET    | `/api/matter/{socketId}`          | Live state (queries the device)              |
| PUT    | `/api/matter/{socketId}/state`    | `{ on?, level?, color?, ct? }`               |

The `socketId` here is the Socket's id in our store; the Socket's `code`
field holds the Matter node id assigned at commissioning time.

## Troubleshooting

- **"matter bridge is not configured"** — `MATTER_BRIDGE_URL` is `disabled`
  or empty. Set it in `.env` and restart.
- **"matter: bridge returned 502"** — the bridge process is down. Check
  `journalctl -u matter-bridge -e`.
- **Commissioning times out** — make sure the device is in pairing mode
  (most flash for the first few minutes after power-on) and the Pi is
  within BLE range (~5m).
- **"device does not expose OnOff"** — the device doesn't advertise an
  on/off cluster. Most Matter lights and plugs do; sensors don't.
- **"bind EAFNOSUPPORT :::5353"** — the host has IPv6 disabled. Matter
  requires IPv6 multicast for mDNS discovery; enable IPv6 on the Pi
  (`sysctl -w net.ipv6.conf.all.disable_ipv6=0`).
