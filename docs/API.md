# RF Socket Controller - API Documentation

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
