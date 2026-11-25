import { useEffect, useMemo, useRef, useState } from 'react';
import { useHistory, useLocation, useParams } from 'react-router-dom';
import { useVirtualizer } from '@tanstack/react-virtual';
import { toast } from 'sonner';
import { Crosshair, Plus, Play, Square, Archive, Trash2, X, Tag, User } from 'lucide-react';
import {
  useArchiveHunts, useDeleteHunts, useHunts, useStartHunts, useStopHunts, usePermission, useTraits,
} from '../../lib/queries';
import type { Hunt } from '../../lib/types';
import { fmtCompact, timeAgo } from '../../lib/format';
import {
  Button, Checkbox, cn, ConfirmDialog, EmptyState, IconButton, SearchInput, Segmented, Skeleton, Switch,
  Tooltip, Kbd,
} from '../../ui/primitives';
import { effectiveState, HuntProgress, HuntStateBadge, huntCounts, huntTitle, type EffectiveState } from './HuntBits';
import HuntDetail from './HuntDetail';
import NewHuntDialog from './NewHuntDialog';
import { MobileBack } from '../../ui/MobileBack';

type Filter = 'all' | 'running' | 'paused' | 'stopped';

function matchesFilter(h: Hunt, f: Filter): boolean {
  if (f === 'all') return true;
  const s: EffectiveState = effectiveState(h);
  if (f === 'stopped') return s === 'stopped' || s === 'expired' || s === 'archived';
  return s === f;
}

