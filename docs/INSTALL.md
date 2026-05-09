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

### 1. Clone Repository

```bash
git clone https://github.com/Pextar/rf-socket-controller.git
cd rf-socket-controller
```

### 2. Build the frontend (Svelte + Vite + PWA)

The web UI is a Svelte 5 app compiled to static files served by the Go
backend. Node.js 18+ is required at build time only.

```bash
cd frontend
npm install
npm run build
cd ..
```

This produces `frontend/dist/`, which the Go server serves.

### 3. Build Backend

```bash
cd backend
go mod tidy
go build -o rf-controller
```

### 4. Run

```bash
./rf-controller
# Or with a custom port:
PORT=3000 ./rf-controller
```

### 5. Access Web Interface

Open browser to: `http://raspberry-pi-ip:8080`

The page is also a Progressive Web App: on Chrome / Safari you can use
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

## Autostart (Optional)

Create systemd service:

```bash
sudo nano /etc/systemd/system/rf-controller.service
```

Add:
```ini
[Unit]
Description=RF Socket Controller
After=network.target

[Service]
Type=simple
User=pi
WorkingDirectory=/home/pi/rf-socket-controller/backend
ExecStart=/home/pi/rf-socket-controller/backend/rf-controller
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

Enable:
```bash
sudo systemctl enable rf-controller
sudo systemctl start rf-controller
```
