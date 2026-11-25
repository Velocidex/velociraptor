// One collection: what was asked for, what came back (results, files,
// logs) and how to take it away (zip exports).
import { lazy, Suspense, useEffect, useMemo, useState } from 'react';
import { useHistory } from 'react-router-dom';
import { toast } from 'sonner';
import {
  Ban, BookOpen, CircleAlert, Clock, Download, ExternalLink, FileDown, FolderArchive, LayoutList, Logs, MoreHorizontal, RefreshCw,
  Rows3, RotateCcw, ScrollText, Table2,
} from 'lucide-react';
import { useCancelFlow, useFlow, useFlowTable, useArtifactDefinitions } from '../../lib/queries';
import { useFlowDownloader } from '../evidence/FlowExportButton';
import type { AvailableDownloadFile, Flow } from '../../lib/types';
import { fmtBytes, fmtDuration, fmtNum, fmtTime, timeAgo, toNum } from '../../lib/format';
import { archiveState } from '../../lib/downloads';
import {
  Badge, Button, Card, CardHeader, ConfirmDialog, CopyButton, EmptyState, IconButton, Menu, MenuContent, MenuItem, MenuTrigger,
  Mono, Select, Skeleton, Spinner, Tabs, Tooltip,
} from '../../ui/primitives';
import { DataTable } from '../../ui/DataTable';
import { ArchiveList } from '../evidence/ArchiveList';
import { promoteColumns, UploadsTable, withUploadCells } from '../evidence/UploadLink';

type TabKey = 'overview' | 'results' | 'files' | 'logs' | 'notebook';

const FlowNotebook = lazy(() => import('../notebooks/NotebooksPage').then(m => ({ default: m.NotebookView })));
const normalizeTab = (t?: string): TabKey =>
  t === 'results' || t === 'files' || t === 'logs' || t === 'notebook' ? t : 'overview';

const RUNNING = new Set(['RUNNING', 'IN_PROGRESS', 'WAITING', 'UNSET']);

export function FlowStateBadge({ state }: { state: string }) {
  if (state === 'FINISHED') return <Badge tone="ok" dot>Finished</Badge>;
  if (state === 'ERROR') return <Badge tone="bad" dot>Error</Badge>;
  if (RUNNING.has(state)) return <Badge tone="accent" dot>Running</Badge>;
  return <Badge dot>{state.toLowerCase() || 'unknown'}</Badge>;
}

