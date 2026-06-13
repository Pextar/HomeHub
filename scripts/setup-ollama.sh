#!/usr/bin/env bash
# Install and configure Ollama as the local LLM backend for the assistant.
# Run this ON THE PI (it uses the official installer + sudo + systemctl).
#
# The controller talks to Ollama at http://127.0.0.1:11434. Ollama's installer
# ships its own `ollama` systemd service; this script installs a drop-in
# override so the model store lives on a large drive (the 1 TB disk) instead of
# the SD card, keeps the model warm between questions, and caps loaded models to
# protect the Pi's RAM.
#
# Environment:
#   OLLAMA_MODELS_DIR  where model blobs are stored   (default /mnt/storage/ollama-models)
#   LLM_MODEL          model to pull                  (default llama3.2:3b)
#   ENV_FILE           controller .env to wire up     (optional)
#
# Usage (on the Pi):
#   OLLAMA_MODELS_DIR=/mnt/ssd/ollama LLM_MODEL=llama3.2:3b ./setup-ollama.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MODELS_DIR="${OLLAMA_MODELS_DIR:-/mnt/storage/ollama-models}"
MODEL="${LLM_MODEL:-llama3.2:3b}"
DROPIN_SRC="$SCRIPT_DIR/ollama.service.d/override.conf"
DROPIN_DIR=/etc/systemd/system/ollama.service.d

echo "==> Ensuring model directory exists: $MODELS_DIR"
sudo mkdir -p "$MODELS_DIR"

if ! command -v ollama >/dev/null 2>&1; then
  echo "==> Installing Ollama (official script)"
  curl -fsSL https://ollama.com/install.sh | sh
else
  echo "==> Ollama already installed — skipping installer"
fi

# Ollama runs as the dedicated 'ollama' user created by the installer; make sure
# it owns the model store so it can write blobs there.
if id ollama >/dev/null 2>&1; then
  sudo chown -R ollama:ollama "$MODELS_DIR" || true
fi

echo "==> Installing systemd drop-in to $DROPIN_DIR/override.conf"
sudo mkdir -p "$DROPIN_DIR"
if [ -f "$DROPIN_SRC" ]; then
  # Substitute the model dir placeholder so the override matches MODELS_DIR.
  sudo sed "s#@OLLAMA_MODELS@#$MODELS_DIR#g" "$DROPIN_SRC" \
    | sudo tee "$DROPIN_DIR/override.conf" >/dev/null
else
  # Fallback when run without the bundled drop-in.
  sudo tee "$DROPIN_DIR/override.conf" >/dev/null <<EOF
[Service]
Environment=OLLAMA_HOST=127.0.0.1:11434
Environment=OLLAMA_MODELS=$MODELS_DIR
Environment=OLLAMA_KEEP_ALIVE=30m
Environment=OLLAMA_MAX_LOADED_MODELS=1
RequiresMountsFor=$MODELS_DIR
EOF
fi

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
echo "Ollama is running on 127.0.0.1:11434 with models under $MODELS_DIR."
echo "Model pulled: $MODEL"
echo "Restart the controller so it picks up the assistant settings:"
echo "  sudo systemctl restart rf-controller"
