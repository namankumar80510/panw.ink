// Reader preferences (§4.4 ReadingShell, §4.5): stored locally; synced to
// the account in a later phase when signed in. Values are clamped on load so
// a tampered localStorage entry cannot break layout.
import { readJSON, writeJSON } from '../utils/storage';

export type Theme = 'auto' | 'light' | 'dark' | 'sepia';
export type FontFamily = 'serif' | 'sans';

export interface Prefs {
  theme: Theme;
  fontSize: number; // px, 14–26
  lineHeight: number; // 1.45–2.0
  width: number; // px, 560–900
  fontFamily: FontFamily;
  reducedMotion: boolean;
}

const KEY = 'pawn.prefs.v1';

export const DEFAULT_PREFS: Prefs = {
  theme: 'auto',
  fontSize: 19,
  lineHeight: 1.7,
  width: 720,
  fontFamily: 'serif',
  reducedMotion: false,
};

const clamp = (n: unknown, lo: number, hi: number, d: number) =>
  typeof n === 'number' && Number.isFinite(n) ? Math.min(hi, Math.max(lo, n)) : d;

export function sanitize(raw: Partial<Prefs> | null | undefined): Prefs {
  const r = raw ?? {};
  return {
    theme: (['auto', 'light', 'dark', 'sepia'] as const).includes(r.theme as Theme) ? (r.theme as Theme) : DEFAULT_PREFS.theme,
    fontSize: Math.round(clamp(r.fontSize, 14, 26, DEFAULT_PREFS.fontSize)),
    lineHeight: Math.round(clamp(r.lineHeight, 1.45, 2.0, DEFAULT_PREFS.lineHeight) * 100) / 100,
    width: Math.round(clamp(r.width, 560, 900, DEFAULT_PREFS.width)),
    fontFamily: r.fontFamily === 'sans' ? 'sans' : 'serif',
    reducedMotion: r.reducedMotion === true,
  };
}

export const prefs = {
  value: sanitize(readJSON<Partial<Prefs>>(KEY, {})),

  set(patch: Partial<Prefs>): void {
    this.value = sanitize({ ...this.value, ...patch });
    writeJSON(KEY, this.value);
    this.apply();
  },

  /** Applies theme and reader variables to <html>. */
  apply(): void {
    const root = document.documentElement;
    const p = this.value;
    const dark = p.theme === 'dark' || (p.theme === 'auto' && window.matchMedia?.('(prefers-color-scheme: dark)').matches);
    root.setAttribute('data-bs-theme', dark ? 'dark' : 'light');
    if (p.theme === 'sepia') root.setAttribute('data-pawn-theme', 'sepia');
    else root.removeAttribute('data-pawn-theme');
    root.style.setProperty('--pawn-reader-size', p.fontSize + 'px');
    root.style.setProperty('--pawn-reader-leading', String(p.lineHeight));
    root.style.setProperty('--pawn-reader-width', p.width + 'px');
    root.toggleAttribute('data-pawn-sans', p.fontFamily === 'sans');
    root.toggleAttribute('data-pawn-reduced-motion', p.reducedMotion);
  },
};

if (typeof window !== 'undefined' && window.matchMedia) {
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener?.('change', () => {
    if (prefs.value.theme === 'auto') prefs.apply();
  });
}