export default function FlowDetail({ clientId, flowId, tab, hostname, onCollectAgain }: {
  clientId: string; flowId: string; tab?: string; hostname: string; onCollectAgain: (f: Flow) => void;
}) {
  const history = useHistory();
  const q = useFlow(clientId, flowId);
  const flow = q.data?.context;
  const archives = useMemo(() => q.data?.available_downloads?.files ?? [], [q.data]);
  const zip = useFlowDownloader(clientId, flowId, hostname);
  const cancel = useCancelFlow();
  const [confirmCancel, setConfirmCancel] = useState(false);
  const active = normalizeTab(tab);
  const setTab = (t: string) => history.push(`/collected/${clientId}/${flowId}/${t}`);

  if (q.isLoading) {
    return <div className="tw:p-6 tw:space-y-3"><Skeleton className="tw:h-6 tw:w-1/3" /><Skeleton className="tw:w-1/4" /></div>;
  }
  if (!flow) {
    return <EmptyState title="Collection not found">{q.error ? (q.error as Error).message : flowId}</EmptyState>;
  }

  const state = flow.state ?? '';
  const running = RUNNING.has(state);
  const artifacts = flow.request?.artifacts ?? [];
  const files = toNum(flow.total_uploaded_files);
  const bytes = toNum(flow.total_uploaded_bytes);
  const rows = toNum(flow.total_collected_rows);
  const logs = toNum(flow.total_logs);
  const duration = toNum(flow.execution_duration) / 1e9;
  const preparing = zip.busy || archives.some(f => archiveState(f) === 'preparing');
  const exportZip = zip.start;

  return (
    <div className="tw:flex tw:flex-col tw:flex-1 tw:min-h-0">
      <div className="tw:px-4 tw:md:px-5 tw:pt-4 tw:pb-3 tw:bg-panel">
        <div className="tw:flex tw:flex-wrap tw:sm:flex-nowrap tw:items-start tw:gap-3">
          <div className="tw:min-w-0 tw:flex-1 tw:max-sm:basis-full">
            <div className="tw:flex tw:items-center tw:gap-2.5">
              <h2 className="tw:text-lg tw:font-semibold tw:truncate tw:font-mono">
                {artifacts[0] ?? flowId}{artifacts.length > 1 && <span className="tw:text-faint tw:font-sans tw:text-sm"> +{artifacts.length - 1} more</span>}
              </h2>
              <FlowStateBadge state={state} />
            </div>
            <div className="tw:flex tw:items-center tw:gap-2 tw:mt-1 tw:text-xs tw:text-muted tw:flex-wrap">
              <Mono>{flowId}</Mono><CopyButton text={flowId} />
              <span>·</span>
              <span>{hostname}</span>
              <span>·</span>
              <Tooltip content={fmtTime(flow.create_time)}><span>Started {timeAgo(flow.create_time)} by <b className="tw:font-medium tw:text-fg">{flow.request?.creator}</b></span></Tooltip>
            </div>
          </div>
          <div className="tw:flex tw:items-center tw:gap-1.5 tw:shrink-0">
            {running && (
              <Button variant="secondary" icon={<Ban className="tw:size-3.5" />} onClick={() => setConfirmCancel(true)}>Cancel</Button>
            )}
            <Button variant="primary" icon={<Download className="tw:size-3.5" />} loading={preparing}
              disabled={running && rows === 0 && files === 0} onClick={exportZip}>
              {preparing ? 'Building zip…' : 'Download zip'}
            </Button>
            <Button variant="secondary" icon={<RotateCcw className="tw:size-3.5" />} onClick={() => onCollectAgain(flow)}>Collect again</Button>
            <Menu>
              <MenuTrigger asChild>
                <IconButton label="More actions" variant="secondary" icon={<MoreHorizontal className="tw:size-4" />} />
              </MenuTrigger>
              <MenuContent>
                <MenuItem icon={<RefreshCw />} onSelect={() => q.refetch()}>Refresh</MenuItem>
                <MenuItem icon={<ExternalLink />} onSelect={() => history.push(`/classic/collected/${clientId}/${flowId}`)}>Open in classic view</MenuItem>
              </MenuContent>
            </Menu>
          </div>
        </div>

        <div className="tw:grid tw:grid-cols-2 tw:md:grid-cols-4 tw:gap-2.5 tw:mt-4">
          <Mini icon={<Rows3 />} label="Rows" value={fmtNum(rows)} onClick={() => setTab('results')} />
          <Mini icon={<FileDown />} label="Files" value={files ? `${fmtNum(files)} · ${fmtBytes(bytes)}` : '0'} onClick={() => setTab('files')} />
          <Mini icon={<ScrollText />} label="Log lines" value={fmtNum(logs)} onClick={() => setTab('logs')} />
          <Mini icon={<Clock />} label="Duration" value={duration ? fmtDuration(duration) : running ? 'Running…' : '—'} />
        </div>
        {flow.status && state === 'ERROR' && (
          <div className="tw:mt-3 tw:flex tw:items-start tw:gap-2 tw:rounded-lg tw:bg-bad-soft tw:px-3 tw:py-2 tw:text-xs tw:text-bad">
            <CircleAlert className="tw:size-4 tw:shrink-0" /><span className="tw:break-all">{flow.status}</span>
          </div>
        )}
      </div>

      <Tabs value={active} onValueChange={setTab} className="tw:bg-panel" tabs={[
        { value: 'overview', label: 'Overview', icon: <LayoutList />,
          count: preparing ? <Spinner className="tw:size-3" /> : undefined },
        { value: 'results', label: 'Results', icon: <Table2 />, count: rows ? fmtNum(rows) : undefined },
        { value: 'files', label: 'Files', icon: <FileDown />, count: files ? fmtNum(files) : undefined },
        { value: 'logs', label: 'Logs', icon: <Logs />, count: logs ? fmtNum(logs) : undefined },
        { value: 'notebook', label: 'Notebook', icon: <BookOpen /> },
      ]} />

      <div className="tw:flex-1 tw:min-h-0 tw:flex tw:flex-col tw:bg-panel">
        {active === 'overview' && <Overview flow={flow} archives={archives} onExport={exportZip} exporting={preparing} />}
        {active === 'results' && <Results clientId={clientId} flow={flow} live={running} />}
        {active === 'files' && <Files clientId={clientId} flowId={flowId} live={running} onExport={exportZip} />}
        {active === 'logs' && <LogsTab clientId={clientId} flowId={flowId} live={running} />}
        {active === 'notebook' && (
          <div className="tw:flex-1 tw:min-h-0 tw:flex tw:bg-canvas">
            <Suspense fallback={null}>
              <FlowNotebook key={flowId} id={`N.${flowId}-${clientId}`} embedded
                create={{ name: `Notebook for Collection ${flowId}` }} />
            </Suspense>
          </div>
        )}
      </div>

      <ConfirmDialog open={confirmCancel} onOpenChange={setConfirmCancel} danger
        title="Cancel this collection?" confirmLabel="Cancel collection" loading={cancel.isPending}
        onConfirm={() => cancel.mutate({ client_id: clientId, flow_id: flowId }, {
          onSuccess: () => { toast.success('Collection cancelled'); setConfirmCancel(false); },
          onError: e => toast.error((e as Error).message),
        })}>
        The client stops collecting. Anything already collected is kept.
      </ConfirmDialog>
    </div>
  );
}

