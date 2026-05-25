# RF Socket Controller - Installation Guide

## Hardware Requirements

- Raspberry Pi (any model with GPIO)
- 433MHz RF Transmitter Module (e.g., FS1000A)
- 433MHz RF Socket Outlets (Nexa, KAKU, Intertechno compatible)
- Jumper wires
- Breadboard (optional)

## Hardware Setup

### Wiring the RF Transmitter

Connect the 433MHz transmitter to Raspberry Pi GPIO:

| Transmitter | Raspberry Pi |
|-------------|--------------|
| VCC | 5V (Pin 2) |
| GND | GND (Pin 6) |
| DATA | GPIO 17 (Pin 11) |

### Enable GPIO Access

```bash
sudo raspi-config
# Interface Options -> GPIO -> Enable
```

### Install RF Tools

Option 1: Using rpi-rf (Python)
```bash
sudo pip3 install rpi-rf
# This provides rpi-rf_send command
```

Option 2: Using wiringPi
```bash
sudo apt-get install wiringpi
# This provides codesend command
```

### 433 MHz receiver (for sensors)

Sensor pairing requires a 433 MHz receiver.  There are two options:

---

#### Option A — superheterodyne module wired to Pi GPIO (recommended)

If you already have a superheterodyne 433 MHz OOK receiver module (the kind
with a green trimmer capacitor and a copper coil), wire it directly to the
Raspberry Pi header.  No extra hardware, no SDR dongle needed.

**Wiring**

| Receiver pin | Raspberry Pi |
|---|---|
| VCC | **3.3 V** — Pin 1 or 17 |
| GND | GND — Pin 6, 9, 14, or any other GND |
| DATA | **GPIO 4** — Pin 7 (or override with `RF_RX_GPIO`) |

> ⚠️  Use **3.3 V**, not 5 V.  Most superheterodyne modules work at 3.3 V; the
> DATA output is then 3.3 V logic — safe for the Pi's GPIO pins.  If you must
> power from 5 V, add a 10 kΩ / 20 kΩ voltage divider on DATA.

Install lgpio (already present on Pi OS Bookworm; install manually on older
releases):

```bash
sudo pip3 install lgpio
```

Set `SENSOR_RX_CMD` in `.env`:

```dotenv
SENSOR_RX_CMD=python3 /home/pi/rf-socket-controller/scripts/ft007th_rx.py
```

Optional environment variables (add to `.env` if needed):

```dotenv
RF_RX_GPIO=4    # BCM pin number, default 4
RF_RX_CHIP=0    # /dev/gpiochipN, default 0
```

Smoke-test before starting the service:

```bash
python3 scripts/ft007th_rx.py
# → trigger the FT007TH; you should see JSON lines appear within 60 s
```

Restart the service and pair via the UI:

```bash
sudo systemctl restart rf-controller
```

---

#### Option B — RTL-SDR USB dongle + rtl_433

The RTL-SDR dongle (RTL2832U + R820T2, ~€15) is a software-defined radio that
covers **all** 433 MHz sensor families (Acurite, Nexus, LaCrosse, Oregon,
Fineoffset, Telldus, etc.) without any sensor-specific code.

```bash
sudo apt install -y rtl-433
rtl_433 -F json      # smoke-test; trigger a sensor and watch for JSON
```

If you see `usb_claim_interface error -6`, unbind the DVB driver first:

```bash
sudo modprobe -r dvb_usb_rtl28xxu rtl2832 rtl2830
```

`SENSOR_RX_CMD` stays **unset** — the controller defaults to `rtl_433 -F json`.

```bash
sudo systemctl restart rf-controller
```

---

#### Option C — nRF52840 Dongle (advanced, requires soldering)

Flash the Zephyr firmware in `firmware/ft007th-rx/` to an nRF52840 Dongle,
solder the receiver DATA line to test pad **TP8 (P0.29)** on the Dongle's
underside, and plug the Dongle into the Pi's USB port.  Then set:

```dotenv
SENSOR_SERIAL_PORT=/dev/ttyACM0
```

See [`firmware/ft007th-rx/README.md`](../firmware/ft007th-rx/README.md) for
full build and flash instructions.  This option makes sense only if all Pi GPIO
pins are in use — Option A is much simpler.

### MQTT broker (for MQTT devices & sensors)

