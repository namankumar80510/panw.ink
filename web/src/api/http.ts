// Shared fetch plumbing for both API clients. Credentials are the
// same-origin HttpOnly session cookie; mutations carry the CSRF token the
// server returned from /auth/me or login (kept in memory only).

let csrfToken = '';

export function setCSRFToken(token: string): void {
  csrfToken = token;
}

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
    public readonly details: unknown = null,
  ) {
    super(message);
  }

  /** Per-field messages of a validation error (custom API), if any. */
  get fields(): Record<string, string> {
    const d = this.details as { fields?: Record<string, string> } | null;
    return d?.fields ?? {};
  }
}

export interface RequestOptions {
  method?: string;
  body?: unknown;
  signal?: AbortSignal;
  headers?: Record<string, string>;
}

export async function send(url: string, opts: RequestOptions = {}): Promise<Response> {
  const method = opts.method ?? 'GET';
  const headers: Record<string, string> = { Accept: 'application/json', ...opts.headers };
  let body: string | undefined;
  if (opts.body !== undefined) {
    headers['Content-Type'] = 'application/json';
    body = JSON.stringify(opts.body);
  }
  if (method !== 'GET' && method !== 'HEAD' && csrfToken) {
    headers['X-CSRF-Token'] = csrfToken;
  }
  try {
    return await fetch(url, { method, headers, body, credentials: 'same-origin', signal: opts.signal ?? null });
  } catch (e) {
    if ((e as Error).name === 'AbortError') throw e;
    throw new ApiError(0, 'NETWORK', 'Could not reach Pawn.Ink. Check your connection and try again.');
  }
}

export async function readJSON(res: Response): Promise<unknown> {
  if (res.status === 204) return null;
  const text = await res.text();
  if (!text) return null;
  try {
    return JSON.parse(text);
  } catch {
    throw new ApiError(res.status, 'BAD_RESPONSE', 'The server sent an unexpected response.');
  }
}