function Mini({ icon, label, value, onClick }: { icon: React.ReactNode; label: string; value: string; onClick?: () => void }) {
  const Comp = onClick ? 'button' : 'div';
  return (
    <Comp type={onClick ? 'button' : undefined} onClick={onClick}
      className={`tw:flex tw:items-center tw:gap-2.5 tw:rounded-lg tw:border tw:border-line tw:bg-canvas/60 tw:px-3 tw:py-2 tw:text-left ${onClick ? 'tw:hover:border-line-strong tw:hover:bg-hover' : ''}`}>
      <span className="tw:flex tw:text-faint tw:[&>svg]:size-4">{icon}</span>
      <div className="tw:min-w-0">
        <div className="tw:text-2xs tw:text-muted">{label}</div>
        <div className="tw:text-[15px] tw:font-semibold rx-num tw:leading-tight tw:truncate">{value}</div>
      </div>
    </Comp>
  );
}

// ------------------------------------------------------------ Overview

function Overview({ flow, archives, onExport, exporting }: {
  flow: Flow; archives: AvailableDownloadFile[];
  onExport: () => void; exporting: boolean;
}) {
  const req = flow.request ?? {};
  const artifacts = req.artifacts ?? [];
  const { data: defs } = useArtifactDefinitions(artifacts);
  const stats = flow.query_stats ?? [];
  return (
    <div className="tw:flex-1 tw:overflow-auto tw:bg-canvas tw:p-5">
      <div className="tw:grid tw:grid-cols-1 tw:xl:grid-cols-3 tw:gap-4">
        <div className="tw:xl:col-span-2 tw:space-y-4">
          <Card>
            <CardHeader title="Take it away" subtitle="A zip with every result, log and uploaded file from this collection"
              actions={<Button size="sm" variant="primary" icon={<Download className="tw:size-3.5" />} loading={exporting} onClick={onExport}>Download zip</Button>} />
            <ArchiveList files={archives} empty={
              <div className="tw:px-4 tw:py-4 tw:text-xs tw:text-faint">No zip built yet. <b className="tw:font-medium tw:text-muted">Download zip</b> builds one and downloads it when ready.</div>
            } />
          </Card>
          <Card>
            <CardHeader title="Artifacts" subtitle={`${artifacts.length} artifact${artifacts.length === 1 ? '' : 's'} requested`} />
            <div className="tw:divide-y tw:divide-line">
              {artifacts.map(a => {
                const env = (req.specs ?? []).find(s => s.artifact === a)?.parameters?.env ?? [];
                return (
                  <div key={a} className="tw:px-4 tw:py-3">
                    <span className="tw:font-mono tw:text-[13px] tw:font-medium tw:text-accent">{a}</span>
                    {defs?.get(a)?.description && <p className="tw:text-xs tw:text-muted tw:mt-1 tw:line-clamp-2">{defs.get(a)!.description}</p>}
                    {env.length > 0 && (
                      <dl className="tw:mt-2 tw:grid tw:grid-cols-[minmax(120px,auto)_1fr] tw:gap-x-4 tw:gap-y-1 tw:text-xs">
                        {env.map(e => (
                          <div key={e.key} className="tw:contents">
                            <dt className="tw:text-muted tw:font-medium">{e.key}</dt>
                            <dd className="tw:font-mono tw:break-all tw:whitespace-pre-wrap">{e.value || <span className="tw:text-faint">empty</span>}</dd>
                          </div>
                        ))}
                      </dl>
                    )}
                  </div>
                );
              })}
            </div>
          </Card>
        </div>
        <div className="tw:space-y-4">
          <Card>
            <CardHeader title="Queries" subtitle="One per artifact source" />
            <div className="tw:divide-y tw:divide-line">
              {stats.length === 0 && <div className="tw:px-4 tw:py-3 tw:text-xs tw:text-faint">Waiting for the client…</div>}
              {stats.map((s, i) => (
                <div key={i} className="tw:px-4 tw:py-2.5 tw:text-xs tw:space-y-1">
                  <div className="tw:flex tw:items-center tw:gap-2">
                    <span className="tw:font-mono tw:truncate tw:flex-1">{s.names_with_response?.join(', ') || `Query ${i + 1}`}</span>
                    {s.status === 'ERROR' || s.error_message ? <Badge tone="bad">Error</Badge>
                      : s.status === 'OK' || !RUNNING.has(flow.state ?? '') ? <Badge tone="ok">Done</Badge> : <Badge tone="accent">Running</Badge>}
                  </div>
                  <div className="tw:flex tw:gap-3 tw:text-2xs tw:text-muted rx-num">
                    <span>{fmtNum(s.result_rows)} rows</span>
                    {toNum(s.uploaded_files) > 0 && <span>{fmtNum(s.uploaded_files)} files · {fmtBytes(s.uploaded_bytes)}</span>}
                    <span>{fmtDuration(toNum(s.duration) / 1e9)}</span>
                  </div>
                  {s.error_message && <div className="tw:text-2xs tw:text-bad tw:break-all">{s.error_message}</div>}
                </div>
              ))}
            </div>
          </Card>
          <Card>
            <CardHeader title="Limits" />
            <dl className="tw:p-4 tw:space-y-2 tw:text-xs">
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

// ------------------------------------------------------------- Results

function Results({ clientId, flow, live }: { clientId: string; flow: Flow; live: boolean }) {
  const sources = flow.artifacts_with_results ?? [];
  const [source, setSource] = useState(sources[0]);
  useEffect(() => {
    if (!source || !sources.includes(source)) setSource(sources[0]);
  }, [sources, source]);
  const q = useFlowTable(clientId, flow.session_id, { artifact: source }, live);
  const rows = useMemo(() => q.data?.pages.flatMap(p => p.rows) ?? [], [q.data]);
  const columns = q.data?.pages[0]?.columns;
  const uploadRenderers = useMemo(
    () => withUploadCells(columns, rows, { clientId, flowId: flow.session_id }), [columns, rows, clientId, flow.session_id]);
  const ordered = useMemo(() => promoteColumns(columns, Object.keys(uploadRenderers)), [columns, uploadRenderers]);

  if (!sources.length) {
    return <EmptyState icon={<Rows3 />} title={live ? 'Waiting for results' : 'No results'}>
      {live ? 'Rows appear here as the client sends them.' : 'This collection returned no rows.'}
    </EmptyState>;
  }
  return (
    <DataTable key={source} rows={rows} columns={ordered} loading={q.isLoading}
      hasMore={q.hasNextPage} loadingMore={q.isFetchingNextPage} onLoadMore={() => q.fetchNextPage()}
      storageKey={`flow-results:${source}`} exportName={`${flow.session_id}-${source?.replace(/[^\w.-]+/g, '_')}`}
      renderers={uploadRenderers}
      toolbar={sources.length > 1
        ? <Select value={source} onChange={e => setSource(e.target.value)} className="tw:max-w-[360px] tw:font-mono tw:text-xs">
            {sources.map(s => <option key={s} value={s}>{s}</option>)}
          </Select>
        : <Mono className="tw:text-muted">{source}</Mono>} />
  );
}

// --------------------------------------------------------------- Files

function Files({ clientId, flowId, live, onExport }: { clientId: string; flowId: string; live: boolean; onExport: () => void }) {
  const q = useFlowTable(clientId, flowId, { type: 'uploads' }, live);
  const rows = useMemo(() => q.data?.pages.flatMap(p => p.rows) ?? [], [q.data]);
  const total = rows.reduce((a, r) => a + toNum(r.file_size as number), 0);
  return (
    <UploadsTable rows={rows} loading={q.isLoading} storageKey="flow-files" exportName={`${flowId}-files`}
      fixed={{ clientId, flowId }}
      hasMore={q.hasNextPage} loadingMore={q.isFetchingNextPage} onLoadMore={() => q.fetchNextPage()}
      toolbar={rows.length > 0 && <span className="tw:text-xs tw:text-muted"><b className="tw:text-fg rx-num">{fmtNum(rows.length)}</b> files · {fmtBytes(total)}</span>}
      toolbarRight={rows.length > 0 && <Button size="sm" variant="secondary" icon={<FolderArchive className="tw:size-3.5" />} onClick={onExport}>All files as zip</Button>} />
  );
}

// ---------------------------------------------------------------- Logs

function LogsTab({ clientId, flowId, live }: { clientId: string; flowId: string; live: boolean }) {
  const q = useFlowTable(clientId, flowId, { type: 'log' }, live);
  const rows = useMemo(() => q.data?.pages.flatMap(p => p.rows) ?? [], [q.data]);
  return (
    <DataTable rows={rows} columns={['client_time', 'level', 'message']} loading={q.isLoading}
      hasMore={q.hasNextPage} loadingMore={q.isFetchingNextPage} onLoadMore={() => q.fetchNextPage()}
      storageKey="flow-logs" exportName={`${flowId}-logs`} inspector={false}
      columnLabels={{ client_time: 'Time', level: 'Level', message: 'Message' }}
      columnWidths={{ client_time: 170, level: 90, message: 900 }}
      renderers={{
        client_time: v => <span className="rx-num tw:text-muted">{fmtTime(v as number)}</span>,
        level: v => {
          const l = String(v ?? '');
          return <Badge tone={l === 'ERROR' ? 'bad' : l === 'WARN' ? 'warn' : l === 'DEBUG' ? 'neutral' : 'info'}>{l}</Badge>;
        },
        message: (v, r) => <span className={`tw:font-mono tw:text-[12px] tw:whitespace-pre-wrap ${r.level === 'ERROR' ? 'tw:text-bad' : ''}`}>{String(v ?? '')}</span>,
      }}
      empty={<EmptyState icon={<ScrollText />} title="No log lines" />} />
  );
}
