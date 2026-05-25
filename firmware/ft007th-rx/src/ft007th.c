/**
 * @file ft007th.c
 * @brief Telldus FT007TH 36-bit packet decoder.
 *
 * Packet layout (MSB = bit 35, 9 nibbles total):
 *
 *   [35:33]  channel    3 bits  (0 = CH1 … 7 = CH8)
 *   [32]     battery    1 bit   (1 = OK)
 *   [31:24]  device ID  8 bits  (random per sensor)
 *   [23:12]  temp raw  12 bits  (unsigned; temp_C = (raw − 400) / 10.0)
 *   [11:4]   humidity   8 bits  (%RH)
 *   [3:0]    checksum   4 bits  (XOR of all nine nibbles = 0 when valid)
 *
 * Source: community reverse-engineering of Telldus FT007TH / Proove AB 313160,
 * validated against rtl_433.
 */
#include "ft007th.h"

static bool checksum_ok(uint64_t bits)
{
	uint8_t xor = 0U;

	/* XOR nine 4-bit nibbles, starting from the MSB nibble [35:32] */
	for (int shift = 32; shift >= 0; shift -= 4) {
		xor ^= (uint8_t)((bits >> shift) & 0xFU);
	}
	return xor == 0U;
}

ft007th_reading_t ft007th_decode(uint64_t bits)
{
	ft007th_reading_t r = {0};

	r.valid         = checksum_ok(bits);
	r.channel       = (uint8_t)((bits >> 33) & 0x7U) + 1U;
	r.battery_ok    = (bool)((bits >> 32) & 0x1U);
	r.device_id     = (uint8_t)((bits >> 24) & 0xFFU);

	uint16_t temp_raw = (uint16_t)((bits >> 12) & 0xFFFU);
	r.temperature_c   = ((float)temp_raw - 400.0f) / 10.0f;

	r.humidity      = (uint8_t)((bits >> 4) & 0xFFU);

	return r;
}
