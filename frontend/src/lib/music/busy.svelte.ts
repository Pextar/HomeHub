import { toasts } from "../stores.svelte";

/**
 * What is in flight, keyed by `"<action>:<id>"`.
 *
 * Every control in Music disables itself while its own call is out — a second
 * tap on play must not queue a second SOAP round trip — and the key is what
 * lets one map serve prev/play/next on a dozen cards without any of them
 * disabling each other.
 *
 * It also does the guarding: `claim` and `run` both refuse to start when the
 * key is already busy, so no call site has to remember the check. `run` adds
 * the error toast and the re-read behind it; `claim` is for the handful of
 * actions that need their own catch (an optimistic flip to roll back, a toast
 * that names what started playing).
 */
export interface Busy {
  /** Is this key in flight? What `disabled` on a control reads. */
  is(key: string): boolean;
  /**
   * Run `fn` under the key, guarding re-entry and always releasing.
   * Resolves `undefined` without calling `fn` when the key is already busy —
   * errors are the caller's to handle.
   */
  claim<T>(key: string, fn: () => Promise<T>): Promise<T | undefined>;
  /**
   * `claim` plus the two things most actions want: a toast titled `errTitle`
   * when it throws, and `after` — the re-read that reflects the change —
   * when it doesn't.
   *
   * `after` is passed in rather than inferred. It used to be picked by
   * sniffing the key for a `kef` prefix, which quietly meant a new bridge, or
   * a key that didn't follow the convention, re-read the wrong speakers.
   */
  run(
    key: string,
    fn: () => Promise<unknown>,
    errTitle: string,
    after?: () => Promise<unknown> | void,
  ): Promise<void>;
}

export function createBusy(): Busy {
  const flags = $state<Record<string, boolean>>({});

  async function claim<T>(key: string, fn: () => Promise<T>): Promise<T | undefined> {
    if (flags[key]) return undefined;
    flags[key] = true;
    try {
      return await fn();
    } finally {
      flags[key] = false;
    }
  }

  return {
    is: (key) => !!flags[key],
    claim,
    async run(key, fn, errTitle, after) {
      await claim(key, async () => {
        try {
          await fn();
          await after?.();
        } catch (e) {
          toasts.error(errTitle, (e as Error).message);
        }
      });
    },
  };
}
