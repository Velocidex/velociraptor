import type { Num } from './types';

export function toNum(v: Num | null | boolean): number {
  if (v === undefined || v === null || v === '' || typeof v === 'boolean') return 0;
  const n = typeof v === 'number' ? v : Number(v);
  return Number.isFinite(n) ? n : 0;
}

// Velociraptor timestamps come in seconds, milliseconds, microseconds
// or nanoseconds depending on the API. Normalise to a Date by
// magnitude.
export function toDate(v: Num | null): Date | null {
  const n = toNum(v);
  if (!n) return null;
  let ms: number;
  if (n > 1e17) ms = n / 1e6;        // ns
  else if (n > 1e14) ms = n / 1e3;   // us
  else if (n > 1e11) ms = n;         // ms
  else ms = n * 1000;                // s
  return new Date(ms);
}

const nf = new Intl.NumberFormat();
export function fmtNum(v: Num | null): string {
  return nf.format(toNum(v));
}

const compact = new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 });
export function fmtCompact(v: Num | null): string {
  return compact.format(toNum(v));
}

export function fmtBytes(v: Num | null): string {
  let n = toNum(v);
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  let i = 0;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i++;
  }
  return `${n >= 100 || i === 0 ? Math.round(n) : n.toFixed(1)} ${units[i]}`;
}

export function fmtDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return '';
  if (seconds < 1) return `${Math.round(seconds * 1000)} ms`;
  if (seconds < 60) return `${Math.round(seconds)}s`;
  const m = Math.floor(seconds / 60);
  if (m < 60) return `${m}m ${Math.round(seconds % 60)}s`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ${m % 60}m`;
  const d = Math.floor(h / 24);
  return `${d}d ${h % 24}h`;
}

const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto', style: 'short' });
export function timeAgo(v: Num | Date | null, now = Date.now()): string {
  const d = v instanceof Date ? v : toDate(v);
  if (!d) return '';
  const diff = (d.getTime() - now) / 1000;
  const abs = Math.abs(diff);
  if (abs < 10) return 'just now';
  if (abs < 60) return rtf.format(Math.round(diff), 'second');
  if (abs < 3600) return rtf.format(Math.round(diff / 60), 'minute');
  if (abs < 86400) return rtf.format(Math.round(diff / 3600), 'hour');
  if (abs < 86400 * 30) return rtf.format(Math.round(diff / 86400), 'day');
  if (abs < 86400 * 365) return rtf.format(Math.round(diff / (86400 * 30)), 'month');
  return rtf.format(Math.round(diff / (86400 * 365)), 'year');
}

function userTimezone(): string | undefined {
  const tz = (window as unknown as { globals?: { timezone?: string } }).globals?.timezone;
  return tz || undefined;
}

export function fmtTime(v: Num | Date | null): string {
  const d = v instanceof Date ? v : toDate(v);
  if (!d) return '';
  try {
    return new Intl.DateTimeFormat(undefined, {
      year: 'numeric', month: 'short', day: '2-digit',
      hour: '2-digit', minute: '2-digit', second: '2-digit',
      hour12: false, timeZone: userTimezone(),
    }).format(d);
  } catch {
    return d.toISOString();
  }
}

export function isoTime(v: Num | Date | null): string {
  const d = v instanceof Date ? v : toDate(v);
  return d ? d.toISOString() : '';
}

export function pct(part: Num, total: Num): number {
  const t = toNum(total);
  if (!t) return 0;
  return Math.min(100, Math.max(0, (toNum(part) / t) * 100));
}

// A client is considered online if it was seen in the last 15
// minutes, matching the legacy UI's green/yellow/red dots.
export function clientStatus(lastSeen: Num): 'online' | 'recent' | 'offline' {
  const d = toDate(lastSeen);
  if (!d) return 'offline';
  const age = (Date.now() - d.getTime()) / 1000;
  if (age < 15 * 60) return 'online';
  if (age < 24 * 3600) return 'recent';
  return 'offline';
}

export function cellToString(v: unknown): string {
  if (v === null || v === undefined) return '';
  if (typeof v === 'string') return v;
  if (typeof v === 'number' || typeof v === 'boolean') return String(v);
  return JSON.stringify(v);
}
