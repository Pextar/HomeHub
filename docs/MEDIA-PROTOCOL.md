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
internal/airplay/ RAOP (RTSP + RTP) sender   ~2400 lines   mDNS discovery, and pushing audio at receivers
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
type AirPlayTarget interface { AirPlayDest() AirPlayDest }   // where to push, and what it takes
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

| Capability | Sonos | KEF | AirPlay | Meaning |
|---|:---:|:---:|:---:|---|
| `CapTransport` | ● | ● | ◐ | play / pause / next / previous |
| `CapVolume` | ● | ● | ● | 0-100 volume, mute |
| `CapSeek` | ● | ○ | ○ | seek within a track |
| `CapQueue` | ● | ○ | ○ | inspect and mutate a queue |
| `CapGroup` | ● | ○ | ○ | native multi-speaker grouping |
| `CapPlayURI` | ● | ● | ○ | be handed an arbitrary stream URL |
| `CapNativeService` | ● | ○ | ○ | stream a service itself, from its own account link |
| `CapConnect` | ○ | ● | ○ | be targeted by Spotify Connect |
| `CapWake` | ○ | ● | ○ | be woken from standby / switched to network input |
| `CapAirPlay` | ○ | ○ | ● | be *pushed* audio, with HomeHub keeping the clock |

● supported ○ not supported ◐ play and pause only

Three asymmetries drive the design. Sonos can stream a service from its own
linked account and group natively; KEF cannot do either, and has to be woken
before it exists on the network at all. An AirPlay receiver can do none of it:
it is a **sink**. It holds no content, no queue and no account, cannot be
handed a URL to fetch, and has no state of its own to report — what it is
playing is whatever HomeHub is sending it. That inversion is why it needs a
route of its own rather than a variation on the stream route.

`CapTransport` is half-claimed for AirPlay, and the half is documented rather
than papered over: play and pause genuinely work (pause means stop sending and
flush what is buffered), while next and previous return an error saying that
skipping is the music service's job. Dropping the capability outright would
have taken play and pause with it.

`CapAirPlay` is declared **only** for receivers registered through
`internal/airplay`. Plenty of Sonos and KEF speakers also answer AirPlay, and
this deliberately does not claim it for them: they have better routes of their
own, and a capability that lies is worse than one that is absent.

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

### `airplay` — HomeHub is the decoder, and the clock

The same decode as `stream`, pushed instead of fetched. HomeHub opens a RAOP
session with each receiver, sends 44.1 kHz 16-bit samples as RTP packets, and
answers the timing questions every receiver asks — so all of them are placing
the same sample at the same moment on one clock.

```
Spotify cloud
     │ Connect session ("HomeHub Multiroom")
     ▼
librespot ──PCM──> internal/airplay ──RTP/UDP──┬──> RoPieee  (shairport-sync)
                    one clock, one decode      └──> RoPieee  (shairport-sync)
```

- **Applies when** every endpoint has `CapAirPlay` and the provider implements
  `StreamProvider`. Ranked above `stream` because it is the same decode with a
  real clock on the end of it.
- **Speaks** classic AirPlay (AirPlay 1, RAOP). That is a statement about the
  session HomeHub *opens*, and it is worth separating from which receivers it
  can reach, because the obvious reading is wrong. A receiver being "AirPlay 2"
  does not put it out of reach: shairport-sync in AirPlay 2 mode — what a
  current RoPieee runs — keeps answering classic senders on the same port.
  AirPlay 2 changes what an iPhone chooses to speak to a box, not what the box
  accepts. What is genuinely out of reach is a receiver that *requires* the
  AirPlay 2 handshake, i.e. Apple's own speakers, where HomeKit pairing comes
  first.
- **Asks rather than assumes.** The mDNS advertisement describes what a
  receiver prefers, which on an AirPlay 2 box is a different question from what
  it will accept — so a scan asks each one (RTSP `OPTIONS`, which is stateless
  and takes nothing away from whatever is playing) and records the answer. A
  session then offers each shape the device might take — advertised codec in
  the clear, then with a key, then ALAC either way — and only the receiver's
  own refusal closes the door. This is the same shape as the KEF scan, where
  SSDP narrows the subnet down and the API probe settles it.
- **Quality**: bit-exact. Raw PCM when the receiver advertises it (`cn=0`),
  uncompressed ALAC frames when it does not — ALAC's verbatim escape hatch, so
  no encoder dependency, the same trade `stream` makes by serving WAV. Nothing
  re-encodes and nothing resamples; a receiver wanting a rate AirPlay 1 does
  not carry is refused rather than resampled for.
