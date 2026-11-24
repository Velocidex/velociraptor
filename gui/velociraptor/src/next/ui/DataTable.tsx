// A fast, virtualised data table used for every result set in the new
// UI. Only the visible rows are in the DOM, so tens of thousands of
// rows scroll smoothly. Sorting, filtering, column sizing and
// visibility all run client side over the rows that are loaded.
import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  type ColumnDef, type SortingState, type VisibilityState, type ColumnSizingState,
  flexRender, getCoreRowModel, getFilteredRowModel, getSortedRowModel, useReactTable,
} from '@tanstack/react-table';
import { useVirtualizer } from '@tanstack/react-virtual';
import {
  ArrowDown, ArrowUp, Columns3, Download, FileJson, X, Braces, ChevronsUpDown,
} from 'lucide-react';
import type { Row } from '../lib/table';
import { toCSV, downloadText } from '../lib/table';
import { cellToString, fmtNum } from '../lib/format';
import {
  Button, cn, CopyButton, EmptyState, IconButton, Menu, MenuContent, MenuItem, MenuLabel,
  MenuSeparator, MenuTrigger, SearchInput, Skeleton,
} from './primitives';
import { DropdownMenu as RMenu } from 'radix-ui';

export type CellRenderer = (value: unknown, row: Row) => React.ReactNode;

export interface DataTableProps {
  rows: Row[];
  columns?: string[];
  renderers?: Record<string, CellRenderer>;
  columnWidths?: Record<string, number>;
  columnLabels?: Record<string, string>;
  headerRenderers?: Record<string, () => React.ReactNode>;
  hiddenColumns?: string[];
  loading?: boolean;
  total?: number;
  hasMore?: boolean;
  loadingMore?: boolean;
  onLoadMore?: () => void;
  onRowClick?: (row: Row) => void;
  isRowSelected?: (row: Row) => boolean;
  rowKey?: (row: Row, idx: number) => string;
  toolbar?: React.ReactNode;
  toolbarRight?: React.ReactNode;
  empty?: React.ReactNode;
  exportName?: string;
  inspector?: boolean;
  dense?: boolean;
  className?: string;
  // Persist column sizing/visibility per table under this key.
  storageKey?: string;
  // Controlled global filter (optional).
  filter?: string;
  onFilterChange?: (v: string) => void;
  hideSearch?: boolean;
}

function loadState<T>(key: string | undefined, fallback: T): T {
  if (!key) return fallback;
  try {
    const raw = localStorage.getItem(`rx-table:${key}`);
    return raw ? { ...fallback, ...JSON.parse(raw) } : fallback;
  } catch {
    return fallback;
  }
}

function saveState(key: string | undefined, value: unknown) {
  if (!key) return;
  try {
    localStorage.setItem(`rx-table:${key}`, JSON.stringify(value));
  } catch {
    // ignore
  }
}

export function DefaultCell({ value }: { value: unknown }) {
  if (value === null || value === undefined || value === '') {
    return <span className="tw:text-faint">–</span>;
  }
  if (typeof value === 'boolean') {
    return <span className={value ? 'tw:text-ok' : 'tw:text-faint'}>{String(value)}</span>;
  }
  if (typeof value === 'number') {
    return <span className="rx-num">{value}</span>;
  }
  if (typeof value === 'object') {
    const s = JSON.stringify(value);
    return <span className="tw:font-mono tw:text-[11.5px] tw:text-muted" title={s.length < 2000 ? s : undefined}>{s}</span>;
  }
  return <span title={String(value).length > 40 && String(value).length < 2000 ? String(value) : undefined}>{String(value)}</span>;
}

function guessWidth(col: string, rows: Row[]): number {
  let max = col.length * 7.5 + 36;
  for (let i = 0; i < Math.min(rows.length, 50); i++) {
    const s = cellToString(rows[i][col]);
    max = Math.max(max, Math.min(s.length, 60) * 7 + 20);
  }
  return Math.round(Math.min(Math.max(max, 70), 480));
}

