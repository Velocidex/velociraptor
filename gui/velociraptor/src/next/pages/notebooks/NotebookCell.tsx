// One notebook cell: VQL, Markdown or artifact YAML in, rendered
// report out. Click (or double click Markdown) to edit, Ctrl+Enter to run.
import React, { lazy, Suspense, useEffect, useMemo, useRef, useState } from 'react';
import { toast } from 'sonner';
import {
  ArrowDown, ArrowUp, Ban, ChevronDown, ChevronRight, Code2, Copy, FileCode, History, MoreHorizontal, Pencil,
  Play, Plus, Trash2, Type, Wand2, AlertTriangle,
} from 'lucide-react';
import parseHTML from '../../../components/core/sanitize.jsx';
import {
  notebookCellAction, reformatVQL, useNotebookCell, useNotebookTable, useUpdateNotebookCell,
  type Notebook, type NotebookCell as Cell, type NotebookCellMeta,
} from '../../lib/queries';
import { fmtDuration, timeAgo, toNum } from '../../lib/format';
import {
  Badge, Button, IconButton, Kbd, Menu, MenuContent, MenuItem, MenuLabel, MenuSeparator, MenuTrigger, Skeleton,
  Spinner, Tooltip, cn,
} from '../../ui/primitives';
import { DataTable } from '../../ui/DataTable';

const CodeEditor = lazy(() => import('../../ui/CodeEditor.jsx'));
const LegacyCellOutput = lazy(() => import('./LegacyCellOutput.jsx'));
const NotebookChart = lazy(() => import('./NotebookChart'));
const NotebookTimeline = lazy(() => import('./NotebookTimeline'));
const CHART_TAGS: Record<string, 'line' | 'time' | 'bar' | 'scatter'> = {
  'notebook-line-chart': 'line', 'notebook-time-chart': 'time', 'notebook-bar-chart': 'bar', 'notebook-scatter-chart': 'scatter',
};

// Output tags that only the legacy renderer understands.
const LEGACY_TAGS = /<(velo-line-chart|time-chart|velo-timeline(?![^>]*\bname=)|velo-sigma-editor|velo-tool-viewer|velo-button|grr-[a-z-]+)\b/;

export const CELL_TYPES = [
  { value: 'vql', label: 'VQL', icon: <Code2 /> },
  { value: 'markdown', label: 'Markdown', icon: <Type /> },
  { value: 'artifact', label: 'Artifact', icon: <FileCode /> },
] as const;

const aceMode = (t?: string) => {
  const s = (t || 'vql').toLowerCase();
  return s === 'markdown' ? 'markdown' : s === 'vql' ? 'vql' : 'yaml';
};