- **Encryption**: cleartext when the receiver allows it (`et=0`), AES-128-CBC
  under Apple's published RSA key when it doesn't. Preferring cleartext is a
  considered choice: the key is in every open-source sender, it protects
  nothing, and skipping it removes a per-packet AES pass on the user's own LAN.
- **Sync**: `clocked`. Each receiver measures its offset against HomeHub's
  clock and is told which RTP timestamp belongs at which moment, so they start
  together and are corrected rather than merely stable. **This is better than
  `buffered` and it is not a vendor's own multi-room bus** — the clock is
  HomeHub's, disciplined over UDP on a home network. The doc must not imply
  more.
- **Latency**: two seconds, which is what AirPlay has always used and what
  receivers size their buffers for. It is what absorbs a Wi-Fi hiccup without a
  dropout, and it is why play takes a moment to be heard.
- **Loss**: receivers ask for missed packets by sequence number and the sender
  answers from a backlog of the last ~1024 packets (about eight seconds).
- **Metadata**: track title, artist and album as DAAP, so a RoPieee's display
  fills in. Artwork is not sent — see `internal/airplay/daap.go` on why.

### `stream` — HomeHub is the decoder

The cross-vendor path, and the only one that answers the original question.
HomeHub registers itself as a Spotify Connect receiver, decodes the audio once,
and re-serves it over plain HTTP. Every endpoint in the zone is pointed at that
URL with `PlayURI`.

```
Spotify cloud
     │ Connect session ("HomeHub Multiroom" — one active device, as the rules require)
     ▼
librespot ──PCM──> internal/stream ──HTTP (WAV)──┬──> Sonos  (SetAVTransportURI)
                    frame + fan-out              └──> KEF    (UPnP AVTransport)
```

While a mixed zone plays, **HomeHub is the account's active Spotify device**.
Starting Spotify on a phone takes the session away and the zone stops — the
same single-session rule that makes this route necessary, seen from the other
side.

The stream URL carries a fresh 128-bit random id per playback and is served
without the admin session middleware, because the clients are speakers and
speakers have no cookies. The id is the capability: unguessable, and invalid
the moment playback stops.

- **Applies when** every endpoint has `CapPlayURI` and the provider implements
  `StreamProvider`. It is the fallback, never the first choice.
- **Sync**: buffer-level, not sample-level. Each speaker has its own jitter
  buffer and starts when it has filled it, so they land within a few hundred
  milliseconds of each other and stay there — both are pulling from one source
  at the same rate, so they don't drift apart once running. The transport can
  space out the start commands per vendor (`Config.StartDelays`) to close some
  of that gap. **It ships with no delays configured**: the right values depend
  on the speakers, the network and the firmware, and inventing numbers nobody
  measured would be worse than leaving them at zero for someone who can
  actually hear the result to tune. **This is not Roon-grade sync and this doc
  must not imply it is.**
- **Quality**: no re-encode. librespot decodes to PCM and HomeHub serves that
  PCM as WAV, so the only lossy step is the one Spotify already performed.
  A 44-byte header is the entire conversion, which means no dependency on lame
  or ffmpeg and no second transcode of already-lossy audio. It costs 1.4 Mbit/s
  per stream, which is nothing on a LAN and is why this trade is worth making.
  No gapless across tracks.
- **Metadata**: pushed as ICY headers and DIDL, so the speaker shows title and
  artist. It will still present as a stream, because it is one.
- **Dependency**: a `librespot` binary on the host, and Spotify Premium. When
  absent the route reports unavailable with a reason — everything else keeps
  working.

### Route selection

The rule, in one line: **the best route that can serve the entire zone wins,
and `stream` is always last.**

```
for route in [native, connect, group, airplay, stream]:
    if route.supports(provider) and route.canServe(zone):
        return route
return error explaining which endpoint disqualified which route
```

This ordering is the guarantee that adding cross-vendor playback does not
regress anything:

| Zone | Route chosen | Change vs. before |
|---|---|---|
| One Sonos | `native` | none — same SOAP calls |
| Several Sonos | `group` → `native` | none — grouping already existed |
| One KEF | `connect` | none — same Web API calls |
| KEF + Sonos | `stream` | the cross-vendor case |
| One RoPieee | `airplay` | **new** |
| Several RoPieees | `airplay` | **new**, on one clock |
| RoPieee + Sonos | *nothing* | see below |

