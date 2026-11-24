import type { TableResponse } from './types';
import { toNum } from './format';

export type Row = Record<string, unknown>;

export interface ParsedTable {
  columns: string[];
  rows: Row[];
  total: number;
}

// Converts the GetTable style response (columns + JSON encoded row
// arrays) into an array of objects.
export function parseTable(resp: TableResponse | undefined): ParsedTable {
  const columns = resp?.columns ?? [];
  const rows: Row[] = [];
  for (const r of resp?.rows ?? []) {
    let cells: unknown[] = [];
    if (r.json) {
      try {
        cells = JSON.parse(r.json);
      } catch {
        cells = [];
      }
    } else if (r.cell) {
      cells = r.cell;
    }
    const row: Row = {};
    columns.forEach((c, i) => {
      row[c] = cells[i];
    });
    rows.push(row);
  }
  return { columns, rows, total: toNum(resp?.total_rows) };
}

// Turns rows into CSV text for export.
export function toCSV(columns: string[], rows: Row[]): string {
  const esc = (v: unknown) => {
    let s: string;
    if (v === null || v === undefined) s = '';
    else if (typeof v === 'object') s = JSON.stringify(v);
    else s = String(v);
    if (/[",\n\r]/.test(s)) s = `"${s.replace(/"/g, '""')}"`;
    return s;
  };
  const lines = [columns.map(esc).join(',')];
  for (const r of rows) lines.push(columns.map(c => esc(r[c])).join(','));
  return lines.join('\n');
}

export function downloadText(filename: string, text: string, mime = 'text/plain') {
  const blob = new Blob([text], { type: mime });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
