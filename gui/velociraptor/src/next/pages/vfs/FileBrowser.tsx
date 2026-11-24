// Browse a client's filesystem (and registry) as last seen by the server,
// refresh folders live from the client, and pull files back with one click.
import { useEffect, useMemo, useState } from 'react';
import { Link, useHistory, useParams } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import {
  ChevronRight, CloudDownload, Download, File, Folder, FolderOpen, FolderSync, HardDrive, Home, Loader2, RefreshCw,
} from 'lucide-react';
import {
  useClient, useFlow, useVfsDir, useVfsFiles, useVfsStat, vfsDownload, vfsRefresh, runArtifact, useTraits,
} from '../../lib/queries';
import type { Row } from '../../lib/table';
import { fmtBytes, fmtTime, timeAgo, toNum } from '../../lib/format';
import { href } from '../../lib/api';
import { DataTable } from '../../ui/DataTable';
import { Badge, Button, ConfirmDialog, EmptyState, IconButton, Spinner, Tooltip, cn } from '../../ui/primitives';
import { useSelectedClient } from '../../shell/client-context';

const encodePath = (c: string[]) => '/' + c.map(encodeURIComponent).join('/');
const decodePath = (p: string) => p.split('/').filter(Boolean).map(s => { try { return decodeURIComponent(s); } catch { return s; } });
const isDir = (mode?: string) => (mode ?? '')[0] === 'd';