function HuntRow({ hunt, selected, checked, onCheck, onClick }: {
  hunt: Hunt; selected: boolean; checked: boolean; onCheck: (v: boolean) => void; onClick: () => void;
}) {
  const c = huntCounts(hunt);
  const artifacts = hunt.artifacts ?? hunt.start_request?.artifacts ?? [];
  return (
    <div onClick={onClick} role="button" tabIndex={-1}
      className={cn(
        'tw:group tw:relative tw:flex tw:gap-2.5 tw:px-3 tw:py-2.5 tw:border-b tw:border-line tw:cursor-pointer tw:h-full',
        selected ? 'tw:bg-accent-soft' : 'tw:hover:bg-hover')}>
      {selected && <span className="tw:absolute tw:left-0 tw:top-0 tw:bottom-0 tw:w-0.5 tw:bg-accent" />}
      <div className={cn('tw:pt-0.5', !checked && 'tw:opacity-0 tw:group-hover:opacity-100')}>
        <Checkbox checked={checked} onChange={onCheck} label={`Select ${hunt.hunt_id}`} />
      </div>
      <div className="tw:min-w-0 tw:flex-1 tw:space-y-1.5">
        <div className="tw:flex tw:items-center tw:gap-2">
          <span className="tw:font-medium tw:text-[13px] tw:truncate tw:flex-1">{huntTitle(hunt)}</span>
          <HuntStateBadge hunt={hunt} />
        </div>
        <div className="tw:flex tw:items-center tw:gap-1.5 tw:text-2xs tw:text-faint tw:min-w-0">
          <span className="tw:font-mono tw:shrink-0">{hunt.hunt_id}</span>
          {artifacts.length > 0 && <span className="tw:truncate tw:font-mono">· {artifacts.join(', ')}</span>}
        </div>
        <HuntProgress hunt={hunt} />
        <div className="tw:flex tw:items-center tw:gap-2 tw:text-2xs tw:text-muted">
          <span className="rx-num"><b className="tw:text-fg tw:font-semibold">{c.finished}</b>/{c.scheduled} clients</span>
          {c.errors > 0 && <span className="tw:text-bad rx-num">{c.errors} failed</span>}
          <span className="rx-num">{fmtCompact(c.rows)} rows</span>
          <span className="tw:ml-auto tw:truncate">{timeAgo(hunt.create_time)} · {hunt.creator}</span>
        </div>
        {(hunt.tags?.length ?? 0) > 0 && (
          <div className="tw:flex tw:flex-wrap tw:gap-1">
            {hunt.tags!.map(t => (
              <span key={t} className="tw:inline-flex tw:items-center tw:gap-1 tw:rounded tw:bg-hover tw:px-1.5 tw:text-2xs tw:text-muted">
                <Tag className="tw:size-2.5" />{t}
              </span>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

export default function HuntsPage() {
  const { hunt_id, tab } = useParams<{ hunt_id?: string; tab?: string }>();
  const history = useHistory();
  const location = useLocation();
  const { traits } = useTraits();
  const canHunt = usePermission('start_hunt');

  const [includeArchived, setIncludeArchived] = useState(false);
  const { data: hunts = [], isLoading, error } = useHunts(includeArchived);

  const [q, setQ] = useState('');
  const [filter, setFilter] = useState<Filter>('all');
  const [mine, setMine] = useState(false);
  const [tagFilter, setTagFilter] = useState<string | null>(null);
  const [checked, setChecked] = useState<Set<string>>(new Set());
  const [newOpen, setNewOpen] = useState(false);
  const [copyFrom, setCopyFrom] = useState<Hunt | undefined>();
  const [confirm, setConfirm] = useState<null | 'archive' | 'delete'>(null);

  const startHunts = useStartHunts();
  const stopHunts = useStopHunts();
  const archiveHunts = useArchiveHunts();
  const deleteHunts = useDeleteHunts();

  // ?new=1 opens the new hunt form (used by the command palette).
  useEffect(() => {
    const params = new URLSearchParams(location.search);
    if (params.get('new')) {
      setCopyFrom(undefined);
      setNewOpen(true);
      history.replace(location.pathname);
    }
  }, [location.search, location.pathname, history]);

  const counts = useMemo(() => {
    const c = { all: hunts.length, running: 0, paused: 0, stopped: 0 };
    for (const h of hunts) {
      const s = effectiveState(h);
      if (s === 'running') c.running++;
      else if (s === 'paused') c.paused++;
      else c.stopped++;
    }
    return c;
  }, [hunts]);

  const allTags = useMemo(() => {
    const m = new Map<string, number>();
    for (const h of hunts) for (const t of h.tags ?? []) m.set(t, (m.get(t) ?? 0) + 1);
    return [...m.entries()].sort((a, b) => b[1] - a[1]).map(([t]) => t);
  }, [hunts]);

  const visible = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return hunts.filter(h => {
      if (!matchesFilter(h, filter)) return false;
      if (mine && h.creator !== traits?.username) return false;
      if (tagFilter && !(h.tags ?? []).includes(tagFilter)) return false;
      if (!needle) return true;
      const hay = `${h.hunt_id} ${h.hunt_description ?? ''} ${h.creator ?? ''} ${(h.artifacts ?? []).join(' ')} ${(h.tags ?? []).join(' ')}`.toLowerCase();
      return hay.includes(needle);
    });
  }, [hunts, q, filter, mine, tagFilter, traits?.username]);

  const selected = hunts.find(h => h.hunt_id === hunt_id);

  // Keyboard: j/k to move through the list, n for new hunt.
  const listRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return;
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      const idx = visible.findIndex(h => h.hunt_id === hunt_id);
      if (e.key === 'j' || e.key === 'ArrowDown') {
        const next = visible[Math.min(idx + 1, visible.length - 1)];
        if (next) history.push(`/hunts/${next.hunt_id}${tab ? `/${tab}` : ''}`);
        e.preventDefault();
      } else if (e.key === 'k' || e.key === 'ArrowUp') {
        const prev = visible[Math.max(idx - 1, 0)];
        if (prev) history.push(`/hunts/${prev.hunt_id}${tab ? `/${tab}` : ''}`);
        e.preventDefault();
      } else if (e.key === 'n' && canHunt) {
        setCopyFrom(undefined);
        setNewOpen(true);
        e.preventDefault();
      } else if (e.key === '/') {
        searchRef.current?.focus();
        e.preventDefault();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [visible, hunt_id, tab, history, canHunt]);

  const virtualizer = useVirtualizer({
    count: visible.length,
    getScrollElement: () => listRef.current,
    estimateSize: i => ((visible[i]?.tags?.length ?? 0) > 0 ? 118 : 96),
    overscan: 8,
  });

  // Keep the selected hunt in view when navigating with the keyboard.
  useEffect(() => {
    const idx = visible.findIndex(h => h.hunt_id === hunt_id);
    if (idx >= 0) virtualizer.scrollToIndex(idx, { align: 'auto' });
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hunt_id]);

  const checkedIds = [...checked].filter(id => hunts.some(h => h.hunt_id === id));
  const allVisibleChecked = visible.length > 0 && visible.every(h => checked.has(h.hunt_id));

  const bulk = async (fn: () => Promise<unknown>, msg: string) => {
    try {
      await fn();
      toast.success(msg);
      setChecked(new Set());
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  return (
    <div className="rx-split tw:flex tw:flex-1 tw:min-h-0" data-detail={hunt_id ? 1 : 0}>
      {/* ------------------------------------------------ List panel */}
      <section className="rx-split-list tw:w-[400px] tw:shrink-0 tw:flex tw:flex-col tw:border-r tw:border-line tw:bg-panel tw:min-h-0">
        <div className="tw:px-3 tw:pt-3 tw:pb-2 tw:space-y-2.5 tw:border-b tw:border-line">
          <div className="tw:flex tw:items-center tw:gap-2">
            <h1 className="tw:text-[15px] tw:font-semibold tw:flex-1">Hunts</h1>
            <Tooltip content={canHunt ? <span>New hunt <Kbd>N</Kbd></span> : 'You do not have permission to start hunts'}>
              <span>
                <Button variant="primary" size="sm" disabled={!canHunt} icon={<Plus className="tw:size-3.5" />}
                  onClick={() => {
                    setCopyFrom(undefined);
                    setNewOpen(true);
                  }}>New hunt</Button>
              </span>
            </Tooltip>
          </div>
          <SearchInput inputRef={searchRef} value={q} onChange={setQ} placeholder="Filter by name, artifact, tag, creator…" />
          <div className="tw:flex tw:items-center tw:gap-2 tw:flex-wrap">
            <Segmented value={filter} onChange={setFilter} options={[
              { value: 'all', label: 'All', count: counts.all },
              { value: 'running', label: 'Running', count: counts.running },
              { value: 'paused', label: 'Paused', count: counts.paused },
              { value: 'stopped', label: 'Done', count: counts.stopped },
            ]} />
            <Tooltip content={mine ? 'Showing only your hunts' : 'Show only my hunts'}>
              <button type="button" onClick={() => setMine(!mine)}
                className={cn('tw:flex tw:items-center tw:gap-1 tw:h-7 tw:px-2 tw:rounded-md tw:text-xs tw:font-medium',
                  mine ? 'tw:bg-accent-soft tw:text-accent' : 'tw:text-muted tw:hover:bg-hover')}>
                <User className="tw:size-3.5" /> Mine
              </button>
            </Tooltip>
          </div>
          {allTags.length > 0 && (
            <div className="tw:flex tw:items-center tw:gap-1 tw:flex-wrap">
              {allTags.slice(0, 12).map(t => (
                <button key={t} type="button" onClick={() => setTagFilter(tagFilter === t ? null : t)}
                  className={cn('tw:inline-flex tw:items-center tw:gap-1 tw:h-5 tw:px-1.5 tw:rounded tw:text-2xs tw:font-medium',
                    tagFilter === t ? 'tw:bg-accent tw:text-accent-fg' : 'tw:bg-hover tw:text-muted tw:hover:text-fg')}>
                  <Tag className="tw:size-2.5" />{t}
                  {tagFilter === t && <X className="tw:size-2.5" />}
                </button>
              ))}
            </div>
          )}
        </div>

        {checkedIds.length > 0 ? (
          <div className="tw:flex tw:items-center tw:gap-1 tw:px-3 tw:h-10 tw:border-b tw:border-line tw:bg-accent-soft">
            <Checkbox checked={allVisibleChecked} indeterminate={!allVisibleChecked}
              onChange={v => setChecked(v ? new Set(visible.map(h => h.hunt_id)) : new Set())} label="Select all" />
            <span className="tw:text-xs tw:font-medium tw:ml-1.5 tw:flex-1">{checkedIds.length} selected</span>
            <IconButton size="sm" label="Run" disabled={!canHunt} icon={<Play className="tw:size-3.5" />}
              onClick={() => bulk(() => startHunts.mutateAsync(checkedIds), `Started ${checkedIds.length} hunt(s)`)} />
            <IconButton size="sm" label="Stop" disabled={!canHunt} icon={<Square className="tw:size-3.5" />}
              onClick={() => bulk(() => stopHunts.mutateAsync(checkedIds), `Stopped ${checkedIds.length} hunt(s)`)} />
            <IconButton size="sm" label="Archive" icon={<Archive className="tw:size-3.5" />} onClick={() => setConfirm('archive')} />
            <IconButton size="sm" label="Delete" icon={<Trash2 className="tw:size-3.5 tw:text-bad" />} onClick={() => setConfirm('delete')} />
            <IconButton size="sm" label="Clear selection" icon={<X className="tw:size-3.5" />} onClick={() => setChecked(new Set())} />
          </div>
        ) : (
          <div className="tw:flex tw:items-center tw:px-3 tw:h-8 tw:border-b tw:border-line tw:text-2xs tw:text-faint">
            <span className="tw:flex-1 rx-num">{visible.length} of {hunts.length} hunts</span>
            <Switch checked={includeArchived} onChange={setIncludeArchived} label={<span className="tw:text-2xs">Archived</span>} />
          </div>
        )}

        <div ref={listRef} className="tw:flex-1 tw:overflow-y-auto tw:min-h-0">
          {isLoading ? (
            <div className="tw:p-3 tw:space-y-4">
              {Array.from({ length: 6 }).map((_, i) => (
                <div key={i} className="tw:space-y-2"><Skeleton className="tw:w-3/4" /><Skeleton className="tw:w-1/2" /><Skeleton className="tw:h-1.5" /></div>
              ))}
            </div>
          ) : error ? (
            <EmptyState title="Could not load hunts">{(error as Error).message}</EmptyState>
          ) : visible.length === 0 ? (
            <EmptyState icon={<Crosshair />} title={hunts.length ? 'No hunts match' : 'No hunts yet'}
              action={!hunts.length && canHunt
                ? <Button variant="primary" icon={<Plus className="tw:size-3.5" />} onClick={() => setNewOpen(true)}>Start your first hunt</Button>
                : undefined}>
              {hunts.length ? 'Try a different filter.' : 'A hunt runs artifacts across many endpoints at once.'}
            </EmptyState>
          ) : (
            <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
              {virtualizer.getVirtualItems().map(vi => {
                const h = visible[vi.index];
                return (
                  <div key={h.hunt_id} data-index={vi.index} ref={virtualizer.measureElement}
                    style={{ position: 'absolute', top: 0, left: 0, right: 0, transform: `translateY(${vi.start}px)` }}>
                    <HuntRow hunt={h}
                      selected={h.hunt_id === hunt_id}
                      checked={checked.has(h.hunt_id)}
                      onCheck={v => setChecked(prev => {
                        const n = new Set(prev);
                        if (v) n.add(h.hunt_id);
                        else n.delete(h.hunt_id);
                        return n;
                      })}
                      onClick={() => history.push(`/hunts/${h.hunt_id}${tab ? `/${tab}` : ''}`)} />
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </section>

      {/* ---------------------------------------------- Detail panel */}
      <section className="rx-split-detail tw:flex-1 tw:min-w-0 tw:flex tw:flex-col tw:min-h-0">
        <MobileBack to="/hunts" label="All hunts" />
        {hunt_id ? (
          <HuntDetail huntId={hunt_id} summary={selected} tab={tab}
            onCopy={h => {
              setCopyFrom(h);
              setNewOpen(true);
            }} />
        ) : (
          <EmptyState className="tw:flex-1" icon={<Crosshair />} title="Select a hunt"
            action={canHunt ? <Button variant="primary" icon={<Plus className="tw:size-3.5" />} onClick={() => setNewOpen(true)}>New hunt</Button> : undefined}>
            Pick a hunt on the left to see its progress, results and clients. Use <Kbd>J</Kbd> <Kbd>K</Kbd> to move through the list.
          </EmptyState>
        )}
      </section>

      {newOpen && (
        <NewHuntDialog open={newOpen} baseHunt={copyFrom}
          onOpenChange={v => {
            setNewOpen(v);
            if (!v) setCopyFrom(undefined);
          }}
          onCreated={id => {
            setNewOpen(false);
            setCopyFrom(undefined);
            if (id) history.push(`/hunts/${id}`);
          }} />
      )}

      <ConfirmDialog open={confirm === 'archive'} onOpenChange={v => !v && setConfirm(null)}
        title={`Archive ${checkedIds.length} hunt(s)?`} confirmLabel="Archive"
        loading={archiveHunts.isPending}
        onConfirm={() => bulk(() => archiveHunts.mutateAsync(checkedIds), 'Archived').then(() => setConfirm(null))}>
        Archived hunts are hidden from the list but their results are kept.
      </ConfirmDialog>
      <ConfirmDialog open={confirm === 'delete'} onOpenChange={v => !v && setConfirm(null)} danger
        title={`Delete ${checkedIds.length} hunt(s)?`} confirmLabel="Delete permanently"
        loading={deleteHunts.isPending}
        onConfirm={() => bulk(() => deleteHunts.mutateAsync(checkedIds), 'Deletion started').then(() => setConfirm(null))}>
        This cancels the hunts and deletes all of their collected data. This cannot be undone.
      </ConfirmDialog>
    </div>
  );
}
