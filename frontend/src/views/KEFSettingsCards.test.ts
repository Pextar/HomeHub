import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/svelte";
import KEFSettingsCards from "./KEFSettingsCards.svelte";
import type { KEFSettings } from "../lib/types";

/**
 * The KEF settings cards' contract.
 *
 * §15's capability honesty, applied per field: a card renders only where
 * the speaker answered for the fields in it, because a control the speaker
 * would refuse is worse than one that isn't there. And a write moves on
 * screen first, then rolls back — that field only — if it is refused.
 */

const updateSettings = vi.fn(async () => {});
vi.mock("../lib/api", () => ({
  api: { kefUpdateSettings: (...a: unknown[]) => updateSettings(...(a as [])) },
}));
const errorToast = vi.fn();
vi.mock("../lib/stores.svelte", () => ({ toasts: { error: (...a: unknown[]) => errorToast(...a) } }));

const base = { speakerId: "study", poweredOn: true, onSetPower: vi.fn() };
beforeEach(() => {
  updateSettings.mockClear();
  updateSettings.mockImplementation(async () => {});
  errorToast.mockClear();
});

describe("a KEF speaker's settings cards", () => {
  it("draws only the cards the speaker answered for", () => {
    // An LSX II has no sub output — the whole card is absent, not disabled.
    render(KEFSettingsCards, {
      ...base, busy: {}, settings: { bass_extension: "standard" } as KEFSettings,
    });
    expect(screen.getByRole("heading", { name: "Sound" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /Subwoofer/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /Power/i })).not.toBeInTheDocument();
  });

  it("marks the chosen value and writes the one that was tapped", async () => {
    // A plain object: the component mutates it with Object.assign, which is
    // what the rollback assertion below reads back. Runes need the Svelte
    // compiler and a .test.ts file does not get it (see test-runes.svelte.ts).
    const settings = { bass_extension: "standard" } as KEFSettings;
    render(KEFSettingsCards, { ...base, busy: {}, settings });
    const extra = screen.getByRole("radio", { name: "Extra" });
    expect(screen.getByRole("radio", { name: "Standard" })).toHaveAttribute("aria-checked", "true");
    extra.click();
    await vi.waitFor(() =>
      expect(updateSettings).toHaveBeenCalledWith("study", { bass_extension: "extra" }),
    );
  });

  it("puts the value back — that field only — when the speaker refuses", async () => {
    updateSettings.mockImplementation(async () => {
      throw new Error("refused");
    });
    const settings = { bass_extension: "standard", treble: 0 } as KEFSettings;
    render(KEFSettingsCards, { ...base, busy: {}, settings });
    screen.getByRole("radio", { name: "Extra" }).click();
    await vi.waitFor(() => expect(errorToast).toHaveBeenCalled());
    expect(settings.bass_extension).toBe("standard");
    expect(settings.treble).toBe(0);
  });
});
