# HomeHub - API Documentation

## Base URL

```
http://raspberry-pi-ip:8080/api
```

## Endpoints

### Health

#### Health check
```
GET /api/health
```

Response:
```json
{
  "status": "ok",
  "sockets": 3,
  "schedules": 2,
  "time": "2026-05-09T16:02:00Z"
}
```

### Sockets

#### List All Sockets
```
GET /api/sockets
```

Response:
```json
[
  {
    "id": "socket_1234567890",
    "name": "Living Room Lamp",
    "code": "12345",
    "protocol": "nexa",
    "state": true,
    "room": "Living Room"
  }
]
```

#### Create Socket
```
POST /api/sockets
```

Request body:
```json
{
  "name": "Bedroom Light",
  "code": "54321",
  "protocol": "nexa",
  "room": "Bedroom"
}
```

#### Get Socket
```
GET /api/sockets/{id}
```

#### Update Socket
```
PUT /api/sockets/{id}
```

Request body (all fields optional):
```json
{
  "name": "Updated Name",
  "code": "99999",
  "protocol": "kaku",
  "room": "Kitchen"
}
```

#### Delete Socket
```
DELETE /api/sockets/{id}
```

#### Turn Socket On
```
POST /api/sockets/{id}/on
```

Response:
```json
{
  "id": "socket_1234567890",
  "name": "Living Room Lamp",
  "state": true
}
```

#### Turn Socket Off
```
POST /api/sockets/{id}/off
```

#### Toggle Socket
```
POST /api/sockets/{id}/toggle
```

#### Bulk: Turn All On/Off
```
POST /api/sockets/all/on
POST /api/sockets/all/off
```

Response:
```json
{
  "updated": 3,
  "failures": []
}
```

### Rooms

#### List Rooms
```
GET /api/rooms
```

Response:
```json
[
  { "name": "Living Room", "sockets": 2, "on": 1 },
  { "name": "Bedroom", "sockets": 1, "on": 0 }
]
```

#### Turn All Sockets in a Room On/Off
```
POST /api/rooms/{room}/on
POST /api/rooms/{room}/off
```

### Schedules

#### List All Schedules
```
GET /api/schedules
```

Response:
```json
[
  {
    "id": "schedule_1234567890",
    "socket_id": "socket_1234567890",
    "action": "on",
    "time": "18:00",
    "days": [1, 2, 3, 4, 5],
    "enabled": true
  }
]
```

#### Create Schedule
```
POST /api/schedules
```

Request body:
```json
{
  "socket_id": "socket_1234567890",
  "action": "on",
  "time": "18:00",
  "days": [1, 2, 3, 4, 5],
  "enabled": true
}
```

Days: 0=Sunday, 1=Monday, ..., 6=Saturday

#### Update Schedule
```
PUT /api/schedules/{id}
```

Request body (any subset of fields can be updated; `enabled` is always honored):
```json
{
  "time": "19:30",
  "enabled": false
}
```

#### Delete Schedule
```
DELETE /api/schedules/{id}
```

### Groups

A group is a curated collection of socket IDs that can be controlled together.

```
GET    /api/groups
POST   /api/groups          { "name": "...", "socket_ids": ["...", "..."] }
GET    /api/groups/{id}
PUT    /api/groups/{id}
DELETE /api/groups/{id}
POST   /api/groups/{id}/on
POST   /api/groups/{id}/off
POST   /api/groups/{id}/toggle
```

Schedules can target a group by setting `target_type` to `"group"` and
`target_id` to the group's ID.

### Scenes

A scene drives a specific set of sockets to specific states in one call.

```
GET    /api/scenes
POST   /api/scenes          { "name": "...", "actions": [{"socket_id": "...", "action": "on"}] }
GET    /api/scenes/{id}
PUT    /api/scenes/{id}
DELETE /api/scenes/{id}
POST   /api/scenes/{id}/activate
```

Schedules can target a scene by setting `target_type` to `"scene"` and
`target_id` to the scene's ID. The `action` field is then implicitly
"activate".

### Timers (one-shot)

Persistent fire-once timers — useful for "off in 30 minutes" actions.

```
GET    /api/timers
POST   /api/timers          { "target_type": "socket"|"group"|"scene",
                              "target_id":   "...",
                              "action":      "on"|"off"|"toggle",
                              "in_seconds":  900 }
DELETE /api/timers/{id}
POST   /api/sockets/{id}/timer   (convenience: target inferred from URL)
```

