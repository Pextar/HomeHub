# Media Protocol

A vendor-neutral layer for playing any music service on any speaker HomeHub
knows about — including the case the vendors themselves refuse: **one service
playing on a KEF and a Sonos at the same time**.

This document is the contract. `backend/internal/media/` implements it.

---

## Why this exists

HomeHub grew two speaker bridges independently, and it shows:

```
internal/sonos/   UPnP/SOAP over the LAN     ~4600 lines   queue, favorites, topology, GENA events
internal/kef/     HTTP JSON over the LAN     ~2600 lines   transport, source, volume — no content
internal/spotify/ Spotify Web API (PKCE)     ~800 lines    search + Connect playback
```

Every capability is reachable, but only through a bridge-shaped door. The Music
view holds two of everything, `/api/sonos/{id}/play-item` and
`/api/kef/{id}/play-item` take the same body and share no code, and there is no
noun in the system that means "these speakers, playing this, together."

The protocol adds that noun and puts the bridges behind one interface.

### The constraint that shapes everything

Roon can drive a KEF and a Sonos together because **Roon Core is the single
decoder**. It holds the decoded audio and clock-syncs it out to both endpoints —
RAAT to the KEF, Sonos' own streaming protocol to the Sonos. One source of
truth for the samples, one clock, two outputs.

Spotify offers no such thing:

- There is no API that returns raw audio. The catalog is only ever decoded by
  a Spotify-licensed client.
- **A Spotify account has exactly one active playback session.** Starting
  Connect playback on the KEF stops the Sonos, and vice versa. This is not a
  quality-of-sync problem that better engineering fixes. It is a hard no.

So "start it natively on both at once" is not an implementation we skipped. It
is a thing that cannot exist. Any honest cross-vendor path needs HomeHub to
become the single decoder — exactly the role Roon Core plays.

---

## The model

Four nouns. Everything else is machinery.

```
Provider ──resolves──> Content ──played on──> Zone ──via──> Route ──drives──> Endpoint(s)
(service)              (a thing)              (speakers)    (how)            (a speaker)
```

### Endpoint

One speaker, addressed uniformly. Backed by an adapter over an existing bridge —
adapters translate, they do not add behaviour the hardware lacks.

```go
type Endpoint interface {
    ID() string
    Descriptor() Descriptor        // name, room, vendor, model, capabilities
    State(ctx) (*NowPlaying, error)

    Play(ctx) error
    Pause(ctx) error
    Next(ctx) error
    Previous(ctx) error
    SetVolume(ctx, level int) error   // 0-100, normalised
    SetMute(ctx, muted bool) error
}
```

Optional behaviour is an optional interface, never a method that returns
`ErrUnsupported` — a caller should be able to ask before it commits:

```go
type Seeker      interface { Seek(ctx, pos time.Duration) error }
type Queuer      interface { Queue(ctx) ([]QueueItem, error); Enqueue(...) error }
type Grouper     interface { Join(ctx, coordinator Endpoint) error; Leave(ctx) error }
type URIPlayer   interface { PlayURI(ctx, uri string, meta Metadata) error }
type Waker       interface { Wake(ctx) error }
```

`URIPlayer` is the load-bearing one: it is what makes the stream route possible,
and both vendors have it (Sonos `SetAVTransportURI`, KEF over UPnP AVTransport).

### Capabilities

A bitset on `Descriptor`, so the route engine and the UI can both reason about a
speaker without knowing its vendor.

| Capability | Sonos | KEF | Meaning |
|---|:---:|:---:|---|
| `CapTransport` | ● | ● | play / pause / next / previous |
| `CapVolume` | ● | ● | 0-100 volume, mute |
| `CapSeek` | ● | ○ | seek within a track |
| `CapQueue` | ● | ○ | inspect and mutate a queue |
| `CapGroup` | ● | ○ | native multi-speaker grouping |
| `CapPlayURI` | ● | ● | be handed an arbitrary stream URL |
| `CapNativeService` | ● | ○ | stream a service itself, from its own account link |
| `CapConnect` | ○ | ● | be targeted by Spotify Connect |
| `CapWake` | ○ | ● | be woken from standby / switched to network input |

● supported ○ not supported

Two asymmetries drive most of the design. Sonos can stream a service from its
own linked account and group natively; KEF cannot do either, and has to be
woken before it exists on the network at all.

### Provider

A music service. Search and browse are uniform; *how content reaches a speaker*
is not, so a provider declares the routes it can serve rather than exposing a
single `Play`.

