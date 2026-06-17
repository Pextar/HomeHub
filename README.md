# HomeHub

Control 433 MHz RF sockets, Tasmota Wi-Fi devices, Matter smart bulbs/plugs, and MQTT devices
from a single installable PWA — with scheduling, automations, scenes, sensors, and an on-device
AI assistant.

## Architecture

| Layer | Technology |
|---|---|
| **Backend** | Go — REST + SSE API, gorilla/mux, GPIO/RF control |
| **Frontend** | Svelte 5 + Vite + vite-plugin-pwa (installable, offline-capable) |
| **Matter bridge** | Node.js sidecar (matter-bridge/) wrapping matter.js, containerized |
| **LLM assistant** | Ollama running on-device (llama3.2:1b / qwen2.5:1.5b / qwen3.5) |
| **MQTT** | Optional broker connection — control devices and ingest sensor readings |
| **Hardware** | 433 MHz transmitter + superheterodyne receiver on Raspberry Pi GPIO |

## Features

### Device control
- On/off control for 433 MHz RF sockets
- Brightness, color, and color-temperature for smart bulbs (Tasmota, Matter)
- Protocol support: RF (433 MHz), Wi-Fi (Tasmota), Matter-over-Wi-Fi/Thread, MQTT
- Single-socket and bulk fan-out (group, room, scene) via a staged-send flow that keeps RF/IP I/O off the store lock

### Organisation
- **Rooms** — top-level screen; sockets and bulbs grouped by room
- **Groups** — named collections of sockets for one-tap bulk control
- **Scenes** — saved looks with per-lamp brightness/color presets; room/group scoping; capture from live state; test before saving

### Scheduling & Automations
- **Schedules** — fixed-time or sunrise/sunset (with offset) timers targeting sockets, groups, rooms, or scenes
- **Automations** — multiple When→Then rules per automation; triggers: time, sensor readings, device state; conditions: time-range, time_before, time_after (with sunrise/sunset support); per-lamp action customization

### Sensors
- Temperature, humidity, motion, light, power, and custom sensor types
- Configurable alerts that feed into automation conditions
- RF receiver (superheterodyne, GPIO 4) for 433 MHz sensor pairing
- Debounced reading persistence; DST-safe timestamps

### AI Assistant
- Natural language device control and Q&A powered by Ollama on-device
- Streaming SSE responses; summoned as a floating overlay (non-modal on desktop)
- Tool calling for structured actions (on/off, brightness, scenes, etc.)
- Tuned for Pi-speed inference — compact system prompt, small default model

### Push Notifications
- Web Push (VAPID) with categories: sensor alerts, state changes, schedules, device offline
- Per-device muting and quiet-hours configuration

### Users & Access Control
- Owner / admin / limited roles
- **Kids mode** — oversized playful layout, login-code auth, device restrictions, own schedule view
- Session rolling renewal; login brute-force protection; CSRF hardening

### Matter
- BLE → Wi-Fi / Thread commissioning flow
- QR-code scanner in-app
- Matter bridge containerized as a sidecar; device discovery and state sync

### UX
- Installable PWA with offline support and service-worker pre-caching
- Apple-style mobile tab bar with lamp-glow pill indicator
- Skeleton loaders instead of spinners; view transitions; reduced-motion support
- Unsaved-changes guard on all form sheets
- Sunrise/sunset times resolved and displayed on schedule/automation cards

## Hardware Requirements

- Raspberry Pi (any model with GPIO, Wi-Fi, and Bluetooth)
- 433 MHz RF transmitter module (data pin → GPIO 17 by default)
- 433 MHz RF receiver module — superheterodyne recommended (data pin → GPIO 4) for sensor pairing
- 433 MHz RF sockets (optional)
- Matter-over-Wi-Fi or Matter-over-Thread devices (optional — bulbs, plugs)
- Tasmota flashed Wi-Fi devices (optional)
- SSD recommended for Ollama model storage

## Installation

See [docs/INSTALL.md](docs/INSTALL.md) for hardware wiring, RF tools, Ollama setup, and Matter bridge container instructions.

## API

See [docs/API.md](docs/API.md) for REST endpoint reference and [docs/MATTER.md](docs/MATTER.md) for the Matter bridge protocol.