`fires_at` (RFC3339) may be sent instead of `in_seconds`.

### Media (zones, speakers, services)

The vendor-neutral layer. `/api/sonos/*` and `/api/kef/*` remain for
per-speaker specifics; these endpoints address any speaker uniformly and add
zones — sets of speakers that play together regardless of make. See
[MEDIA-PROTOCOL.md](MEDIA-PROTOCOL.md) for the model.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/media/endpoints` | Every speaker, with its capabilities and its zone member id |
| GET | `/api/media/providers` | Music services, availability, and whether cross-vendor streaming works |
| GET | `/api/media/search?q=&provider=&limit=` | Search a service |
| GET | `/api/media/zones` | Zones with live speaker state and the route each would take |
| POST | `/api/media/zones` | Create a zone |
| PUT | `/api/media/zones/{id}` | Rename / change membership |
| DELETE | `/api/media/zones/{id}` | Delete a zone |
| GET | `/api/media/zones/{id}/routes` | What this zone can do, and which speaker blocks what it can't |
| POST | `/api/media/zones/{id}/play` | Start content; answers with the route chosen and why |
| POST | `/api/media/zones/{id}/pause` | Pause |
| POST | `/api/media/zones/{id}/resume` | Resume |
| POST | `/api/media/zones/{id}/next` | Next track |
| POST | `/api/media/zones/{id}/previous` | Previous track |
| POST | `/api/media/zones/{id}/stop` | Stop, and release a stream session |
| PUT | `/api/media/zones/{id}/volume` | `{"level": 0-100}` across the zone |
| PUT | `/api/media/zones/{id}/mute` | `{"muted": bool}` across the zone |
| GET | `/api/media/history?room=&limit=` | What a room has been asked to play, newest first |
| DELETE | `/api/media/history?room=&uri=` | One room stops remembering one thing; without `uri`, the lot. Admin-only |
| GET | `/api/media/history/top?room=&limit=&hour=` | What a room keeps coming back to — at a given local hour with `hour=` (`0`–`23` or `now`) |
| GET | `/api/media/insights?limit=` | The household's listening summed over every room |
| GET | `/api/media/timers` | Every music timer, soonest first |
| POST | `/api/media/timers` | Create one (a wake-up: a time of day, days, and something to play) |
| PUT | `/api/media/timers/{id}` | Replace one wholesale |
| DELETE | `/api/media/timers/{id}` | Remove one, cancelling a ramp already in flight |
| POST | `/api/media/timers/sleep` | `{"room","minutes","fade_minutes","volume"}` — quiet this room in N minutes |
| POST | `/api/media/timers/fade/cancel` | `{"room"}` — stop a ramp without deleting anything |

Zone members are bridge-qualified speaker ids: `sonos:abc`, `kef:def`.

`POST /play` answers with the route it chose, so a client can be honest about
what is about to happen:

```json
{
  "route": "stream",
  "sync": "buffered",
  "reason": "Kitchen can't stream Spotify itself, so HomeHub is decoding for all of them",
  "stream_url": "http://192.168.1.10:8080/stream/9f2c…",
  "speakers": ["Living Room", "Kitchen"]
}
```

`route` is one of `native`, `connect`, `group`, `stream` — best first, with
`stream` chosen only when nothing else can serve the whole zone. `sync` is
`exact`, `single` or `buffered`; `buffered` is the honest label for the stream
route and is never reported for a zone a native route can serve.

A **409** means something the user can fix: connect an account, wake a speaker,
install librespot, or pick different speakers. The body says which.

#### Audio stream

`GET /stream/{id}` serves the decoded audio for the cross-vendor route. It sits
outside `/api` and is unauthenticated, because the clients are speakers and
speakers have no credentials — the same reason Sonos event callbacks do. The
128-bit id is minted per playback and stops resolving the moment it ends.

Environment: `HOMEHUB_STREAM_URL` overrides the address speakers fetch from,
`HOMEHUB_LIBRESPOT_BIN` / `HOMEHUB_LIBRESPOT_NAME` configure the decoder, and
`HOMEHUB_STREAM_DELAY_SONOS` / `HOMEHUB_STREAM_DELAY_KEF` (Go durations) space
out the start commands to line up buffers. All optional.

#### Play history

`GET /api/media/history?room=sonos:abc&limit=8` answers with what that room has
been asked to play, newest first and de-duplicated by URI. `room` is the
bridge-qualified destination — `sonos:<id>`, `kef:<id>`, `zone:<id>`.

```json
{
  "plays": [
    {
      "provider": "spotify",
      "kind": "album",
      "uri": "spotify:album:…",
      "title": "Kaos",
      "sub": "Bo Kaspers Orkester",
      "art_uri": "https://…",
      "room_name": "Kitchen",
      "at": "2026-08-07T18:12:04Z"
    }
  ],
  "household": false
}
```

`household` is true when the room has no history of its own and the answer is
every room's merged — surfaces must label the two differently rather than
implying a room played something it didn't. A `provider` of `sonos` is a
household favorite and is replayed through `/api/sonos/{id}/favorites/play`;
anything else goes back through the play endpoint it came from.

This is HomeHub's own memory, not Spotify's: the account's history is one list
for the whole household and cannot say what a given room plays. It is recorded
when a play succeeds, kept per room, and dropped when the speaker or zone is
deleted.

Each entry also carries `count`, `first_at` and a 24-slot `hours` histogram
(absent on entries written before those fields existed, which read as one play
and as no evidence about any hour).

`GET /api/media/history/top?room=&hour=now` ranks the same entries by how often
that room has started them, and with `hour` by how often it has started them at
that local hour. `by_hour` in the answer says which of the two it gave — a room
with no habit at that hour falls back to its favourites overall, and a surface
that labelled the fallback as a habit would be claiming evidence it doesn't
have. Unlike the plain history this never softens into the household's list.

`GET /api/media/insights` sums every room: `plays`, `items`, per-room and
per-artist tallies, the most-played items merged across rooms, a 24-slot
histogram, and `since` — the oldest play still remembered, so a surface can say
what window the numbers cover instead of implying they cover everything.

#### Music timers

Music that starts and stops without anyone tapping anything — the half the
socket scheduler can't reach, since `ExecuteAction` stops at sockets, groups,
rooms and scenes. One resource covers both uses, which differ only in which end
of the fade they are on:

```json
{
  "id": "mt_1786…",
  "room": "sonos:abc",
  "action": "start",
  "enabled": true,
  "time": "06:45",
  "days": [1, 2, 3, 4, 5],
  "item": { "provider": "spotify", "kind": "playlist", "uri": "spotify:playlist:…", "title": "Mornings" },
  "volume": 20,
  "fade_minutes": 10
}
```

`room` is the same bridge-qualified destination the play history uses. Exactly
one schedule applies: `time` + `days` repeats, `fires_at` runs once and is
deleted. `volume` is where the room ends up — omit it to leave the volume alone,
which also makes `fade_minutes` meaningless and clears it. A timer whose room is
deleted is pruned with it, the way a shelf is.

`GET /api/media/timers` adds what a surface would otherwise have to work out:
`room_name` (what the house calls that room *now*), `next_at`, and `fading` —
true while a ramp is walking that room's volume right now.

`POST /api/media/timers/sleep` is the gesture the rest exists for: `{"room":
"sonos:abc", "minutes": 40}`. The fade is the tail of the wait rather than time
added to it, so the room is quiet at forty minutes and not at forty-eight, and
the answer carries `quiet_at` — the moment worth reading back. Setting one twice
on a room replaces it. The engine restores the volume it lowered, on the
interrupted path too, and an interrupted sleep leaves the music playing:
`POST /api/media/timers/fade/cancel` is "I'm still up".

### Announcements

Calling the house: a chime, and — when a voice service is configured — the
words after it. Sent to every reachable Sonos coordinator at once, each room's
transport snapshotted before and restored after.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/announce` | Where an announcement would land, and whether it would be spoken |
| POST | `/api/announce` | `{"text": "Dinner's ready"}` — announce it |

