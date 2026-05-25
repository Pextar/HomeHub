/*
 * main.c – FT007TH 433 MHz sensor → USB CDC ACM JSON bridge
 *          for nRF52840 (nRF Connect SDK / Zephyr RTOS)
 *
 * The nRF52840 does NOT have a built-in 433 MHz radio — it needs an external
 * superheterodyne OOK receiver module wired to a GPIO pin.  The module outputs
 * a clean digital signal that this firmware decodes using edge timestamps.
 *
 * Once per sensor transmission (roughly every 60 s) it prints one JSON line
 * to the USB CDC ACM port:
 *   {"model":"Telldus-FT007TH","id":42,"channel":1,
 *    "battery_ok":1,"temperature_C":21.3,"humidity":65}
 *
 * On the Raspberry Pi, set in .env:
 *   SENSOR_SERIAL_PORT=/dev/ttyACM0
 */

#include <zephyr/kernel.h>
#include <zephyr/device.h>
#include <zephyr/drivers/gpio.h>
#include <zephyr/drivers/uart.h>
#include <zephyr/usb/usb_device.h>
#include <zephyr/logging/log.h>
#include <stdio.h>
#include <string.h>

#include "ft007th.h"

LOG_MODULE_REGISTER(ft007th_rx, LOG_LEVEL_INF);

/* ── Devicetree ──────────────────────────────────────────────────────────── */

/* rf-rx alias defined in boards/<board>.overlay */
static const struct gpio_dt_spec rf_rx =
	GPIO_DT_SPEC_GET(DT_ALIAS(rf_rx), gpios);

/* led0 — already defined in the board's base DTS; blinks on valid packet */
static const struct gpio_dt_spec led =
	GPIO_DT_SPEC_GET(DT_ALIAS(led0), gpios);

/* CDC ACM UART — created automatically when CONFIG_USB_CDC_ACM=y */
static const struct device *const cdc_uart =
	DEVICE_DT_GET(DT_NODELABEL(cdc_acm_uart0));

/* ── ISR → main-thread packet slot ──────────────────────────────────────── */

static struct {
	uint64_t      bits;
	volatile bool ready;
} g_pkt;

/* ── GPIO ISR ─────────────────────────────────────────────────────────────── */

static struct gpio_callback rf_cb;

/*
 * Protocol timing (µs):
 *   Sync / idle gap  > 3 500      → reset accumulator
 *   Long gap           1 200–2 800 → bit 1
 *   Short gap            350–1 200 → bit 0
 *
 * Pulses (HIGH) are approximately 500 µs and carry no information; only the
 * gap (LOW) duration between pulses encodes the bit value.
 */
static void rf_isr(const struct device *dev,
		   struct gpio_callback *cb,
		   uint32_t pins)
{
	static uint32_t fall_cy;   /* cycle count at last falling edge */
	static uint64_t bit_buf;
	static uint8_t  bit_cnt;

	const uint32_t now   = k_cycle_get_32();
	const int      level = gpio_pin_get_dt(&rf_rx);

	if (level <= 0) {
		/* Falling edge — record gap start */
		fall_cy = now;
		return;
	}

	/* Rising edge — gap just ended; measure its duration */
	const uint32_t gap_us = k_cyc_to_us_near32(now - fall_cy);

	if (gap_us > 3500U) {
		bit_buf = 0U;
		bit_cnt = 0U;
		return;
	}

	uint8_t new_bit;

	if (gap_us >= 1200U && gap_us < 2800U) {
		new_bit = 1U;
	} else if (gap_us >= 350U && gap_us < 1200U) {
		new_bit = 0U;
	} else {
		return; /* out-of-range — noise, ignore */
	}

	bit_buf = (bit_buf << 1) | new_bit;

	if (++bit_cnt == 36U) {
		if (!g_pkt.ready) {
			g_pkt.bits  = bit_buf;
			g_pkt.ready = true;
		}
		bit_buf = 0U;
		bit_cnt = 0U;
	}
}

/* ── CDC ACM helpers ─────────────────────────────────────────────────────── */

/** Returns true when a host has opened the serial port (DTR asserted). */
static bool cdc_open(void)
{
#ifdef CONFIG_UART_LINE_CTRL
	uint32_t dtr = 0U;

	(void)uart_line_ctrl_get(cdc_uart, UART_LINE_CTRL_DTR, &dtr);
	return dtr != 0U;
#else
	return true;
#endif
}

static void cdc_puts(const char *s)
{
	for (; *s != '\0'; ++s) {
		uart_poll_out(cdc_uart, (unsigned char)*s);
	}
}

/* ── Entry point ─────────────────────────────────────────────────────────── */

int main(void)
{
	int ret;

	/* Start USB — must happen before any other USB call */
	ret = usb_enable(NULL);
	if (ret != 0) {
		LOG_ERR("usb_enable failed: %d", ret);
		return ret;
	}

	/* Status LED (optional — keep going if the board lacks one) */
	if (device_is_ready(led.port)) {
		gpio_pin_configure_dt(&led, GPIO_OUTPUT_INACTIVE);
	}

	/* RF RX GPIO */
	if (!device_is_ready(rf_rx.port)) {
		LOG_ERR("RF RX GPIO port not ready");
		return -ENODEV;
	}

	ret = gpio_pin_configure_dt(&rf_rx, GPIO_INPUT);
	if (ret < 0) {
		LOG_ERR("gpio_pin_configure_dt: %d", ret);
		return ret;
	}

	ret = gpio_pin_interrupt_configure_dt(&rf_rx, GPIO_INT_EDGE_BOTH);
	if (ret < 0) {
		LOG_ERR("gpio_pin_interrupt_configure_dt: %d", ret);
		return ret;
	}

	gpio_init_callback(&rf_cb, rf_isr, BIT(rf_rx.pin));
	gpio_add_callback(rf_rx.port, &rf_cb);

	LOG_INF("FT007TH receiver ready — listening for 433 MHz packets");

	char json[160];

	for (;;) {
		if (g_pkt.ready) {
			const uint64_t        bits = g_pkt.bits;
			const ft007th_reading_t r  = ft007th_decode(bits);

			g_pkt.ready = false;

			if (r.valid && cdc_open()) {
				snprintf(json, sizeof(json),
					 "{\"model\":\"Telldus-FT007TH\","
					 "\"id\":%u,"
					 "\"channel\":%u,"
					 "\"battery_ok\":%u,"
					 "\"temperature_C\":%.1f,"
					 "\"humidity\":%u}\n",
					 (unsigned)r.device_id,
					 (unsigned)r.channel,
					 (unsigned)r.battery_ok,
					 (double)r.temperature_c,
					 (unsigned)r.humidity);
				cdc_puts(json);

				/* Brief LED blink to confirm reception */
				if (device_is_ready(led.port)) {
					gpio_pin_set_dt(&led, 1);
					k_sleep(K_MSEC(100));
					gpio_pin_set_dt(&led, 0);
				}
			}
		}

		k_sleep(K_MSEC(10));
	}

	return 0;
}
