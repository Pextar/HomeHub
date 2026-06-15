#!/usr/bin/env bash
# Install and configure Ollama as the local LLM backend for the assistant.
# Run this ON THE PI (it uses sudo + systemctl).
#
# The controller talks to Ollama at http://127.0.0.1:11434.
#
# We deliberately do NOT use ollama.com/install.sh: it hardcodes the install
# path to /usr/local and extracts ~1.7 GB (including CUDA/ROCm GPU backends that
# are dead weight on an ARM Pi) onto the SD-card root, which is too small. It is
# also all-or-nothing and would force /usr/local/lib onto the big drive — but
# that directory also holds rpi_rf, the 433 MHz Python library on the RF control
# path (see backend/internal/rf/rf.go -> scripts/nexa_tx.py). rpi_rf must stay
# on the SD card so RF never depends on the big drive being mounted. So this
# script installs Ollama's binaries/libs onto the big drive ourselves, strips
# the unused GPU backends, leaves /usr/local/lib untouched, and writes a
# self-contained systemd unit.
#
# Environment:
#   OLLAMA_MODELS_DIR  where model blobs are stored   (default /mnt/ssd/ollama-models)
#   OLLAMA_LIB_DIR     where binaries/libs are stored (default <models-parent>/ollama-lib)
#   LLM_MODEL          model to pull                  (default qwen2.5:1.5b)
#   ENV_FILE           controller .env to wire up     (optional)
#
# Usage (on the Pi):
#   OLLAMA_MODELS_DIR=/mnt/ssd/ollama LLM_MODEL=qwen2.5:1.5b ./setup-ollama.sh
#
# NOTE: the big drive must be mounted WITHOUT `noexec` — Ollama runs its binary
# and dlopen()s its .so backends from $OLLAMA_LIB_DIR. Check with:
#   findmnt -no OPTIONS "$(df --output=target "$OLLAMA_LIB_DIR" | tail -1)"
set -euo pipefail

MODELS_DIR="${OLLAMA_MODELS_DIR:-/mnt/ssd/ollama-models}"
LIB_DIR="${OLLAMA_LIB_DIR:-$(dirname "$MODELS_DIR")/ollama-lib}"
MODEL="${LLM_MODEL:-qwen2.5:1.5b}"  # Pi default; use qwen3.5:9b-mlx on Apple Silicon

ollama_service_present() {
  [ -f /etc/systemd/system/ollama.service ]
}

echo "==> Ensuring directories exist: $LIB_DIR, $MODELS_DIR"
sudo mkdir -p "$LIB_DIR" "$MODELS_DIR"

# Dedicated service user (mirrors what the official installer creates).
if ! id ollama >/dev/null 2>&1; then
  echo "==> Creating 'ollama' service user"
  sudo useradd -r -s /bin/false -U -m -d /usr/share/ollama ollama
fi

ARCH="$(uname -m)"
case "$ARCH" in
  aarch64|arm64) ARCH=arm64 ;;
  x86_64|amd64)  ARCH=amd64 ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

# Download + extract straight onto the big drive. The archive layout is
# bin/ollama + lib/ollama/*; the binary locates its libraries relative to its
# own real path ($LIB_DIR/lib/ollama), so this relocated layout works as-is.
# Current releases ship only .tar.zst (the .tgz fallback URL 404s), so zstd is
# required to decompress.
if [ ! -x "$LIB_DIR/bin/ollama" ] || ! ollama_service_present; then
  if ! command -v zstd >/dev/null 2>&1; then
    echo "==> Installing zstd (needed to decompress the Ollama archive)"
    sudo apt-get update -qq && sudo apt-get install -y zstd
  fi
  echo "==> Downloading + extracting Ollama to $LIB_DIR (on the big drive)"
  # Clear any previous extraction so stale files don't linger across versions.
  sudo rm -rf "$LIB_DIR/bin/ollama" "$LIB_DIR/lib/ollama"
  curl --fail --show-error --location --progress-bar \
    "https://ollama.com/download/ollama-linux-${ARCH}.tar.zst" \
    | zstd -d | sudo tar -xf - -C "$LIB_DIR"

  # Reclaim space: strip GPU acceleration backends (CUDA for NVIDIA, ROCm for
  # AMD, JetPack for Jetson). A Pi has no discrete GPU and only ever loads the
  # CPU backend (libggml-cpu-armv8.*).
  echo "==> Removing unused GPU backends (CUDA/ROCm/JetPack) to reclaim space"
  sudo find "$LIB_DIR/lib/ollama" -maxdepth 1 -type d \
    \( -name 'cuda*' -o -name 'rocm*' -o -name 'jetpack*' \) \
    -exec rm -rf {} + 2>/dev/null || true