`GET` answers `{"available": bool, "rooms": [...], "voice": bool, "max_text": 200}`.
`voice` is false when no text-to-speech endpoint is configured, in which case
every announcement is the chime alone — clients should say so rather than
taking a sentence nobody will hear.

`POST` answers **202** once every room has accepted the clip:

```json
{
  "rooms": ["Kitchen", "Living Room"],
  "unreachable": ["Bedroom"],
  "spoken": true,
  "duration_ms": 4400
}
```

The rooms are put back — transport URI, metadata, queue position, elapsed time
and group volume — `duration_ms` after the response, in the background, because
a wall panel that blocks for six seconds on a tap reads as broken. Only one
announcement runs at a time, household-wide: a second one starting mid-clip
would snapshot the clip itself as what the rooms were playing. While one is
audible, `POST` answers **409**. A room whose
snapshot could not be read is never interrupted at all: interrupting a room
that cannot be restored is the one thing this must not do. KEF speakers are
excluded for the same reason — their API cannot report what they were playing.

`GET /announce/{id}.wav` serves the clip to the speakers. Outside `/api` and
unauthenticated, exactly like the audio stream and for the same reason, guarded
by an unguessable id that expires a couple of minutes after it is minted.

Environment (all optional): `HOMEHUB_TTS_URL` points at the text-to-speech
service, with `HOMEHUB_TTS_MODEL`, `HOMEHUB_TTS_VOICE` and `HOMEHUB_TTS_KEY`
alongside it. Two request shapes are spoken, chosen by `HOMEHUB_TTS_KIND`:
`openai` (the default — `{model, voice, input, response_format: "wav"}`, which
OpenAI and most self-hosted servers take) and `piper` (`{text, voice}`, Piper's
own HTTP server, auto-detected from a URL ending in `/synthesize`). Either way
the answer must be 16-bit PCM WAV, the one format that needs no decoder here
and can be joined to the chime without resampling. With none of this set,
announcements are the chime. `homehub --check-voice "…"` verifies a service
through the same code path an announcement uses; see
[INSTALL.md](INSTALL.md).

