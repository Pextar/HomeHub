// Assistant (local LLM).

export interface AssistantStatus {
  enabled: boolean;
  model?: string;
  reachable?: boolean;
  last_error?: string;
}

// One tool the assistant ran during a turn, shown as a small card in the thread.
export interface AssistantToolCall {
  name: string;
  result: string;
}

// A pending bulk/destructive action awaiting the user's confirmation. The
// signed token is opaque — it round-trips to the backend untouched.
export interface AssistantConfirmation {
  token: string;
  summary: string;
  affected?: string[];
  tool: string;
}

export type AssistantRole = "user" | "assistant";

export interface AssistantMessage {
  role: AssistantRole;
  content: string;
  tools?: AssistantToolCall[];
  // true while the assistant bubble is still waiting on its first token.
  pending?: boolean;
  // set when this assistant turn ended in an error, so it can render as such.
  error?: boolean;
}

// Events streamed from /api/assistant/chat and /confirm.
export type AssistantStreamEvent =
  | { type: "token"; text: string }
  | { type: "tool"; name: string; result: string }
  | { type: "confirmation"; confirmation: AssistantConfirmation }
  | { type: "error"; message: string }
  | { type: "done" };
