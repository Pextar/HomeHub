import type { KEFSource } from "./types";

/**
 * The speaker's physical inputs, in the order the KEF Connect app lists
 * them. Every model reports the same vocabulary and simply refuses the ones
 * it doesn't have (an LS50 Wireless II has no USB), so this is the render
 * order rather than a per-model capability list — there is no "what inputs
 * do you have" call to build one from.
 */
export const KEF_SOURCES: { value: KEFSource; label: string }[] = [
    { value: "wifi", label: "Wi-Fi" },
    { value: "bluetooth", label: "Bluetooth" },
    { value: "tv", label: "TV" },
    { value: "optic", label: "Optical" },
    { value: "coaxial", label: "Coaxial" },
    { value: "analog", label: "Analogue" },
    { value: "usb", label: "USB" },
];

/**
 * Human label for a source the speaker reported. Unknown values — a source
 * added by a firmware we don't know about, or "standby" — are passed through
 * rather than mapped to a guess.
 */
export function kefSourceLabel(source: string | undefined): string {
    if (!source) return "";
    return KEF_SOURCES.find((s) => s.value === source)?.label ?? source;
}
