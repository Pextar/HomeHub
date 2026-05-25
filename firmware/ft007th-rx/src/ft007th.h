/**
 * @file ft007th.h
 * @brief Decoder for the Telldus FT007TH / Proove AB 313160
 *        433 MHz thermo-hygrometer (OOK PWM, 36 bits).
 */
#pragma once

#include <stdbool.h>
#include <stdint.h>

/** Decoded reading from one FT007TH packet. */
typedef struct {
	/** Random 8-bit ID, stable until battery swap. */
	uint8_t  device_id;
	/** Channel 1–8 selected by the DIP switches on the sensor back. */
	uint8_t  channel;
	/** 1 = battery OK, 0 = replace batteries. */
	bool     battery_ok;
	/** Temperature in degrees Celsius (resolution 0.1 °C). */
	float    temperature_c;
	/** Relative humidity 0–99 %RH. */
	uint8_t  humidity;
	/** false when the 4-bit XOR checksum does not match. */
	bool     valid;
} ft007th_reading_t;

/**
 * @brief Decode a 36-bit FT007TH OOK packet.
 *
 * @param bits  36 bits of data, MSB at bit 35 (bit 0 = checksum LSB).
 * @return Decoded reading; inspect .valid before using any field.
 */
ft007th_reading_t ft007th_decode(uint64_t bits);
