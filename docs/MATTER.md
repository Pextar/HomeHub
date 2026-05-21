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
Border Router**. After commissioning, the Pi speaks to them over regular IP via
the Border Router.

```
Thread device ←→ Thread mesh ←→ Border Router (Pi + dongle) ←→ IP network ←→ Pi (matter.js)
```

**The Pi does not need a Thread radio for the matter.js controller role.** However,
you do need something on your network acting as a Thread Border Router, and you
need the **Thread Operational Dataset** from it — the ~100-byte hex credential
that lets you commission devices onto that Thread network.

### What you need

| Item | Notes |
|------|-------|
| **nRF52840 Dongle** (Nordic PCA10059) | ~$10 from Mouser, Digikey, or Nordic directly. **Get the official Nordic PCA10059** — clones exist but the pre-built firmware and guides all target this specific board. |
| **Raspberry Pi** | The same Pi that runs rf-socket-controller is fine. It gains a second role as the Thread Border Router. |

> **Why not Apple TV or HomePod mini?**
> Apple TV 4K and HomePod mini do act as Thread Border Routers for Apple Home,
> but Apple provides no way to export the Operational Dataset from them to a
> third-party controller. The nRF52840 dongle approach gives you a Thread network
> you fully control — and Thread devices you commission via rf-socket-controller
> will live on *your* network, not Apple's.

---

### Step 1 — Flash RCP firmware onto the dongle

The dongle needs **RCP (Radio Co-Processor) firmware** so the Pi can drive the
Thread radio. This is a one-time step done from any computer (Windows / Mac /
Linux).

1. Install **nRF Connect for Desktop** (free):
   https://www.nordicsemi.com/Products/Development-tools/nRF-Connect-for-Desktop

2. Open it and install the **Programmer** app from inside it.

3. Download the pre-built RCP firmware `.hex` for the nRF52840 Dongle from the
   OpenThread nRF528xx releases page:
   https://github.com/openthread/ot-nrf528xx/releases
   Look for a file containing `nrf52840dongle` and `rcp` in the name.

4. Put the dongle in bootloader mode: press the small **RESET button** (on the
   side, next to the USB connector) until the red LED starts **pulsing slowly**.

5. Plug the dongle into your computer. In the Programmer app, select it from the
   device list, load the `.hex` file, and click **Write**.

6. The dongle is ready. Plug it into the Pi.

---

### Step 2 — Run OpenThread Border Router on the Pi

The easiest way is Docker. SSH into the Pi:

```sh
# Install Docker (skip if already installed)
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER
# Log out and back in so the group change takes effect.

# Find the dongle's serial port (usually ttyACM0)
ls /dev/ttyACM*

# Run OTBR. Replace ttyACM0 if your port is different.
docker run --name otbr --sysctl "net.ipv6.conf.all.disable_ipv6=0 \
  net.ipv4.conf.all.forwarding=1 net.ipv6.conf.all.forwarding=1" \
  -p 8080:80 --dns=127.0.0.1 --restart unless-stopped -d \
  --volume /dev/ttyACM0:/dev/ttyACM0 \
  --privileged openthread/otbr \
  --radio-url spinel+hdlc+uart:///dev/ttyACM0
```

OTBR's web UI is now at `http://<pi-ip>:8080`. On first run, go there and click
**Form** to create a new Thread network (leave all defaults or pick a name).
Wait about 30 seconds for the network to form.

> **Native install (no Docker):** If you prefer not to use Docker, follow the
> official guide at https://openthread.io/guides/border-router/raspberry-pi —
> it uses the same `ot-br-posix` scripts but installs as a systemd service.

---

### Step 3 — Get the Operational Dataset

Once the Thread network is formed:

```sh
# From the Pi host (Docker):
sudo docker exec otbr ot-ctl dataset active -x

# Or if you used the native install:
sudo ot-ctl dataset active -x
```

