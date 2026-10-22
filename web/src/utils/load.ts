// Tiny async loader for page data with consistent error handling.
import m from 'mithril';
import { ApiError } from '../api/http';

export interface Loaded<T> {
  data: T | null;
  error: ApiError | null;
  loading: boolean;
}

/** Runs fn, redraws, and follows MOVED (slug redirect) errors with replaceState. */
export function load<T>(target: Loaded<T>, fn: () => Promise<T>): Promise<void> {
  target.loading = true;
  target.error = null;
  return fn()
    .then((d) => { target.data = d; })
    .catch((e: unknown) => {
      const err = e instanceof ApiError ? e : new ApiError(0, 'UNKNOWN', 'Something went wrong. Please try again.');
      const loc = (err.details as { location?: string } | null)?.location;
      if (err.code === 'MOVED' && loc && loc.startsWith('/') && !loc.startsWith('//')) {
        m.route.set(loc, {}, { replace: true });
        return;
      }
      target.error = err;
    })
    .finally(() => { target.loading = false; m.redraw(); });
}

export function loaded<T>(): Loaded<T> {
  return { data: null, error: null, loading: true };
}