else
  echo "==> Ollama already present at $LIB_DIR — skipping download"
fi

# Remove any leftover binary/libs from a previous official-installer attempt on
# the SD card, and expose `ollama` on PATH via a symlink to the big-drive binary
# (it resolves its real path via /proc/self/exe, so libraries are still found).
sudo rm -rf /usr/local/lib/ollama
sudo ln -sf "$LIB_DIR/bin/ollama" /usr/local/bin/ollama

# The service user owns the big-drive trees (it writes model blobs at pull time).
sudo chown -R ollama:ollama "$LIB_DIR" "$MODELS_DIR" || true

echo "==> Writing systemd unit /etc/systemd/system/ollama.service"
sudo tee /etc/systemd/system/ollama.service >/dev/null <<EOF
[Unit]
Description=Ollama Service
After=network-online.target
Wants=network-online.target
# Binaries, libraries and models all live on the big drive — don't start until
# it's mounted.
RequiresMountsFor=$LIB_DIR $MODELS_DIR

[Service]
ExecStart=$LIB_DIR/bin/ollama serve
User=ollama
Group=ollama
Restart=always
RestartSec=3
# Bind to loopback only — the assistant is reached via the controller, never
# directly from the LAN.
Environment=OLLAMA_HOST=127.0.0.1:11434
# Keep model blobs off the SD card and on the big drive.
Environment=OLLAMA_MODELS=$MODELS_DIR
# Keep the model resident for 30 min so back-to-back questions skip the brutal
# cold-reload penalty on a Pi.
Environment=OLLAMA_KEEP_ALIVE=30m
# One model in RAM at a time — protects the RAM ceiling.
Environment=OLLAMA_MAX_LOADED_MODELS=1

[Install]
WantedBy=multi-user.target
EOF

echo "==> Reloading + restarting ollama"
sudo systemctl daemon-reload
sudo systemctl enable ollama
sudo systemctl restart ollama

# Give the server a moment to come up before pulling.
echo "==> Waiting for Ollama to be ready"
for _ in $(seq 1 30); do
  if curl -fsS http://127.0.0.1:11434/api/version >/dev/null 2>&1; then break; fi
  sleep 1
done

echo "==> Pulling model: $MODEL (this can be several GB on first run)"
ollama pull "$MODEL"

# Optionally wire the controller's .env to enable the assistant.
if [ -n "${ENV_FILE:-}" ] && [ -f "$ENV_FILE" ]; then
  if grep -q '^LLM_ENABLED=' "$ENV_FILE"; then
    echo "==> $ENV_FILE already sets LLM_ENABLED — leaving it untouched"
  else
    echo "==> Enabling the assistant in $ENV_FILE"
    {
      echo ""
      echo "# Local LLM assistant (configured by setup-ollama.sh)"
      echo "LLM_ENABLED=true"
      echo "OLLAMA_URL=http://127.0.0.1:11434"
      echo "LLM_MODEL=$MODEL"
      echo "LLM_TIMEOUT=120s"
    } >> "$ENV_FILE"
  fi
fi

echo
echo "Ollama is running on 127.0.0.1:11434."
echo "  binaries/libs: $LIB_DIR    models: $MODELS_DIR    model: $MODEL"
echo "Restart the controller so it picks up the assistant settings:"
echo "  sudo systemctl restart rf-controller"
