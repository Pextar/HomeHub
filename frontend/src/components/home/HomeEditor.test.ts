import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/svelte";
import HomeEditor from "./HomeEditor.svelte";
import { homeLayout } from "../../lib/home-layout.svelte";

/**
 * Arranging the home screen, from the outside.
 *
 * The three things a person can do here — move a section, switch one off,
 * open the one section that has settings — and the rule that a section with
 * nothing to configure gets no button rather than a dead one (§15.1).
 *
 * The pointer drag isn't tested here: it is a stream of pointer events over
 * live layout, and jsdom gives every element a zero-height rect, so the
 * arithmetic it exists to do can't happen. The keyboard path below moves the
 * same list through the same store.
 */

vi.mock("../../lib/stores.svelte", () => ({
  data: {
    value: { sockets: [], groups: [], rooms: [], sensors: [], timers: [], loaded: true },
  },
  session: { isAdmin: true },
  toasts: { error: vi.fn(), warn: vi.fn(), show: vi.fn() },
  route: { go: vi.fn() },
}));

const order = () => [...homeLayout.order];
const rows = () =>
  Array.from(document.querySelectorAll("[data-row]")).map((el) => el.getAttribute("data-row"));
const grip = (title: string) => screen.getByRole("button", { name: `Reorder ${title}` });

beforeEach(() => {
  localStorage.clear();
  homeLayout.reset();
});

describe("customising the home screen", () => {
  it("lists every section in the order the screen has them", () => {
    render(HomeEditor, { onDone: vi.fn() });
    expect(rows()).toEqual(order());
  });

  it("moves a section down a place, and says where it landed", async () => {
    render(HomeEditor, { onDone: vi.fn() });
    const [first, second] = order();
    await fireEvent.keyDown(grip("Whole home"), { key: "ArrowDown" });

    expect(order().slice(0, 2)).toEqual([second, first]);
    expect(rows()).toEqual(order());
    expect(screen.getByRole("status")).toHaveTextContent("Whole home moved to position 2 of 8");
  });

  it("refuses to move the top section any higher", async () => {
    render(HomeEditor, { onDone: vi.fn() });
    const before = order();
    await fireEvent.keyDown(grip("Whole home"), { key: "ArrowUp" });
    expect(order()).toEqual(before);
  });

  it("switches a section off without losing its place", async () => {
    render(HomeEditor, { onDone: vi.fn() });
    await fireEvent.keyDown(grip("Pending timers"), { key: "ArrowUp" });
    const at = order().indexOf("timers");

    await fireEvent.click(
      screen.getByRole("checkbox", { name: "Show Pending timers on the home screen" }),
    );

    expect(homeLayout.isHidden("timers")).toBe(true);
    expect(order().indexOf("timers")).toBe(at);
  });

  it("offers settings only on the section that has some", () => {
    render(HomeEditor, { onDone: vi.fn() });
    expect(screen.getByRole("button", { name: "Sensors settings" })).toBeInTheDocument();
    // Rooms has nothing to configure: no button at all, not a disabled one.
    expect(screen.queryByRole("button", { name: "Rooms settings" })).not.toBeInTheDocument();
  });

  it("leaves the mode on Escape", async () => {
    const onDone = vi.fn();
    render(HomeEditor, { onDone });
    await fireEvent.keyDown(window, { key: "Escape" });
    expect(onDone).toHaveBeenCalled();
  });
});
