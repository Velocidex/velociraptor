import React, { lazy, Suspense, useEffect, useMemo, useRef, useState } from 'react';
import { Link, useHistory } from 'react-router-dom';
import { toast } from 'sonner';
import {
  Play, Square, Copy, Pencil, MoreHorizontal, Download, RefreshCw, Archive, Trash2, ExternalLink,
  Users, CircleCheck, CircleX, Loader, Rows3, HardDrive, BookOpen, Table2, LayoutList, TriangleAlert,
  FileDown, FolderArchive,
} from 'lucide-react';
import {
  useArchiveHunts, useDeleteHunts, useHunt, useHuntErrors, useHuntFlows, useHuntResults, useModifyHunt,
  usePermission, useStartHunts, useStopHunts, useArtifactDefinitions, useHuntUploads,
  HUNT_UPLOADS_MAX_FLOWS,
} from '../../lib/queries';
import type { Hunt, AvailableDownloadFile } from '../../lib/types';
import { fmtBytes, fmtNum, fmtTime, timeAgo, toNum, fmtDuration } from '../../lib/format';
import {
  Badge, Button, Card, CardHeader, ConfirmDialog, CopyButton, EmptyState, IconButton, Menu, MenuContent,
  MenuItem, MenuSeparator, MenuTrigger, Mono, Select, Skeleton, Spinner, Tabs, Tooltip,
} from '../../ui/primitives';
import { DataTable } from '../../ui/DataTable';
import type { Row } from '../../lib/table';
import { effectiveState, HuntStateBadge, huntCounts, huntTitle } from './HuntBits';
import EditHuntDialog from './EditHuntDialog';
import { ClientLink, FlowLink } from '../evidence/links';
import ExportHuntDialog from '../evidence/ExportHuntDialog';
import { ArchiveList, useArchiveReadyToast } from '../evidence/ArchiveList';
import { promoteColumns, UploadsTable, withUploadCells } from '../evidence/UploadLink';
import { FlowDownloadButton } from '../evidence/FlowExportButton';
import { archiveState } from '../../lib/downloads';

const HuntNotebook = lazy(() => import('../notebooks/NotebooksPage').then(m => ({ default: m.NotebookView })));

type TabKey = 'overview' | 'results' | 'clients' | 'files' | 'exports' | 'errors' | 'notebook';

function normalizeTab(t: string | undefined): TabKey {
  switch (t) {
    case 'results': case 'clients': case 'files': case 'exports': case 'errors': case 'notebook': return t;
    default: return 'overview';
  }
}

