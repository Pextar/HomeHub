# HomeHub backend — Go conventions

Scoped guide for `backend/`. See the root `CLAUDE.md` for project-wide
layout and workflow, and `frontend/CLAUDE.md` for the Svelte side.

## Build & test

```bash
cd backend && go build ./...
cd backend && go test ./...
```

## Package map (`internal/`)

One line per package — read the package doc comment for the real detail.

- **`api/`** — HTTP handlers (net/http, gorilla/mux).
- **`store/`** — state, persistence, validation, actions. Everything
  else hangs off `store.Store`.
- **`scheduler/`** — schedule + automation engine, 5-second tick.
- **`sender/`** — multi-protocol dispatcher implementing `store.RFSender`;
  routes each transmission to the right backend (Tasmota, Matter, MQTT, RF).
- **`rf/`** — userspace 433 MHz transmitter (logs instead of transmitting
  when no hardware is present).
- **`rx/`** — 433 MHz receiver subprocess, feeds sensor readings into the store.
- **`tasmota/`** — Wi-Fi smart-light bridge (Tasmota's local `/cm?cmnd=` API).
- **`matter/`** — Go-side client for the `matter-bridge` Node sidecar
  (matter.js owns the IP/BLE conversation).
- **`mqtt/`** — thin wrapper around the Eclipse Paho MQTT client, one
  shared broker connection.
- **`sonos/`** — Sonos speaker bridge (local UPnP/SOAP), including
  interrupt-and-restore for announcements.
- **`kef/`** — KEF speaker bridge (local HTTP JSON API + SSDP discovery).
- **`airplay/`** — AirPlay (RAOP) sender: mDNS discovery + RTP push.
- **`spotify/`** — Spotify Connect / catalog client.
- **`media/`**, **`mediabridge/`** — vendor-neutral media layer and
  capability model that the scene/automation engine and API target
  instead of talking to a specific speaker bridge directly.
- **`stream/`** — turns one decoded audio source into something several
  speakers can play in sync.
- **`announce/`** — text-to-speech announcements, served to speakers.
- **`llm/`** — Go client for the local Ollama server (assistant/tool-calling).
- **`reachability/`** — polls Wi-Fi/Matter devices, fires push
  notifications on drop/recovery. RF sockets are fire-and-forget, so skipped.
- **`push/`** — notification categories and per-user preference matching.
- **`solar/`** — sunrise/sunset via the NOAA solar position algorithm.
- **`lanhost/`** — validates LAN device addresses before they're
  interpolated into a server-side URL.

## Conventions

- **All state lives in `store.Store`**; callers acquire `Mu` (RWMutex)
  for multi-step operations. Methods annotated "Caller must hold Mu"
  do not lock themselves.
- **`ValidateX` functions** normalise and check; they are always called
  before persisting. Never skip them.
- **`Save()`** writes every JSON file atomically. Call it after any
  mutation; callers hold the lock when calling it.
- **`CascadeDeleteSocket`** must be kept in sync with any new field that
  references a socket ID.
- Scheduler ticks every 5 s; automation engine runs inside the same
  tick. Both use the staged flow below.
- **Multi-socket fan-out** (bulk, group, room, scene, scheduler,
  automations) uses the staged flow in `store/staged.go`:
  `StageAction`/`StageSocketSend` under `Mu` → `SendStaged` off-lock →
  `ApplyStaged` under `Mu`, then `Save()` and `FlushLights()`. Device I/O
  must never run while `Mu` is held. Single-socket toggles use
  `ApplyState`, which transmits synchronously so the HTTP response can
  report the device error directly.
- All transmissions go through `store.Transmit` — never `RF.Send`
  directly. It serializes 433 MHz sends (`txMu`) so concurrent
  transmissions can't overlap on air.
- Smart-light bridge calls (Tasmota, Matter) are always deferred to
  `FlushLights()` so they never block the store lock.
- **A scene step or automation rule may also drive music** (`MusicAction`:
  pause / resume / volume, aimed at a media room key). It rides the same
  buffer the lights do — `QueueMusic` under `Mu`, `FlushMusic()` off it —
  because a scene is activated from six places and every one of them
  already stages and drains. The store never reaches a speaker itself: it
  calls `Store.OnMusic`, installed by the API as `runSceneMusic`, which
  drives the vendor-neutral media layer. **Call `FlushMusic()` beside every
  `FlushLights()`.**
- **An automation rule may also be triggered or gated by what a room is
  playing** (trigger/condition type `music`, same media room key). The read
  half of the hook above: `Store.MusicPlaying`, installed by the API as
  `roomPlaying` (`api/automation_music.go`). It returns a third answer —
  `known=false` — for a room nothing can report on, and both the trigger and
  the condition treat that as "don't fire" rather than as "quiet". It runs on
  the 5 s tick, so it reads the bridges' **caches** (`Monitor.Cached`) and
  must never touch a speaker.
