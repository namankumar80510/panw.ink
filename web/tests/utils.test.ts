import { describe, expect, it } from 'vitest';
import { safeNext } from '../src/utils/redirect';
import { DEFAULT_PREFS, sanitize } from '../src/stores/prefs';

describe('safeNext', () => {
  it('keeps same-site paths', () => {
    expect(safeNext('/fiction/holunkai/ch/the-last-breath')).toBe('/fiction/holunkai/ch/the-last-breath');
    expect(safeNext('/search?q=a')).toBe('/search?q=a');
  });
  it('rejects external, protocol-relative and odd targets', () => {
    for (const bad of ['https://evil.example', '//evil.example', '/\\evil.example', 'javascript:alert(1)', '/x\u0000', '', undefined, 42]) {
      expect(safeNext(bad)).toBe('/');
    }
  });
  it('never loops back into sign-in', () => {
    expect(safeNext('/login?next=/x')).toBe('/');
    expect(safeNext('/register')).toBe('/');
  });
});

describe('prefs.sanitize', () => {
  it('clamps reader settings to the allowed ranges', () => {
    const p = sanitize({ fontSize: 99, lineHeight: 0.5, width: 10, theme: 'neon' as never, fontFamily: 'comic' as never });
    expect(p.fontSize).toBe(26);
    expect(p.lineHeight).toBe(1.45);
    expect(p.width).toBe(560);
    expect(p.theme).toBe(DEFAULT_PREFS.theme);
    expect(p.fontFamily).toBe('serif');
  });
  it('fills defaults for missing or corrupt data', () => {
    expect(sanitize(null)).toEqual(DEFAULT_PREFS);
    expect(sanitize({ fontSize: Number.NaN })).toEqual(DEFAULT_PREFS);
  });
});