export default function HuntDetail({ huntId, summary, tab, onCopy }: {
  huntId: string; summary?: Hunt; tab?: string; onCopy: (h: Hunt) => void;
}) {
  const history = useHistory();
  const { data: full, isLoading, error } = useHunt(huntId);
  const hunt: Hunt | undefined = full?.hunt_id ? { ...summary, ...full } : summary;
  const active = normalizeTab(tab);
  const canHunt = usePermission('start_hunt');
  const [editOpen, setEditOpen] = useState(false);
  const [exportOpen, setExportOpen] = useState(false);
  const [confirm, setConfirm] = useState<null | 'archive' | 'delete' | 'stop'>(null);

  const startHunts = useStartHunts();
  const stopHunts = useStopHunts();
  const archiveHunts = useArchiveHunts();
  const deleteHunts = useDeleteHunts();
  const modify = useModifyHunt();

  const { data: flows } = useHuntFlows(huntId);
  const { data: errors } = useHuntErrors(huntId);
  const errorRows = useMemo(
    () => (errors?.rows ?? []).filter(r => r.Status === 'ERROR' || (r.Error && r.Error !== '')),
    [errors]);
  const archives: AvailableDownloadFile[] = useMemo(
    () => hunt?.stats?.available_downloads?.files ?? [], [hunt?.stats?.available_downloads]);
  // Exports started from this page download themselves when ready.
  const autoDownload = useRef(new Set<string>());
  useArchiveReadyToast(archives, autoDownload);

  if (isLoading && !hunt) {
    return (
      <div className="tw:p-6 tw:space-y-4">
        <Skeleton className="tw:h-6 tw:w-1/3" />
        <Skeleton className="tw:w-1/4" />
        <div className="tw:grid tw:grid-cols-4 tw:gap-3">{[0, 1, 2, 3].map(i => <Skeleton key={i} className="tw:h-20" />)}</div>
      </div>
    );
  }
  if (!hunt) {
    return <EmptyState title="Hunt not found">{error ? (error as Error).message : huntId}</EmptyState>;
  }

  const state = effectiveState(hunt);
  const c = huntCounts(hunt);
  const setTab = (t: string) => history.push(`/hunts/${huntId}/${t}`);

  const run = async (fn: () => Promise<unknown>, ok: string) => {
    try {
      await fn();
      toast.success(ok);
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  const canStart = state === 'paused' || state === 'stopped';
  const canStop = state === 'running';

  return (
    <div className="tw:flex tw:flex-col tw:flex-1 tw:min-h-0">
      {/* ------------------------------------------------ Header */}
      <div className="tw:px-4 tw:md:px-5 tw:pt-4 tw:pb-3 tw:bg-panel">
        <div className="tw:flex tw:flex-wrap tw:sm:flex-nowrap tw:items-start tw:gap-3">
          <div className="tw:min-w-0 tw:flex-1 tw:max-sm:basis-full">
            <div className="tw:flex tw:items-center tw:gap-2.5">
              <h2 className="tw:text-lg tw:font-semibold tw:truncate">{huntTitle(hunt)}</h2>
              <HuntStateBadge hunt={hunt} />
            </div>
            <div className="tw:flex tw:items-center tw:gap-2 tw:mt-1 tw:text-xs tw:text-muted tw:flex-wrap">
              <Mono>{hunt.hunt_id}</Mono><CopyButton text={hunt.hunt_id} />
              <span>·</span>
              <span>Created {timeAgo(hunt.create_time)} by <b className="tw:font-medium tw:text-fg">{hunt.creator}</b></span>
              {toNum(hunt.expires) > 0 && (
                <>
                  <span>·</span>
                  <Tooltip content={fmtTime(hunt.expires)}>
                    <span>{state === 'expired' ? 'Expired' : 'Expires'} {timeAgo(hunt.expires)}</span>
                  </Tooltip>
                </>
              )}
              {(hunt.tags ?? []).map(t => <Badge key={t}>{t}</Badge>)}
            </div>
          </div>
          <div className="tw:flex tw:items-center tw:gap-1.5 tw:shrink-0">
            {canStart && (
              <Button variant="primary" icon={<Play className="tw:size-3.5" />} disabled={!canHunt}
                loading={startHunts.isPending}
                onClick={() => run(() => startHunts.mutateAsync([hunt.hunt_id]), 'Hunt started')}>
                {state === 'paused' ? 'Start' : 'Resume'}
              </Button>
            )}
            {canStop && (
              <Button variant="secondary" icon={<Square className="tw:size-3.5" />} disabled={!canHunt}
                onClick={() => setConfirm('stop')}>Stop</Button>
            )}
            <Button variant={canStart ? 'secondary' : 'primary'} icon={<Download className="tw:size-3.5" />}
              onClick={() => setExportOpen(true)}>Export</Button>
            <Button variant="secondary" icon={<Copy className="tw:size-3.5" />} disabled={!canHunt}
              onClick={() => onCopy(hunt)}>Copy</Button>
            <IconButton label="Edit" variant="secondary" icon={<Pencil className="tw:size-3.5" />}
              onClick={() => setEditOpen(true)} />
            <Menu>
              <MenuTrigger asChild>
                <IconButton label="More actions" variant="secondary" icon={<MoreHorizontal className="tw:size-4" />} />
              </MenuTrigger>
              <MenuContent>
                <MenuItem icon={<RefreshCw />} onSelect={() => run(
                  () => modify.mutateAsync({ hunt_id: hunt.hunt_id, recalculate: true }), 'Statistics recalculated')}>
                  Recalculate statistics
                </MenuItem>
                <MenuItem icon={<ExternalLink />} onSelect={() => history.push(`/classic/hunts/${hunt.hunt_id}/overview`)}>
                  Open in classic view
                </MenuItem>
                <MenuSeparator />
                <MenuItem icon={<Archive />} disabled={state === 'archived'} onSelect={() => setConfirm('archive')}>Archive</MenuItem>
                <MenuItem icon={<Trash2 />} danger onSelect={() => setConfirm('delete')}>Delete…</MenuItem>
              </MenuContent>
            </Menu>
          </div>
        </div>

        {/* ---------------------------------------------- Stats */}
        <div className="tw:grid tw:grid-cols-2 tw:md:grid-cols-3 tw:xl:grid-cols-6 tw:gap-2.5 tw:mt-4">
          <MiniStat icon={<Users />} label="Scheduled" value={fmtNum(c.scheduled)} />
          <MiniStat icon={<CircleCheck />} label="Succeeded" value={fmtNum(c.succeeded)} tone="ok"
            onClick={() => setTab('clients')} />
          <MiniStat icon={<CircleX />} label="Failed" value={fmtNum(c.errors)} tone={c.errors ? 'bad' : undefined}
            onClick={() => setTab('errors')} />
          <MiniStat icon={<Loader />} label="In progress" value={fmtNum(c.running)} tone={c.running ? 'accent' : undefined} />
          <MiniStat icon={<Rows3 />} label="Rows" value={fmtNum(c.rows)} onClick={() => setTab('results')} />
          <MiniStat icon={<HardDrive />} label="Files collected" value={fmtBytes(c.bytes)} onClick={() => setTab('files')} />
        </div>
      </div>

      <Tabs value={active} onValueChange={setTab} className="tw:bg-panel" tabs={[
        { value: 'overview', label: 'Overview', icon: <LayoutList /> },
        { value: 'results', label: 'Results', icon: <Table2 />, count: c.rows ? fmtNum(c.rows) : undefined },
        { value: 'clients', label: 'Clients', icon: <Users />, count: flows ? fmtNum(flows.total || flows.rows.length) : undefined },
        { value: 'files', label: 'Files', icon: <FileDown />, count: c.bytes ? fmtBytes(c.bytes) : undefined },
        { value: 'exports', label: 'Exports', icon: <FolderArchive />,
          count: archives.some(f => archiveState(f) === 'preparing') ? <Spinner className="tw:size-3" /> : archives.length || undefined },
        { value: 'errors', label: 'Errors', icon: <TriangleAlert />, count: errorRows.length || undefined },
        { value: 'notebook', label: 'Notebook', icon: <BookOpen /> },
      ]} />

      <div className="tw:flex-1 tw:min-h-0 tw:flex tw:flex-col tw:bg-panel">
        {active === 'overview' && <OverviewTab hunt={hunt} onExport={() => setExportOpen(true)} onTab={setTab} />}
        {active === 'results' && <ResultsTab hunt={hunt} />}
        {active === 'clients' && <ClientsTab huntId={huntId} rows={flows?.rows} columns={flows?.columns} loading={!flows} />}
        {active === 'files' && <FilesTab huntId={huntId} flows={flows?.rows} onExport={() => setExportOpen(true)} />}
        {active === 'exports' && <ExportsTab files={archives} onExport={() => setExportOpen(true)} />}
        {active === 'errors' && <ErrorsTab rows={errorRows} loading={!errors} />}
        {active === 'notebook' && (
          <div className="tw:flex-1 tw:min-h-0 tw:flex tw:relative tw:bg-canvas">
            <Suspense fallback={<div className="tw:p-6"><Spinner /></div>}>
              <HuntNotebook id={`N.${huntId}`} embedded
                create={{ name: `Notebook for Hunt ${huntId}`, description: hunt.hunt_description || 'This is a notebook for processing a hunt.' }} />
            </Suspense>
          </div>
        )}
      </div>

      {editOpen && <EditHuntDialog hunt={hunt} open={editOpen} onOpenChange={setEditOpen} />}
      {exportOpen && <ExportHuntDialog hunt={hunt} open={exportOpen} onOpenChange={setExportOpen}
        onStarted={() => { autoDownload.current.add('*'); setTab('exports'); }} />}

      <ConfirmDialog open={confirm === 'stop'} onOpenChange={v => !v && setConfirm(null)}
        title="Stop this hunt?" confirmLabel="Stop hunt" loading={stopHunts.isPending}
        onConfirm={() => run(() => stopHunts.mutateAsync([hunt.hunt_id]), 'Hunt stopped').then(() => setConfirm(null))}>
        No new clients will be scheduled and collections still in flight will be cancelled. You can resume it later.
      </ConfirmDialog>
      <ConfirmDialog open={confirm === 'archive'} onOpenChange={v => !v && setConfirm(null)}
        title="Archive this hunt?" confirmLabel="Archive" loading={archiveHunts.isPending}
        onConfirm={() => run(() => archiveHunts.mutateAsync([hunt.hunt_id]), 'Hunt archived').then(() => setConfirm(null))}>
        Archived hunts are hidden from the hunt list. Their results are kept.
      </ConfirmDialog>
      <ConfirmDialog open={confirm === 'delete'} onOpenChange={v => !v && setConfirm(null)} danger
        title="Delete this hunt?" confirmLabel="Delete permanently" loading={deleteHunts.isPending}
        onConfirm={() => run(() => deleteHunts.mutateAsync([hunt.hunt_id]), 'Deletion started').then(() => {
          setConfirm(null);
          history.push('/hunts');
        })}>
        This cancels <b>{huntTitle(hunt)}</b> and deletes all collected data from {fmtNum(c.scheduled)} clients.
        This cannot be undone.
      </ConfirmDialog>
    </div>
  );
}

function MiniStat({ icon, label, value, tone, onClick }: {
  icon: React.ReactNode; label: string; value: React.ReactNode; tone?: 'ok' | 'bad' | 'accent'; onClick?: () => void;
}) {
  const color = tone === 'ok' ? 'tw:text-ok' : tone === 'bad' ? 'tw:text-bad' : tone === 'accent' ? 'tw:text-accent' : 'tw:text-faint';
  const Comp = onClick ? 'button' : 'div';
  return (
    <Comp onClick={onClick} type={onClick ? 'button' : undefined}
      className={`tw:flex tw:items-center tw:gap-2.5 tw:rounded-lg tw:border tw:border-line tw:bg-canvas/60 tw:px-3 tw:py-2 tw:text-left ${onClick ? 'tw:hover:border-line-strong tw:hover:bg-hover' : ''}`}>
      <span className={`tw:flex tw:[&>svg]:size-4 ${color}`}>{icon}</span>
      <div className="tw:min-w-0">
        <div className="tw:text-2xs tw:text-muted">{label}</div>
        <div className="tw:text-[15px] tw:font-semibold rx-num tw:leading-tight">{value}</div>
      </div>
    </Comp>
  );
}

// ------------------------------------------------------------ Overview

function OverviewTab({ hunt, onExport, onTab }: { hunt: Hunt; onExport: () => void; onTab: (t: string) => void }) {
  const req = hunt.start_request ?? {};
  const artifacts = req.artifacts ?? hunt.artifacts ?? [];
  const { data: defs } = useArtifactDefinitions(artifacts);
  const cond = hunt.condition ?? {};
  const labels = cond.labels?.label ?? [];
  const excluded = cond.excluded_labels?.label ?? [];
  const os = cond.os?.os;

  const paramsFor = (name: string) => {
    const spec = (req.specs ?? []).find(s => s.artifact === name);
    const env = spec?.parameters?.env ?? [];
    const def = defs?.get(name);
    return env.map(e => ({
      ...e,
      isDefault: def?.parameters?.find(p => p.name === e.key)?.default === e.value,
    }));
  };

  return (
    <div className="tw:flex-1 tw:overflow-auto tw:bg-canvas tw:p-5">
      <div className="tw:grid tw:grid-cols-1 tw:xl:grid-cols-3 tw:gap-4 tw:max-w-[1400px]">
        <div className="tw:xl:col-span-2 tw:space-y-4">
          <Card>
            <CardHeader title="Artifacts" subtitle={`${artifacts.length} artifact${artifacts.length === 1 ? '' : 's'} collected on each client`} />
            <div className="tw:divide-y tw:divide-line">
              {artifacts.map(a => {
                const params = paramsFor(a);
                const def = defs?.get(a);
                return (
                  <div key={a} className="tw:px-4 tw:py-3">
                    <Link to={`/artifacts/${a}`} className="tw:font-mono tw:text-[13px] tw:font-medium tw:text-accent tw:hover:underline">{a}</Link>
                    {def?.description && (
                      <p className="tw:text-xs tw:text-muted tw:mt-1 tw:line-clamp-2">{def.description}</p>
                    )}
                    {params.length > 0 && (
                      <dl className="tw:mt-2 tw:grid tw:grid-cols-[minmax(120px,auto)_1fr] tw:gap-x-4 tw:gap-y-1 tw:text-xs">
                        {params.map(p => (
                          <React.Fragment key={p.key}>
                            <dt className="tw:text-muted tw:font-medium">{p.key}</dt>
                            <dd className="tw:font-mono tw:break-all tw:whitespace-pre-wrap">
                              {p.value || <span className="tw:text-faint">empty</span>}
                              {p.isDefault && <span className="tw:ml-2 tw:text-2xs tw:text-faint tw:font-sans">default</span>}
                            </dd>
                          </React.Fragment>
                        ))}
                      </dl>
                    )}
                  </div>
                );
              })}
            </div>
          </Card>
          <EvidenceCard hunt={hunt} onExport={onExport} onTab={onTab} />
        </div>

        <div className="tw:space-y-4">
          <Card>
            <CardHeader title="Targeting" />
            <dl className="tw:p-4 tw:space-y-3 tw:text-xs">
              <Def label="Include">
                {labels.length ? labels.map(l => <Badge key={l} tone="accent" className="tw:mr-1">{l}</Badge>)
                  : os ? <Badge tone="info">{os}</Badge> : 'All clients'}
              </Def>
              {excluded.length > 0 && (
                <Def label="Exclude">{excluded.map(l => <Badge key={l} tone="bad" className="tw:mr-1">{l}</Badge>)}</Def>
              )}
              {toNum(hunt.client_limit) > 0 && <Def label="Client limit">{fmtNum(hunt.client_limit)}</Def>}
              {(hunt.org_ids?.length ?? 0) > 0 && <Def label="Orgs">{hunt.org_ids!.join(', ')}</Def>}
            </dl>
          </Card>
          <Card>
            <CardHeader title="Schedule & limits" />
            <dl className="tw:p-4 tw:space-y-3 tw:text-xs">
              <Def label="Created">{fmtTime(hunt.create_time)}</Def>
              {toNum(hunt.start_time) > 0 && <Def label="Started">{fmtTime(hunt.start_time)}</Def>}
              <Def label="Expires">{fmtTime(hunt.expires)}</Def>
              <Def label="Timeout">{toNum(req.timeout) ? fmtDuration(toNum(req.timeout)) : 'Default'}</Def>
              <Def label="Max rows">{toNum(req.max_rows) ? fmtNum(req.max_rows) : 'Default'}</Def>
              <Def label="Max upload">{toNum(req.max_upload_bytes) ? fmtBytes(req.max_upload_bytes) : 'Default'}</Def>
              {!!req.cpu_limit && <Def label="CPU limit">{req.cpu_limit}%</Def>}
              {req.urgent && <Def label="Priority"><Badge tone="warn">Urgent</Badge></Def>}
            </dl>
          </Card>
        </div>
      </div>
    </div>
  );
}

function Def({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="tw:grid tw:grid-cols-[96px_1fr] tw:gap-3">
      <dt className="tw:text-muted">{label}</dt>
      <dd>{children}</dd>
    </div>
  );
}

function EvidenceCard({ hunt, onExport, onTab }: { hunt: Hunt; onExport: () => void; onTab: (t: string) => void }) {
  const files: AvailableDownloadFile[] = hunt.stats?.available_downloads?.files ?? [];
  const c = huntCounts(hunt);
  const latest = [...files].sort((a, b) => toNum(b.stats?.timestamp) - toNum(a.stats?.timestamp)).slice(0, 2);
  return (
    <Card>
      <CardHeader title="Evidence" subtitle="Everything this hunt collected, ready to take away"
        actions={<Button size="sm" variant="primary" icon={<Download className="tw:size-3.5" />} onClick={onExport}>Export…</Button>} />
      <div className="tw:grid tw:grid-cols-3 tw:divide-x tw:divide-line tw:border-b tw:border-line">
        <EvidenceStat label="Result rows" value={fmtNum(c.rows)} onClick={() => onTab('results')} />
        <EvidenceStat label="Files collected" value={fmtBytes(c.bytes)} onClick={() => onTab('files')} />
        <EvidenceStat label="Clients with results" value={fmtNum(c.withResults)} onClick={() => onTab('clients')} />
      </div>
      <ArchiveList files={latest}
        empty={<div className="tw:px-4 tw:py-4 tw:text-xs tw:text-faint">No exports yet. Use <b className="tw:font-medium tw:text-muted">Export</b> to package results and files into a zip.</div>} />
      {files.length > latest.length && (
        <button type="button" onClick={() => onTab('exports')}
          className="tw:w-full tw:border-t tw:border-line tw:px-4 tw:py-2 tw:text-left tw:text-xs tw:font-medium tw:text-accent tw:hover:bg-hover">
          All {files.length} exports
        </button>
      )}
    </Card>
  );
}

function EvidenceStat({ label, value, onClick }: { label: string; value: string; onClick: () => void }) {
  return (
    <button type="button" onClick={onClick} className="tw:px-4 tw:py-3 tw:text-left tw:hover:bg-hover">
      <div className="tw:text-2xs tw:text-muted">{label}</div>
      <div className="tw:text-[15px] tw:font-semibold rx-num">{value}</div>
    </button>
  );
}

// --------------------------------------------------------------- Files

function FilesTab({ huntId, flows, onExport }: { huntId: string; flows: Row[] | undefined; onExport: () => void }) {
  const q = useHuntUploads(huntId, flows, true);
  const rows = q.data?.rows ?? [];
  const total = rows.reduce((a, r) => a + toNum(r.file_size as number), 0);
  return (
    <UploadsTable
      rows={rows}
      loading={q.isLoading || flows === undefined}
      storageKey="hunt-files"
      exportName={`${huntId}-files`}
      showHost
      toolbar={
        <div className="tw:flex tw:items-center tw:gap-2 tw:text-xs tw:text-muted">
          {q.isFetching && <Spinner className="tw:size-3.5" />}
          {q.data && <span><b className="tw:text-fg rx-num">{fmtNum(rows.length)}</b> files · {fmtBytes(total)} from {fmtNum(new Set(rows.map(r => r.ClientId)).size)} client{new Set(rows.map(r => r.ClientId)).size === 1 ? '' : 's'}</span>}
          {q.data?.truncated && <Badge tone="warn">Showing the first {HUNT_UPLOADS_MAX_FLOWS} clients. Export for everything.</Badge>}
        </div>
      }
      toolbarRight={<Button size="sm" variant="secondary" icon={<Download className="tw:size-3.5" />} onClick={onExport}>Export all as zip</Button>}
    />
  );
}

// ------------------------------------------------------------- Exports

function ExportsTab({ files, onExport }: { files: AvailableDownloadFile[]; onExport: () => void }) {
  return (
    <div className="tw:flex-1 tw:overflow-auto tw:bg-canvas tw:p-5">
      <Card className="tw:max-w-[960px]">
        <CardHeader title="Exports" subtitle="Zip archives prepared from this hunt. They are kept on the server until the hunt is deleted."
          actions={<Button size="sm" variant="primary" icon={<Download className="tw:size-3.5" />} onClick={onExport}>New export…</Button>} />
        <ArchiveList files={files} empty={
          <EmptyState icon={<FolderArchive />} title="No exports yet"
            action={<Button variant="primary" icon={<Download className="tw:size-3.5" />} onClick={onExport}>Export hunt…</Button>}>
            Package every client’s results and collected files into one zip, or just the combined result tables.
          </EmptyState>} />
      </Card>
    </div>
  );
}

// ------------------------------------------------------------- Results

function ResultsTab({ hunt }: { hunt: Hunt }) {
  const sources = hunt.artifact_sources?.length ? hunt.artifact_sources : (hunt.artifacts ?? []);
  const [source, setSource] = useState(sources[0]);
  useEffect(() => {
    if (!source || !sources.includes(source)) setSource(sources[0]);
  }, [sources, source]);

  const q = useHuntResults(hunt.hunt_id, source);
  const rows = useMemo(() => q.data?.pages.flatMap(p => p.rows) ?? [], [q.data]);
  const columns = q.data?.pages[0]?.columns;
  const uploadRenderers = useMemo(() => withUploadCells(columns, rows, {}), [columns, rows]);
  const ordered = useMemo(() => promoteColumns(columns, Object.keys(uploadRenderers)), [columns, uploadRenderers]);

  if (!sources.length) {
    return <EmptyState title="This hunt has no artifact sources" />;
  }

  return (
    <DataTable
      key={source}
      rows={rows}
      columns={ordered}
      loading={q.isLoading}
      hasMore={q.hasNextPage}
      loadingMore={q.isFetchingNextPage}
      onLoadMore={() => q.fetchNextPage()}
      storageKey={`hunt-results:${source}`}
      exportName={`${hunt.hunt_id}-${source?.replace(/[^\w.-]+/g, '_')}`}
      renderers={{
        ...uploadRenderers,
        ClientId: v => <ClientLink id={String(v ?? '')} />,
        FlowId: (v, r) => <FlowLink clientId={String(r.ClientId ?? '')} flowId={String(v ?? '')} />,
      }}
      toolbar={
        <div className="tw:flex tw:items-center tw:gap-2">
          {sources.length > 1 ? (
            <Select value={source} onChange={e => setSource(e.target.value)} className="tw:max-w-[360px] tw:font-mono tw:text-xs">
              {sources.map(s => <option key={s} value={s}>{s}</option>)}
            </Select>
          ) : <Mono className="tw:text-muted">{source}</Mono>}
          <IconButton size="sm" label="Refresh" icon={<RefreshCw className={`tw:size-3.5 ${q.isFetching ? 'tw:animate-spin' : ''}`} />}
            onClick={() => q.refetch()} />
        </div>
      }
      empty={<EmptyState icon={<Rows3 />} title="No results yet">
        Results appear here as clients finish collecting {source}.
      </EmptyState>}
    />
  );
}

export { ClientLink, FlowLink };

const flowStateTone = (s: string) =>
  s === 'FINISHED' ? 'ok' : s === 'ERROR' ? 'bad' : s === 'RUNNING' || s === 'IN_PROGRESS' ? 'accent' : 'neutral';

// ------------------------------------------------------------- Clients

function ClientsTab({ huntId, rows = [], columns, loading }: {
  huntId: string; rows?: Row[]; columns?: string[]; loading: boolean;
}) {
  return (
    <DataTable
      rows={rows}
      columns={columns?.length ? ['Actions', ...columns] : columns}
      loading={loading}
      storageKey="hunt-clients"
      exportName={`${huntId}-clients`}
      inspector={false}
      columnLabels={{ ClientId: 'Client', StartedTime: 'Started', TotalBytes: 'Files', TotalRows: 'Rows', Actions: '' }}
      columnWidths={{ Actions: 56 }}
      renderers={{
        Actions: (_v, r) => r.State === 'FINISHED' || toNum(r.TotalRows as number) > 0 || toNum(r.TotalBytes as number) > 0
          ? <FlowDownloadButton compact clientId={String(r.ClientId ?? '')} flowId={String(r.FlowId ?? '')} />
          : null,
        ClientId: (v, r) => <ClientLink id={String(v ?? '')} hostname={undefined} key={String(r.ClientId)} />,
        Hostname: v => <span className="tw:font-medium">{String(v ?? '')}</span>,
        FlowId: (v, r) => <FlowLink clientId={String(r.ClientId ?? '')} flowId={String(v ?? '')} />,
        State: v => <Badge tone={flowStateTone(String(v))} dot>{String(v ?? '')}</Badge>,
        StartedTime: v => <Tooltip content={fmtTime(v as number)}><span>{timeAgo(v as number)}</span></Tooltip>,
        Duration: v => <span className="rx-num">{fmtDuration(toNum(v as number))}</span>,
        TotalBytes: v => <span className="rx-num">{fmtBytes(v as number)}</span>,
        TotalRows: v => <span className="rx-num">{fmtNum(v as number)}</span>,
      }}
      empty={<EmptyState icon={<Users />} title="No clients scheduled yet">
        Clients are scheduled when they check in, as long as the hunt is running.
      </EmptyState>}
    />
  );
}

// -------------------------------------------------------------- Errors

function ErrorsTab({ rows, loading }: { rows: Row[]; loading: boolean }) {
  return (
    <DataTable
      rows={rows}
      columns={['ClientId', 'FlowId', 'StartTime', 'EndTime', 'Status', 'Error']}
      loading={loading}
      storageKey="hunt-errors"
      exportName="hunt-errors"
      columnWidths={{ Error: 600 }}
      renderers={{
        ClientId: v => <ClientLink id={String(v ?? '')} />,
        FlowId: (v, r) => <FlowLink clientId={String(r.ClientId ?? '')} flowId={String(v ?? '')} />,
        Status: v => <Badge tone={flowStateTone(String(v))} dot>{String(v ?? '')}</Badge>,
        StartTime: v => <span>{fmtTime(new Date(String(v)))}</span>,
        EndTime: v => <span>{v ? fmtTime(new Date(String(v))) : ''}</span>,
        Error: v => <span className="tw:text-bad">{String(v ?? '')}</span>,
      }}
      empty={<EmptyState icon={<CircleCheck />} title="No errors">Every client that finished did so without errors.</EmptyState>}
    />
  );
}
