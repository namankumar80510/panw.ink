// Display helpers. The server is authoritative in UTC; times are shown in the
// signed-in reader's time zone, else the browser's.
import { session } from '../stores/session';

function zone(): string | undefined {
  return session.user?.timezone || undefined;
}

export function formatDate(ts: string | null | undefined, withTime = false): string {
  if (!ts) return '';
  try {
    return new Intl.DateTimeFormat(undefined, withTime ? { dateStyle: 'medium', timeStyle: 'short', timeZone: zone() } : { dateStyle: 'medium', timeZone: zone() }).format(new Date(ts));
  } catch {
    return ts.slice(0, 10);
  }
}

export function formatCount(n: number): string {
  return new Intl.NumberFormat().format(n);
}

export function minutes(seconds: number): string {
  const m = Math.max(1, Math.round(seconds / 60));
  return `${m} min read`;
}

export function workHref(slug: string): string {
  return '/fiction/' + slug;
}

export function entryHref(kind: 'novel' | 'short_story', workSlug: string, entrySlug: string): string {
  return kind === 'short_story' ? `/stories/${workSlug}/read` : `/fiction/${workSlug}/ch/${entrySlug}`;
}

export const STATUS_LABEL: Record<string, string> = {
  ongoing: 'Ongoing', completed: 'Complete', hiatus: 'On hiatus', cancelled: 'Discontinued',
};
