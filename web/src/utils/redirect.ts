// Safe "return to where I was" handling for sign-in links. Only same-site
// absolute paths are accepted, never protocol-relative or external URLs.

export function safeNext(raw: unknown, fallback = '/'): string {
  if (typeof raw !== 'string' || raw === '') return fallback;
  if (!raw.startsWith('/') || raw.startsWith('//') || raw.startsWith('/\\')) return fallback;
  if (/[\u0000-\u001f]/.test(raw)) return fallback;
  if (raw.startsWith('/login') || raw.startsWith('/register')) return fallback;
  return raw;
}

export function currentPath(): string {
  return window.location.pathname + window.location.search;
}

export function loginHref(next = currentPath()): string {
  return '/login?next=' + encodeURIComponent(safeNext(next));
}
