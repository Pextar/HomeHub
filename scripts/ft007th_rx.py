#!/usr/bin/env python3
"""
ft007th_rx.py – Telldus FT007TH 433 MHz thermo-hygrometer receiver.

Wires a superheterodyne 433 MHz OOK receiver module directly to a Raspberry
Pi GPIO pin using lgpio, decodes the 36-bit PWM protocol of the Telldus
FT007TH / Proove AB 313160, and emits one JSON object per line to stdout —
the same format produced by `rtl_433 -F json`.

Set SENSOR_RX_CMD in .env:
    SENSOR_RX_CMD=python3 /home/pi/rf-socket-controller/scripts/ft007th_rx.py

Environment variables:
    RF_RX_GPIO   BCM pin number wired to the receiver DATA pin   (default 4)
    RF_RX_CHIP   /dev/gpiochipN to open                          (default 0)

Wiring:
    Receiver VCC  →  Pi 3.3 V  (pin 1 or 17)   ← power from 3.3 V, not 5 V
    Receiver GND  →  Pi GND    (any GND pin)
    Receiver DATA →  Pi GPIO 4 (pin 7)           ← or set RF_RX_GPIO

Protocol – 36 bits, OOK gap-based PWM (all pulses HIGH ~500 µs, gap encodes bit):
    Sync gap   > 3 500 µs  → reset accumulator
    Short gap  350 – 1 200 µs  → bit 0
    Long gap   1 200 – 2 800 µs → bit 1

36-bit packet layout (MSB = bit 35):
    [35:33]  channel   3 bits  (0 = CH1 … 7 = CH8, set by DIP switches)
    [32]     battery   1 bit   (1 = OK)
    [31:24]  device ID 8 bits  (random per sensor, stable until battery swap)
    [23:12]  temp raw  12 bits (temp_C = (raw − 400) / 10.0)
    [11:4]   humidity  8 bits  (%RH)
    [3:0]    checksum  4 bits  (XOR of all nine 4-bit nibbles = 0 when valid)
"""
import json
import os
import sys
import time

try:
    import lgpio  # type: ignore[import-untyped]
except ModuleNotFoundError:
    sys.stderr.write(
        "lgpio not found. Install it with:  sudo pip3 install lgpio\n"
        "(lgpio ships with Raspberry Pi OS Bookworm and is the recommended\n"
        "replacement for pigpio on modern Pi OS kernels.)\n"
    )
    sys.exit(1)

# ── Protocol timing thresholds (µs) ───────────────────────────────────────────

_SYNC_GAP_US  = 3_500   # gap longer than this → sync/reset
_LONG_MIN_US  = 1_200   # long gap  → bit 1
_LONG_MAX_US  = 2_800
_SHORT_MIN_US =   350   # short gap → bit 0
_SHORT_MAX_US = 1_200
_BITS         =    36

# ── Packet decoder ─────────────────────────────────────────────────────────────

def _xor_nibbles(bits_int: int) -> int:
    """XOR all nine 4-bit nibbles of the 36-bit word; valid packet → 0."""
    acc = 0
    for shift in range(32, -1, -4):
        acc ^= (bits_int >> shift) & 0xF
    return acc


def _decode(bits_int: int) -> dict | None:
    """Decode a 36-bit FT007TH packet.  Returns a dict or None on bad checksum."""
    if _xor_nibbles(bits_int) != 0:
        return None
    temp_raw = (bits_int >> 12) & 0xFFF
    return {
        "model":         "Telldus-FT007TH",
        "id":            (bits_int >> 24) & 0xFF,
        "channel":       ((bits_int >> 33) & 0x7) + 1,
        "battery_ok":    (bits_int >> 32) & 0x1,
        "temperature_C": round((temp_raw - 400) / 10.0, 1),
        "humidity":      (bits_int >>  4) & 0xFF,
    }


# ── Edge callback (called from lgpio's background thread) ─────────────────────

class _Decoder:
    """Accumulates GPIO edges and emits JSON when a complete packet arrives."""

    __slots__ = ("_last_fall_us", "_bit_buf", "_bit_count")

    def __init__(self) -> None:
        self._last_fall_us: int = 0
        self._bit_buf:      int = 0
        self._bit_count:    int = 0

    def on_edge(self, chip: int, gpio: int, level: int, tick_us: int) -> None:
        if level == 0:
            # Falling edge — start of gap; record timestamp
            self._last_fall_us = tick_us
            return

        # Rising edge — measure how long the line was LOW (the gap)
        # tick_us is µs since lgpio was initialised; 64-bit, no wrap needed
        gap_us = tick_us - self._last_fall_us

        if gap_us > _SYNC_GAP_US:
            # Sync pulse / long idle — reset accumulator
            self._bit_buf   = 0
            self._bit_count = 0
            return

        if _LONG_MIN_US <= gap_us < _LONG_MAX_US:
            self._bit_buf   = (self._bit_buf << 1) | 1
            self._bit_count += 1
        elif _SHORT_MIN_US <= gap_us < _SHORT_MAX_US:
            self._bit_buf   = self._bit_buf << 1
            self._bit_count += 1
        else:
            return  # out-of-range timing — noise, discard

        if self._bit_count == _BITS:
            result = _decode(self._bit_buf)
            if result is not None:
                # One JSON line to stdout — picked up by the backend listener
                print(json.dumps(result), flush=True)
            # Reset regardless of checksum result; next packet starts fresh
            self._bit_buf   = 0
            self._bit_count = 0


# ── Entry point ────────────────────────────────────────────────────────────────

def main() -> int:
    pin  = int(os.environ.get("RF_RX_GPIO", "4"))
    chip = int(os.environ.get("RF_RX_CHIP", "0"))

    h = lgpio.gpiochip_open(chip)
    cb = None
    try:
        # gpio_claim_input sets direction; callback() also does this, but being
        # explicit lets us specify pull configuration if ever needed.
        lgpio.gpio_claim_input(h, pin)
        decoder = _Decoder()
        cb = lgpio.callback(h, pin, lgpio.BOTH_EDGES, decoder.on_edge)
        print(
            f"# ft007th_rx: listening on gpiochip{chip} GPIO {pin} (BCM)",
            file=sys.stderr, flush=True,
        )
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        pass
    finally:
        if cb is not None:
            try:
                cb.cancel()
            except Exception:
                pass
        lgpio.gpiochip_close(h)
    return 0


if __name__ == "__main__":
    sys.exit(main())
