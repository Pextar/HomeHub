/**
 * The one-second beat that makes progress *creep*.
 *
 * Speaker state is polled every 5s at best, and every 20s once push is up
 * (DESIGN.md §15), so a scrubber or progress hairline driven straight off the
 * poll would step forward in five-second jumps and then sit still for twenty.
 * Every surface that shows a position instead extrapolates from the last
 * reading — and needs something to re-run that derivation once a second.
 *
 * That something is this counter. Reading `clock.beat` inside a `$derived`
 * subscribes to it; nothing else about the value matters.
 *
 * It runs only while something is actually moving, which is the caller's
 * judgement, not this module's — so `start()` is explicit and returns its own
 * stop. Starts are refcounted so two surfaces that both want a beat share one
 * interval rather than racing two.
 */

const state = $state({ beat: 0 });

let timer: ReturnType<typeof setInterval> | undefined;
let holders = 0;

export const clock = {
  /** Bumped once a second while anything holds the clock. Read it to subscribe. */
  get beat() {
    return state.beat;
  },

  /**
   * Ask for a beat. Returns the release — hand it straight back from an
   * `$effect` so the clock stops with the surface that wanted it.
   */
  start(): () => void {
    holders++;
    timer ??= setInterval(() => state.beat++, 1000);
    let released = false;
    return () => {
      if (released) return; // an effect can re-run its teardown; don't double-count
      released = true;
      if (--holders > 0) return;
      clearInterval(timer);
      timer = undefined;
    };
  },
};