export default function NotebookCellView({ notebook, meta, index, count, selected, onSelect, onMove, onDelete, onInsert }: {
  notebook: Notebook; meta: NotebookCellMeta; index: number; count: number; selected: boolean;
  onSelect: () => void; onMove: (dir: -1 | 1) => void; onDelete: () => void;
  onInsert: (where: 'above' | 'below', type: string) => void;
}) {
  const nbId = notebook.notebook_id;
  const { data: cell, isLoading, refetch } = useNotebookCell(nbId, meta.cell_id, meta.current_version);
  const update = useUpdateNotebookCell();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState('');
  const [type, setType] = useState('vql');
  const rootRef = useRef<HTMLDivElement>(null);

  const cellType = (cell?.type || meta.type || 'vql').toLowerCase();
  const calculating = !!cell?.calculating || update.isPending;

  // New empty cells open straight in the editor.
  useEffect(() => {
    if (cell && !cell.input && !cell.output && !editing && selected) startEdit();
  }, [cell?.cell_id]); // eslint-disable-line react-hooks/exhaustive-deps

  const startEdit = () => {
    setDraft(cell?.input ?? '');
    setType(cellType);
    setEditing(true);
    onSelect();
  };

  const save = () => {
    update.mutate({ notebook_id: nbId, cell_id: meta.cell_id, type, input: draft, env: cell?.env }, {
      onSuccess: () => setEditing(false),
      onError: e => toast.error(`Could not run cell: ${(e as Error).message}`),
    });
  };

  const rerun = () => cell && update.mutate({
    notebook_id: nbId, cell_id: meta.cell_id, type: cellType, input: cell.input ?? '', env: cell.env,
  }, { onError: e => toast.error((e as Error).message) });

  const cancel = async () => {
    await notebookCellAction('CancelNotebookCell', { notebook_id: nbId, cell_id: meta.cell_id, version: meta.current_version });
    refetch();
  };

  const versions = meta.available_versions ?? [];
  const vIdx = versions.indexOf(meta.current_version ?? '');
  const revert = async (dir: -1 | 1) => {
    const v = versions[vIdx + dir];
    if (!v) return;
    try {
      await notebookCellAction('RevertNotebookCell', { notebook_id: nbId, cell_id: meta.cell_id, version: v });
      update.reset();
      toast.success(dir < 0 ? 'Restored the previous version' : 'Moved to the next version');
      refetch();
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
      e.preventDefault();
      e.stopPropagation();
      save();
    } else if (e.key === 'Escape') {
      e.stopPropagation();
      setEditing(false);
    }
  };

  const typeInfo = CELL_TYPES.find(t => t.value === (editing ? type : cellType)) ?? CELL_TYPES[0];
  const errors = (cell?.messages ?? []).filter(m => m.startsWith('ERROR:'));
  const others = (cell?.messages ?? []).filter(m => !m.startsWith('ERROR:') && !m.startsWith('DEBUG:Query Stats'));

  return (
    <div ref={rootRef} id={`cell-${meta.cell_id}`} onClick={onSelect}
      className={cn('tw:group tw:relative tw:rounded-xl tw:border tw:bg-panel tw:transition-shadow tw:scroll-mt-4',
        selected ? 'tw:border-accent/60 rx-shadow-lg' : 'tw:border-line tw:hover:border-line-strong')}>
      {/* Toolbar */}
      <div className={cn('tw:flex tw:items-center tw:gap-1 tw:h-9 tw:px-2 tw:border-b tw:border-line tw:rounded-t-xl',
        !selected && !editing && 'tw:border-transparent')}>
        <span className="tw:flex tw:items-center tw:gap-1.5 tw:text-2xs tw:font-semibold tw:uppercase tw:tracking-wide tw:text-faint tw:px-1 tw:[&>svg]:size-3.5">
          {typeInfo.icon}{typeInfo.label}
        </span>
        {calculating && <Badge tone="info"><Spinner className="tw:size-3" />Running</Badge>}
        {!calculating && cell?.timestamp && cellType === 'vql' && (
          <Tooltip content={cell.duration ? `Took ${fmtDuration(toNum(cell.duration))}` : undefined}>
            <span className="tw:text-2xs tw:text-faint">{timeAgo(cell.timestamp)}</span>
          </Tooltip>
        )}
        {versions.length > 1 && (
          <span className="tw:text-2xs tw:text-faint">v{vIdx + 1}/{versions.length}</span>
        )}
        <div className={cn('tw:ml-auto tw:flex tw:items-center tw:gap-0.5 tw:transition-opacity',
          selected || editing ? 'tw:opacity-100' : 'tw:opacity-0 tw:group-hover:opacity-100')}>
          {editing ? (
            <>
              {type === 'vql' && (
                <Button size="xs" variant="ghost" icon={<Wand2 />} onClick={async e => {
                  e.stopPropagation();
                  try { setDraft(await reformatVQL(draft)); } catch (err) { toast.error((err as Error).message); }
                }}>Format</Button>
              )}
              <Menu>
                <MenuTrigger asChild>
                  <Button size="xs" variant="ghost">{CELL_TYPES.find(t => t.value === type)?.label}<ChevronDown className="tw:size-3" /></Button>
                </MenuTrigger>
                <MenuContent>
                  {CELL_TYPES.map(t => <MenuItem key={t.value} icon={t.icon} onSelect={() => setType(t.value)}>{t.label}</MenuItem>)}
                </MenuContent>
              </Menu>
              <Button size="xs" variant="ghost" onClick={e => { e.stopPropagation(); setEditing(false); }}>Discard</Button>
              <Button size="xs" variant="primary" icon={<Play />} loading={update.isPending} onClick={e => { e.stopPropagation(); save(); }}>
                {type === 'markdown' ? 'Save' : 'Run'} <Kbd className="tw:ml-1 tw:bg-white/15 tw:border-white/20 tw:text-white">Ctrl ↵</Kbd>
              </Button>
            </>
          ) : (
            <>
              {calculating
                ? <Button size="xs" variant="ghost" icon={<Ban />} onClick={cancel}>Stop</Button>
                : cellType !== 'markdown' && <IconButton size="xs" label="Run again" icon={<Play />} onClick={rerun} />}
              <IconButton size="xs" label="Edit" icon={<Pencil />} onClick={startEdit} />
              <IconButton size="xs" label="Move up" icon={<ArrowUp />} disabled={index === 0} onClick={() => onMove(-1)} />
              <IconButton size="xs" label="Move down" icon={<ArrowDown />} disabled={index === count - 1} onClick={() => onMove(1)} />
              <Menu>
                <MenuTrigger asChild>
                  <Button size="xs" variant="ghost" square icon={<MoreHorizontal />} aria-label="More" />
                </MenuTrigger>
                <MenuContent>
                  <MenuLabel>Insert</MenuLabel>
                  {CELL_TYPES.map(t => (
                    <MenuItem key={`a-${t.value}`} icon={<Plus />} onSelect={() => onInsert('above', t.value)}>{t.label} cell above</MenuItem>
                  ))}
                  {CELL_TYPES.map(t => (
                    <MenuItem key={`b-${t.value}`} icon={<Plus />} onSelect={() => onInsert('below', t.value)}>{t.label} cell below</MenuItem>
                  ))}
                  <MenuSeparator />
                  <MenuItem icon={<Copy />} onSelect={() => { navigator.clipboard.writeText(cell?.input ?? ''); toast.success('Copied cell source'); }}>Copy source</MenuItem>
                  {versions.length > 1 && <>
                    <MenuItem icon={<History />} disabled={vIdx <= 0} onSelect={() => revert(-1)}>Undo: previous version</MenuItem>
                    <MenuItem icon={<History />} disabled={vIdx >= versions.length - 1} onSelect={() => revert(1)}>Redo: next version</MenuItem>
                  </>}
                  <MenuSeparator />
                  <MenuItem danger icon={<Trash2 />} disabled={count <= 1} onSelect={onDelete}>Delete cell</MenuItem>
                </MenuContent>
              </Menu>
            </>
          )}
        </div>
      </div>

      {/* Editor */}
      {editing && (
        <div className="tw:border-b tw:border-line" onKeyDownCapture={onKeyDown}>
          <Suspense fallback={<Skeleton className="tw:h-24 tw:m-3" />}>
            <CodeEditor value={draft} onChange={setDraft} mode={aceMode(type)} minLines={4} maxLines={30} focus />
          </Suspense>
        </div>
      )}

      {/* Output */}
      <div className={cn('tw:min-h-[24px]', cellType === 'markdown' && !editing && 'tw:cursor-text')}
        onDoubleClick={() => cellType === 'markdown' && !editing && startEdit()}>
        {isLoading ? <Skeleton className="tw:h-16 tw:m-3" /> : cell && (
          <CellOutput cell={cell} notebookId={nbId} calculating={calculating} onRefresh={rerun} />
        )}
        {cell && !cell.output && !calculating && !editing && (
          <button onClick={startEdit} className="tw:w-full tw:text-left tw:px-4 tw:py-3 tw:text-xs tw:text-faint tw:hover:text-muted">
            Empty cell. Click to write {cellType === 'markdown' ? 'Markdown' : cellType === 'vql' ? 'a VQL query' : 'an artifact'}.
          </button>
        )}
      </div>

      {(errors.length > 0 || others.length > 0) && (
        <CellMessages errors={errors} others={others} />
      )}
    </div>
  );
}

