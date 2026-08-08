# HomeHub

Control 433 MHz RF sockets, Tasmota Wi-Fi devices, Matter smart bulbs/plugs, MQTT devices, and
Sonos/KEF speakers from a single installable PWA — with scheduling, automations, scenes, sensors,
whole-home music, voice announcements, a kiosk wall panel, and an on-device AI assistant.

## Architecture

| Layer | Technology |
|---|---|
| **Backend** | Go — REST + SSE API, gorilla/mux, GPIO/RF control |
| **Frontend** | Svelte 5 + Vite + vite-plugin-pwa (installable, offline-capable) |
| **Matter bridge** | Node.js sidecar (matter-bridge/) wrapping matter.js, containerized |
| **Media** | Vendor-neutral media protocol (`internal/media/`) unifying Sonos (UPnP/SOAP), KEF (HTTP JSON), and Spotify (Web API/PKCE, Connect playback) behind one interface |
| **Announcements** | Chime always available; optional TTS via any OpenAI-compatible `/audio/speech` endpoint (e.g. local Piper) for spoken household announcements |
| **Wall Panel** | Kiosk mode (`/panel`) — ambient clock face, room tiles, music dashboard, full-screen player, and search, built for an always-on tablet |
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

### Music
- One noun — a **room** — across two genuinely different bridges: a Sonos household (grouping, shared queue, favorites, topology) and a standalone KEF stereo pair (transport, source, volume), plus HomeHub zones that span both
- Spotify search, library/saved content, and Connect playback; per-account multi-session handling since Spotify allows only one active Connect session at a time
- Drag one room onto another to group them and play together; per-room and per-zone volume
- Favorites, recent-search history, "Play similar," and continuous queue playback (auto-refills so a queue never runs dry)
- Full-screen player with scrubbing, queue view, and speaker grouping controls
- Music timers — wake up to music, fall asleep to it, with fade in/out
- Per-room and household-wide playback history

### Announcements
- Text-to-voice "call the house" announcements, targetable at specific rooms or the whole home
- Always-available synthesized chime; spoken words layer on top via an optional, self-hosted OpenAI-compatible TTS endpoint (e.g. Piper) — degrades gracefully to the chime if unset or unreachable
- Snapshots and restores whatever a room was already playing around the interruption

### Wall Panel (kiosk mode)
- Dedicated `/panel` kiosk surface for an always-mounted tablet: no app chrome, large touch targets
- Ambient idle face (clock + glanceable status) that the dashboard, music browse, and full-screen player all fall back to when untouched
- Room tiles, live music dashboard with quick-start shelf, and Spotify search with on-screen keyboard support
- Per-room targeting for the wall announcement feature
- Auto-applies PWA updates for panel-homed kiosk devices

### Insights & Monitoring
- **Insights** — live power draw from power sensors, per-meter history; only ever shows real metered data
- **Console** — at-a-glance system status: devices online, protocol breakdown, hub health
- **Floor Plan** — drag-and-drop blueprint view of lights by saved x/y position
- **Activity log** — filterable history of automation, scene, and manual actions
- Background reachability sweep for Wi-Fi/Matter devices (RF sockets excluded — no return channel) with debounced offline/online push notifications
- Sunrise/sunset resolution (`internal/solar/`) shared by schedules, automations, and announcements
- LAN host validation (`internal/lanhost/`) hardens the Sonos/KEF/Tasmota bridges against SSRF when a device address is interpolated into a server-side URL

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
- **Kids mode** — oversized playful layout, login-code auth, device restrictions, own schedule view, own dedicated music player
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
- Sonos speakers and/or KEF wireless speakers on the local network (optional)
- A tablet or spare display for the wall panel kiosk mode (optional)
- SSD recommended for Ollama model storage

## Installation

See [docs/INSTALL.md](docs/INSTALL.md) for hardware wiring, RF tools, Ollama setup, and Matter bridge container instructions.

## API

See [docs/API.md](docs/API.md) for REST endpoint reference, [docs/MATTER.md](docs/MATTER.md) for the Matter bridge protocol, and [docs/MEDIA-PROTOCOL.md](docs/MEDIA-PROTOCOL.md) for the vendor-neutral music/speaker protocol.
