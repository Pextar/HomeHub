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

### 433MHz Receiver (for sensors)

Sensor pairing needs a receiver. The recommended setup is an **RTL-SDR
USB dongle** (any RTL2832U + R820T2 stick, ~$15–25); decoded by
`rtl_433`, it covers Acurite, Nexus, LaCrosse, Oregon, Fineoffset,
Telldus and most other common 433 MHz sensor families out of the box.

```bash
sudo apt install -y rtl-433
```

Plug in the dongle, then smoke-test from a terminal:

```bash
rtl_433 -F json
```

Trigger a sensor (press a doorbell, wait for the thermometer to beacon).
You should see one JSON line per packet. If you get `usb_claim_interface
error -6` or "device busy", the kernel's DVB driver auto-bound to the
dongle — unbind it and retry:

```bash
sudo modprobe -r dvb_usb_rtl28xxu rtl2832 rtl2830
```

The `rtl-433` package ships a udev rule that grants non-root access; if
you still hit permission errors after a reboot, add yourself to the
`plugdev` group: `sudo usermod -aG plugdev $USER` and log out / back in.

`SENSOR_RX_CMD` in `.env` stays unset — the controller's default is
already `rtl_433 -F json`. Restart the service after install:

```bash
sudo systemctl restart rf-controller
```

Then hit **Pair** on the Sensors page in the UI and trigger your sensor.

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
