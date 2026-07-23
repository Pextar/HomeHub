#!/usr/bin/env python3
"""
ft007th_rx.py – Telldus FT007TH 433 MHz thermo-hygrometer receiver.

Wires a superheterodyne 433 MHz OOK receiver module directly to a Raspberry
Pi GPIO pin using lgpio, decodes the 36-bit PWM protocol of the Telldus
FT007TH / Proove AB 313160, and emits one JSON object per line to stdout —
the same format produced by `rtl_433 -F json`.

Set SENSOR_RX_CMD in .env:
    SENSOR_RX_CMD=python3 /home/pi/homehub/scripts/ft007th_rx.py

Environment variables:
    RF_RX_GPIO   BCM pin number wired to the receiver DATA pin   (default 4)
    RF_RX_CHIP   /dev/gpiochipN to open                          (default 0)

Wiring:
    Receiver VCC  →  Pi 5 V    (pin 2 or 4)     ← MX-RM-5V needs 5 V, not 3.3 V
    Receiver GND  →  Pi GND    (any GND pin)
    Receiver DATA →  Pi GPIO 4 (pin 7)           ← or set RF_RX_GPIO

Protocol – 36 bits, OOK gap-based PWM, gap encodes bit (empirical timing):
    Sync gap   > 2 000 µs  → reset accumulator  (sensor emits ~2 873–3 385 µs)
    Short gap    50 – <threshold µs → bit 0  (~95–115 µs observed)
    Long gap   ≥ threshold µs      → bit 1

    The threshold is position-adaptive (receiver AGC effect):
      Bits  0–23 (ID / battery / channel / temperature): threshold = 150 µs
            long bits appear at ~200–600 µs here
      Bits 24–35 (humidity / checksum):                  threshold = 120 µs
            AGC compresses long bits to ~120–165 µs by this point

36-bit packet layout — Nexus/FT007TH wire format (MSB = bit 35):
    [35:28]  device ID 8 bits  (random per sensor, stable until battery swap)
    [27]     battery   1 bit   (1 = OK)
    [26]     (0)       1 bit   always 0
    [25:24]  channel   2 bits  (0 = CH1, 1 = CH2, 2 = CH3, set by DIP switches)
    [23:12]  temp raw  12 bits (signed 12-bit two's-complement; temp_C = raw / 10.0)
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

_SYNC_GAP_US  = 2_000   # gap longer than this → sync/reset
                        # FT007TH 313160 sync gap is ~2 873–3 385 µs (empirical).
                        # Must sit above LONG_MAX_US so valid long bits are not
                        # mistaken for sync events.
_LONG_MAX_US       = 1_500
_SHORT_MIN_US      =    50   # discard anything shorter (noise spikes)

# Position-adaptive long-bit threshold — the receiver AGC progressively
# compresses gaps as the packet proceeds.
#
# Bits 0–23 (ID, battery, channel, temp): threshold 150 µs.
#   Long bits appear at ~200–600 µs here; short bits ~95–120 µs.
#   This matches the threshold that reliably decoded the temperature
#   field before battery replacement.
#
# Bits 24–35 (humidity, checksum): threshold 120 µs.
#   By this point AGC has compressed long bits to ~120–165 µs while
#   short bits remain ~95–115 µs.  Gap of 121 µs was observed for a
#   bit that should have been long, confirming a lower threshold is needed.
_LONG_MIN_EARLY = 150   # threshold for bits 0–23  (ID / bat / ch / temperature)
_LONG_MIN_LATE  = 120   # threshold for bits 24–35 (humidity + checksum)
_AGC_CROSSOVER  =  24   # bit index where late threshold kicks in
_BITS         =    36

# ── Packet deduplication window ────────────────────────────────────────────────
# The FT007TH sends 3 identical repeats in ~200 ms.  We require a second
# matching packet within this window before emitting to stdout.  Noise almost
# never repeats identically, so this eliminates the vast majority of false
# positives that happen to pass the 4-bit XOR checksum.
_CONFIRM_WINDOW_NS = 1 * 10**9    # 1 second — the FT007TH sends 3 repeats in ~200 ms;
                                   # 1 s is generous enough to catch the second repeat
                                   # while stopping noise pairs seconds apart from matching.

# ── Packet decoder ─────────────────────────────────────────────────────────────

def _xor_nibbles(bits_int: int) -> int:
    """XOR all nine 4-bit nibbles of the 36-bit word; valid packet → 0."""
    acc = 0
    for shift in range(32, -1, -4):
        acc ^= (bits_int >> shift) & 0xF
    return acc


def _decode(bits_int: int) -> dict | None:
    """Decode a 36-bit FT007TH packet.  Returns a dict or None if invalid."""
    if _xor_nibbles(bits_int) != 0:
        return None
    # Nexus/FT007TH bit layout (MSB first, see module docstring):
    device_id  = (bits_int >> 28) & 0xFF
    battery_ok = (bits_int >> 27) & 0x1
    reserved   = (bits_int >> 26) & 0x1   # protocol specifies this is always 0
    channel    = ((bits_int >> 24) & 0x3) + 1
    temp_raw   = (bits_int >> 12) & 0xFFF
    hum        = (bits_int >>  4) & 0xFF
    # Nexus/FT007TH temperature is a signed 12-bit two's complement integer
    # (unit = 0.1 °C).  Sign-extend before dividing.
    if temp_raw & 0x800:
        temp_raw -= 0x1000
    temp_c     = round(temp_raw / 10.0, 1)
    sys.stderr.write(
        f"# candidate bits={bits_int:036b} "
        f"id={device_id} ch={channel} temp={temp_c} hum={hum} "
        f"bat={battery_ok} rsv={reserved}\n"
    )
    sys.stderr.flush()
    # Sanity checks.  Noise passes the 4-bit XOR checksum ~1/16 of the time;
    # these extra guards cut the false-positive rate dramatically.
    #   • device_id=0 means the opening byte is all zeros — always noise
    #   • reserved bit must be 0 per protocol; noise often sets it to 1
    #   • temperature outside [-30, 60] °C is implausible for a home sensor
    # Note: hum range check removed — AGC compression on MX-RM-5V receivers
    # raises all late-field gaps above the threshold, producing hum=255 even
    # when the sensor is transmitting a valid humidity.  Temperature is still
    # correctly decoded for temperatures where temp-nibble XOR = 0 (e.g.
    # 25.5 °C, 27.2 °C); other temperatures (e.g. 26.5 °C) cannot pass the
    # 4-bit checksum when humidity is corrupted to 0xFF.  Use a receiver with
    # a better data-slicer (RXB6, SRX882) for reliable humidity and full
    # temperature coverage.
    reject_reason = (
        "id=0"          if device_id == 0         else
        "reserved=1"    if reserved  != 0         else
        f"temp={temp_c}" if not (-30.0 <= temp_c <= 60.0) else
        None
    )
    if reject_reason:
        sys.stderr.write(f"# rejected ({reject_reason})\n")
        sys.stderr.flush()
        return None
    return {
        "model":         "Telldus-FT007TH",
        "id":            device_id,
        "channel":       channel,
        "battery_ok":    battery_ok,
        "temperature_C": temp_c,
        "humidity":      hum,
    }


# ── Edge callback (called from lgpio's background thread) ─────────────────────

class _Decoder:
    """Accumulates GPIO edges and emits JSON when a complete packet arrives."""

    __slots__ = (
        "_last_fall_ns", "_bit_buf", "_bit_count",
        "_gap_buf",
        "_confirm_key", "_confirm_ns",
        "edges", "syncs", "packets",
    )

    def __init__(self) -> None:
        self._last_fall_ns: int         = 0
        self._bit_buf:      int         = 0
        self._bit_count:    int         = 0
        self._gap_buf:      list        = []      # raw gap µs per counted bit (diagnostic)
        self._confirm_key:  tuple | None = None   # (id, ch) of last candidate
        self._confirm_ns:   int         = 0       # tick_ns when that candidate was seen
        self.edges:         int         = 0
        self.syncs:         int         = 0
        self.packets:       int         = 0

    def on_edge(self, chip: int, gpio: int, level: int, tick_ns: int) -> None:
        self.edges += 1
        if level == 0:
            # Falling edge — start of gap; record timestamp
            self._last_fall_ns = tick_ns
            return

        # Rising edge — measure how long the line was LOW (the gap).
        # lgpio tick is nanoseconds (kernel event timestamp); convert to µs.
        gap_us = (tick_ns - self._last_fall_ns) // 1000

        if gap_us > _SYNC_GAP_US:
            # Sync pulse / long idle — reset accumulator
            self.syncs     += 1
            self._bit_buf   = 0
            self._bit_count = 0
            self._gap_buf   = []
            return

        if gap_us < _SHORT_MIN_US or gap_us >= _LONG_MAX_US:
            return  # out-of-range timing — noise, discard

        # Position-adaptive threshold: early bits use 200 µs (AGC unsettled),
        # late bits use 120 µs (AGC compressed).
        threshold = (
            _LONG_MIN_EARLY if self._bit_count < _AGC_CROSSOVER
            else _LONG_MIN_LATE
        )
        if gap_us >= threshold:
            self._bit_buf   = (self._bit_buf << 1) | 1
        else:
            self._bit_buf   = self._bit_buf << 1
        self._bit_count += 1
        self._gap_buf.append(gap_us)

        if self._bit_count == _BITS:
            gaps = self._gap_buf[:]
            bits_val = self._bit_buf

            # Only print the detailed pkt diagnostic when the XOR checksum
            # passes — roughly 1/16 of accumulations, vs every single one.
            # This keeps stderr readable while still showing all candidates.
            if _xor_nibbles(bits_val) == 0:
                bstr = f"{bits_val:036b}"
                def _bg(start: int, stop: int) -> str:
                    return " ".join(f"{bstr[i]}/{gaps[i]}" for i in range(start, stop))
                sys.stderr.write(
                    f"# pkt id/bat/ch: {_bg(0, 12)}\n"
                    f"# pkt temp:      {_bg(12, 24)}\n"
                    f"# pkt hum:       {_bg(24, 32)}\n"
                    f"# pkt ck:        {_bg(32, 36)}\n"
                )
                sys.stderr.flush()
            result = _decode(bits_val)
            if result is not None:
                # Deduplication: the FT007TH sends 3 identical repeats within
                # ~200 ms.  Only emit after seeing the same packet twice within
                # _CONFIRM_WINDOW_NS.  Include temperature (rounded to 0.5 °C)
                # in the match key so that noise packets at 0.0 °C cannot
                # confirm against each other across bursts — they would need
                # identical id + channel + temperature within 1 second.
                key = (result["id"], result["channel"],
                       round(result["temperature_C"] * 2) / 2)
                if key == self._confirm_key and (tick_ns - self._confirm_ns) < _CONFIRM_WINDOW_NS:
                    # Confirmed — emit and reset so a 3rd repeat is not re-emitted
                    self.packets += 1
                    print(json.dumps(result), flush=True)
                    self._confirm_key = None
                    self._confirm_ns  = 0
                    sys.stderr.write(
                        f"# confirmed → emitted id={result['id']} "
                        f"temp={result['temperature_C']} hum={result['humidity']}\n"
                    )
                    sys.stderr.flush()
                else:
                    # First occurrence — buffer and wait for the repeat
                    self._confirm_key = key
                    self._confirm_ns  = tick_ns
                    sys.stderr.write(
                        f"# buffered (awaiting repeat) id={result['id']} "
                        f"temp={result['temperature_C']} hum={result['humidity']}\n"
                    )
                    sys.stderr.flush()
            # Reset regardless; next packet starts fresh
            self._bit_buf   = 0
            self._bit_count = 0
            self._gap_buf   = []


# ── Entry point ────────────────────────────────────────────────────────────────

def main() -> int:
    pin  = int(os.environ.get("RF_RX_GPIO", "4"))
    chip = int(os.environ.get("RF_RX_CHIP", "0"))

    h = lgpio.gpiochip_open(chip)
    cb = None
    try:
        # gpio_claim_alert enables edge-triggered interrupt alerts — required for
        # callbacks to fire.  gpio_claim_input alone does not enable interrupts
        # on this lgpio version (0.2.x).
        ret = lgpio.gpio_claim_alert(h, pin, lgpio.BOTH_EDGES)
        if ret < 0:
            print(f"# ft007th_rx: gpio_claim_alert failed: {lgpio.error_text(ret)}",
                  file=sys.stderr, flush=True)
            return 1
        decoder = _Decoder()
        cb = lgpio.callback(h, pin, lgpio.BOTH_EDGES, decoder.on_edge)
        print(
            f"# ft007th_rx: listening on gpiochip{chip} GPIO {pin} (BCM)",
            file=sys.stderr, flush=True,
        )
        t_last = time.time()
        while True:
            time.sleep(10)
            now = time.time()
            if now - t_last >= 30:
                print(
                    f"# ft007th_rx: edges={decoder.edges} "
                    f"syncs={decoder.syncs} packets={decoder.packets}",
                    file=sys.stderr, flush=True,
                )
                t_last = now
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
