# RF Socket Controller

Control 433MHz RF sockets via a web / installable PWA.

## Architecture
- **Backend**: Go REST API with GPIO/RF control
- **Frontend**: Svelte 5 + Vite + vite-plugin-pwa (installable, works offline)
- **Hardware**: 433MHz transmitter on Raspberry Pi

## Features
- Turn sockets on/off remotely
- Schedule timers
- Group control
- Status monitoring

## Hardware Requirements
- Raspberry Pi (any model with GPIO)
- 433MHz RF transmitter module
- 433MHz RF sockets (common smart plugs)

## Installation
See docs/INSTALL.md

## API
See docs/API.md

