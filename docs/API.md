# RF Socket Controller - API Documentation

## Base URL

```
http://raspberry-pi-ip:8080/api
```

## Endpoints

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

#### Delete Schedule
```
DELETE /api/schedules/{id}
```

## Protocols

Supported RF protocols:
- `nexa` - Nexa/Proove
- `kaku` - KlikAanKlikUit (KAKU)
- `intertechno` - Intertechno
- `raw` - Raw codes

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
