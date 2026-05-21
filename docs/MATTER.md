# Matter Support

The controller can drive any Matter-certified device — both **Matter over Wi-Fi**
(smart bulbs, plugs) and **Matter over Thread** (sensors, some Eve / Aqara /
Nanoleaf devices) — alongside the existing 433MHz RF sockets and Tasmota devices.

## How it works

```
Browser ── HTTP ──> Go backend ── HTTP (loopback) ──> matter-bridge ── BLE/IP ──> device
                       │
                       └── 433MHz RF, Tasmota (unchanged)
```

`matter-bridge/` is a small Node.js process that owns the matter.js library and
the on-the-wire conversation. The Go backend never speaks the Matter protocol
directly — it just calls a tiny loopback JSON API.

---

## Matter over Wi-Fi

Devices connect directly to your Wi-Fi network. The Pi commissions them via BLE
and hands them the SSID + password.

**Required env vars:**
```
MATTER_BRIDGE_WIFI_SSID=YourWiFiNetwork
MATTER_BRIDGE_WIFI_PASS=YourWiFiPassword
```

**Compatible devices (no extra hardware beyond the Pi):**
- Linkind / AiDot Matter smart bulbs
- Philips Hue bulbs (via the Hue Bridge with Matter support)
- IKEA TRÅDFRI bulbs (via the DIRIGERA hub)
- Nanoleaf Essentials Matter (Wi-Fi variants)
- TP-Link Tapo, Aqara, Eve Energy WiFi smart plugs

---

## Matter over Thread

Thread is a low-power mesh network designed for IoT. Thread devices don't join
Wi-Fi — they form their own mesh and reach the IP network through a **Thread
Border Router**. After commissioning the Pi speaks to them over regular IP, via
the Border Router.

```
Thread device ←→ Thread mesh ←→ Border Router ←→ IP network ←→ Pi (matter.js)
```

**The Pi does not need a Thread radio.** It is the *controller*, not a Thread
node. All traffic between the Pi and Thread devices flows over normal IP through
the Border Router.

### What you need

| Component | Notes |
|-----------|-------|
| **Thread Border Router** | Apple TV 4K (3rd gen 2022+), HomePod mini, or any OpenThread Border Router |
| **Thread Operational Dataset** | A ~100-byte hex TLV containing the Thread network key, channel, PAN ID, etc. — fetched from the Border Router (once) |

### Getting the Thread Operational Dataset

The Operational Dataset is the credential you put in `.env`. You only need to
do this once; existing Thread devices continue working across restarts.

#### From an Apple TV 4K or HomePod mini (easiest)

1. Install the free **Thread** app by the Thread Group on your iPhone or iPad:
   [App Store →](https://apps.apple.com/app/thread/id1499524355)
2. Open the app on the same local network as your Apple TV / HomePod. It
   auto-discovers the Border Router and shows the active Thread network.
3. Tap **Copy Active Dataset** to copy the hex string, then paste it as
   `MATTER_BRIDGE_THREAD_DATASET` in your `.env` file.

> **Note:** Apple TV 4K (3rd gen, A15, 2022 or later) and HomePod mini both
> have a Thread radio built in and act as a Border Router automatically once
> configured in the Apple Home app. No extra setup on the Apple device is
> needed.

#### From an OpenThread Border Router (OTBR)

```sh
# SSH into the border router, then:
sudo ot-ctl dataset active -x
# Copy the hex string printed.
```

#### Via chip-tool (advanced)

```sh
# Build the Matter SDK and run:
chip-tool pairing get-commissioner-node-id
# Then query the Border Agent on your network (UDP 49191) — see the
# Matter SDK docs for the full flow.
```

### Configuration

Add to your `.env`:

```bash
# Thread Operational Dataset from your Border Router (hex TLV)
MATTER_BRIDGE_THREAD_DATASET=0e080000000000010000000300001235...

# The Thread network name is parsed automatically from the dataset TLV.
# Only set this if the bridge fails to start with a "Could not determine
# Thread network name" error (shouldn't happen with a well-formed dataset).
#MATTER_BRIDGE_THREAD_NETWORK_NAME=MyThreadNet

# Do NOT also set MATTER_BRIDGE_WIFI_SSID — Thread takes priority and
# only one transport credential set is sent per commissioning.
```

**Both can be set simultaneously.** When both `MATTER_BRIDGE_THREAD_DATASET`
and `MATTER_BRIDGE_WIFI_SSID` are configured, the commission wizard shows a
"Thread / Wi-Fi" picker so you can choose per device — no need to comment and
uncomment env vars.

### Compatible Thread devices

- **Eve** — Energy, Door & Window, Motion (Thread variants)
- **Aqara** — Door and Window Sensor P2, Motion Sensor P2, Temperature & Humidity E1
- **Nanoleaf** — Essentials A19/BR30 (Thread variant), Matter Buttons
- **IKEA DIRIGERA** — sensors commissioned via the DIRIGERA hub
- Any device with the official **Matter** logo that lists Thread as a transport

---

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

The Pi's Bluetooth adapter is used for BLE commissioning (both Wi-Fi and Thread
devices are discovered over BLE). Make sure the service user is in the
`bluetooth` group:

```sh
sudo usermod -aG bluetooth <your-service-user>
```

---

## Commissioning a device

1. Plug in / power on the device. Most Matter devices flash on first boot to
   indicate they're in pairing mode.
2. Make sure the relevant env var(s) are set in `.env`:
   - Wi-Fi device → `MATTER_BRIDGE_WIFI_SSID` / `MATTER_BRIDGE_WIFI_PASS`
   - Thread device → `MATTER_BRIDGE_THREAD_DATASET`
   - Both can coexist — the wizard will ask which to use.
3. In the web UI tap **Add socket**, pick **Matter (Wi-Fi)** or **Matter (Thread)**,
   paste the 11-digit manual pairing code (or the `MT:` QR-code payload) printed
   on the device, and press **Commission device**.
4. The bridge discovers the device via BLE, hands it the network credentials,
   and assigns it a stable Matter node id. Save the socket and it'll appear
   alongside your RF sockets.

Commissioning takes 30–60 seconds.

---

## Disabling Matter

Set `MATTER_BRIDGE_URL=disabled` in `.env`. The Go backend will skip the
Matter codepath entirely and the Matter options in the socket editor will
fail-soft (the bridge call returns 503).

---

## HTTP API (proxied through the Go backend)

| Method | Path                                    | Notes                                             |
|--------|-----------------------------------------|---------------------------------------------------|
| GET    | `/api/matter/transport`                 | Returns `{ transport: "thread"\|"wifi"\|"none" }` |
| GET    | `/api/matter/devices`                   | List every node the bridge knows about             |
| POST   | `/api/matter/commission`                | `{ pairing_code }`, returns `{ job_id }`           |
| GET    | `/api/matter/commission/jobs/{id}`      | Poll commissioning job status                      |
| GET    | `/api/matter/{socketId}`                | Live state (queries the device)                    |
| PUT    | `/api/matter/{socketId}/state`          | `{ on?, level?, color?, ct? }`                     |

The `socketId` here is the Socket's id in our store; the Socket's `code`
field holds the Matter node id assigned at commissioning time.

---

## Troubleshooting

- **"matter bridge is not configured"** — `MATTER_BRIDGE_URL` is `disabled`
  or empty. Set it in `.env` and restart.
- **"matter: bridge returned 502"** — the bridge process is down. Check
  `journalctl -u matter-bridge -e`.
- **Commissioning times out (Wi-Fi device)** — make sure the device is in
  pairing mode, Bluetooth is available on the Pi, and `MATTER_BRIDGE_WIFI_SSID`
  is set to a 2.4 GHz network.
- **Commissioning times out (Thread device)** — confirm the Thread Border
  Router (Apple TV / HomePod) is online, the Operational Dataset in
  `MATTER_BRIDGE_THREAD_DATASET` is correct, and the device is within BLE
  range of the Pi (~5m). Re-copy the dataset from the Thread app if in doubt.
- **"device does not expose OnOff"** — the device doesn't advertise an
  on/off cluster. Most Matter lights and plugs do; sensors don't.
- **"bind EAFNOSUPPORT :::5353"** — the host has IPv6 disabled. Matter
  requires IPv6 multicast for mDNS discovery; enable IPv6 on the Pi:
  ```sh
  sysctl -w net.ipv6.conf.all.disable_ipv6=0
  # Make permanent:
  echo "net.ipv6.conf.all.disable_ipv6=0" | sudo tee -a /etc/sysctl.conf
  ```
- **Thread device unreachable after commissioning** — the Thread Border Router
  may not yet have advertised a route to the new node. Wait 30–60 s and
  retry; mDNS resolution across Thread can be slower than Wi-Fi.
