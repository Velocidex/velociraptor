// Asks for a time window (and drive) before collecting the MFT, so only
// entries with a timestamp inside the window come back instead of the
// whole volume.
import { useMemo, useState } from 'react';
import { HardDrive } from 'lucide-react';
import { Button, Dialog, Input, cn } from '../../ui/primitives';

export interface MftWindow {
  DateAfter: string;
  DateBefore: string;
  MFTDrive: string;
}

interface Preset { label: string; hours: number }
const PRESETS: Preset[] = [
  { label: 'Last hour', hours: 1 },
  { label: 'Last 24 hours', hours: 24 },
  { label: 'Last 3 days', hours: 72 },
  { label: 'Last 7 days', hours: 168 },
  { label: 'Last 30 days', hours: 720 },
];

const HOUR_MS = 3_600_000;
const DRIVE_RE = /^[A-Za-z]:$/;

// <input type="datetime-local"> works in local time without a zone.
function toLocalInput(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function parseLocal(v: string): Date | null {
  if (!v) return null;
  const d = new Date(v);
  return Number.isNaN(d.getTime()) ? null : d;
}

interface MftWindowDialogProps {
  hostname: string;
  onClose: () => void;
  onConfirm: (w: MftWindow) => Promise<void>;
}

export function MftWindowDialog({ hostname, onClose, onConfirm }: MftWindowDialogProps) {
  const [start, setStart] = useState(() => toLocalInput(new Date(Date.now() - 24 * HOUR_MS)));
  const [end, setEnd] = useState(() => toLocalInput(new Date()));
  const [drive, setDrive] = useState('C:');
  const [busy, setBusy] = useState(false);

  const error = useMemo(() => {
    const s = parseLocal(start);
    const e = parseLocal(end);
    if (!s || !e) return 'Pick both a start and an end time.';
    if (s >= e) return 'The start must be before the end.';
    if (!DRIVE_RE.test(drive.trim())) return 'Drive must look like C:';
    return '';
  }, [start, end, drive]);

  const applyPreset = (hours: number) => {
    const now = new Date();
    setEnd(toLocalInput(now));
    setStart(toLocalInput(new Date(now.getTime() - hours * HOUR_MS)));
  };

  const submit = async () => {
    const s = parseLocal(start);
    const e = parseLocal(end);
    if (error || !s || !e) return;
    setBusy(true);
    try {
      await onConfirm({
        DateAfter: s.toISOString(),
        DateBefore: e.toISOString(),
        MFTDrive: drive.trim().toUpperCase(),
      });
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open onOpenChange={v => !v && onClose()} title={`Collect MFT from ${hostname}`}
      description="Only MFT entries created, modified or changed inside this window are collected."
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancel</Button>
        <Button variant="primary" icon={<HardDrive className="tw:size-3.5" />} loading={busy} disabled={!!error} onClick={submit}>
          Collect MFT
        </Button>
      </>}>
      <div className="tw:space-y-4">
        <div className="tw:flex tw:flex-wrap tw:gap-1.5">
          {PRESETS.map(p => (
            <button key={p.hours} type="button" onClick={() => applyPreset(p.hours)}
              className="tw:rounded-md tw:border tw:border-line tw:px-2 tw:py-1 tw:text-xs tw:hover:border-accent tw:hover:bg-accent-soft">
              {p.label}
            </button>
          ))}
        </div>
        <div className="tw:grid tw:grid-cols-1 tw:sm:grid-cols-2 tw:gap-3">
          <label className="tw:block">
            <span className="tw:block tw:text-xs tw:font-medium tw:mb-1.5">From</span>
            <Input type="datetime-local" value={start} onChange={e => setStart(e.target.value)} />
          </label>
          <label className="tw:block">
            <span className="tw:block tw:text-xs tw:font-medium tw:mb-1.5">To</span>
            <Input type="datetime-local" value={end} onChange={e => setEnd(e.target.value)} />
          </label>
        </div>
        <label className="tw:block tw:max-w-[120px]">
          <span className="tw:block tw:text-xs tw:font-medium tw:mb-1.5">Drive</span>
          <Input value={drive} onChange={e => setDrive(e.target.value)} className="tw:font-mono" />
        </label>
        <p className={cn('tw:text-2xs', error ? 'tw:text-bad' : 'tw:text-muted')}>
          {error || `Times are in your local time zone (${Intl.DateTimeFormat().resolvedOptions().timeZone}) and sent to the client as UTC.`}
        </p>
      </div>
    </Dialog>
  );
}
