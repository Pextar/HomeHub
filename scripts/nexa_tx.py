#!/usr/bin/env python3
"""
nexa_tx.py - transmit a Nexa / Arctech "self-learning" 433 MHz frame.

Usage:
    nexa_tx.py <house_id> <unit> <on|off>

    house_id   26-bit controller id   (0 .. 67108863)
    unit        4-bit device code      (0 .. 15)
    on|off      command to send

Environment:
    NEXA_TX_GPIO     BCM pin wired to the transmitter DATA pin   (default 27)
    NEXA_TX_CHIP     /dev/gpiochipN to open                      (default 0)
    NEXA_TX_REPEAT   how many times the frame is repeated        (default 8)
    NEXA_TX_PULSE    base pulse width in microseconds            (default 260)
    NEXA_TX_LONG     long-gap multiplier (LONG = PULSE * MULT)   (default 5)

Pairing a socket: put the socket in learn mode (long-press its button
until the indicator flashes), then send any on/off command for the
house_id + unit you want to bind to it. The socket memorises that pair.

Uses the lgpio library, the modern replacement for pigpio on Pi OS
Bookworm. It talks to the kernel's character-device GPIO interface
(/dev/gpiochip0 on Pi 4) and produces microsecond-accurate waveforms,
which the Nexa encoding needs.
"""
import os
import sys
import time

import lgpio  # type: ignore[import-untyped]

T        = int(os.environ.get("NEXA_TX_PULSE", "260"))
SHORT    = T
LONG     = T * int(os.environ.get("NEXA_TX_LONG", "5"))
SYNC_LOW = T * 10
STOP_LOW = T * 40


def build_pulses(pin, house_id, unit, on, repeat):
    """Build the lgpio pulse list for `repeat` back-to-back frames.

    Each "physical pulse" is the pin high for T, then low for either a
    short (1T) or long (5T) gap. A logical bit is two physical pulses:
    logical 1 = long-then-short, logical 0 = short-then-long.
    """
    mask = 1  # group-relative bit 0, since gpio_claim_output creates a single-GPIO group
    out = []

    def hi_lo(low_us):
        # lgpio.pulse(group_bits, group_mask, micros): drive pins in
        # `mask` to the levels in `bits`, then wait `micros`.
        out.append(lgpio.pulse(mask, mask, T))
        out.append(lgpio.pulse(0, mask, low_us))

    def bit(b):
        if b:
            hi_lo(LONG)
            hi_lo(SHORT)
        else:
            hi_lo(SHORT)
            hi_lo(LONG)

    # 32-bit message, MSB first:
    #   bits 0..25  26-bit house / controller id
    #   bit 26      group flag (0 = address a single unit)
    #   bit 27      command   (1 = on, 0 = off)
    #   bits 28..31 4-bit unit / device code
    bits = [(house_id >> i) & 1 for i in range(25, -1, -1)]
    bits.append(0)
    bits.append(1 if on else 0)
    bits.extend((unit >> i) & 1 for i in range(3, -1, -1))

    for _ in range(repeat):
        hi_lo(SYNC_LOW)          # start pulse
        for b in bits:
            bit(b)
        hi_lo(STOP_LOW)          # frame separator
    return out


def main():
    if len(sys.argv) != 4:
        sys.stderr.write(__doc__)
        return 2

    try:
        house_id = int(sys.argv[1])
        unit = int(sys.argv[2])
    except ValueError:
        sys.stderr.write("house_id and unit must be integers\n")
        return 2

    cmd = sys.argv[3].strip().lower()
    if cmd not in ("on", "off"):
        sys.stderr.write("third argument must be 'on' or 'off'\n")
        return 2
    if not 0 <= house_id < (1 << 26):
        sys.stderr.write("house_id out of range (0..67108863)\n")
        return 2
    if not 0 <= unit < 16:
        sys.stderr.write("unit out of range (0..15)\n")
        return 2

    pin = int(os.environ.get("NEXA_TX_GPIO", "27"))
    chip = int(os.environ.get("NEXA_TX_CHIP", "0"))
    repeat = int(os.environ.get("NEXA_TX_REPEAT", "8"))

    h = lgpio.gpiochip_open(chip)
    try:
        lgpio.gpio_claim_output(h, pin, 0)
        lgpio.tx_wave(h, pin, build_pulses(pin, house_id, unit, cmd == "on", repeat))
        while lgpio.tx_busy(h, pin, lgpio.TX_WAVE):
            time.sleep(0.002)
        lgpio.gpio_write(h, pin, 0)
    finally:
        lgpio.gpiochip_close(h)
    return 0


if __name__ == "__main__":
    sys.exit(main())
