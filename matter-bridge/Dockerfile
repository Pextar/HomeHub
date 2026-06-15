# matter-bridge container.
#
# Runs the matter.js sidecar in its OWN network namespace so its embedded
# mDNS responder gets a private UDP :5353, instead of fighting the host's
# avahi / otbr-agent / other responders over a single shared socket (which
# silently breaks Matter-over-Thread operational discovery — see docs/MATTER.md).
#
# BLE commissioning still works from inside the container: noble talks to the
# host's HCI adapter over raw Bluetooth sockets, which are not isolated by the
# network namespace. The container must be run with --cap-add=NET_ADMIN and
# --cap-add=NET_RAW (see deploy notes) so it can open those sockets.
FROM node:22-bookworm-slim AS build

# Native build deps for noble (BLE) — node-gyp needs python3 + a toolchain,
# and the bluetooth bindings need libudev headers.
RUN apt-get update && apt-get install -y --no-install-recommends \
        build-essential python3 libudev-dev ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY tsconfig.json ./
COPY src ./src
RUN npm run build

# --- runtime image ---
FROM node:22-bookworm-slim

# libudev is needed at runtime by the compiled noble bindings.
RUN apt-get update && apt-get install -y --no-install-recommends \
        libudev1 ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY package.json ./
# Copy the node_modules built in the toolchain stage rather than reinstalling:
# @stoprocent/noble is an OPTIONAL NATIVE dep, so a runtime-stage `npm ci`
# (no compiler) silently drops it and BLE commissioning breaks. Both stages use
# the same node:22-bookworm-slim base + arch, so the compiled bindings are valid.
COPY --from=build /app/node_modules ./node_modules
COPY --from=build /app/dist ./dist

# Fabric / credential storage — MUST be a persisted volume. Losing it means
# re-commissioning every device.
ENV MATTER_BRIDGE_DATA=/data
VOLUME ["/data"]

# Loopback JSON API the Go backend calls.
EXPOSE 8765

CMD ["node", "--enable-source-maps", "dist/index.js"]
