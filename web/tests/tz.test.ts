import { describe, expect, it } from 'vitest';
import { utcToWall, wallToUTC } from '../src/utils/tz';

describe('wallToUTC', () => {
  it('converts plain offsets', () => {
    expect(wallToUTC('2026-11-01T09:30', 'UTC')!.toISOString()).toBe('2026-11-01T09:30:00.000Z');
    expect(wallToUTC('2026-11-01T09:30', 'Asia/Kolkata')!.toISOString()).toBe('2026-11-01T04:00:00.000Z');
  });
  it('handles both sides of a DST change', () => {
    // New York: EDT (UTC-4) until 2026-11-01 02:00, then EST (UTC-5).
    expect(wallToUTC('2026-10-31T12:00', 'America/New_York')!.toISOString()).toBe('2026-10-31T16:00:00.000Z');
    expect(wallToUTC('2026-11-02T12:00', 'America/New_York')!.toISOString()).toBe('2026-11-02T17:00:00.000Z');
  });
  it('picks the earlier instant in an overlap and skips a gap', () => {
    expect(wallToUTC('2026-11-01T01:30', 'America/New_York')!.toISOString()).toBe('2026-11-01T05:30:00.000Z');
    // 2026-03-08 02:30 does not exist in New York; it lands after the gap.
    expect(wallToUTC('2026-03-08T02:30', 'America/New_York')!.toISOString()).toBe('2026-03-08T07:30:00.000Z');
  });
  it('round-trips', () => {
    const d = new Date('2026-07-15T18:45:00Z');
    expect(wallToUTC(utcToWall(d, 'Europe/London'), 'Europe/London')!.toISOString()).toBe(d.toISOString());
  });
  it('rejects malformed input', () => {
    expect(wallToUTC('tomorrow', 'UTC')).toBeNull();
  });
});