The last row is the honest gap. A Sonos fetches and a receiver is pushed to;
no route does both, so a zone mixing them fails with both halves named — "the
Sonos can't be sent an AirPlay stream, the RoPieee can't play a stream URL".
Serving it would mean teeing one decode into both an HTTP host and a cast,
which is buildable and is not built: the two halves would be a clock apart
anyway, which is most of the reason to use AirPlay in the first place.

A Sonos-only listener never reaches the stream route, so librespot is never
started, no transcode happens, and the Sonos app behaves exactly as it does
now. The stream route is strictly additive: it only claims the case that
previously returned an error.

Failure is explained per endpoint rather than as one flat "unsupported" — "the
KEF can't stream Tidal natively, and Tidal has no Connect" is actionable;
"unsupported" is not.

---

## Sound quality

"Am I hearing lossless?" has no single answer, because the audio passes through
two hands: the service that encoded it, and the path it took to the speaker.
A lossless path carrying a lossy source is not lossless. So quality is reported
as a **chain with the weakest link named**, never as one badge — a badge would
have to pick something to lie about.

```go
type Chain struct {
    Source    Stage   // what the service hands over
    Transport Stage   // what the route does to it
    Lossless  bool    // both, or neither
    LimitedBy string  // the stage that caps it
    Summary   string  // the whole thing in a sentence
    Fix       *Fix    // the change that would improve it, or nil
}
```

**Transport, per route.** None of the five re-encodes. `native`, `group` and
`connect` add nothing because the speaker fetches the service's own stream;
`stream` serves the decoded PCM with a 44-byte header on it; `airplay` packs
the same samples into RTP. Every route in this system is a lossless carrier,
which is exactly why the source is where the answer usually comes from.

**Source, per provider.** Spotify's catalogue is Ogg Vorbis — lossy at the
source, on every route, forever. What differs is how well HomeHub knows the
number: on `stream` and `airplay` the bitrate is HomeHub's own decoder setting
and is known exactly, while on the routes the speaker serves for itself the
speaker negotiates with the service and never says what it settled on. Those
are marked `approximate` and shown as "up to" rather than printed as a
measurement.

**The one lever.** `Settings.StreamQuality` — `best` (320 kbps), `balanced`
(160), `saver` (96) — is how hard HomeHub's own decoder asks the service to
compress. It is household-wide because the decoder is a single process holding
a single service session, so two zones cannot be decoded at two bitrates at
once and a per-zone control would promise what the architecture cannot keep.
It moves `stream` and `airplay` and nothing else, and `Fix` is offered only on
those routes and only when the setting is below `best`.

What this must never do is offer to make Spotify lossless. It cannot be done,
and a control implying otherwise is worse than no control — so where the source
is the limit and the setting is already at its top, the answer is a sentence
explaining why, and no button.

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
GET    /api/media/quality              what the audio is, per route, + the decode setting
PUT    /api/media/quality              {stream_quality} — best | balanced | saver
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

## The Connect picker is not a route

`/api/spotify/connect` (see API.md) sits beside this protocol rather than
inside it, and the distinction is worth keeping sharp.

Everything in this document is about **speakers as the subject**: a zone is a
set of them, a route is how content reaches them, and HomeHub decides. The
Connect picker's subject is **the account's single playback session** — where
Spotify is playing right now, which may be a phone on a bus. It is a remote
control for something HomeHub does not own.

They meet at exactly one point, and it is the same single-session rule that
shapes `stream` and `airplay`: while HomeHub decodes for a room, HomeHub *is*
the account's active device. Moving the session from the picker therefore stops
that room, which is why the read names it before the tap and why a successful
transfer releases those zone sessions. No route was involved in either
direction — the audio never passed through this layer.

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
- **It does not implement the AirPlay 2 handshake.** The `airplay` route opens
  a classic session, which shairport-sync receivers accept whichever mode they
  are running in. What is missing is HomeKit pairing, the PTP clock and
  buffered mode — so a receiver that *insists* on AirPlay 2, meaning Apple's
  own speakers, is refused with that named as the reason. Sonos and KEF
  speakers' own AirPlay support is not used either; they keep their native
  routes, which are better for them anyway.
- **It cannot mix pushed and fetched speakers in one zone.** See the route
  table above.