export function DataTable(props: DataTableProps) {
  const {
    rows, renderers = {}, columnWidths = {}, columnLabels = {}, loading, total, hasMore,
    loadingMore, onLoadMore, onRowClick, isRowSelected, rowKey, toolbar, toolbarRight, empty,
    exportName = 'export', inspector = true, dense, className, storageKey,
  } = props;

  const columnNames = useMemo(() => {
    if (props.columns?.length) return props.columns;
    const seen = new Set<string>();
    for (const r of rows.slice(0, 200)) for (const k of Object.keys(r)) seen.add(k);
    return [...seen];
  }, [props.columns, rows]);

  const [sorting, setSorting] = useState<SortingState>([]);
  const [innerFilter, setInnerFilter] = useState('');
  const filter = props.filter ?? innerFilter;
  const setFilter = props.onFilterChange ?? setInnerFilter;

  const defaultHidden = useMemo(() => {
    const v: VisibilityState = {};
    for (const c of columnNames) if (c.startsWith('_')) v[c] = false;
    for (const c of props.hiddenColumns ?? []) v[c] = false;
    return v;
  }, [columnNames, props.hiddenColumns]);

  const [visibility, setVisibility] = useState<VisibilityState>(
    () => loadState(storageKey && `${storageKey}:vis`, defaultHidden));
  const [sizing, setSizing] = useState<ColumnSizingState>(
    () => loadState(storageKey && `${storageKey}:size`, {}));
  const [inspected, setInspected] = useState<Row | null>(null);

  useEffect(() => saveState(storageKey && `${storageKey}:vis`, visibility), [storageKey, visibility]);
  useEffect(() => saveState(storageKey && `${storageKey}:size`, sizing), [storageKey, sizing]);

  // Apply default hidden columns for columns that appear later.
  useEffect(() => {
    setVisibility(v => {
      let changed = false;
      const next = { ...v };
      for (const [k, val] of Object.entries(defaultHidden)) {
        if (!(k in next)) {
          next[k] = val;
          changed = true;
        }
      }
      return changed ? next : v;
    });
  }, [defaultHidden]);

  // Renderers may close over parent state; read them through a ref so
  // the (memoised) column definitions always call the latest version.
  const renderersRef = useRef(renderers);
  renderersRef.current = renderers;

  const columns = useMemo<ColumnDef<Row>[]>(() => columnNames.map(name => ({
    id: name,
    accessorFn: (r: Row) => r[name],
    header: columnLabels[name] ?? name,
    size: columnWidths[name] ?? guessWidth(name, rows),
    minSize: 50,
    maxSize: 2000,
    sortingFn: (a, b, id) => {
      const x = a.getValue(id);
      const y = b.getValue(id);
      if (typeof x === 'number' && typeof y === 'number') return x - y;
      return cellToString(x).localeCompare(cellToString(y), undefined, { numeric: true });
    },
    cell: ctx => {
      const r = renderersRef.current[name];
      const v = ctx.getValue();
      return r ? r(v, ctx.row.original) : <DefaultCell value={v} />;
    },
  // eslint-disable-next-line react-hooks/exhaustive-deps
  })), [columnNames, rows.length === 0]);

  const table = useReactTable({
    data: rows,
    columns,
    state: { sorting, globalFilter: filter, columnVisibility: visibility, columnSizing: sizing },
    onSortingChange: setSorting,
    onColumnVisibilityChange: setVisibility,
    onColumnSizingChange: setSizing,
    onGlobalFilterChange: setFilter,
    globalFilterFn: (row, _col, value: string) => {
      if (!value) return true;
      const needle = value.toLowerCase();
      for (const c of columnNames) {
        if (cellToString(row.original[c]).toLowerCase().includes(needle)) return true;
      }
      return false;
    },
    columnResizeMode: 'onChange',
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getRowId: rowKey ? (r, i) => rowKey(r, i) : undefined,
  });

  const modelRows = table.getRowModel().rows;
  const scrollRef = useRef<HTMLDivElement>(null);
  const rowHeight = dense ? 26 : 30;
  const virtualizer = useVirtualizer({
    count: modelRows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => rowHeight,
    overscan: 12,
  });

  // Auto load more when scrolled near the end.
  const items = virtualizer.getVirtualItems();
  const lastIndex = items.length ? items[items.length - 1].index : 0;
  useEffect(() => {
    if (hasMore && !loadingMore && onLoadMore && !filter && lastIndex >= modelRows.length - 20) {
      onLoadMore();
    }
  }, [hasMore, loadingMore, onLoadMore, lastIndex, modelRows.length, filter]);

  const totalWidth = table.getVisibleLeafColumns().reduce((s, c) => s + c.getSize(), 0);

  const exportRows = () => modelRows.map(r => r.original);
  const visibleCols = table.getVisibleLeafColumns().map(c => c.id);

  const showInspector = inspector && inspected;

  return (
    <div className={cn('tw:flex tw:flex-col tw:min-h-0 tw:h-full', className)}>
      <div className="tw:flex tw:items-center tw:gap-2 tw:px-3 tw:py-2 tw:border-b tw:border-line tw:flex-wrap">
        {!props.hideSearch && (
          <SearchInput value={filter} onChange={setFilter} placeholder="Filter rows…" className="tw:w-64" />
        )}
        {toolbar}
        <div className="tw:ml-auto tw:flex tw:items-center tw:gap-1">
          <span className="tw:text-xs tw:text-faint rx-num tw:mr-2">
            {filter ? `${fmtNum(modelRows.length)} of ` : ''}
            {fmtNum(rows.length)}
            {total !== undefined && total > rows.length ? ` / ${fmtNum(total)}` : ''} rows
          </span>
          {toolbarRight}
          <Menu>
            <MenuTrigger asChild>
              <IconButton label="Columns" size="sm" icon={<Columns3 className="tw:size-3.5" />} />
            </MenuTrigger>
            <MenuContent className="tw:max-h-[60vh] tw:overflow-auto">
              <MenuLabel>Columns</MenuLabel>
              {table.getAllLeafColumns().map(c => (
                <RMenu.CheckboxItem key={c.id} checked={c.getIsVisible()}
                  onSelect={e => e.preventDefault()}
                  onCheckedChange={v => c.toggleVisibility(!!v)}
                  className="tw:flex tw:items-center tw:gap-2 tw:h-7 tw:px-2 tw:rounded-md tw:text-[13px] tw:outline-none tw:cursor-pointer tw:data-[highlighted]:bg-hover">
                  <span className={cn('tw:size-3.5 tw:rounded tw:border tw:flex tw:items-center tw:justify-center',
                    c.getIsVisible() ? 'tw:bg-accent tw:border-accent tw:text-accent-fg' : 'tw:border-line-strong')}>
                    {c.getIsVisible() && <svg viewBox="0 0 12 12" className="tw:size-2.5"><path d="M2 6l3 3 5-6" stroke="currentColor" strokeWidth="2" fill="none" /></svg>}
                  </span>
                  <span className="tw:truncate">{String(c.columnDef.header)}</span>
                </RMenu.CheckboxItem>
              ))}
              <MenuSeparator />
              <MenuItem onSelect={() => setVisibility(defaultHidden)}>Reset columns</MenuItem>
              <MenuItem onSelect={() => setSizing({})}>Reset widths</MenuItem>
            </MenuContent>
          </Menu>
          <Menu>
            <MenuTrigger asChild>
              <IconButton label="Export" size="sm" icon={<Download className="tw:size-3.5" />} />
            </MenuTrigger>
            <MenuContent>
              <MenuLabel>Export {fmtNum(modelRows.length)} rows</MenuLabel>
              <MenuItem icon={<Download />} onSelect={() =>
                downloadText(`${exportName}.csv`, toCSV(visibleCols, exportRows()), 'text/csv')}>CSV</MenuItem>
              <MenuItem icon={<FileJson />} onSelect={() =>
                downloadText(`${exportName}.json`, exportRows().map(r => JSON.stringify(r)).join('\n'), 'application/json')}>
                JSON lines
              </MenuItem>
            </MenuContent>
          </Menu>
        </div>
      </div>

      <div className="tw:flex tw:flex-1 tw:min-h-0">
        <div ref={scrollRef} className="tw:flex-1 tw:overflow-auto tw:relative tw:min-w-0">
          <div style={{ width: Math.max(totalWidth, 100) }} className="tw:min-w-full">
            {/* Header */}
            <div className="tw:sticky tw:top-0 tw:z-10 tw:flex tw:bg-canvas tw:border-b tw:border-line" style={{ width: totalWidth }}>
              {table.getHeaderGroups()[0]?.headers.map(h => {
                const sorted = h.column.getIsSorted();
                return (
                  <div key={h.id} style={{ width: h.getSize() }}
                    className="tw:relative tw:flex tw:items-center tw:h-8 tw:px-2.5 tw:text-2xs tw:font-semibold tw:uppercase tw:tracking-wide tw:text-muted tw:select-none tw:group tw:shrink-0">
                    {props.headerRenderers?.[h.column.id] ? props.headerRenderers[h.column.id]() : (
                    <button type="button" onClick={h.column.getToggleSortingHandler()}
                      className="tw:flex tw:items-center tw:gap-1 tw:min-w-0 tw:hover:text-fg">
                      <span className="tw:truncate">{flexRender(h.column.columnDef.header, h.getContext())}</span>
                      {sorted === 'asc' ? <ArrowUp className="tw:size-3 tw:shrink-0" />
                        : sorted === 'desc' ? <ArrowDown className="tw:size-3 tw:shrink-0" />
                          : <ChevronsUpDown className="tw:size-3 tw:shrink-0 tw:opacity-0 tw:group-hover:opacity-50" />}
                    </button>)}
                    <div
                      onMouseDown={h.getResizeHandler()}
                      onTouchStart={h.getResizeHandler()}
                      onDoubleClick={() => h.column.resetSize()}
                      className={cn('tw:absolute tw:right-0 tw:top-1.5 tw:bottom-1.5 tw:w-1 tw:cursor-col-resize tw:rounded',
                        h.column.getIsResizing() ? 'tw:bg-accent' : 'tw:bg-line tw:opacity-0 tw:group-hover:opacity-100')} />
                  </div>
                );
              })}
            </div>

            {/* Body */}
            {loading && rows.length === 0 ? (
              <div className="tw:p-3 tw:space-y-2.5">
                {Array.from({ length: 8 }).map((_, i) => <Skeleton key={i} className="tw:h-4" />)}
              </div>
            ) : modelRows.length === 0 ? (
              <div className="tw:sticky tw:left-0" style={{ width: scrollRef.current?.clientWidth }}>
                {empty ?? <EmptyState title={filter ? 'No rows match your filter' : 'No rows'} />}
              </div>
            ) : (
              <div style={{ height: virtualizer.getTotalSize(), position: 'relative', width: totalWidth }}>
                {items.map(vr => {
                  const row = modelRows[vr.index];
                  const selected = (isRowSelected?.(row.original)) || inspected === row.original;
                  return (
                    <div key={row.id}
                      onClick={() => {
                        if (onRowClick) onRowClick(row.original);
                        else if (inspector) setInspected(row.original);
                      }}
                      className={cn(
                        'tw:absolute tw:left-0 tw:flex tw:items-center tw:border-b tw:border-line/60 tw:text-[12.5px]',
                        (onRowClick || inspector) && 'tw:cursor-pointer',
                        selected ? 'tw:bg-accent-soft' : vr.index % 2 ? 'tw:bg-canvas/40 tw:hover:bg-hover' : 'tw:hover:bg-hover')}
                      style={{ top: vr.start, height: rowHeight, width: totalWidth }}>
                      {row.getVisibleCells().map(cell => (
                        <div key={cell.id} style={{ width: cell.column.getSize() }}
                          className="tw:px-2.5 tw:truncate tw:shrink-0">
                          {flexRender(cell.column.columnDef.cell, cell.getContext())}
                        </div>
                      ))}
                    </div>
                  );
                })}
              </div>
            )}
            {hasMore && (
              <div className="tw:sticky tw:left-0 tw:flex tw:justify-center tw:p-3" style={{ width: scrollRef.current?.clientWidth }}>
                <Button size="sm" variant="secondary" loading={loadingMore} onClick={onLoadMore}>
                  Load more rows
                </Button>
              </div>
            )}
          </div>
        </div>
        {showInspector && (
          <RowInspector row={inspected!} columns={columnNames} onClose={() => setInspected(null)} />
        )}
      </div>
    </div>
  );
}

