# HomeHub backend architecture

This describes how the Go backend is put together and, more usefully, why.
`backend/CLAUDE.md` is the working reference — the package map and the rules
you need before editing. This is the reasoning behind them.

## The shape

```
                        cmd/homehub
                             │  flags, and which of three things to do
                             ▼
                       internal/app
        the composition root: Config, New, Run, shutdown, store hooks
                             │
        ┌────────────────────┼────────────────────┐
        ▼                    ▼                    ▼
   internal/api          services            background engines
   HTTP: routes,     control · music      scheduler · autoplay
   middleware,       assistant · audio    musictimer · listening
   sessions, SSE     announce·speakermon  reachability · rx
        └────────────────────┼────────────────────┘
                             ▼
                       internal/store
              all persisted state, one lock, JSON on disk
                             │
                             ▼
                          drivers
     sonos · kef · airplay · upnp · spotify · qobuz · matter · mqtt
     tasmota · rf · rx · llm · stream    +    media (the protocol)
```

Four properties are load-bearing:

**The arrows point one way.** A driver knows nothing about the store. The
store knows nothing about a speaker. A service knows about the store and the
drivers it needs. `internal/app` knows about everything, and nothing knows
about `internal/app`.

**`internal/api` is beside the services, not above them.** It is one caller
among several. A sleep timer firing at 23:40 and a phone tapping *pause* take
the same path to the same speaker, because both call `music.Service.Pause`.
That is the property that keeps the two from drifting into different
behaviour — which they had, once: the timer released a streamed zone's
Spotify session and a scene did not.

**Nothing but the composition root assembles anything.** Every package
declares what it needs as `Config` fields or narrow interfaces, and
`internal/app` fills them in. A package that reaches for a collaborator it
was not given is the one thing that breaks this design.

**Where the store calls back, it calls into one file.** `app/hooks.go` holds
all five hooks the store offers, each with what it is for. They are features
that fail *silently* when unwired — an uninstalled `OnMusic` means no scene
ever touches a speaker and nothing reports an error — so there is a test that
says they were wired.

## Why services and not handlers

The backend used to have one `api.Server` with 359 methods, twenty-odd
fields and eight mutexes. It was simultaneously the HTTP layer, the device
runtime (three monitors, a subprocess decoder, an AirPlay sender, a clip
host) and four background engines. `main.go` started those engines by calling
methods on the HTTP server, which meant the transport owned the lifetime of
things that had nothing to do with a request.

Three consequences, in the order they hurt:

1. **Nothing could be tested without an HTTP server.** The rule that a
   room's fade must be released even when a timer fails before starting one
   was reachable only through a request.
2. **The same logic drifted.** Two copies of the TTL cache, two switches on
   `socket.Protocol`, two ways to work out an address a speaker can reach.
3. **Ownership was invisible.** Whether the Sonos subscriptions were released
   before or after the HTTP server stopped mattered a great deal, and lived
   in the middle of a 405-line `main`.

Each service extracted since is the answer to one of those: it holds the
state that outlives a request, exposes `Run(ctx)` if it has its own clock,
and takes what it needs through `Config`.

## The rules that carry real risk

### One lock, and device I/O outside it

Every piece of persisted state is behind `store.Store`'s single `RWMutex`.
That is deliberate: an automation reading six collections and writing two has
to see one consistent house, and per-collection locks would make that a
question rather than a fact.

The cost is that the lock must never be held across a network call. One
unreachable speaker would stall every request in the process. So multi-device
actions use the staged flow in `internal/control`:

```
under the lock   →  resolve the target into a flat list of sends
off the lock     →  transmit
under the lock   →  fold the results in, log once, save
```

A single socket is the exception and transmits synchronously, because the
caller is waiting on one device and can be told directly that it refused.

Smart-light brightness and scene music follow the same shape through a
different door: both are *queued* while the lock is held and drained after,
by `FlushLights()` and `FlushMusic()`. Call them beside each other; a scene
that dims lamps and quiets a radio does both or neither.

### Caches, not speakers, on the tick

The scheduler runs every five seconds whether or not anyone has the app open.
Anything it reaches must be free. `speakermon` exists so a speaker is asked
once for the whole process however many phones are watching, and everything
on the tick reads `Monitor.Cached` rather than `Monitor.Snapshot` — the
second would turn a cold cache into a household-wide read every five seconds.

### Three answers, not two

`store.MusicPlaying` returns `(playing, known bool)`. "The living room is
quiet" and "we have no idea what the living room is doing" must not fire the
same rule: a speaker that dropped off the Wi-Fi is not a silent one. The
same discipline shows up in the media layer, where an unconfigured provider
reports *why* it cannot play rather than being omitted from the list.

### Addresses a speaker can actually reach

A home server is usually multi-homed — a Pi on Wi-Fi and Ethernet, a VPN,
Docker's bridge — and all but one of its addresses are unreachable from any
given speaker. Three subsystems need the answer (Sonos callbacks, the audio
stream, announcement clips) and all three go through
`platform/lanaddr`, which asks the kernel which route it would take. All
three use plain HTTP even when TLS is up: speakers will not accept a
self-signed certificate.

## What is deliberately *not* abstracted

**The vendor routes.** `/api/sonos/*` and `/api/kef/*` stay alongside the
vendor-neutral `/api/media/*`, because crossfade, KEF source selection and
Sonos queues do not generalise and should not be flattened into a common
shape just to have one. See `docs/MEDIA-PROTOCOL.md`.

**The store's single lock.** See above — the atomicity is the feature.

**The `api` package's one `Server` type.** Its handlers share exactly the
injected service handles, which is what they are supposed to share. Splitting
them into per-domain packages would mean each one carrying a near-copy of the
same dependency struct, and would hide the route-ordering constraint that
gorilla/mux imposes across groups.

## Testing

Each service is tested against its own contract rather than through a
request. The tests worth knowing about are the ones covering rules that
would otherwise fail silently:

| Package | The rule |
|---|---|
| `app` | every store hook is installed; `New` binds nothing |
| `control` | an empty house is a success, an empty room is a 404; one notification per bulk action |
| `musictimer` | a failed timer releases its room's fade; shutdown stops every ramp |
| `music` | a native zone is not "decoding"; an unknown room is not an empty one |
| `listening` | a skipped track is not one the room played |
| `autoplay` | a pause stays a pause; only a spent queue is picked back up |
| `audio` | a quality change rebuilds the decoder, and nothing else does |
| `speakermon` | the callback address is routable and plain HTTP |
