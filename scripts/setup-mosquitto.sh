#!/usr/bin/env bash
# Install and configure Mosquitto as the local MQTT broker for the
# RF Socket Controller. Run this ON THE PI (it uses apt + sudo + systemctl).
#
# The controller connects to the broker at tcp://127.0.0.1:1883; the broker
# also listens on the LAN so Wi-Fi devices (Tasmota, Zigbee2MQTT, etc.) and
# sensors can publish to it.
#
# Authentication:
#   - Set MQTT_USERNAME and MQTT_PASSWORD to require a login (recommended).
#     A password file is created and anonymous access is disabled.
#   - Omit them to allow anonymous access on the LAN — convenient, but any
#     device on your network can then publish/subscribe. Trusted LANs only.
#
# If ENV_FILE points at the controller's .env, the MQTT_* settings are
# appended to it (only when MQTT_BROKER_URL is not already present), so the
# controller starts using the broker on its next restart.
#
# Usage (on the Pi):
#   MQTT_USERNAME=ctrl MQTT_PASSWORD=secret ./setup-mosquitto.sh
#   ./setup-mosquitto.sh                      # anonymous (trusted LAN)
set -euo pipefail

CONF_SRC="$(cd "$(dirname "$0")" && pwd)/mosquitto.conf"
CONF=/etc/mosquitto/conf.d/rf-socket-controller.conf
PWFILE=/etc/mosquitto/rf-socket-controller.passwd

USER="${MQTT_USERNAME:-}"
PASS="${MQTT_PASSWORD:-}"

echo "==> Installing mosquitto + clients"
sudo apt-get update -y
sudo apt-get install -y mosquitto mosquitto-clients

echo "==> Installing broker config to $CONF"
if [ -f "$CONF_SRC" ]; then
  sudo install -m 644 "$CONF_SRC" "$CONF"
else
  # Fallback when the script is run without the bundled mosquitto.conf.
  printf 'persistence true\npersistence_location /var/lib/mosquitto/\nlistener 1883\n' \
    | sudo tee "$CONF" >/dev/null
fi

if [ -n "$USER" ] && [ -n "$PASS" ]; then
  echo "==> Enabling password auth for user '$USER'"
  sudo mosquitto_passwd -b -c "$PWFILE" "$USER" "$PASS"
  sudo chown mosquitto:mosquitto "$PWFILE"
  sudo chmod 600 "$PWFILE"
  printf 'allow_anonymous false\npassword_file %s\n' "$PWFILE" | sudo tee -a "$CONF" >/dev/null
else
  echo "==> WARNING: no MQTT_USERNAME/MQTT_PASSWORD set — allowing anonymous access."
  echo "             Any device on your LAN can publish/subscribe. Trusted LANs only."
  echo 'allow_anonymous true' | sudo tee -a "$CONF" >/dev/null
fi

echo "==> Enabling + restarting mosquitto"
sudo systemctl enable mosquitto
sudo systemctl restart mosquitto

# Optionally wire the controller's .env to the local broker.
if [ -n "${ENV_FILE:-}" ] && [ -f "$ENV_FILE" ]; then
  if grep -q '^MQTT_BROKER_URL=' "$ENV_FILE"; then
    echo "==> $ENV_FILE already sets MQTT_BROKER_URL — leaving it untouched"
  else
    echo "==> Adding MQTT settings to $ENV_FILE"
    {
      echo ""
      echo "# Local Mosquitto broker (configured by setup-mosquitto.sh)"
      echo "MQTT_BROKER_URL=tcp://127.0.0.1:1883"
      if [ -n "$USER" ] && [ -n "$PASS" ]; then
        echo "MQTT_USERNAME=$USER"
        echo "MQTT_PASSWORD=$PASS"
      fi
    } >> "$ENV_FILE"
  fi
fi

echo
echo "Mosquitto is running and listening on 1883."
if [ -z "${ENV_FILE:-}" ]; then
  echo "Add these to your controller's .env, then restart it:"
  echo "  MQTT_BROKER_URL=tcp://127.0.0.1:1883"
  if [ -n "$USER" ] && [ -n "$PASS" ]; then
    echo "  MQTT_USERNAME=$USER"
    echo "  MQTT_PASSWORD=$PASS"
  fi
fi
echo "Quick test:  mosquitto_sub -h 127.0.0.1 -t 'test/#' -v ${USER:+-u $USER -P <password>} &"
echo "             mosquitto_pub -h 127.0.0.1 -t test/hello -m world ${USER:+-u $USER -P <password>}"
