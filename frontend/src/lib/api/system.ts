/**
 * Push notifications, the MQTT broker, and the local assistant's
 * status read.
 *
 * One slice of the flat `api` object — lib/api.ts spreads these together,
 * so every call site's `api.foo()` is unchanged. Split by domain because a
 * thousand-line object is not a thing anyone reads.
 */

import { req, json } from "./http";
import type {
  AssistantStatus,
  NotifPrefs,
  PushSubscriptionBody,
} from "../types";

export const systemApi = {
  // Push notifications
  getPushVapidKey() {
    return req<{ public_key: string }>("/push/vapid-key");
  },
  subscribePush(sub: PushSubscriptionBody) {
    return req<{ status: string }>("/push/subscribe", { method: "POST", body: json(sub) });
  },
  unsubscribePush(endpoint: string) {
    return req<{ status: string }>("/push/unsubscribe", {
      method: "DELETE",
      body: json({ endpoint }),
    });
  },
  updatePushPrefs(prefs: NotifPrefs) {
    return req<NotifPrefs>("/push/prefs", { method: "PUT", body: json(prefs) });
  },
  testPush() {
    return req<{ status: string }>("/push/test", { method: "POST" });
  },

  // MQTT — control devices and ingest sensors over a broker.
  mqttStatus() {
    return req<{ enabled: boolean; broker?: string; connected?: boolean }>("/mqtt/status");
  },
  mqttPublish(body: { topic: string; payload?: string }) {
    return req<{ status: string; topic: string }>("/mqtt/publish", {
      method: "POST",
      body: json(body),
    });
  },

  // Assistant (local LLM). Status is a plain request; chat/confirm stream and
  // are handled by streamAssistantChat / streamAssistantConfirm below.
  assistantStatus() {
    return req<AssistantStatus>("/assistant/status");
  },
};
