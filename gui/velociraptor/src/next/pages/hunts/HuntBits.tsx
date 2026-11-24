import type { Hunt } from '../../lib/types';
import { pct, toDate, toNum } from '../../lib/format';
import { Badge, Progress, Tooltip } from '../../ui/primitives';

export type EffectiveState = 'running' | 'paused' | 'stopped' | 'expired' | 'archived' | 'unknown';

export function effectiveState(h: Hunt): EffectiveState {
  if (h.state === 'ARCHIVED') return 'archived';
  if (h.stats?.stopped || h.state === 'STOPPED') return 'stopped';
  if (h.state === 'PAUSED') return 'paused';
  if (h.state === 'RUNNING') {
    const exp = toDate(h.expires);
    if (exp && exp.getTime() < Date.now()) return 'expired';
    return 'running';
  }
  return 'unknown';
}

const LABEL: Record<EffectiveState, string> = {
  running: 'Running', paused: 'Paused', stopped: 'Stopped', expired: 'Expired', archived: 'Archived', unknown: 'Unknown',
};

export function HuntStateBadge({ hunt }: { hunt: Hunt }) {
  const s = effectiveState(hunt);
  const tone = s === 'running' ? 'accent' : s === 'paused' ? 'warn' : s === 'unknown' ? 'bad' : 'neutral';
  return (
    <Badge tone={tone} className="tw:gap-1.5">
      {s === 'running'
        ? <span className="tw:relative tw:flex tw:size-1.5">
          <span className="tw:absolute tw:inset-0 tw:rounded-full tw:bg-current tw:opacity-60 tw:animate-ping" />
          <span className="tw:relative tw:size-1.5 tw:rounded-full tw:bg-current" />
        </span>
        : <span className="tw:size-1.5 tw:rounded-full tw:bg-current" />}
      {LABEL[s]}
    </Badge>
  );
}

export function huntCounts(h: Hunt) {
  const st = h.stats ?? {};
  const scheduled = toNum(st.total_clients_scheduled);
  const finished = toNum(st.total_finished_clients);
  const errors = toNum(st.total_clients_with_errors);
  const withResults = toNum(st.total_clients_with_results);
  const rows = toNum(st.total_collected_rows);
  const bytes = toNum(st.total_uploaded_bytes);
  // total_finished_clients includes clients that finished with errors.
  const succeeded = Math.max(0, finished - errors);
  const running = Math.max(0, scheduled - finished);
  return { scheduled, finished, succeeded, errors, withResults, rows, bytes, running };
}

export function HuntProgress({ hunt, className }: { hunt: Hunt; className?: string }) {
  const c = huntCounts(hunt);
  if (!c.scheduled) {
    return <Progress value={0} className={className} />;
  }
  const done = pct(c.succeeded, c.scheduled);
  const err = pct(c.errors, c.scheduled);
  return (
    <Tooltip content={`${c.succeeded} succeeded · ${c.errors} failed · ${c.running} in progress · ${c.scheduled} scheduled`}>
      <div className={className}>
        <Progress segments={[
          { value: done, tone: 'ok' },
          { value: err, tone: 'bad' },
        ]} />
      </div>
    </Tooltip>
  );
}

export function huntTitle(h: Hunt): string {
  return h.hunt_description || (h.artifacts ?? h.start_request?.artifacts ?? []).join(', ') || h.hunt_id;
}
