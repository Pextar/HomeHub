# RF Socket Controller

Control 433MHz RF sockets, Tasmota Wi-Fi devices and Matter-over-Wi-Fi
smart bulbs/plugs from a single web / installable PWA.

## Architecture
- **Backend**: Go REST API with GPIO/RF control
- **Frontend**: Svelte 5 + Vite + vite-plugin-pwa (installable, works offline)
- **Matter bridge**: Node.js sidecar (matter-bridge/) wrapping matter.js
- **Hardware**: 433MHz transmitter on Raspberry Pi

## Features
- Turn sockets on/off remotely
- Brightness, color and color-temperature for smart bulbs
- Schedule timers
- Group and scene control
- Status monitoring
- Matter commissioning (BLE → Wi-Fi onboarding)

## Hardware Requirements
- Raspberry Pi (any model with GPIO + Wi-Fi + Bluetooth)
- 433MHz RF transmitter module (for RF sockets)
- 433MHz RF sockets (optional — common smart plugs)
- Matter-over-Wi-Fi devices (optional — bulbs, plugs)

## Installation
See docs/INSTALL.md

## API
See docs/API.md and docs/MATTER.md