## Error Format

All errors are returned as JSON:

```json
{ "error": "name and code are required" }
```

## Protocols

A Socket's `protocol` field selects how it's controlled. The `code` field
means different things per protocol — see below.

| protocol      | transport       | `code` field                          |
|---------------|-----------------|----------------------------------------|
| `nexa`        | 433 MHz RF      | `houseID:unit` (e.g. `12345678:0`)     |
| `kaku`        | 433 MHz RF      | numeric                                |
| `intertechno` | 433 MHz RF      | numeric                                |
| `raw`         | 433 MHz RF      | raw code                               |
| `tasmota`     | Wi-Fi (HTTP)    | device IP (e.g. `192.168.1.50`)        |
| `matter`      | Wi-Fi (matter.js)| Matter node id assigned at commissioning |
| `mqtt`        | MQTT broker     | command topic (e.g. `cmnd/plug/POWER`) |

The `matter` protocol is served via a Node.js sidecar — see
[MATTER.md](MATTER.md) for setup and the `/api/matter/...` endpoints.

### MQTT

Set `MQTT_BROKER_URL` (and optionally `MQTT_USERNAME`/`MQTT_PASSWORD`,
`MQTT_CLIENT_ID`, `MQTT_TLS_INSECURE`) to enable the MQTT codepaths. The
broker connection is shared by two features:

- **Control**: a socket with protocol `mqtt` publishes the literal payload
  `ON`/`OFF` to the topic in its `code` field (QoS 1, non-retained). This
  matches Tasmota's `cmnd/<topic>/POWER` convention and works for any device
  that takes an `ON`/`OFF` command on a topic.
- **Sensors**: a sensor with protocol `mqtt` subscribes to the topic in its
  `code` field (the `+` and `#` wildcards are allowed). Incoming payloads are
  parsed as a JSON object (read `field`, or the first numeric key when
  `field` is empty), a bare number, or an `ON`/`OFF`-style state mapped to
  `1`/`0`. Subscriptions are reconciled with the configured sensors every few
  seconds and re-established automatically after a broker reconnect.

MQTT endpoints (admin only):

| Method | Path | Description |
|--------|------|-------------|
| GET  | `/api/mqtt/status`  | `{ enabled, broker?, connected? }` |
| POST | `/api/mqtt/publish` | publish `{ topic, payload? }` (payload defaults to `ON`); used by the editor's "Send test signal" button |

## Hardware Interface

The backend attempts to use these tools in order:
1. `rpi-rf_send` - Python rpi-rf library
2. `codesend` - wiringPi
3. Simulation mode (logs only, for testing)

## Error Responses

| Status | Meaning |
|--------|---------|
| 200 | Success |
| 400 | Bad request (invalid JSON) |
| 404 | Socket not found |
| 500 | Internal error (RF transmission failed) |

## Static Files

The frontend is served from `/`:
- `index.html` - Main web interface
- Static assets from `frontend/` directory
