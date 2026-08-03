// Test harness for rune-driven stores.
//
// A store built out of `$state` / `$derived` / `$effect` normally lives
// inside a component, which is what owns its effects. A test has no
// component, so it opens an effect root of its own and hands back the
// flush and the teardown — the same three moves every such test needs, in
// one place rather than copied into each.
//
// It is a `.svelte.ts` module because runes are compiled, and a plain
// `.test.ts` file is not run through the Svelte compiler.

import { flushSync } from "svelte";

export interface RunesHarness<T> {
  /** Whatever the factory returned, created inside the effect root. */
  value: T;
  /** Settle every pending effect — call after anything that mutates state. */
  flush(): void;
  /** Tear the root down. */
  stop(): void;
}

export function withRoot<T>(create: () => T): RunesHarness<T> {
  let value!: T;
  const stop = $effect.root(() => {
    value = create();
  });
  flushSync();
  return {
    get value() {
      return value;
    },
    flush: () => flushSync(),
    stop,
  };
}
