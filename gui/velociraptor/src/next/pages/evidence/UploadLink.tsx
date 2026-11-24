// Inline download chip for an "Upload" object found in a results row, and
// the uploads table shared by hunts and single-client collections.
import { Download, FileDown, Lock } from 'lucide-react';
import type { Row } from '../../lib/table';
import { isUploadValue, uploadHref, type UploadValue } from '../../lib/downloads';
import { useTraits } from '../../lib/queries';
import { fmtBytes, fmtTime, timeAgo, toNum } from '../../lib/format';
import { DataTable } from '../../ui/DataTable';
import { EmptyState, Tooltip } from '../../ui/primitives';
import { ClientLink, FlowLink } from './links';

function useLock(): boolean {
  const { traits } = useTraits();
  return !!traits?.default_password;
}

export function UploadChip({ value, clientId, flowId }: { value: UploadValue; clientId: string; flowId: string }) {
  const lock = useLock();
  const name = (value.StoredName || value.Path || '').split(/[\\/]/).pop() || 'file';
  return (
    <Tooltip content={<span className="tw:font-mono tw:break-all">{value.StoredName || value.Path}{value.sha256 ? <><br />sha256 {value.sha256}</> : null}</span>}>
      <a href={uploadHref({ clientId, flowId, components: value.Components, accessor: value.Accessor, path: value.StoredName || value.Path || '', lock })}
        onClick={e => e.stopPropagation()} download
        className="tw:inline-flex tw:items-center tw:gap-1 tw:h-5 tw:max-w-full tw:rounded tw:bg-accent-soft tw:px-1.5 tw:text-[11.5px] tw:font-medium tw:text-accent tw:hover:underline">
        <FileDown className="tw:size-3 tw:shrink-0" />
        <span className="tw:truncate">{name}</span>
        <span className="tw:text-faint tw:font-normal">{fmtBytes(value.StoredSize ?? value.Size)}</span>
        {lock && <Lock className="tw:size-2.5 tw:shrink-0" />}
      </a>
    </Tooltip>
  );
}

// Wraps a set of DataTable renderers so that any cell holding an Upload
// object becomes a download chip. Needs ClientId/FlowId either from the
// row (hunt results) or fixed (single collection).
export function withUploadCells(columns: string[] | undefined, rows: Row[],
  ctx: { clientId?: string; flowId?: string }) {
  const out: Record<string, (v: unknown, r: Row) => React.ReactNode> = {};
  const sample = rows.slice(0, 50);
  for (const c of columns ?? []) {
    if (sample.some(r => isUploadValue(r[c]))) {
      out[c] = (v, r) => isUploadValue(v)
        ? <UploadChip value={v} clientId={ctx.clientId ?? String(r.ClientId ?? '')} flowId={ctx.flowId ?? String(r.FlowId ?? '')} />
        : v == null ? '' : typeof v === 'object' ? JSON.stringify(v) : String(v);
    }
  }
  return out;
}

// Moves the given columns right after the first one so download chips are
// visible without scrolling.
export function promoteColumns(columns: string[] | undefined, promote: string[]): string[] | undefined {
  if (!columns || !promote.length) return columns;
  const rest = columns.filter(c => !promote.includes(c));
  return [...rest.slice(0, 1), ...promote, ...rest.slice(1)];
}

export function UploadsTable({ rows, loading, storageKey, exportName, fixed, showHost, toolbar, toolbarRight, hasMore, loadingMore, onLoadMore }: {
  rows: Row[]; loading: boolean; storageKey: string; exportName: string;
  fixed?: { clientId: string; flowId: string }; showHost?: boolean;
  toolbar?: React.ReactNode; toolbarRight?: React.ReactNode;
  hasMore?: boolean; loadingMore?: boolean; onLoadMore?: () => void;
}) {
  const lock = useLock();
  const columns = [
    'Download', ...(showHost ? ['Hostname'] : []), 'vfs_path', 'file_size', 'uploaded_size', 'Timestamp',
    ...(showHost ? ['ClientId', 'FlowId'] : []), 'Type', '_accessor', '_Components', 'started',
  ];
  const href = (r: Row) => uploadHref({
    clientId: fixed?.clientId ?? String(r.ClientId ?? ''),
    flowId: fixed?.flowId ?? String(r.FlowId ?? ''),
    components: r._Components as string[] | undefined,
    accessor: String(r._accessor ?? 'auto'),
    path: String(r.vfs_path ?? ''),
    lock,
  });
  return (
    <DataTable
      rows={rows}
      columns={columns}
      loading={loading}
      storageKey={storageKey}
      exportName={exportName}
      inspector={false}
      hasMore={hasMore}
      loadingMore={loadingMore}
      onLoadMore={onLoadMore}
      toolbar={toolbar}
      toolbarRight={toolbarRight}
      columnLabels={{ vfs_path: 'Path', file_size: 'Size', uploaded_size: 'Stored', Timestamp: 'Collected', ClientId: 'Client', FlowId: 'Collection', Download: '' }}
      columnWidths={{ Download: 44, vfs_path: 420, Hostname: 160, file_size: 90, uploaded_size: 90, Timestamp: 130 }}
      renderers={{
        Download: (_v, r) => (
          <Tooltip content={lock ? 'Download (password-protected zip)' : 'Download file'}>
            <a href={href(r)} download onClick={e => e.stopPropagation()}
              className="tw:inline-flex tw:size-6 tw:items-center tw:justify-center tw:rounded tw:text-accent tw:hover:bg-accent-soft">
              <Download className="tw:size-3.5" />
            </a>
          </Tooltip>
        ),
        vfs_path: (v, r) => (
          <a href={href(r)} download onClick={e => e.stopPropagation()}
            className="tw:font-mono tw:text-[12px] tw:hover:text-accent tw:hover:underline tw:truncate">{String(v ?? '')}</a>
        ),
        Hostname: v => <span className="tw:font-medium">{String(v ?? '')}</span>,
        ClientId: v => <ClientLink id={String(v ?? '')} />,
        FlowId: (v, r) => <FlowLink clientId={String(r.ClientId ?? '')} flowId={String(v ?? '')} />,
        file_size: v => <span className="rx-num">{fmtBytes(toNum(v as number))}</span>,
        uploaded_size: v => <span className="rx-num">{fmtBytes(toNum(v as number))}</span>,
        Timestamp: v => <Tooltip content={fmtTime(v as number)}><span>{timeAgo(v as number)}</span></Tooltip>,
      }}
      empty={<EmptyState icon={<FileDown />} title="No files collected">
        Files appear here when an artifact uploads them, for example with an <span className="tw:font-mono">Upload</span> or <span className="tw:font-mono">Collect</span> option enabled.
      </EmptyState>}
    />
  );
}