export default function FileBrowser() {
  const { client_id: clientId = '', vfs_path: vfsPath = '' } = useParams<{ client_id: string; vfs_path?: string }>();
  const history = useHistory();
  const qc = useQueryClient();
  const { data: client } = useClient(clientId);
  const { client: selected, setClient } = useSelectedClient();
  const { traits } = useTraits();
  const path = useMemo(() => decodePath(vfsPath), [vfsPath]);
  const [refreshFlow, setRefreshFlow] = useState<{ id: string; depth: number } | null>(null);
  const [collectFlow, setCollectFlow] = useState<string | null>(null);
  const [confirmRecursive, setConfirmRecursive] = useState<null | 'refresh' | 'collect'>(null);
  const [pending, setPending] = useState<Record<string, string>>({});

  useEffect(() => {
    if (client?.client_id && selected?.client_id !== client.client_id) setClient(client);
  }, [client, selected?.client_id, setClient]);

  const stat = useVfsStat(clientId, path);
  const version = `${stat.data?.timestamp ?? ''}:${stat.data?.download_version ?? ''}`;
  const files = useVfsFiles(clientId, path, stat.data?.flow_id, version);

  // Track refresh/collect flows and reload when they finish.
  const rf = useFlow(clientId, refreshFlow?.id, { fast: true });
  const cf = useFlow(clientId, collectFlow ?? undefined, { fast: true });
  useEffect(() => {
    const st = rf.data?.context?.state;
    if (refreshFlow && st && st !== 'RUNNING' && st !== 'IN_PROGRESS' && st !== 'WAITING') {
      setRefreshFlow(null);
      qc.invalidateQueries({ queryKey: ['vfs-stat', clientId] });
      qc.invalidateQueries({ queryKey: ['vfs-dir', clientId] });
      qc.invalidateQueries({ queryKey: ['vfs-files', clientId] });
      if (st === 'ERROR') toast.error(`Refresh failed: ${rf.data?.context?.status}`);
    }
  }, [rf.data, refreshFlow, qc, clientId]);
  useEffect(() => {
    const st = cf.data?.context?.state;
    if (collectFlow && st && st !== 'RUNNING' && st !== 'IN_PROGRESS' && st !== 'WAITING') {
      setCollectFlow(null);
      toast.success('Files collected', {
        description: `${toNum(cf.data?.context?.total_uploaded_files)} files · ${fmtBytes(cf.data?.context?.total_uploaded_bytes)}`,
        action: { label: 'Open', onClick: () => history.push(`/collected/${clientId}/${collectFlow}/files`) },
      });
      qc.invalidateQueries({ queryKey: ['vfs-files', clientId] });
    }
  }, [cf.data, collectFlow, qc, clientId, history]);

  // Per-file downloads: reload the listing once each finishes.
  useEffect(() => {
    if (!Object.keys(pending).length) return;
    const t = setInterval(() => {
      qc.invalidateQueries({ queryKey: ['vfs-stat', clientId] });
      qc.invalidateQueries({ queryKey: ['vfs-files', clientId] });
    }, 2000);
    return () => clearInterval(t);
  }, [pending, qc, clientId]);
  const rows = files.data?.rows ?? [];
  useEffect(() => {
    if (!Object.keys(pending).length) return;
    const done = rows.filter(r => pending[String(r._OSPath)] && r.Download && (r.Download as { flow_id?: string }).flow_id === pending[String(r._OSPath)]);
    if (done.length) {
      setPending(p => {
        const n = { ...p };
        for (const r of done) delete n[String(r._OSPath)];
        return n;
      });
    }
  }, [rows, pending]);

  const go = (c: string[]) => history.push(`/vfs/${clientId}${encodePath(c)}`);

  const refresh = async (depth: number) => {
    try {
      const r = await vfsRefresh(clientId, path, depth);
      if (r.flow_id) setRefreshFlow({ id: r.flow_id, depth });
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  const collectAll = async () => {
    try {
      const r = await runArtifact(clientId, 'System.VFS.DownloadFile', {
        Components: JSON.stringify(path.slice(1)), Accessor: path[0], Recursively: 'Y',
      }, true);
      if (r.flow_id) setCollectFlow(r.flow_id);
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  const fetchFile = async (r: Row) => {
    const comps = (r._Components as string[] | undefined) ?? [...path.slice(1), String(r.Name)];
    try {
      const res = await vfsDownload(clientId, String(r._Accessor ?? path[0]), comps);
      if (res.flow_id) setPending(p => ({ ...p, [String(r._OSPath)]: res.flow_id! }));
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  const hostname = client?.os_info?.hostname || clientId;
  const refreshing = !!refreshFlow;
  const listed = !!stat.data?.flow_id;
  const lock = !!traits?.default_password;

  return (
    <div className="tw:flex tw:flex-1 tw:min-h-0">
      {/* -------------------------------------------------------- Tree */}
      <div className="tw:w-[300px] tw:shrink-0 tw:border-r tw:border-line tw:flex tw:flex-col tw:min-h-0 tw:bg-panel">
        <div className="tw:px-4 tw:h-12 tw:flex tw:items-center tw:gap-2 tw:border-b tw:border-line">
          <HardDrive className="tw:size-4 tw:text-faint" />
          <Link to={`/host/${clientId}`} className="tw:text-[13px] tw:font-semibold tw:truncate tw:hover:text-accent">{hostname}</Link>
        </div>
        <div className="tw:flex-1 tw:overflow-auto tw:py-1.5">
          <TreeNode clientId={clientId} comps={[]} name="" current={path} onSelect={go} depth={0} root />
        </div>
      </div>

      {/* ------------------------------------------------------- Files */}
      <div className="tw:flex-1 tw:min-w-0 tw:flex tw:flex-col tw:min-h-0">
        <div className="tw:flex tw:items-center tw:gap-2 tw:px-4 tw:h-12 tw:border-b tw:border-line tw:bg-panel">
          <nav className="tw:flex tw:items-center tw:gap-0.5 tw:min-w-0 tw:flex-1 tw:overflow-x-auto tw:text-[13px]">
            <button type="button" onClick={() => go([])} className="tw:p-1 tw:rounded tw:text-faint tw:hover:text-fg tw:hover:bg-hover"><Home className="tw:size-3.5" /></button>
            {path.map((c, i) => (
              <span key={i} className="tw:flex tw:items-center tw:gap-0.5 tw:shrink-0">
                <ChevronRight className="tw:size-3.5 tw:text-faint" />
                <button type="button" onClick={() => go(path.slice(0, i + 1))}
                  className={cn('tw:px-1.5 tw:py-0.5 tw:rounded tw:hover:bg-hover tw:font-mono tw:text-[12.5px]', i === path.length - 1 ? 'tw:font-semibold' : 'tw:text-muted')}>{c}</button>
              </span>
            ))}
          </nav>
          {path.length > 0 && <>
            {stat.data?.timestamp && <span className="tw:text-2xs tw:text-faint tw:shrink-0">Listed {timeAgo(toNum(stat.data.timestamp))}</span>}
            <Button size="sm" variant="primary" icon={refreshing && refreshFlow?.depth === 0 ? <Loader2 className="tw:size-3.5 tw:animate-spin" /> : <RefreshCw className="tw:size-3.5" />}
              disabled={refreshing} onClick={() => refresh(0)}>
              {refreshing ? 'Listing…' : 'Refresh folder'}
            </Button>
            <Tooltip content="List every subfolder below this one (up to 10 levels)">
              <IconButton size="sm" variant="secondary" label="Refresh recursively" icon={<FolderSync className="tw:size-3.5" />}
                disabled={refreshing} onClick={() => setConfirmRecursive('refresh')} />
            </Tooltip>
            <Tooltip content="Collect every file in this folder and below">
              <IconButton size="sm" variant="secondary" label="Collect all files here" loading={!!collectFlow}
                icon={<CloudDownload className="tw:size-3.5" />} onClick={() => setConfirmRecursive('collect')} />
            </Tooltip>
          </>}
        </div>

        {path.length === 0 ? (
          <RootView clientId={clientId} onSelect={go} />
        ) : !listed && !stat.isLoading ? (
          <EmptyState icon={<FolderOpen />} title="Not listed yet" className="tw:flex-1"
            action={<Button variant="primary" icon={<RefreshCw className="tw:size-3.5" />} loading={refreshing} onClick={() => refresh(0)}>List this folder from the client</Button>}>
            The server hasn’t seen the contents of <span className="tw:font-mono">{path.join('/')}</span> yet.
          </EmptyState>
        ) : (
          <DataTable
            rows={rows}
            columns={['Name', 'Download', 'Size', 'Mode', 'mtime', 'atime', 'ctime', 'btime', '_OSPath']}
            loading={files.isLoading || stat.isLoading}
            storageKey="vfs-files"
            exportName={`${hostname}-${path.join('_')}`}
            onRowClick={r => { if (isDir(r.Mode as string)) go([...path, String(r.Name)]); }}
            columnLabels={{ mtime: 'Modified', atime: 'Accessed', ctime: 'Changed', btime: 'Born', Download: 'Collected' }}
            columnWidths={{ Name: 320, Size: 90, Mode: 110, Download: 140, mtime: 170, atime: 170, ctime: 170, btime: 170 }}
            renderers={{
              Name: (v, r) => (
                <span className="tw:flex tw:items-center tw:gap-2 tw:min-w-0">
                  {isDir(r.Mode as string) ? <Folder className="tw:size-3.5 tw:text-accent tw:shrink-0" /> : <File className="tw:size-3.5 tw:text-faint tw:shrink-0" />}
                  <span className={cn('tw:truncate', isDir(r.Mode as string) && 'tw:font-medium')}>{String(v ?? '')}</span>
                </span>
              ),
              Size: (v, r) => isDir(r.Mode as string) ? <span className="tw:text-faint">—</span> : <span className="rx-num">{fmtBytes(toNum(v as number))}</span>,
              Mode: v => <span className="tw:font-mono tw:text-[11.5px] tw:text-muted">{String(v ?? '')}</span>,
              mtime: v => <TimeCell v={v} />, atime: v => <TimeCell v={v} />, ctime: v => <TimeCell v={v} />, btime: v => <TimeCell v={v} />,
              Download: (v, r) => {
                if (isDir(r.Mode as string)) return null;
                const d = v as { components?: string[]; name?: string; size?: number; mtime?: number; flow_id?: string } | '';
                const waiting = !!pending[String(r._OSPath)];
                if (d && d.components) {
                  return (
                    <span className="tw:flex tw:items-center tw:gap-1.5">
                      <a href={href('/api/v1/DownloadVFSFile', { client_id: clientId, fs_components: d.components, vfs_path: d.name, zip: lock })}
                        download onClick={e => e.stopPropagation()}
                        className="tw:inline-flex tw:items-center tw:gap-1 tw:text-xs tw:font-medium tw:text-accent tw:hover:underline">
                        <Download className="tw:size-3.5" />Download
                      </a>
                      <Tooltip content={`Collected ${fmtTime(d.mtime ? d.mtime : undefined)} · ${fmtBytes(d.size)}`}>
                        <Badge tone="ok">✓</Badge>
                      </Tooltip>
                    </span>
                  );
                }
                return (
                  <Button size="xs" variant="ghost" loading={waiting}
                    icon={<CloudDownload className="tw:size-3.5" />}
                    onClick={e => { e.stopPropagation(); fetchFile(r); }}>
                    {waiting ? 'Collecting…' : 'Collect'}
                  </Button>
                );
              },
            }}
            empty={<EmptyState icon={<FolderOpen />} title="Empty folder" />}
          />
        )}
      </div>

      <ConfirmDialog open={confirmRecursive === 'refresh'} onOpenChange={v => !v && setConfirmRecursive(null)}
        title="List everything below this folder?" confirmLabel="Refresh recursively"
        onConfirm={() => { setConfirmRecursive(null); refresh(10); }}>
        The client walks every subfolder of <span className="tw:font-mono">{path.join('/')}</span>, up to 10 levels deep. This can take a while on large trees.
      </ConfirmDialog>
      <ConfirmDialog open={confirmRecursive === 'collect'} onOpenChange={v => !v && setConfirmRecursive(null)}
        title="Collect every file here?" confirmLabel="Collect files"
        onConfirm={() => { setConfirmRecursive(null); collectAll(); }}>
        Uploads every file in <span className="tw:font-mono">{path.join('/')}</span> and its subfolders to the server. The collection shows up in Collections when it finishes.
      </ConfirmDialog>
    </div>
  );
}

function TimeCell({ v }: { v: unknown }) {
  if (!v) return <span className="tw:text-faint">—</span>;
  const d = new Date(String(v));
  if (isNaN(d.getTime())) return <span>{String(v)}</span>;
  return <Tooltip content={d.toISOString()}><span className="rx-num">{fmtTime(d)}</span></Tooltip>;
}

function RootView({ clientId, onSelect }: { clientId: string; onSelect: (c: string[]) => void }) {
  const { data, isLoading } = useVfsDir(clientId, []);
  const entries = data?.entries ?? [];
  const names = Array.from(new Set(['file', ...entries.map(e => e.Name)]));
  const DESC: Record<string, string> = {
    file: 'The filesystem as the OS sees it',
    auto: 'Filesystem, falling back to raw NTFS for locked files',
    ntfs: 'Raw NTFS parsing (Windows), including deleted and locked files',
    registry: 'The Windows registry',
  };
  return (
    <div className="tw:p-6 tw:grid tw:grid-cols-1 tw:sm:grid-cols-2 tw:xl:grid-cols-4 tw:gap-3 tw:content-start">
      {isLoading && <Spinner />}
      {names.map(n => (
        <button key={n} type="button" onClick={() => onSelect([n])}
          className="tw:flex tw:items-start tw:gap-3 tw:rounded-lg tw:border tw:border-line tw:bg-panel tw:p-4 tw:text-left tw:hover:border-accent tw:hover:bg-accent-soft">
          <HardDrive className="tw:size-5 tw:text-accent tw:shrink-0" />
          <span>
            <span className="tw:block tw:font-mono tw:text-[13px] tw:font-semibold">{n}</span>
            <span className="tw:block tw:text-2xs tw:text-muted tw:mt-0.5">{DESC[n] ?? 'Accessor'}</span>
          </span>
        </button>
      ))}
    </div>
  );
}

function TreeNode({ clientId, comps, name, current, onSelect, depth, root }: {
  clientId: string; comps: string[]; name: string; current: string[]; onSelect: (c: string[]) => void; depth: number; root?: boolean;
}) {
  const onPath = comps.length <= current.length && comps.every((c, i) => current[i] === c);
  const [open, setOpen] = useState(onPath || !!root);
  useEffect(() => { if (onPath) setOpen(true); }, [onPath]);
  const { data, isLoading } = useVfsDir(clientId, comps);
  const children = useMemo(() => {
    let dirs = (data?.entries ?? []).filter(e => isDir(e.Mode)).map(e => e.Name);
    if (root) dirs = Array.from(new Set(['file', ...dirs]));
    // Always show the way down to the folder being viewed, even when the
    // server has not listed the folders above it.
    const next = onPath ? current[comps.length] : undefined;
    if (next && !dirs.includes(next)) dirs = [...dirs, next];
    return dirs.sort((a, b) => a.localeCompare(b));
  }, [data, root, onPath, current, comps.length]);
  const selected = comps.length === current.length && onPath;

  const kids = open && (
    <div>
      {isLoading && <div className="tw:pl-8 tw:py-1"><Spinner className="tw:size-3" /></div>}
      {children.map(c => (
        <TreeNode key={c} clientId={clientId} comps={[...comps, c]} name={c} current={current} onSelect={onSelect} depth={depth + 1} />
      ))}
    </div>
  );
  if (root) return <>{kids}</>;
  return (
    <div>
      <div className={cn('tw:flex tw:items-center tw:gap-1 tw:pr-2 tw:h-7 tw:cursor-pointer tw:text-[12.5px]',
        selected ? 'tw:bg-accent-soft tw:text-accent tw:font-medium' : 'tw:hover:bg-hover')}
        style={{ paddingLeft: 6 + (depth - 1) * 14 }}
        onClick={() => onSelect(comps)}>
        <button type="button" onClick={e => { e.stopPropagation(); setOpen(!open); }} className="tw:p-0.5 tw:text-faint tw:hover:text-fg">
          <ChevronRight className={cn('tw:size-3 tw:transition-transform', open && 'tw:rotate-90')} />
        </button>
        {depth === 1 ? <HardDrive className="tw:size-3.5 tw:text-faint tw:shrink-0" />
          : open ? <FolderOpen className="tw:size-3.5 tw:text-accent tw:shrink-0" /> : <Folder className="tw:size-3.5 tw:text-accent tw:shrink-0" />}
        <span className="tw:truncate tw:font-mono">{name}</span>
      </div>
      {kids}
    </div>
  );
}