function RowInspector({ row, columns, onClose }: { row: Row; columns: string[]; onClose: () => void }) {
  const [raw, setRaw] = useState(false);
  const json = useMemo(() => JSON.stringify(row, null, 2), [row]);
  return (
    <aside className="tw:w-[420px] tw:max-w-[50%] tw:shrink-0 tw:border-l tw:border-line tw:bg-panel tw:flex tw:flex-col tw:min-h-0 tw:animate-fade-in">
      <div className="tw:flex tw:items-center tw:gap-2 tw:h-10 tw:px-3 tw:border-b tw:border-line">
        <span className="tw:text-[13px] tw:font-semibold tw:flex-1">Row details</span>
        <IconButton size="xs" label={raw ? 'Show fields' : 'Show JSON'} onClick={() => setRaw(!raw)}
          icon={<Braces className="tw:size-3.5" />} />
        <CopyButton text={json} />
        <IconButton size="xs" label="Close" onClick={onClose} icon={<X className="tw:size-3.5" />} />
      </div>
      <div className="tw:flex-1 tw:overflow-auto tw:p-3">
        {raw ? (
          <pre className="tw:font-mono tw:text-[11.5px] tw:whitespace-pre-wrap tw:break-all tw:m-0">{json}</pre>
        ) : (
          <dl className="tw:space-y-2.5">
            {columns.map(c => {
              const v = row[c];
              const isObj = v !== null && typeof v === 'object';
              return (
                <div key={c}>
                  <dt className="tw:text-2xs tw:font-semibold tw:uppercase tw:tracking-wide tw:text-faint">{c}</dt>
                  <dd className="tw:mt-0.5 tw:text-[12.5px] tw:break-all">
                    {isObj
                      ? <pre className="tw:font-mono tw:text-[11.5px] tw:whitespace-pre-wrap tw:m-0 tw:bg-canvas tw:rounded tw:p-2 tw:border tw:border-line">{JSON.stringify(v, null, 2)}</pre>
                      : <DefaultCell value={v} />}
                  </dd>
                </div>
              );
            })}
          </dl>
        )}
      </div>
    </aside>
  );
}
