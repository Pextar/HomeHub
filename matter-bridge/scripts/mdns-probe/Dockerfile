# Throwaway image for the mDNS discovery probe (see run.sh). No native deps —
# multicast-dns is pure JS — so this builds fast.
FROM node:22-bookworm-slim
WORKDIR /app
RUN npm init -y >/dev/null 2>&1 && npm install multicast-dns
COPY probe.js .
CMD ["node", "probe.js"]