MQTT is a pub/sub protocol, so it needs a **broker** in the middle. The
controller is an MQTT *client* — it does not embed a broker — so you either
point it at an existing broker (Home Assistant's MQTT add-on, an existing
Mosquitto, Zigbee2MQTT's broker) or run one on the Pi itself.

To make the Pi the broker, install Mosquitto. The easiest path is to let the
deploy script do it:

```bash
# Anonymous access on a trusted home LAN:
SETUP_MOSQUITTO=1 scripts/deploy-pi.sh

# Or with a login (recommended — devices and the controller authenticate):
SETUP_MOSQUITTO=1 MQTT_USERNAME=ctrl MQTT_PASSWORD=secret scripts/deploy-pi.sh
```

This installs Mosquitto, writes `/etc/mosquitto/conf.d/rf-socket-controller.conf`
(listening on `1883` for the controller on `127.0.0.1` and for LAN devices),
optionally creates a password file, enables the `mosquitto` service, and adds
`MQTT_BROKER_URL` (plus credentials, if any) to the controller's `.env` — so
the controller starts using the broker on its next restart.

To set it up by hand on the Pi instead, run the bundled script directly:

```bash
cd ~/rf-socket-controller
MQTT_USERNAME=ctrl MQTT_PASSWORD=secret ENV_FILE=.env ./setup-mosquitto.sh
sudo systemctl restart rf-controller
```

Then in the app: add a socket with protocol **MQTT** and its command topic
(e.g. `cmnd/plug/POWER`) as the code, or a sensor with protocol `mqtt` and the
topic to subscribe to. The socket editor's **Send test signal** button
publishes `ON` to confirm the device reacts.

> Security note: with anonymous access enabled, any device on your LAN can
> publish to (and flip) your sockets. Prefer the `MQTT_USERNAME`/`MQTT_PASSWORD`
> form unless your network is fully trusted.

## Software Installation

### Recommended: cross-compile from your laptop

For a 64-bit Pi (Pi 3/4/5 on 64-bit Pi OS or Ubuntu) you do NOT need Go or
Node on the Pi. Build a release locally and rsync it over SSH:

```bash
git clone https://github.com/Pextar/rf-socket-controller.git
cd rf-socket-controller

# 1. Build a release into dist/release/  (binary + frontend + systemd unit)
scripts/build-pi.sh
#    For 32-bit Pi OS / Pi Zero/1/2: GOARCH=arm GOARM=7 scripts/build-pi.sh

# 2. Deploy over SSH (defaults to pi@raspberrypi.local)
scripts/deploy-pi.sh                  # or: scripts/deploy-pi.sh pi@192.168.1.42
```

`scripts/deploy-pi.sh` does the following on the Pi:

- rsyncs the binary and `frontend/dist/` to `~/rf-socket-controller/`,
- seeds `.env` from `env.example` on first run (never overwrites an
  existing `.env`),
- installs `rf-controller.service` into `/etc/systemd/system/`, then
  `daemon-reload` + `enable` + `restart`.

After the first deploy, set credentials and (optionally) the port:

```bash
ssh pi@raspberrypi.local 'nano ~/rf-socket-controller/.env \
  && sudo systemctl restart rf-controller'
```

The systemd unit assumes the SSH user is `pi`; if not, edit
`deploy/rf-controller.service` (User= and the WorkingDirectory paths)
before `scripts/build-pi.sh`.

### Authentication & profiles

Login uses named **profiles** stored on the server (in `data/users.json`,
passwords bcrypt-hashed). On first start, `AUTH_USER` and `AUTH_PASS` seed
an initial **admin** profile — set them once, then manage everyone from
the app's **Settings → Profiles**. Once any profile exists the env vars are
ignored (they only seed an empty install). Leave both blank on a brand-new
install to disable auth entirely (NOT recommended on shared networks).

Profiles come in two kinds:

- **Admin** — full access to every device and all settings, and can
  create/edit/delete other profiles. Signs in with **username + password**.
- **Limited** — sees and controls only the devices an admin assigns to it;
  groups, scenes, schedules, sensors and settings are hidden. Signs in with
  a **6-digit login code** the app generates (no password) — the admin
  shares it from Settings → Profiles and can regenerate it anytime. Access
  is enforced server-side, so a hidden device can't be reached by API
  either.

The login page asks for a code by default, with a "Sign in as admin" link
that reveals the username + password form. The SPA uses a cookie session;
scripted clients (curl, iOS Shortcuts) can still use HTTP Basic Auth with
an admin's credentials. Login codes are short by design — they're a
local-network convenience, not a hardened secret.

### Build on the Pi instead (alternative)

Only if you would rather not cross-compile — Node 18+ and Go 1.21+ are
required on the Pi:

```bash
cd frontend && npm install && npm run build && cd ..
cd backend  && go build -o rf-controller && cd ..
AUTH_USER=admin AUTH_PASS=secret PORT=8080 ./backend/rf-controller
```

### Access the web UI

Open `http://raspberrypi.local:8080` (or `http://<pi-ip>:8080`).

The page is also a Progressive Web App: on Chrome / Safari use
"Install" / "Add to Home Screen" to get an installable shortcut that
works offline (the shell loads even without network — controlling
sockets still requires the Pi to be reachable).

## Enabling HTTPS (Tailscale — recommended for iPhones and Web Push)

Several features require a **secure context** (HTTPS):

- Web Push notifications on iOS / Safari
- QR-code scanner (`getUserMedia`) in mobile browsers
- `Secure` flag on session cookies

The cleanest solution is **Tailscale + Caddy**: Tailscale gives the Pi a
stable `<machine>.ts.net` hostname backed by a real Let's Encrypt certificate
(no browser warnings, works on iPhone out of the box).  Caddy acts as a thin
reverse proxy that terminates TLS and forwards requests to the Go backend on
port 8080.

### What you need

| Component | Install |
|-----------|---------|
| Tailscale on the Pi | `curl -fsSL https://tailscale.com/install.sh \| sh` |
| Tailscale on each phone/laptop | [tailscale.com/download](https://tailscale.com/download) — free personal account |
| Caddy on the Pi | `sudo apt install -y caddy` |

All devices accessing the app need Tailscale running and signed into **the same account** (or a shared tailnet).

### One-time setup

```bash
# 1. Connect the Pi to Tailscale
sudo tailscale up

# 2. Enable HTTPS for the machine in the Tailscale admin panel:
#    https://login.tailscale.com/admin/machines
#    → click the machine → "…" menu → Enable HTTPS

# 3. Install Caddy
sudo apt install -y caddy

# 4. Run the setup script (issues the cert, writes Caddyfile, installs cron)
sudo ./deploy/tailscale-https-setup.sh
```

The script:
- Fetches a real cert from Tailscale / Let's Encrypt (`tailscale cert`)
- Writes `/etc/caddy/Caddyfile` pointed at `localhost:8080`
- Enables and reloads the `caddy` systemd service
- Comments out `HTTPS_PORT` in `.env` (Caddy owns TLS from here)
- Installs a weekly cron (`/etc/cron.weekly/tailscale-cert-renew`) to auto-renew the cert

After setup, the app is available at `https://<machine>.ts.net` — replace
`<machine>` with the name shown by `tailscale status`.

### Fallback: self-signed cert (desktop browsers only)

If you don't want Tailscale, set `HTTPS_PORT=8443` in `.env`.  The app
auto-generates a self-signed certificate.  Desktop browsers warn once and
let you proceed; iOS Safari typically refuses entirely, and Web Push
subscriptions won't work on iPhones.

```bash
# In .env:
HTTPS_PORT=8443
```

Then reach the app at `https://<pi-ip>:8443`.

## Frontend development (optional)

For live-reload hacking on the UI without rebuilding each time:

```bash
# Terminal 1 — backend
cd backend && ./rf-controller

# Terminal 2 — Vite dev server
cd frontend && npm run dev
```

`npm run dev` starts Vite on http://localhost:5173 and proxies `/api/*`
to the Go server on :8080.

## Finding Your Socket Codes

### Method 1: Using Existing Remote

1. Press a button on your existing remote
2. Use receiver to capture code:
   ```bash
   sudo rpi-rf_receive
   ```
3. Note the code displayed

### Method 2: Brute Force (for simple sockets)

Some sockets use simple codes. Try common ranges:
- 10000-19999 for channel 1
- 20000-29999 for channel 2
- etc.

## Configuration

Data is stored in `./data/` directory:
- `sockets.json` - Socket configurations
- `schedules.json` - Timer schedules

## Autostart

`scripts/deploy-pi.sh` installs `deploy/rf-controller.service` for you.
If you ever need to reinstall by hand:

```bash
sudo install -m 644 ~/rf-socket-controller/rf-controller.service \
  /etc/systemd/system/rf-controller.service
sudo systemctl daemon-reload
sudo systemctl enable --now rf-controller
journalctl -u rf-controller -f
```
