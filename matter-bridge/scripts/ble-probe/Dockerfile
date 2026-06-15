# Throwaway image for the BLE-in-netns probe (see run.sh).
#
# Uses the SAME base + native build deps as matter-bridge/Dockerfile so the
# compiled @stoprocent/noble bindings behave identically to the real bridge.
FROM node:22-bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
        build-essential python3 libudev-dev ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
# Pin to the same noble fork matter-node-ble.js depends on.
RUN npm init -y >/dev/null 2>&1 && npm install @stoprocent/noble

COPY probe.js .
CMD ["node", "probe.js"]
