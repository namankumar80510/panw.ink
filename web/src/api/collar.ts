// Client for Collar's native records API, used directly by the studio/admin
// dashboards (IMPLEMENTATION.md §1.1 item 5, §7.5). It keeps Collar's own
// list shape, error envelope and query parameters; never mix it with the
// custom reader envelopes.
import { ApiError, readJSON, send } from './http';

export const COLLAR_BASE = '/api/v1/pawn/content';

export interface CollarList<T> {
  items: T[];
  page?: number;
  perPage?: number;
  totalItems?: number;
  totalPages?: number;
  nextCursor?: string | null;
}

export interface ListParams {
  filter?: string;
  sort?: string;
  fields?: string;
  expand?: string;
  page?: number;
  perPage?: number;
  cursor?: string;
  skipTotal?: boolean;
}

interface CollarErrorBody {
  error?: { code?: string; message?: string; details?: unknown };
}

async function call<T>(url: string, method = 'GET', body?: unknown): Promise<T> {
  const res = await send(url, { method, body });
  const parsed = await readJSON(res);
  if (!res.ok) {
    const e = (parsed as CollarErrorBody | null)?.error;
    throw new ApiError(res.status, e?.code ?? 'http_' + res.status, e?.message ?? 'Request failed.', e?.details ?? null);
  }
  return parsed as T;
}

function query(p: ListParams): string {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(p)) {
    if (v !== undefined && v !== '') q.set(k, String(v));
  }
  const s = q.toString();
  return s ? '?' + s : '';
}

export const collar = {
  list: <T>(table: string, p: ListParams = {}) =>
    call<CollarList<T>>(`${COLLAR_BASE}/${table}${query({ ...p, perPage: Math.min(p.perPage ?? 30, 100) })}`),
  view: <T>(table: string, id: string, p: Pick<ListParams, 'fields' | 'expand'> = {}) =>
    call<T>(`${COLLAR_BASE}/${table}/${encodeURIComponent(id)}${query(p)}`),
  create: <T>(table: string, body: unknown) => call<T>(`${COLLAR_BASE}/${table}`, 'POST', body),
  update: <T>(table: string, id: string, body: unknown) => call<T>(`${COLLAR_BASE}/${table}/${encodeURIComponent(id)}`, 'PATCH', body),
};
