# HomeHub backend — Go conventions

Scoped guide for `backend/`. See the root `CLAUDE.md` for project-wide
layout and workflow, `frontend/CLAUDE.md` for the Svelte side, and
`docs/ARCHITECTURE.md` for why the backend is shaped the way it is.

## Build & test

```bash
cd backend && go build ./...
cd backend && go test ./...
cd backend && golangci-lint run   # CI runs this and it fails the build
```

## The shape of it

Four tiers, and the arrows only point downward:

```
cmd/homehub          flags in, one of three entry points out
     ↓
internal/app         the composition root — builds everything, owns lifecycle
     ↓
services             control · music · assistant · autoplay · musictimer
                     listening · speakermon · audio · announce
     ↓
internal/store       the house's state, one lock, on disk as JSON
     +
drivers              sonos · kef · airplay · upnp · spotify · qobuz · matter
                     mqtt · tasmota · rf · rx · llm · stream · media
```

`internal/api` sits beside the services rather than above them: it is one
of several callers, not the top of the stack. A background engine and an
HTTP handler reach the same service the same way.

**Nothing but `internal/app` assembles anything.** A package builds only
itself and declares what it needs as `Config` fields or interfaces. This is
the rule most likely to be broken by accident: if you find yourself reaching
for another subsystem from inside a service, add a field to its `Config` and
let the composition root fill it in.

## Package map (`internal/`)

One line per package — read the package doc comment for the real detail.

### The application

- **`app/`** — composition root. `Config` (every environment variable the
  application itself reads), `New` (builds every subsystem), `Run` (starts
  the background work and both listeners, then shuts down in order), and
  `hooks.go` (everything the store calls back into).
- **`api/`** — the HTTP surface: routes, middleware, sessions, SPA host,
  Server-Sent Events, and one handler per endpoint. Handlers decode, call a
  service, and encode. They own no long-lived state.

### Services

- **`control/`** — how anything switches a device. The staged flow lives
  here; see "Locking" below.
- **`music/`** — where a household means when it names a room, what can play
  there, and what stays running once it does. Endpoint resolution, provider
  selection, live sessions, the play history, and the scene/automation music
  hooks.
- **`assistant/`** — the local LLM agent: the loop, the state snapshot, the
  entity resolution, the tools, and the signed confirmation tokens.
- **`autoplay/`** — "continue with similar music" when a queue runs out.
- **`musictimer/`** — wake-ups and sleep timers, and the volume fades.
- **`listening/`** — what each room was *heard* playing, written from
  readings something else already had in hand.
- **`speakermon/`** — the cached view of every speaker: Sonos over GENA,
  KEF by polling, plus the two slow per-speaker lookups.
- **`audio/`** — live sound: the HTTP stream speakers pull from, the
  librespot and Qobuz decoders, the AirPlay sender.
- **`announce/`** — text-to-speech announcements, and the clip host.
- **`scheduler/`** — schedules and the automation engine, on a 5 s tick.
- **`reachability/`** — polls Wi-Fi/Matter devices, pushes on drop and
  recovery. RF sockets are fire-and-forget, so they are skipped.
- **`push/`** — Web Push categories and per-user preference matching.

### State

- **`store/`** — every piece of persisted state, one `RWMutex`, atomic JSON
  writes. Validators, cascade deletes, the activity log, the play history
  and the listening log.

### Drivers and protocol

- **`media/`** — the vendor-neutral protocol: endpoints, routes, plans,
  quality chains. Knows nothing about this house.
- **`mediabridge/`** — adapts one make of speaker, or one catalogue, to that
  protocol.
- **`sender/`** — multi-protocol dispatcher implementing both of the store's
  outbound interfaces (on/off and brightness/colour).
- **`sonos/`**, **`kef/`**, **`airplay/`**, **`upnp/`** — speaker bridges.
- **`spotify/`**, **`qobuz/`** — catalogue and account clients.
- **`stream/`** — the HTTP stream host and the two decoders behind it.
- **`matter/`**, **`mqtt/`**, **`tasmota/`**, **`rf/`**, **`rx/`** — device
  transports.
- **`llm/`** — client for the local Ollama server.
- **`solar/`** — sunrise/sunset via the NOAA solar position algorithm.
- **`lanhost/`** — validates LAN addresses before they are interpolated into
  a server-side URL.

### Platform

- **`platform/lanaddr/`** — which of this multi-homed host's addresses a
  given device can reach us at.
- **`platform/ttlcache/`** — read-through cache for answers that cost a
  round trip and change slowly.

## Conventions

### State and locking

- **All persisted state lives in `store.Store`**, behind one `RWMutex`.
  Callers wrap their work in `View`, `ViewValue`, `Update`, `UpdateOr` or
  `Mutate` (see `store/tx.go`). The closure is the unit of atomicity.
- **`Mu` stays exported for one case**: the staged device flow, which has to
  release the lock to transmit and reacquire it to record the result. Inside
  this repo that means `internal/control` and the scheduler. Everything else
  goes through the transaction helpers.
- **`ValidateX` functions** normalise and check; they are always called
  before persisting. Never skip them.
- **`Save()`** writes every JSON file atomically. Hot paths have their own
  files (`SaveSensors`, `SaveHistory`, `SaveHeard`) so that a song changing
  does not rewrite every socket in the house.
- **`CascadeDeleteSocket`** must be kept in sync with any new field that
  references a socket ID. `pruneDeadRooms` is the same promise for speakers
  and zones.

### Device I/O

- **Device I/O must never run while `Mu` is held.** One slow speaker would
  otherwise stall every other request.
- **Multi-socket fan-out** (bulk, room, group, scene, scheduler,
  automations) goes through `internal/control`: stage under the lock → send
  off-lock → apply under the lock → save. A single socket toggle transmits
  synchronously instead, so the caller can be told directly that the device
  refused.
- **All transmissions go through `store.Transmit`** — never `RF.Send`
  directly. It serialises 433 MHz sends so concurrent transmissions cannot
  overlap on air.
- **Smart-light and music commands are buffered** while the lock is held and
  drained after: `FlushLights()` and `FlushMusic()`. Call them beside each
  other, always.
- **Anything on the 5 s tick reads caches, never speakers** — `Monitor.Cached`,
  not `Monitor.Snapshot`. A rule that watches a room must cost the house no
  traffic.

### Adding a subsystem

1. Give it a package with a `Config` struct naming what it needs.
2. Take collaborators as values or narrow interfaces — never reach for a
   global, and never take `*api.Server`.
3. If it runs on its own clock, expose `Run(ctx)`; the composition root
   starts it and decides which context it rides.
4. Wire it in `internal/app`, and if the store has to call back into it, put
   the hook in `app/hooks.go` with the rest.
