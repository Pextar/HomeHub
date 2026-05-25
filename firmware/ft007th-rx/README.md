# ft007th-rx — nRF52840 USB bridge for Telldus FT007TH

> **Easier path first:** connecting the 433 MHz receiver directly to the Raspberry
> Pi GPIO and running `scripts/ft007th_rx.py` requires zero firmware development
> and no soldering.  See [docs/INSTALL.md](../../docs/INSTALL.md).
>
> Use this firmware only if you specifically want the nRF52840 Dongle to act as
> a dedicated USB receiver (e.g. the Pi's GPIO pins are all spoken for).

---

## What it does

Reads OOK pulses from an external 433 MHz superheterodyne receiver module wired
to a GPIO pin, decodes the Telldus FT007TH 36-bit protocol in real time, and
prints one JSON line per valid packet to a USB CDC ACM serial port:

```json
{"model":"Telldus-FT007TH","id":42,"channel":1,"battery_ok":1,"temperature_C":21.3,"humidity":65}
```

The Raspberry Pi backend reads this port and treats it identically to
`rtl_433 -F json` output.

---

## Hardware

### What you need

| Part | Notes |
|------|-------|
| nRF52840 Dongle (PCA10059) | The USB-stick form factor |
| Superheterodyne 433 MHz OOK receiver | The module in the photo — VCC, GND, DATA |
| Thin wire (~28 AWG) + soldering iron | For tapping TP8 on the Dongle |

### The Dongle's exposed GPIO

The Dongle has almost no user-accessible GPIO.  The only practical data pin
without opening/modifying the board is **TP8 = P0.29** — a small test pad on
the **bottom** of the PCB.

```
Bottom of nRF52840 Dongle PCA10059:
┌─────────────────────────────────────┐
│  [USB connector]                    │
│                                     │
│  TP8 (P0.29)  ←── RF receiver DATA │
│  GND          ←── RF receiver GND  │
│  VDD (3.3 V)  ←── RF receiver VCC  │
└─────────────────────────────────────┘
```

> ⚠️  Power the receiver from the Dongle's **3.3 V** VDD, not 5 V.
> Most superheterodyne modules work at 3.3 V; their DATA output will then be
> 3.3 V logic — safe for the nRF52840's GPIO (max 3.6 V).
> If you power from 5 V you need a voltage divider on the DATA line.

---

## Build & flash

### Prerequisites

Install the nRF Connect SDK (NCS) ≥ 2.6 and `west`:

```bash
# Follow https://docs.nordicsemi.com/bundle/ncs-latest/page/nrf/installation.html
pip3 install west
west init ~/ncs --mr v2.7.0
cd ~/ncs && west update
west zephyr-export
```

### Build

```bash
cd firmware/ft007th-rx
west build -b nrf52840dongle/nrf52840
```

### Flash (Dongle)

The Dongle uses a USB bootloader; no J-Link needed.

1. Press the **RESET** button while inserting the Dongle — the red LED pulses.
2. The Dongle appears as a USB DFU device.

```bash
west flash --runner nrfjprog   # if you have a J-Link connected
# --- OR ---
nrfutil device program --firmware build/zephyr/zephyr.hex  # nrfutil ≥ 7
```

Alternatively, use **nRF Connect for Desktop → Programmer** (drag-and-drop the
`build/zephyr/zephyr.hex` file).

---

## Backend configuration

After flashing, plug the Dongle into the Raspberry Pi and add to `.env`:

```dotenv
SENSOR_SERIAL_PORT=/dev/ttyACM0
```

The Dongle enumerates as `/dev/ttyACM0` (or `ttyACM1` if another CDC device is
already present).  Confirm with:

```bash
ls /dev/ttyACM*
dmesg | grep -i "cdc" | tail -5
```

Restart the service and open the **Sensors → Pair** page in the UI.  Trigger
the FT007TH (press its button or wait up to 60 s for an auto-beacon) — the
Dongle's LED will blink and the sensor should appear as a candidate.

---

## Pairing the sensor in the UI

1. Open **Sensors → Pair** (or the ＋ button).
2. Trigger the FT007TH sensor.
3. It appears as `Telldus-FT007TH:<device_id>` — click **Add**.
4. Give it a name, room, and choose whether to track temperature or humidity
   (add a second sensor entry for the other field, pointing at the same code).
5. Set `temperature_C` or `humidity` as the **Field**.
