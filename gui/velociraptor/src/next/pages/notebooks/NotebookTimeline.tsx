// Super-timelines in notebooks ({{ Timeline "name" }}): several sources
// merged on one clock. A coverage strip shows where each source has
// events (click it to jump), chips toggle sources, and the merged events
// page forward in time. Any event can carry an analyst note.
import { useMemo, useState } from 'react';
import { keepPreviousData, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { ChevronLeft, ChevronRight, ChevronsLeft, Clock, MessageSquarePlus, StickyNote, Trash2 } from 'lucide-react';
import * as api from '../../lib/api';
import { parseTable, type Row } from '../../lib/table';
import { fmtNum, fmtTime, toDate, toNum } from '../../lib/format';
import type { Num, TableResponse } from '../../lib/types';
import { Button, Dialog, Field, IconButton, Input, Skeleton, Textarea, cn } from '../../ui/primitives';
import { DataTable } from '../../ui/DataTable';

interface TimelineInfo { id: string; start_time?: Num; end_time?: Num; active?: boolean }
interface Pos { ns: number; skip: number }
interface TimelineResponse extends TableResponse { start_time?: Num; end_time?: Num; timelines?: TimelineInfo[] }

const COLORS = ['var(--rx-accent)', 'var(--rx-info)', 'var(--rx-ok)', 'var(--rx-warn)', '#a855f7', '#14b8a6', '#f97316', 'var(--rx-bad)'];
const PAGE = 50;

const nsOf = (v: unknown) => {
  const d = toDate(v as Num);
  return d ? d.getTime() * 1e6 : 0;
};

export default function NotebookTimeline({ notebookId, name }: { notebookId: string; name: string }) {
  const qc = useQueryClient();
  // Many events can share one timestamp, so a page position is a time
  // plus how many events at exactly that time were already shown.
  const [pos, setPos] = useState<Pos>({ ns: 0, skip: 0 });
  const [history, setHistory] = useState<Pos[]>([]);
  const [skip, setSkip] = useState<Set<string>>(new Set());
  const [filter, setFilter] = useState('');
  const [annotate, setAnnotate] = useState<Row | null>(null);

  const q = useQuery({
    queryKey: ['timeline', notebookId, name, pos, [...skip].sort(), filter],
    queryFn: ({ signal }) => api.get<TimelineResponse>('v1/GetTable', {
      type: 'TIMELINE', timeline: name, notebook_id: notebookId, start_time: pos.ns, rows: PAGE + pos.skip,
      skip_components: [...skip], filter_column: filter ? 'Message' : undefined, filter_regex: filter || undefined,
    }, signal),
    placeholderData: keepPreviousData,
  });

  const full = useMemo(() => parseTable(q.data), [q.data]);
  const table = useMemo(() => ({ ...full, rows: full.rows.slice(pos.skip) }), [full, pos.skip]);
  const timelines = useMemo(() => {
    const t = [...(q.data?.timelines ?? [])];
    if (!t.some(x => x.id === 'Annotation')) t.push({ id: 'Annotation' });
    return t;
  }, [q.data]);
  const colorOf = (id: string) => COLORS[Math.max(0, timelines.findIndex(t => t.id === id)) % COLORS.length];

  // Overall span across sources, in seconds.
  const span = useMemo(() => {
    const s = timelines.map(t => toNum(t.start_time ?? 0)).filter(Boolean);
    const e = timelines.map(t => toNum(t.end_time ?? 0)).filter(Boolean);
    if (!s.length) return null;
    const lo = Math.min(...s);
    const hi = Math.max(...e, lo + 1);
    return { lo, hi };
  }, [timelines]);

  const go = (ns: number, skipRows = 0) => {
    setHistory(h => [...h, pos]);
    setPos({ ns, skip: skipRows });
  };
  const next = () => {
    const last = full.rows[full.rows.length - 1];
    if (!last) return;
    const lastMs = Math.floor(nsOf(last.Timestamp) / 1e6);
    const end = toNum(q.data?.end_time ?? 0) || nsOf(last.Timestamp);
    const same = full.rows.filter(r => Math.floor(nsOf(r.Timestamp) / 1e6) === lastMs).length;
    go(end, same);
  };
  const prev = () => {
    const n = [...history];
    setPos(n.pop() ?? { ns: 0, skip: 0 });
    setHistory(n);
  };

  const current = table.rows[0]?.Timestamp;
  const cursor = span && current ? (nsOf(current) / 1e9 - span.lo) / (span.hi - span.lo) : null;

  const removeNote = async (r: Row) => {
    try {
      await api.post('v1/AnnotateTimeline', {
        notebook_id: notebookId, super_timeline: name,
        event_json: JSON.stringify({ _AnnotationID: r._AnnotationID, _OriginalTime: r.Timestamp }),
      });
      qc.invalidateQueries({ queryKey: ['timeline', notebookId, name] });
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  const columns = useMemo(() => {
    const cols = table.columns.filter(c => c !== '_Source' && !c.startsWith('_'));
    const notes = cols.includes('Notes') ? ['Notes'] : [];
    return ['Note', 'Timestamp', '_Source', ...notes, ...cols.filter(c => c !== 'Timestamp' && c !== 'Notes')];
  }, [table.columns]);

  return (
    <div className="tw:not-prose tw:my-1 tw:-mx-4 tw:border-y tw:border-line" onClick={e => e.stopPropagation()}>
      <div className="tw:px-4 tw:py-3 tw:flex tw:flex-col tw:gap-3 tw:bg-canvas/40">
        <div className="tw:flex tw:items-center tw:gap-2 tw:flex-wrap">
          <Clock className="tw:size-4 tw:text-faint" />
          <span className="tw:text-[13px] tw:font-semibold">Timeline: {name}</span>
          <div className="tw:flex tw:flex-wrap tw:gap-1 tw:ml-2">
            {timelines.map(t => {
              const off = skip.has(t.id);
              return (
                <button key={t.id} onClick={() => setSkip(s => { const n = new Set(s); if (off) n.delete(t.id); else n.add(t.id); return n; })}
                  className={cn('tw:h-6 tw:px-2 tw:rounded-full tw:border tw:text-2xs tw:flex tw:items-center tw:gap-1.5 tw:transition-opacity',
                    off ? 'tw:border-line tw:opacity-50 tw:line-through' : 'tw:border-line-strong')}>
                  <span className="tw:size-2 tw:rounded-full" style={{ background: colorOf(t.id) }} />{t.id}
                </button>
              );
            })}
          </div>
        </div>

        {span && (
          <div className="tw:relative tw:rounded-md tw:border tw:border-line tw:bg-panel tw:px-2 tw:py-1.5 tw:cursor-crosshair"
            title="Click to jump to that time"
            onClick={e => {
              const r = (e.currentTarget as HTMLDivElement).getBoundingClientRect();
              const f = Math.min(1, Math.max(0, (e.clientX - r.left - 8) / (r.width - 16)));
              go(Math.floor((span.lo + f * (span.hi - span.lo)) * 1e9));
            }}>
            {timelines.filter(t => t.start_time).map(t => {
              const a = (toNum(t.start_time!) - span.lo) / (span.hi - span.lo);
              const b = (toNum(t.end_time ?? t.start_time!) - span.lo) / (span.hi - span.lo);
              return (
                <div key={t.id} className="tw:relative tw:h-2.5 tw:my-0.5">
                  <div className="tw:absolute tw:h-full tw:rounded-sm"
                    style={{ left: `${a * 100}%`, width: `max(${(b - a) * 100}%, 3px)`, background: colorOf(t.id), opacity: skip.has(t.id) ? 0.25 : 0.8 }} />
                </div>
              );
            })}
            {cursor !== null && cursor >= 0 && cursor <= 1 && (
              <div className="tw:absolute tw:top-0 tw:bottom-0 tw:w-0.5 tw:bg-fg" style={{ left: `calc(8px + ${cursor} * (100% - 16px))` }} />
            )}
            <div className="tw:flex tw:justify-between tw:text-2xs tw:text-faint tw:mt-1">
              <span>{fmtTime(span.lo)}</span><span>{fmtTime(span.hi)}</span>
            </div>
          </div>
        )}

        <div className="tw:flex tw:items-center tw:gap-1.5 tw:flex-wrap">
          <IconButton size="sm" label="Back to the start" icon={<ChevronsLeft />} onClick={() => { setHistory([]); setPos({ ns: 0, skip: 0 }); }} />
          <IconButton size="sm" label="Previous page" icon={<ChevronLeft />} disabled={!history.length} onClick={prev} />
          <IconButton size="sm" label="Next page" icon={<ChevronRight />} disabled={table.rows.length < PAGE} onClick={next} />
          <Input type="datetime-local" step={1} className="tw:h-7 tw:w-56 tw:text-xs"
            onChange={e => { const d = new Date(e.target.value); if (!Number.isNaN(d.getTime())) go(d.getTime() * 1e6); }} />
          <Input value={filter} onChange={e => setFilter(e.target.value)} placeholder="Filter messages (regex)" className="tw:h-7 tw:w-56 tw:text-xs" />
          <span className="tw:ml-auto tw:text-2xs tw:text-faint">
            {table.rows.length ? `${fmtTime(table.rows[0].Timestamp as string)} → ${fmtTime(table.rows[table.rows.length - 1].Timestamp as string)}` : ''}
          </span>
        </div>
      </div>
      <div style={{ height: Math.min(Math.max(table.rows.length, 3) * 30 + 84, 560) }}>
        {q.isLoading ? <Skeleton className="tw:h-full" /> : (
          <DataTable rows={table.rows} columns={columns} dense hideSearch exportName={`timeline-${name}`}
            columnLabels={{ _Source: 'Source', Note: '' }} columnWidths={{ Note: 40, Timestamp: 172, _Source: 130, Notes: 260 }}
            shownColumns={['_Source']}
            renderers={{
              Note: (_v, r) => r._Source === 'Annotation'
                ? <IconButton size="xs" label="Delete note" icon={<Trash2 />} onClick={e => { e.stopPropagation(); removeNote(r); }} />
                : <IconButton size="xs" label="Add a note to this event" icon={<MessageSquarePlus />} onClick={e => { e.stopPropagation(); setAnnotate(r); }} />,
              _Source: v => (
                <span className="tw:flex tw:items-center tw:gap-1.5 tw:text-xs">
                  <span className="tw:size-2 tw:rounded-full tw:shrink-0" style={{ background: colorOf(String(v)) }} />
                  {v === 'Annotation' ? <span className="tw:flex tw:items-center tw:gap-1 tw:text-warn"><StickyNote className="tw:size-3" />Note</span> : String(v)}
                </span>
              ),
              Notes: v => v ? <span className="tw:text-warn tw:font-medium">{String(v)}</span> : <span className="tw:text-faint">–</span>,
              Timestamp: v => <span className="tw:text-muted tw:whitespace-nowrap">{fmtTime(v as string)}</span>,
            }}
            empty={<div className="tw:p-6 tw:text-xs tw:text-faint tw:text-center">No events from this point on.</div>} />
        )}
      </div>
      {table.rows.length > 0 && (
        <div className="tw:px-4 tw:py-1 tw:text-2xs tw:text-faint">{fmtNum(table.rows.length)} events on this page</div>
      )}
      {annotate && <AnnotateDialog notebookId={notebookId} timeline={name} event={annotate} onClose={() => setAnnotate(null)}
        onSaved={() => qc.invalidateQueries({ queryKey: ['timeline', notebookId, name] })} />}
    </div>
  );
}

function AnnotateDialog({ notebookId, timeline, event, onClose, onSaved }: {
  notebookId: string; timeline: string; event: Row; onClose: () => void; onSaved: () => void;
}) {
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);
  const save = async () => {
    setBusy(true);
    try {
      await api.post('v1/AnnotateTimeline', {
        notebook_id: notebookId, super_timeline: timeline, timestamp: nsOf(event.Timestamp), note,
        event_json: JSON.stringify(event),
      });
      toast.success('Note added to the timeline');
      onSaved();
      onClose();
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title="Add a note" width={520}
      description={<>{fmtTime(event.Timestamp as string)} · {String(event.Message ?? '')}</>}
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancel</Button>
        <Button variant="primary" disabled={!note.trim()} loading={busy} onClick={save}>Add note</Button>
      </>}>
      <Field label="Note" hint="Notes appear as their own source in this timeline.">
        <Textarea rows={4} value={note} onChange={e => setNote(e.target.value)} autoFocus placeholder="Why does this event matter?" />
      </Field>
    </Dialog>
  );
}