function CellMessages({ errors, others }: { errors: string[]; others: string[] }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="tw:border-t tw:border-line tw:px-3 tw:py-1.5 tw:text-2xs">
      {errors.map((m, i) => (
        <div key={i} className="tw:flex tw:items-start tw:gap-1.5 tw:text-bad tw:font-mono tw:py-0.5 tw:whitespace-pre-wrap tw:break-all">
          <AlertTriangle className="tw:size-3 tw:mt-0.5 tw:shrink-0" />{m.replace(/^ERROR:/, '')}
        </div>
      ))}
      {others.length > 0 && (
        <>
          <button onClick={() => setOpen(o => !o)} className="tw:flex tw:items-center tw:gap-1 tw:text-faint tw:hover:text-muted">
            {open ? <ChevronDown className="tw:size-3" /> : <ChevronRight className="tw:size-3" />}
            {others.length} log message{others.length === 1 ? '' : 's'}
          </button>
          {open && (
            <div className="tw:mt-1 tw:max-h-48 tw:overflow-auto tw:font-mono tw:text-muted tw:whitespace-pre-wrap tw:break-all">
              {others.map((m, i) => <div key={i}>{m}</div>)}
            </div>
          )}
        </>
      )}
    </div>
  );
}

function CellOutput({ cell, notebookId, calculating, onRefresh }: {
  cell: Cell; notebookId: string; calculating: boolean; onRefresh: () => void;
}) {
  const out = cell.output ?? '';
  const legacy = LEGACY_TAGS.test(out);
  const content = useMemo(() => {
    if (!out || legacy) return null;
    return parseHTML(out, {
      replace: (node: unknown) => {
        const n = node as { name?: string; attribs?: Record<string, string> };
        if (n.name === 'velo-csv-viewer') {
          let params: Record<string, unknown> = {};
          try { params = JSON.parse(decodeURIComponent(n.attribs?.params ?? '')); } catch { /* ignore */ }
          return <CellTable params={params} />;
        }
        if (n.name && CHART_TAGS[n.name]) {
          let params: Record<string, unknown> = {};
          try { params = JSON.parse(decodeURIComponent(n.attribs?.params ?? '')); } catch { /* ignore */ }
          return <Suspense fallback={<Skeleton className="tw:h-72 tw:my-2" />}><NotebookChart kind={CHART_TAGS[n.name]} params={params} /></Suspense>;
        }
        if (n.name === 'velo-timeline' && n.attribs?.name) {
          return (
            <Suspense fallback={<Skeleton className="tw:h-72 tw:my-2" />}>
              <NotebookTimeline notebookId={notebookId} name={decodeURIComponent(n.attribs.name)} />
            </Suspense>
          );
        }
        if (n.name === 'velo-value') {
          return <span className="tw:font-mono">{decodeURIComponent(n.attribs?.value ?? '')}</span>;
        }
        return undefined;
      },
    });
  }, [out, legacy, notebookId]);

  return (
    <div className="tw:relative">
      {calculating && (
        <div className="tw:h-0.5 tw:overflow-hidden tw:bg-accent-soft"><div className="rx-indeterminate tw:h-full tw:bg-accent" /></div>
      )}
      {legacy ? (
        <Suspense fallback={<Skeleton className="tw:h-24 tw:m-3" />}>
          <LegacyCellOutput cell={cell} notebookId={notebookId} refresh={onRefresh} />
        </Suspense>
      ) : content && <div className="rx-md rx-cell-output tw:px-4 tw:py-3">{content}</div>}
    </div>
  );
}

function CellTable({ params }: { params: Record<string, unknown> }) {
  const q = useNotebookTable(params);
  const rows = useMemo(() => (q.data?.pages ?? []).flatMap(p => p.rows), [q.data]);
  const columns = q.data?.pages[0]?.columns;
  const total = q.data?.pages[0]?.total;
  const height = Math.min(Math.max(rows.length, 3) * 30 + 84, 520);
  return (
    <div className="tw:not-prose tw:-mx-4 tw:my-1 tw:border-y tw:border-line" style={{ height }} onClick={e => e.stopPropagation()}>
      <DataTable rows={rows} columns={columns} loading={q.isLoading} total={total}
        hasMore={q.hasNextPage} loadingMore={q.isFetchingNextPage} onLoadMore={() => q.fetchNextPage()}
        exportName={String(params.cell_id ?? 'notebook')} dense
        empty={<div className="tw:p-6 tw:text-xs tw:text-faint tw:text-center">The query returned no rows.</div>} />
    </div>
  );
}
