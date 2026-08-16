/**
 * The one door to the backend: the fetch wrapper every typed call goes
 * through, and the error it throws.
 *
 * Split out of api.ts so that each domain's calls can live in a file of
 * their own without any of them owning the transport. Nothing here knows
 * what a speaker or a socket is.
 */

/**
 * The body every "play this" endpoint takes.
 *
 * Only service/uri/title reach the speaker. The rest is carried so the room's
 * history has something worth drawing — a shelf tile needs a picture and a
 * second line, and asking the catalog for them again later would mean a
 * service round-trip to redraw a row we already had in hand.
 */
export interface PlayItemBody {
  service: string;
  uri: string;
  title: string;
  kind?: string;
  sub?: string;
  art_uri?: string;
}

const BASE = "/api";

export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

export async function req<T>(path: string, opts: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = { ...((opts.headers as Record<string, string>) ?? {}) };
  if (opts.body && !headers["Content-Type"]) headers["Content-Type"] = "application/json";

  const res = await fetch(BASE + path, { ...opts, headers });
  if (res.status === 204) return undefined as T;

  const text = await res.text();
  let data: unknown = null;
  if (text) {
    try { data = JSON.parse(text); } catch { /* non-JSON body, leave data null */ }
  }
  if (!res.ok) {
    const msg =
      (data && typeof data === "object" && "error" in data && typeof (data as { error: unknown }).error === "string"
        ? (data as { error: string }).error
        : text || res.statusText || "Request failed");
    throw new ApiError(msg, res.status);
  }
  return data as T;
}

export const json = (body: unknown) => JSON.stringify(body);

/** Where the assistant's streaming calls open their own connections. */
export const API_BASE = BASE;