This prints a single hex string, for example:
```
0e080000000000010000000300001235060004001fffe002083d3fccb9e36e2b7d0708fd9e...
```

Copy the entire string — that's your `MATTER_BRIDGE_THREAD_DATASET`.

---

### Step 4 — Configure the bridge

Add to your `.env` on the Pi:

```bash
# Thread Operational Dataset from your OTBR (hex TLV)
MATTER_BRIDGE_THREAD_DATASET=0e080000000000010000000300001235...

# The Thread network name is parsed automatically from the dataset above.
# Only set this if the bridge refuses to start with a "Could not determine
# Thread network name" error (shouldn't happen with a well-formed dataset).
#MATTER_BRIDGE_THREAD_NETWORK_NAME=MyThreadNet
```

Then restart the bridge:

```sh
sudo systemctl restart matter-bridge
```

**Both Wi-Fi and Thread credentials can coexist in `.env`.** When both
`MATTER_BRIDGE_WIFI_SSID` and `MATTER_BRIDGE_THREAD_DATASET` are set, the
commission wizard shows a **Thread / Wi-Fi picker** so you choose per device.

---

### Compatible Thread devices

- **Eve** — Energy, Door & Window, Motion (Thread variants)
- **Aqara** — Door and Window Sensor P2, Motion Sensor P2, Temperature & Humidity E1
- **Nanoleaf** — Essentials A19/BR30 (Thread variant), Matter Buttons
- **IKEA DIRIGERA** — sensors commissioned via the DIRIGERA hub
- Any device with the official **Matter** logo that lists Thread as a transport

---

## Setup on the Pi (matter-bridge)

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
   - Thread device → `MATTER_BRIDGE_THREAD_DATASET` (and OTBR running)
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

| Method | Path                               | Notes                                                          |
|--------|------------------------------------|----------------------------------------------------------------|
| GET    | `/api/matter/transport`            | Returns `{ transports: ["thread","wifi"] }` (configured ones) |
| GET    | `/api/matter/devices`              | List every node the bridge knows about                         |
| POST   | `/api/matter/commission`           | `{ pairing_code, transport? }`, returns `{ job_id }`          |
| GET    | `/api/matter/commission/jobs/{id}` | Poll commissioning job status                                  |
| GET    | `/api/matter/{socketId}`           | Live state (queries the device)                                |
| PUT    | `/api/matter/{socketId}/state`     | `{ on?, level?, color?, ct? }`                                 |

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
- **Commissioning times out (Thread device)** — confirm OTBR is running
  (`docker ps` or `systemctl status otbr`), the Operational Dataset in
  `MATTER_BRIDGE_THREAD_DATASET` matches the active network (`ot-ctl dataset
  active -x`), and the device is within BLE range of the Pi (~5 m).
- **"Could not determine Thread network name"** — the dataset TLV may be
  malformed. Re-run `ot-ctl dataset active -x` and re-paste. As a workaround
  set `MATTER_BRIDGE_THREAD_NETWORK_NAME` to your Thread network name manually.
- **"device does not expose OnOff"** — the device doesn't advertise an
  on/off cluster. Most Matter lights and plugs do; sensors don't.
- **"bind EAFNOSUPPORT :::5353"** — IPv6 is disabled on the host. Matter
  requires IPv6 multicast for mDNS discovery; enable it on the Pi:
  ```sh
  sysctl -w net.ipv6.conf.all.disable_ipv6=0
  # Make permanent:
  echo "net.ipv6.conf.all.disable_ipv6=0" | sudo tee -a /etc/sysctl.conf
  ```
- **Thread device unreachable after commissioning** — the Border Router may
  not yet have advertised a route to the new node. Wait 30–60 s and retry;
  mDNS resolution across Thread can be slower than Wi-Fi.
- **OTBR web UI unreachable at :8080** — check `docker logs otbr`. If the
  dongle isn't found, confirm it appears as `/dev/ttyACM0` (`ls /dev/ttyACM*`)
  and that no other process has it open.
