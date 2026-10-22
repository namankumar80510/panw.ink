// Time-zone conversion for scheduling: the author picks a wall-clock time in
// an IANA zone; the server only ever receives UTC (§11.1 step 7). Uses Intl
// only (no tz database bundled), and handles DST gaps/overlaps.

/** Offset (ms) of `zone` from UTC at instant `t`. */
export function zoneOffset(t: number, zone: string): number {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: zone, hourCycle: 'h23', year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit',
  }).formatToParts(new Date(t));
  const get = (k: string) => Number(parts.find((p) => p.type === k)?.value);
  const asUTC = Date.UTC(get('year'), get('month') - 1, get('day'), get('hour') % 24, get('minute'), get('second'));
  return asUTC - Math.floor(t / 1000) * 1000;
}

/**
 * Converts "YYYY-MM-DDTHH:mm" in `zone` to a UTC Date. In a DST gap (a
 * wall time that does not exist) the result moves forward past the gap; in
 * an overlap the earlier instant is chosen.
 */
export function wallToUTC(local: string, zone: string): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/.exec(local);
  if (!m) return null;
  const wall = Date.UTC(+m[1]!, +m[2]! - 1, +m[3]!, +m[4]!, +m[5]!);
  // The offsets in force half a day either side bracket any DST change.
  const offsets = [zoneOffset(wall - 12 * 3600_000, zone), zoneOffset(wall + 12 * 3600_000, zone)];
  const valid = offsets.map((o) => wall - o).filter((t) => t + zoneOffset(t, zone) === wall);
  if (valid.length) return new Date(Math.min(...valid)); // overlap: the earlier instant
  return new Date(wall - Math.min(...offsets)); // gap: just after the skipped hour
}

/** Formats a UTC instant as "YYYY-MM-DDTHH:mm" wall time in `zone`. */
export function utcToWall(d: Date, zone: string): string {
  const shifted = new Date(d.getTime() + zoneOffset(d.getTime(), zone));
  return shifted.toISOString().slice(0, 16);
}

export function browserZone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  } catch {
    return 'UTC';
  }
}
