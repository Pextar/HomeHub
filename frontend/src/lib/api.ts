/**
 * The typed door to the backend.
 *
 * `api` is one flat object — `api.sonosPlayItem(...)`, `api.listSockets()` —
 * assembled here from one slice per domain under lib/api/. It reads as a
 * single surface at every call site, which is what a caller wants, without
 * being a single thousand-line file, which is what nobody wants to read.
 *
 * The transport itself (the fetch wrapper, ApiError) is lib/api/http.ts, so
 * no slice owns it. The assistant's streaming calls stay below rather than
 * joining the object: they are not request/response, they hand back an
 * event stream, and folding them into `api` would make its shape a lie.
 */

import { homeApi } from "./api/home";
import { sonosApi } from "./api/sonos";
import { kefApi } from "./api/kef";
import { spotifyApi } from "./api/spotify";
import { systemApi } from "./api/system";
import { connectApi } from "./api/connect";
import { airplayApi } from "./api/airplay";
import { mediaApi } from "./api/media";
import { ApiError, API_BASE } from "./api/http";
import type {
  AssistantMessage,
  AssistantStreamEvent,
  AssistantConfirmation,
} from "./types";

export type { PlayItemBody } from "./api/http";

export const api = {
  ...homeApi,
  ...sonosApi,
  ...kefApi,
  ...spotifyApi,
  ...systemApi,
  ...connectApi,
  ...airplayApi,
  ...mediaApi,
};

export type Api = typeof api;
export { ApiError };

// ---- Assistant streaming (SSE over POST) ----

// The wire shape sent to the backend: only role + content (system prompt and
// tool results are reconstructed server-side).
type ChatHistory = { role: "user" | "assistant"; content: string }[];

function historyOf(messages: AssistantMessage[]): ChatHistory {
  return messages
    .filter((m) => m.content.trim() !== "")
    .map((m) => ({ role: m.role, content: m.content }));
}

// Parse one SSE frame ("event: X\ndata: <json>") into a typed event. Returns
// null for frames we don't recognise (e.g. keepalive comments).
function parseFrame(frame: string): AssistantStreamEvent | null {
  let event = "";
  const dataLines: string[] = [];
  for (const line of frame.split("\n")) {
    if (line.startsWith("event:")) event = line.slice(6).trim();
    else if (line.startsWith("data:")) dataLines.push(line.slice(5).trim());
  }
  if (!event) return null;
  let data: unknown = null;
  if (dataLines.length) {
    try { data = JSON.parse(dataLines.join("\n")); } catch { return null; }
  }
  switch (event) {
    case "token":
      return { type: "token", text: String(data ?? "") };
    case "tool": {
      const d = data as { name?: string; result?: string };
      return { type: "tool", name: d?.name ?? "", result: d?.result ?? "" };
    }
    case "confirmation":
      return { type: "confirmation", confirmation: data as AssistantConfirmation };
    case "error":
      return { type: "error", message: String(data ?? "error") };
    case "done":
      return { type: "done" };
    default:
      return null;
  }
}

// streamPost opens a streaming POST and dispatches each parsed SSE event to
// onEvent until the body closes or the AbortSignal fires.
async function streamPost(
  path: string,
  body: unknown,
  onEvent: (e: AssistantStreamEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  const res = await fetch(API_BASE + path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
    signal,
  });
  if (!res.ok || !res.body) {
    const text = await res.text().catch(() => "");
    let msg = res.statusText || "assistant request failed";
    try {
      const j = JSON.parse(text);
      if (j && typeof j.error === "string") msg = j.error;
    } catch { /* keep statusText */ }
    throw new ApiError(msg, res.status);
  }
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buf = "";
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buf += decoder.decode(value, { stream: true });
    let idx: number;
    // Frames are separated by a blank line.
    while ((idx = buf.indexOf("\n\n")) >= 0) {
      const frame = buf.slice(0, idx);
      buf = buf.slice(idx + 2);
      const ev = parseFrame(frame);
      if (ev) onEvent(ev);
    }
  }
}

// streamAssistantChat sends the conversation and streams the assistant's reply.
export function streamAssistantChat(
  messages: AssistantMessage[],
  onEvent: (e: AssistantStreamEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  return streamPost("/assistant/chat", { messages: historyOf(messages) }, onEvent, signal);
}

// streamAssistantConfirm executes a previously-paused action and streams the
// model's closing summary.
export function streamAssistantConfirm(
  token: string,
  messages: AssistantMessage[],
  onEvent: (e: AssistantStreamEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  return streamPost("/assistant/confirm", { token, messages: historyOf(messages) }, onEvent, signal);
}
