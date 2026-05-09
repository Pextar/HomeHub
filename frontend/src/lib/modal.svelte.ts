// Imperative modal stack. Components register the currently-open modal
// (and the "previously focused" element) here; <Modal /> in App.svelte
// renders whichever component is on top of the stack.
//
// Using a Svelte module file (.svelte.ts) lets us use $state outside of a
// .svelte component.

import type { Component } from "svelte";

interface ModalEntry {
  id: number;
  component: Component<any>;
  props: Record<string, unknown>;
  resolve: (value: unknown) => void;
  previousFocus: Element | null;
}

const stack = $state<ModalEntry[]>([]);
let nextId = 1;

export function openModal<R = unknown>(
  component: Component<any>,
  props: Record<string, unknown> = {},
): Promise<R> {
  return new Promise<R>(resolve => {
    stack.push({
      id: nextId++,
      component,
      props,
      resolve: resolve as (value: unknown) => void,
      previousFocus: document.activeElement,
    });
  });
}

export function closeModal(value: unknown = undefined) {
  const entry = stack.pop();
  if (!entry) return;
  if (entry.previousFocus instanceof HTMLElement) entry.previousFocus.focus();
  entry.resolve(value);
}

export function modalStack() {
  return stack;
}
