/**
 * push.svelte.ts — Web Push subscription management.
 *
 * Usage:
 *   import { pushClient } from "./push.svelte";
 *   await pushClient.init();          // read current browser state on mount
 *   await pushClient.subscribe();     // request permission + subscribe
 *   await pushClient.unsubscribe();   // remove subscription
 */

import { api } from "./api";

/** Convert a URL-safe base64 VAPID public key to a Uint8Array. */
function urlBase64ToUint8Array(base64: string): Uint8Array {
  const padding = "=".repeat((4 - (base64.length % 4)) % 4);
  const b64 = (base64 + padding).replace(/-/g, "+").replace(/_/g, "/");
  const raw = atob(b64);
  return Uint8Array.from(raw, (c) => c.charCodeAt(0));
}

/** True when the browser supports the Web Push API. */
export const pushSupported =
  typeof window !== "undefined" &&
  "serviceWorker" in navigator &&
  "PushManager" in window &&
  "Notification" in window;

class PushClient {
  permission = $state<NotificationPermission>(
    pushSupported ? Notification.permission : "denied"
  );
  isSubscribed = $state(false);
  loading = $state(false);

  /** Call once on component mount to read current browser state. */
  async init() {
    if (!pushSupported) return;
    this.permission = Notification.permission;
    try {
      const reg = await navigator.serviceWorker.ready;
      const sub = await reg.pushManager.getSubscription();
      this.isSubscribed = sub !== null;
    } catch {
      this.isSubscribed = false;
    }
  }

  /**
   * Request notification permission from the browser, subscribe to push,
   * and register the subscription with the backend.
   */
  async subscribe(): Promise<boolean> {
    if (!pushSupported || this.loading) return false;
    this.loading = true;
    try {
      const permission = await Notification.requestPermission();
      this.permission = permission;
      if (permission !== "granted") return false;

      const { public_key } = await api.getPushVapidKey();
      const reg = await navigator.serviceWorker.ready;
      const sub = await reg.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlBase64ToUint8Array(public_key),
      });

      // sub.toJSON() returns { endpoint, keys: { p256dh, auth } }
      await api.subscribePush(sub.toJSON() as {
        endpoint: string;
        keys: { p256dh: string; auth: string };
      });

      this.isSubscribed = true;
      return true;
    } catch (e) {
      console.error("push subscribe:", e);
      return false;
    } finally {
      this.loading = false;
    }
  }

  /** Remove the push subscription from both the browser and backend. */
  async unsubscribe(): Promise<void> {
    if (!pushSupported || this.loading) return;
    this.loading = true;
    try {
      const reg = await navigator.serviceWorker.ready;
      const sub = await reg.pushManager.getSubscription();
      if (sub) {
        await api.unsubscribePush(sub.endpoint);
        await sub.unsubscribe();
      }
      this.isSubscribed = false;
    } catch (e) {
      console.error("push unsubscribe:", e);
    } finally {
      this.loading = false;
    }
  }
}

export const pushClient = new PushClient();
