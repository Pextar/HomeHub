import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/svelte";

/**
 * The wizard's first screen is the whole point of the rework: it is the one
 * question the user has to answer, so what it offers has to be exactly what
 * the house can actually do.
 *
 * The rule under test is DESIGN.md §15.1 — a control that would be refused
 * is worse than a control that isn't there. Matter needs the bridge; without
 * it the card must be absent, not present-and-failing.
 */

const matterTransport = vi.fn();

vi.mock("../lib/api", () => ({
  api: {
    matterTransport: () => matterTransport(),
    tasmotaDiscover: vi.fn().mockResolvedValue({ devices: [] }),
    startSensorPair: vi.fn().mockResolvedValue({ until: new Date().toISOString() }),
    discoverSensors: vi.fn().mockResolvedValue({ candidates: [], until: new Date().toISOString() }),
  },
}));

vi.mock("../lib/stores.svelte", () => ({
  data: { value: { rooms: [], sensors: [] }, refresh: vi.fn() },
  toasts: { error: vi.fn(), warn: vi.fn() },
}));

vi.mock("../lib/modal.svelte", () => ({
  closeModal: vi.fn(),
  openModal: vi.fn(),
}));

// The scanner reaches for getUserMedia on mount, which jsdom has not got.
// The wizard's first screen never renders it; stubbing keeps the module
// graph loadable.
vi.mock("../components/QRScanner.svelte", async () => {
  const stub = (await import("./__fixtures__/EmptyStub.svelte")).default;
  return { default: stub };
});

import AddDeviceModal from "./AddDeviceModal.svelte";

describe("the add-device wizard's first question", () => {
  beforeEach(() => {
    matterTransport.mockReset();
  });

  it("offers Matter when the bridge answers", async () => {
    matterTransport.mockResolvedValue({ transports: ["wifi"] });
    render(AddDeviceModal, {});
    expect(await screen.findByRole("button", { name: /Matter device/ })).toBeInTheDocument();
  });

  it("leaves Matter out entirely when the bridge is not configured", async () => {
    matterTransport.mockRejectedValue(new Error("matter bridge is not configured"));
    render(AddDeviceModal, {});
    // The other methods still arrive, so this is "absent", not "nothing rendered".
    expect(await screen.findByRole("button", { name: /433 MHz socket/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Matter device/ })).not.toBeInTheDocument();
    expect(screen.getByText(/Matter isn't set up on this server/)).toBeInTheDocument();
  });

  it("names every way into the house exactly once", async () => {
    matterTransport.mockResolvedValue({ transports: ["thread", "wifi"] });
    render(AddDeviceModal, {});
    await screen.findByRole("button", { name: /Matter device/ });
    for (const label of [
      /Matter device/, /Wi-Fi device \(Tasmota\)/, /433 MHz socket/,
      /433 MHz sensor/, /MQTT device/,
    ]) {
      expect(screen.getAllByRole("button", { name: label })).toHaveLength(1);
    }
  });

  it("starts on step 1 of 3", async () => {
    matterTransport.mockResolvedValue({ transports: ["wifi"] });
    render(AddDeviceModal, {});
    expect(await screen.findByText(/Step 1 of 3/)).toBeInTheDocument();
  });

  it("walks to the discovery step when a method is chosen", async () => {
    matterTransport.mockResolvedValue({ transports: ["wifi"] });
    render(AddDeviceModal, {});
    (await screen.findByRole("button", { name: /433 MHz socket/ })).click();
    await waitFor(() => expect(screen.getByText(/Step 2 of 3/)).toBeInTheDocument());
    // The RF socket step is the one that transmits, so it leads with that.
    expect(screen.getByRole("button", { name: /Pair with socket/ })).toBeInTheDocument();
  });

  it("can go back from discovery to the method list", async () => {
    matterTransport.mockResolvedValue({ transports: ["wifi"] });
    render(AddDeviceModal, {});
    (await screen.findByRole("button", { name: /MQTT device/ })).click();
    await waitFor(() => expect(screen.getByText(/Step 2 of 3/)).toBeInTheDocument());
    screen.getByRole("button", { name: "Back" }).click();
    await waitFor(() => expect(screen.getByText(/Step 1 of 3/)).toBeInTheDocument());
  });
});