```go
type Provider interface {
    ID() string                   // "spotify"
    Name() string
    Available() (bool, string)    // configured + connected, with a reason when not
    Search(ctx, query string, limit int) (*Results, error)
    Browse(ctx) ([]Item, error)   // playlists / favorites, no typing
    Routes() RouteSet             // which of the routes below this service supports
}
```

Route-specific behaviour is, again, an optional interface:

```go
// Served natively by the speaker's own account link (Sonos + Spotify/Tidal/…).
type NativeProvider  interface { NativeItem(uri, title string, acct Account) (URI, Metadata, error) }
// Served by pointing the service's own cloud at a device (Spotify Connect).
type ConnectProvider interface { ConnectDevices(ctx) ([]Device, error); PlayOn(ctx, deviceID, uri string) error }
// Served by HomeHub decoding it and re-serving it (the cross-vendor path).
type StreamProvider  interface { OpenStream(ctx, uri string) (Stream, error) }
```

### Zone

A named set of endpoints that play together. This is the new noun, and it is
persisted (`store.Zone`) because it is a user's arrangement of their house, not
a transient.

```go
type Zone struct {
    ID        string
    Name      string          // "Downstairs"
    Endpoints []string        // endpoint IDs, any mix of vendors
    Room      string          // optional, ties into existing rooms
}
```

A zone of one is legal and is the common case — it is how a single speaker is
addressed, so there is exactly one playback path in the system rather than a
single-speaker path plus a multi-speaker special case.

---

## Routes

A route is *how* content gets from a provider to a zone. This is where the
protocol earns its keep: it picks the best available path per playback, so
callers say what they want and never encode vendor knowledge.

### `native` — the speaker streams it itself

Content is handed to the speaker as a service URI plus DIDL metadata; the
speaker streams it from its own linked account. HomeHub's command never leaves
the LAN.

- **Applies when** every endpoint in the zone has `CapNativeService`, shares a
  coordinator, and the provider implements `NativeProvider`.
- **Sync**: perfect. Sonos' own multi-room clock.
- **Quality**: full service bitrate, gapless, full metadata, and the Sonos app
  shows it as normal music.
- **Today**: this is `sonos.PlayServiceItem` + `sonos.SpotifyItem`, unchanged.

### `connect` — the service's cloud targets the speaker

Content is started by asking the provider's cloud to play on a device that *is*
the speaker.

- **Applies when** the zone is exactly one endpoint with `CapConnect`, and the
  provider implements `ConnectProvider`.
- **Sync**: n/a, single speaker.
- **Quality**: full service bitrate and metadata.
- **Cost**: needs the *user's* account (Premium + player scopes), a round trip
  to the cloud, and the speaker awake first — hence `CapWake`.
- **Today**: this is `internal/api/kef_spotify.go`, unchanged.

### `group` — native grouping, then one of the above

Endpoints that can group natively are grouped, and the coordinator takes a
`native` or `connect` play.

- **Applies when** every endpoint has `CapGroup` and the same vendor.
- **Sync**: perfect.
- **Today**: Sonos-to-Sonos. `sonos.Join` / `sonos.Leave` already exist; the
  route engine just drives them.

### `stream` — HomeHub is the decoder

The cross-vendor path, and the only one that answers the original question.
HomeHub registers itself as a Spotify Connect receiver, decodes the audio once,
and re-serves it over plain HTTP. Every endpoint in the zone is pointed at that
URL with `PlayURI`.

```
Spotify cloud
     │ Connect session ("HomeHub Multiroom" — one active device, as the rules require)
     ▼
librespot ──PCM──> internal/stream ──HTTP──┬──> Sonos  (SetAVTransportURI)
                    encode + fan-out       └──> KEF    (UPnP AVTransport)
```

- **Applies when** every endpoint has `CapPlayURI` and the provider implements
  `StreamProvider`. It is the fallback, never the first choice.
- **Sync**: buffer-level, not sample-level. Each speaker has its own jitter
  buffer and starts when it has filled it. Measured skew is a few hundred ms;
  the transport applies a per-vendor start offset to compensate, and it is
  stable once playing because both are pulling from the same source at the same
  rate. **This is not Roon-grade sync and the doc must not imply it is.**
- **Quality**: transcoded once (librespot decodes Ogg/Vorbis → PCM → we serve
  FLAC where the endpoint accepts it, MP3 otherwise). No gapless across tracks.
- **Metadata**: pushed as ICY headers and DIDL, so the speaker shows title and
  artist. It will still present as a stream, because it is one.
