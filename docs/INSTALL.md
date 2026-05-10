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

### Authentication

The server reads `AUTH_USER` and `AUTH_PASS` from the environment. When
both are set, every HTTP request requires HTTP Basic Auth — browsers
prompt once and remember credentials for the session, and the PWA install
keeps working. Leave both blank to disable auth (NOT recommended on
shared networks).

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
