/// <reference lib="webworker" />
import {
  cleanupOutdatedCaches,
  createHandlerBoundToURL,
  precacheAndRoute,
} from "workbox-precaching";
import { NavigationRoute, registerRoute } from "workbox-routing";
import { NetworkOnly } from "workbox-strategies";

declare let self: ServiceWorkerGlobalScope;

// Remove caches from old Workbox versions.
cleanupOutdatedCaches();

// Precache all assets emitted by Vite (manifest injected at build time).
precacheAndRoute(self.__WB_MANIFEST);

// /api routes must never be served from cache — always hit the network so
// the app never shows stale socket state. This mirrors the previous workbox
// runtimeCaching config.
registerRoute(
  ({ url }) => url.pathname.startsWith("/api"),
  new NetworkOnly()
);

// Shell navigation fallback: serve the precached /index.html for all
// page navigations except /api (handled above). This keeps the SPA
// loadable offline without hitting the server for every route change.
const navHandler = createHandlerBoundToURL("/index.html");
registerRoute(new NavigationRoute(navHandler, { denylist: [/^\/api/] }));

// ─── Web Push ────────────────────────────────────────────────────────────────

// Handle incoming push messages and show a system notification.
self.addEventListener("push", (event: PushEvent) => {
  let payload: { title?: string; body?: string; url?: string; tag?: string } =
    {};
  try {
    payload = event.data?.json() ?? {};
  } catch {
    payload = { title: event.data?.text() };
  }

  const title = payload.title ?? "HomeHub";
  const options: NotificationOptions = {
    body: payload.body,
    icon: "/pwa-icon.svg",
    badge: "/pwa-icon.svg",
    // tag collapses duplicate notifications: a second "device turned on"
    // replaces the first rather than stacking.
    tag: payload.tag,
    data: { url: payload.url ?? "/" },
    requireInteraction: false,
  };

  event.waitUntil(self.registration.showNotification(title, options));
});

// When the user taps a notification, focus an existing tab or open a new one.
self.addEventListener("notificationclick", (event: NotificationClickEvent) => {
  event.notification.close();
  const url: string = event.notification.data?.url ?? "/";

  event.waitUntil(
    clients
      .matchAll({ type: "window", includeUncontrolled: true })
      .then((windowClients) => {
        // Focus an existing window if one is open.
        for (const client of windowClients) {
          if ("focus" in client) {
            return client.focus();
          }
        }
        // Otherwise open a new window.
        return clients.openWindow(url);
      })
  );
});