- **Dependency**: a `librespot` binary on the host, and Spotify Premium. When
  absent the route reports unavailable with a reason — everything else keeps
  working.

### Route selection

The rule, in one line: **the best route that can serve the entire zone wins,
and `stream` is always last.**

```
for route in [native, connect, group, stream]:
    if route.supports(provider) and route.canServe(zone):
        return route
return error explaining which endpoint disqualified which route
```

This ordering is the guarantee that adding cross-vendor playback does not
regress anything:

| Zone | Route chosen | Change vs. today |
|---|---|---|
| One Sonos | `native` | none — same SOAP calls |
| Several Sonos | `group` → `native` | none — grouping already existed |
| One KEF | `connect` | none — same Web API calls |
| KEF + Sonos | `stream` | **new**; impossible before |

A Sonos-only listener never reaches the stream route, so librespot is never
started, no transcode happens, and the Sonos app behaves exactly as it does
now. The stream route is strictly additive: it only claims the case that
previously returned an error.

Failure is explained per endpoint rather than as one flat "unsupported" — "the
KEF can't stream Tidal natively, and Tidal has no Connect" is actionable;
"unsupported" is not.

---

## Wire format

One set of endpoints replaces the two vendor-shaped sets. The existing
`/api/sonos/*` and `/api/kef/*` routes stay — they are what the per-speaker
detail views use, and they expose vendor specifics (crossfade, KEF source
selection) that do not generalise.

```
GET    /api/media/endpoints            every speaker, uniform shape + capabilities
GET    /api/media/providers            services, availability + reason
GET    /api/media/zones                zones with live state
POST   /api/media/zones                create
PUT    /api/media/zones/{id}           rename / change membership
DELETE /api/media/zones/{id}

POST   /api/media/zones/{id}/play      {provider, uri, title} → resolves route, plays
POST   /api/media/zones/{id}/pause     transport, fanned out
POST   /api/media/zones/{id}/next
POST   /api/media/zones/{id}/previous
PUT    /api/media/zones/{id}/volume    {level} relative or absolute across members
GET    /api/media/zones/{id}/routes    which routes this zone can serve, and why not

GET    /api/media/search?q=&provider=  federated across available providers
```

`POST /zones/{id}/play` answers with the route it chose and the reason, so the
UI can be honest about what is about to happen:

```json
{
  "route": "stream",
  "reason": "Kitchen (KEF LSX II) can't stream Spotify natively — HomeHub is decoding for both",
  "sync": "buffered",
  "endpoints": ["sonos-living", "kef-kitchen"]
}
```

---

## Locking and I/O

The store rules in `CLAUDE.md` apply unchanged, and the zone work adds one:

- Zones live in `store.Store` under `Mu`, validated by `ValidateZone`,
  persisted by `Save()`.
- **Route resolution reads the store under `Mu`; route execution never holds
  it.** Playing to a zone is multi-speaker device I/O — the exact thing
  `store/staged.go` exists to keep off-lock. Zone playback stages the plan under
  the lock, executes it off-lock, applies results under the lock.
- `CascadeDeleteSocket` has a sibling: deleting a speaker must drop it from
  every zone.
- The stream transport owns its own goroutines and never calls into the store.

## Testing

The protocol is interface-first precisely so the interesting parts are testable
without hardware:

- **Route selection** is a pure function of (zone capabilities, provider
  routes). Table-driven, no I/O — this is the logic that decides whether a tap
  plays in the right room, and it gets the most tests.
- **Adapters** are tested against the existing bridge test doubles.
- **The stream transport** is tested with a fake decoder writing a known PCM
  pattern, asserting fan-out and per-listener buffering rather than audio.

---

## What this does not do

Stated plainly so the next reader does not have to discover it:

- **It is not Roon.** No sample-accurate sync, no DSP, no room correction. For
  KEF + Sonos together with bit-perfect sync, Roon remains the right tool — it
  just cannot carry Spotify.
- **It does not make Spotify multi-session.** The stream route works by having
  exactly one Spotify session (HomeHub's) and fanning the audio out afterwards.
  Starting Spotify elsewhere still takes the session away.
- **It does not re-implement the vendor apps.** Sonos-specific features stay on
  the Sonos endpoints.
- **AirPlay 2** would give better sync than the stream route and both vendors
  support it, but implementing a sender (pairing, PTP clock sync, encrypted
  ALAC) is a large piece of work. The `Route` abstraction is shaped so it can be
  added as a fifth route, ranked above `stream`, without touching callers.
