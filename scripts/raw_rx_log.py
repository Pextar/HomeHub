#!/usr/bin/env python3
"""
raw_rx_log.py – Raw 433 MHz gap-timing logger.

Captures the sequence of LOW-duration gaps (in µs) after every potential
sync event (gap > SYNC_MIN_US) and prints them to stdout, one burst per line.
Use this to reverse-engineer an unknown sensor protocol or to check whether
the timing thresholds in ft007th_rx.py match what your sensor actually sends.

Usage (run while homehub service is stopped):
    RF_RX_GPIO=4 python3 raw_rx_log.py

Output format:
    SYNC gap_us  <gap1> <gap2> ... <gapN>
                 ^^^^^^^^^^^^^^^^^^^^^^^^^^^
                 gap = µs the line was LOW between two rising edges.

    For a bit-encoded sensor:
      Short gaps  → bit 0  (e.g. ~1000 µs for FT007TH/Nexus)
      Long  gaps  → bit 1  (e.g. ~2000 µs for FT007TH/Nexus)
      Very short gaps (<300 µs) are the pulse widths — usually ignored.
"""
import os
import sys
import time

try:
    import lgpio  # type: ignore[import-untyped]
except ModuleNotFoundError:
    sys.exit("lgpio not found — install with: sudo pip3 install lgpio")

_SYNC_MIN_US  = 2_500   # any LOW gap longer than this triggers a new capture
_CAPTURE_GAPS = 60      # gaps to record after each sync before printing

class _RawLogger:
    __slots__ = ("_last_fall_ns", "_sync_gap_us", "_gaps", "syncs")

    def __init__(self) -> None:
        self._last_fall_ns: int  = 0
        self._sync_gap_us:  int  = 0
        self._gaps:         list = []
        self.syncs:         int  = 0

    def on_edge(self, chip: int, gpio: int, level: int, tick_ns: int) -> None:
        if level == 0:
            self._last_fall_ns = tick_ns
            return

        gap_us = (tick_ns - self._last_fall_ns) // 1000

        if gap_us >= _SYNC_MIN_US:
            # Potential sync — flush any previous capture first
            if self._gaps:
                gaps_str = " ".join(str(g) for g in self._gaps)
                print(f"SYNC {self._sync_gap_us:6d}µs  {gaps_str}", flush=True)
            self._sync_gap_us = gap_us
            self._gaps        = []
            self.syncs       += 1
            return

        self._gaps.append(gap_us)

        if len(self._gaps) >= _CAPTURE_GAPS:
            gaps_str = " ".join(str(g) for g in self._gaps)
            print(f"SYNC {self._sync_gap_us:6d}µs  {gaps_str}", flush=True)
            self._gaps = []


def main() -> int:
    pin  = int(os.environ.get("RF_RX_GPIO", "4"))
    chip = int(os.environ.get("RF_RX_CHIP", "0"))

    h = lgpio.gpiochip_open(chip)
    cb = None
    try:
        ret = lgpio.gpio_claim_alert(h, pin, lgpio.BOTH_EDGES)
        if ret < 0:
            sys.stderr.write(f"gpio_claim_alert failed: {lgpio.error_text(ret)}\n")
            return 1

        logger = _RawLogger()
        cb = lgpio.callback(h, pin, lgpio.BOTH_EDGES, logger.on_edge)
        sys.stderr.write(
            f"# raw_rx_log: listening on gpiochip{chip} GPIO {pin}\n"
            f"# Printing {_CAPTURE_GAPS} gaps after each sync (>{_SYNC_MIN_US} µs)\n"
            f"# Hold the sensor close and wait for it to transmit (up to 60 s)\n"
            f"# Short gaps ≈ bit 0, long gaps ≈ bit 1 (for most OOK sensors)\n"
        )
        sys.stderr.flush()
        t_last = time.time()
        while True:
            time.sleep(5)
            now = time.time()
            if now - t_last >= 30:
                sys.stderr.write(f"# syncs seen so far: {logger.syncs}\n")
                sys.stderr.flush()
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
