import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import KidVolumeRow from "./KidVolumeRow.svelte";

/**
 * The row the room's fader and each speaker's fader are both made of.
 *
 * The two used to be separate markup sharing a stylesheet, which is how they
 * came to disagree about what the readout says: the slider is capped at the
 * kid's ceiling (§17), but the number beside it has to tell the truth about a
 * speaker a grown-up already turned up past it.
 */

const props = (over: Record<string, unknown> = {}) => ({
  value: 30,
  max: 50,
  readout: 30,
  label: "Volume",
  muted: false,
  muteLabel: "Mute",
  onMute: vi.fn(),
  onInput: vi.fn(),
  onChange: vi.fn(),
  ...over,
});

describe("the kid module's volume row", () => {
  it("shows the real level, not the capped one", () => {
    // A grown-up left this speaker at 80; the kid's rail stops at 50.
    render(KidVolumeRow, props({ value: 50, readout: 80 }));
    expect(document.querySelector(".km-vol-val")).toHaveTextContent("80");
    expect(screen.getByRole("slider")).toHaveAttribute("aria-valuetext", "80%");
  });

  it("names the speaker on a member's row and nobody on the room's", () => {
    const { unmount } = render(KidVolumeRow, props({ name: "Kitchen" }));
    expect(document.querySelector(".km-member-name")).toHaveTextContent("Kitchen");
    unmount();
    render(KidVolumeRow, props());
    expect(document.querySelector(".km-member-name")).toBeNull();
  });

  it("wears the smaller button only under a room's own fader", () => {
    const { unmount } = render(KidVolumeRow, props({ name: "Kitchen" }));
    expect(document.querySelector(".km-vol-btn")).toHaveClass("small");
    unmount();
    render(KidVolumeRow, props());
    expect(document.querySelector(".km-vol-btn")).not.toHaveClass("small");
  });

  it("says which way the mute button goes, for a reader who can't see it", () => {
    render(KidVolumeRow, props({ muted: true, muteLabel: "Unmute Kitchen" }));
    const btn = screen.getByRole("button", { name: "Unmute Kitchen" });
    expect(btn).toHaveClass("mute");
    expect(btn).toHaveTextContent("🔇");
  });

  it("refuses the tap while the last one is still out", () => {
    const onMute = vi.fn();
    render(KidVolumeRow, props({ muteBusy: true, onMute }));
    expect(screen.getByRole("button")).toBeDisabled();
  });
});
