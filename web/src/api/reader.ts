// Client for Pawn.Ink's custom reader/command API (/api/v1/pawn/x): the
// {data, meta} / {error, request_id} envelopes of IMPLEMENTATION.md §7.1.
import { ApiError, readJSON, send, type RequestOptions } from './http';

export const BASE = '/api/v1/pawn/x';

export interface Envelope<T> {
  data: T;
  meta: { request_id: string; next_cursor: string | null };
}

interface ErrorEnvelope {
  error: { code: string; message: string; details?: unknown };
  request_id?: string;
}

export async function request<T>(path: string, opts: RequestOptions = {}): Promise<Envelope<T>> {
  const res = await send(BASE + path, opts);
  const body = await readJSON(res);
  if (!res.ok) {
    const e = (body as ErrorEnvelope | null)?.error;
    throw new ApiError(res.status, e?.code ?? 'HTTP_' + res.status, e?.message ?? 'Request failed.', e?.details ?? null);
  }
  return body as Envelope<T>;
}

export const reader = {
  get: <T>(path: string, signal?: AbortSignal) => request<T>(path, { signal }).then((e) => e.data),
  post: <T>(path: string, body?: unknown) => request<T>(path, { method: 'POST', body }).then((e) => e?.data as T),
  patch: <T>(path: string, body?: unknown) => request<T>(path, { method: 'PATCH', body }).then((e) => e?.data as T),
  put: <T>(path: string, body?: unknown) => request<T>(path, { method: 'PUT', body }).then((e) => e?.data as T),
  del: (path: string) => request<null>(path, { method: 'DELETE' }).then(() => undefined),
};
