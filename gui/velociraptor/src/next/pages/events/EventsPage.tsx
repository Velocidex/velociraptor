// Event monitoring for the server and for clients.
//
// The legacy screen hid the data behind a timeline widget and a raw JSON
// editor for configuration. Here: pick an artifact with data, pick a time
// window (quick ranges or any day that has data), read the table, which
// keeps refreshing while the window includes "now". "Configure" edits
// which artifacts run, with their parameters, per label for clients.
import { lazy, Suspense, useEffect, useMemo, useState } from 'react';
import { useHistory, useParams } from 'react-router-dom';
import { toast } from 'sonner';
import {
  Activity, CalendarDays, Download, ExternalLink, FileText, MoreHorizontal, Plus, Radio, Search, Server, Settings2,
  Tag, Monitor,
} from 'lucide-react';
import {
  useArtifactDefinitions, useArtifactSearch, useClient, useEventArtifacts, useEventDays, useEventTable,
  useMonitoringState, useSetMonitoringState, type ClientMonitoringState, type MonitoringSpec, type MonitoringTable,
} from '../../lib/queries';
import { href } from '../../lib/api';
import { fmtNum, fmtTime, timeAgo } from '../../lib/format';
import {
  Badge, Button, Dialog, EmptyState, Input, Menu, MenuContent, MenuItem, MenuTrigger, Segmented, Skeleton, Spinner,
  Tooltip, cn,
} from '../../ui/primitives';
import { DataTable } from '../../ui/DataTable';
import { useSelectedClient } from '../../shell/client-context';
import { ArtifactParams } from '../hunts/NewHuntDialog';
import { MobileBack, isPhone } from '../../ui/MobileBack';

const ClientPicker = lazy(() => import('../../ui/ClientPicker'));

type Range = '15m' | '1h' | '24h' | '7d' | number;
const QUICK: { value: Range; label: string; secs: number }[] = [
  { value: '15m', label: '15 min', secs: 900 },
  { value: '1h', label: '1 hour', secs: 3600 },
  { value: '24h', label: '24 hours', secs: 86400 },
  { value: '7d', label: '7 days', secs: 7 * 86400 },
];

function rangeBounds(r: Range, now: number): { start: number; end: number; live: boolean } {
  if (typeof r === 'number') return { start: r, end: r + 86400, live: r + 86400 > now };
  const q = QUICK.find(x => x.value === r)!;
  // Round to the minute so the query key is stable between renders.
  const start = Math.floor((now - q.secs) / 60) * 60;
  return { start, end: 0, live: true };
}

const dayLabel = (ts: number) => new Date(ts * 1000).toLocaleDateString(undefined, { month: 'short', day: 'numeric', weekday: 'short' });

export default function EventsPage() {
  const { client_id: clientId, artifact: rawArtifact } = useParams<{ client_id: string; artifact?: string }>();
  const artifact = rawArtifact ? decodeURIComponent(rawArtifact) : undefined;
  const history = useHistory();
  const server = clientId === 'server';
  const { data: client } = useClient(server ? undefined : clientId);
  const { setClient } = useSelectedClient();
  const { data: available, isLoading } = useEventArtifacts(clientId);
  const { data: state } = useMonitoringState(server);
  const [configOpen, setConfigOpen] = useState(false);
  const [picker, setPicker] = useState(false);
  const [filter, setFilter] = useState('');

  useEffect(() => {
    if (client) setClient(client);
  }, [client, setClient]);

  // Artifacts currently scheduled to run (all labels for clients).
  const running = useMemo(() => {
    const s = new Set<string>(state?.artifacts?.artifacts ?? []);
    for (const l of state?.label_events ?? []) for (const a of l.artifacts?.artifacts ?? []) s.add(a);
    return s;
  }, [state]);

  const list = useMemo(() => {
    const names = new Set((available ?? []).map(a => a.artifact));
    for (const r of running) names.add(r);
    const f = filter.toLowerCase();
    return [...names].filter(n => !f || n.toLowerCase().includes(f)).sort();
  }, [available, running, filter]);
  const withData = new Set((available ?? []).map(a => a.artifact));

  useEffect(() => {
    if (!artifact && list.length && !isPhone()) {
      const first = list.find(n => withData.has(n)) ?? list[0];
      history.replace(`/events/${clientId}/${encodeURIComponent(first)}`);
    }
  }, [artifact, list]); // eslint-disable-line react-hooks/exhaustive-deps

  const title = server ? 'Server events' : `Client events`;
  const host = client?.os_info?.hostname || clientId;

  return (
    <div className="rx-split tw:flex tw:flex-1 tw:min-h-0" data-detail={rawArtifact ? 1 : 0}>
      <aside className="rx-split-list tw:w-80 tw:shrink-0 tw:border-r tw:border-line tw:flex tw:flex-col tw:bg-panel">
        <div className="tw:p-3 tw:border-b tw:border-line tw:flex tw:flex-col tw:gap-2">
          <div className="tw:flex tw:items-center tw:gap-2">
            {server ? <Server className="tw:size-4 tw:text-faint" /> : <Radio className="tw:size-4 tw:text-faint" />}
            <div className="tw:flex-1 tw:min-w-0">
              <h1 className="tw:text-[15px] tw:font-semibold tw:leading-tight">{title}</h1>
              {!server && (
                <button className="tw:text-2xs tw:text-muted tw:hover:text-accent tw:truncate tw:block" onClick={() => setPicker(true)}>
                  {host} · change
                </button>
              )}
            </div>
            <Button size="sm" icon={<Settings2 />} onClick={() => setConfigOpen(true)}>Configure</Button>
          </div>
          <div className="tw:relative">
            <Search className="tw:absolute tw:left-2.5 tw:top-1/2 tw:-translate-y-1/2 tw:size-3.5 tw:text-faint" />
            <Input value={filter} onChange={e => setFilter(e.target.value)} placeholder="Filter artifacts" className="tw:pl-8" />
          </div>
        </div>
        <nav className="tw:flex-1 tw:overflow-y-auto tw:py-1">
          {isLoading && Array.from({ length: 5 }, (_, i) => <Skeleton key={i} className="tw:h-10 tw:mx-2 tw:my-1" />)}
          {list.map(n => (
            <button key={n} onClick={() => history.push(`/events/${clientId}/${encodeURIComponent(n)}`)}
              className={cn('tw:w-full tw:flex tw:items-center tw:gap-2 tw:px-3 tw:py-2 tw:text-left tw:hover:bg-hover',
                n === artifact && 'tw:bg-accent-soft tw:hover:bg-accent-soft')}>
              <Tooltip content={running.has(n) ? 'Running now' : 'Not running (historical data only)'}>
                <span className={cn('tw:size-2 tw:rounded-full tw:shrink-0', running.has(n) ? 'tw:bg-ok' : 'tw:bg-line-strong')} />
              </Tooltip>
              <span className={cn('tw:flex-1 tw:min-w-0 tw:truncate tw:font-mono tw:text-[12px]', n === artifact && 'tw:text-accent tw:font-medium')}>{n}</span>
              {!withData.has(n) && <span className="tw:text-2xs tw:text-faint">no data</span>}
            </button>
          ))}
          {!isLoading && list.length === 0 && (
            <div className="tw:px-4 tw:py-8 tw:text-center tw:text-xs tw:text-faint">
              Nothing is being monitored yet.
              <div className="tw:mt-3"><Button size="sm" icon={<Plus />} onClick={() => setConfigOpen(true)}>Add an artifact</Button></div>
            </div>
          )}
        </nav>
      </aside>
      <main className="rx-split-detail tw:flex-1 tw:min-w-0 tw:flex tw:flex-col">
        <MobileBack to={`/events/${clientId}`} label="All event artifacts" />
        {artifact
          ? <EventView key={`${clientId}:${artifact}`} clientId={clientId} artifact={artifact} running={running.has(artifact)} />
          : !isLoading && <EmptyState className="tw:mt-24" icon={<Activity />} title="Pick an artifact">
              Monitoring artifacts stream rows continuously. Choose one on the left to see what it has recorded.
            </EmptyState>}
      </main>
      {configOpen && <MonitoringDialog server={server} onClose={() => setConfigOpen(false)} />}
      {picker && (
        <Suspense fallback={null}>
          <ClientPicker open onOpenChange={setPicker} title="Show events from which client?" allowServer
            onPick={id => { setPicker(false); history.push(`/events/${id || 'server'}`); }} />
        </Suspense>
      )}
    </div>
  );
}

function EventView({ clientId, artifact, running }: { clientId: string; artifact: string; running: boolean }) {
  const history = useHistory();
  const [mode, setMode] = useState<'rows' | 'logs'>('rows');
  const { data: days } = useEventDays(clientId, artifact);
  const dayList = useMemo(() => [...(mode === 'logs' ? days?.logs : days?.rows) ?? []].sort((a, b) => b - a), [days, mode]);
  const [range, setRange] = useState<Range>('24h');
  const [now, setNow] = useState(() => Math.floor(Date.now() / 1000));

  // When the default window is empty but older days have data, jump to
  // the newest of those instead of showing nothing.
  useEffect(() => {
    if (range === '24h' && dayList.length && dayList[0] + 86400 < now - 86400) setRange(dayList[0]);
  }, [dayList]); // eslint-disable-line react-hooks/exhaustive-deps

  const b = rangeBounds(range, now);
  const q = useEventTable(clientId, artifact, { start: b.start, end: b.end }, mode === 'logs', b.live);
  const rows = useMemo(() => (q.data?.pages ?? []).flatMap(p => p.rows), [q.data]);
  const columns = q.data?.pages[0]?.columns;

  const dl = (format: 'csv' | 'json') => href('/api/v1/DownloadTable', {
    client_id: clientId, artifact,
    type: clientId === 'server' ? (mode === 'logs' ? 'SERVER_EVENT_LOGS' : 'SERVER_EVENT') : (mode === 'logs' ? 'CLIENT_EVENT_LOGS' : 'CLIENT_EVENT'),
    start_time: b.start, end_time: b.end || Math.floor(Date.now() / 1000) + 60, rows: 1,
    download_format: format, download_filename: `${artifact.replace(/\//g, '_')}-${clientId}`,
  });

  return (
    <>
      <div className="tw:px-5 tw:pt-4 tw:pb-3 tw:border-b tw:border-line tw:flex tw:flex-col tw:gap-3 tw:bg-panel">
        <div className="tw:flex tw:flex-wrap tw:items-center tw:gap-3">
          <h2 className="tw:font-mono tw:text-[15px] tw:font-semibold tw:truncate tw:min-w-0">{artifact}</h2>
          {running ? <Badge tone="ok" dot>Running</Badge> : <Badge tone="neutral">Stopped</Badge>}
          <div className="tw:ml-auto tw:flex tw:items-center tw:gap-1.5">
            <Segmented value={mode} onChange={setMode} options={[
              { value: 'rows', label: 'Results' }, { value: 'logs', label: 'Logs' },
            ]} />
            <Menu>
              <MenuTrigger asChild>
                <Button size="sm" icon={<Download />}>Export</Button>
              </MenuTrigger>
              <MenuContent>
                <MenuItem icon={<FileText />} onSelect={() => window.open(dl('csv'), '_blank')}>This window as CSV</MenuItem>
                <MenuItem icon={<FileText />} onSelect={() => window.open(dl('json'), '_blank')}>This window as JSON</MenuItem>
              </MenuContent>
            </Menu>
            <Menu>
              <MenuTrigger asChild>
                <Button size="sm" variant="ghost" square icon={<MoreHorizontal />} aria-label="More" />
              </MenuTrigger>
              <MenuContent>
                <MenuItem icon={<Search />} onSelect={() => history.push(`/artifacts/${encodeURIComponent(artifact)}`)}>View artifact</MenuItem>
                <MenuItem icon={<ExternalLink />} onSelect={() => history.push(`/classic/events/${clientId}/${encodeURIComponent(artifact)}`)}>Classic timeline view</MenuItem>
              </MenuContent>
            </Menu>
          </div>
        </div>
        <div className="tw:flex tw:items-center tw:gap-1.5 tw:flex-wrap">
          <span className="tw:text-2xs tw:text-faint tw:mr-1 tw:uppercase tw:tracking-wide tw:font-semibold">Last</span>
          {QUICK.map(o => (
            <button key={o.label} onClick={() => { setNow(Math.floor(Date.now() / 1000)); setRange(o.value); }}
              className={cn('tw:h-7 tw:px-2.5 tw:rounded-md tw:text-xs tw:border',
                range === o.value ? 'tw:border-accent tw:bg-accent-soft tw:text-accent tw:font-medium' : 'tw:border-line tw:text-muted tw:hover:bg-hover')}>
              {o.label}
            </button>
          ))}
          {dayList.length > 0 && <span className="tw:w-px tw:h-5 tw:bg-line tw:mx-1.5" />}
          {dayList.slice(0, 7).map(d => (
            <button key={d} onClick={() => setRange(d)}
              className={cn('tw:h-7 tw:px-2.5 tw:rounded-md tw:text-xs tw:border tw:flex tw:items-center tw:gap-1',
                range === d ? 'tw:border-accent tw:bg-accent-soft tw:text-accent tw:font-medium' : 'tw:border-line tw:text-muted tw:hover:bg-hover')}>
              <CalendarDays className="tw:size-3" />{dayLabel(d)}
            </button>
          ))}
          {dayList.length > 7 && (
            <select value={typeof range === 'number' && !dayList.slice(0, 7).includes(range) ? range : ''}
              onChange={e => e.target.value && setRange(Number(e.target.value))}
              className="tw:h-7 tw:px-2 tw:rounded-md tw:text-xs tw:border tw:border-line tw:bg-panel tw:text-muted">
              <option value="">Older…</option>
              {dayList.slice(7).map(d => <option key={d} value={d}>{new Date(d * 1000).toLocaleDateString()}</option>)}
            </select>
          )}
          {b.live && (
            <span className="tw:ml-auto tw:flex tw:items-center tw:gap-1.5 tw:text-2xs tw:text-muted">
              <span className="tw:size-1.5 tw:rounded-full tw:bg-ok tw:animate-pulse" />Live, refreshing every 10s
            </span>
          )}
        </div>
      </div>
      <div className="tw:flex-1 tw:min-h-0 tw:bg-panel">
        <DataTable key={`${mode}`} rows={rows} loading={q.isLoading}
          columns={columns?.includes('_ts') ? ['_ts', ...columns.filter(c => c !== '_ts')] : columns}
          shownColumns={['_ts']}
          hasMore={q.hasNextPage} loadingMore={q.isFetchingNextPage} onLoadMore={() => q.fetchNextPage()}
          storageKey={`events:${artifact}:${mode}`} exportName={artifact.replace(/\//g, '_')}
          hiddenColumns={['_Source', '_Writer', '_ClientId']}
          columnLabels={{ _ts: 'Received' }}
          renderers={{
            _ts: v => <Tooltip content={timeAgo(v as number)}><span className="tw:text-muted tw:whitespace-nowrap">{fmtTime(v as number)}</span></Tooltip>,
            client_time: v => <span className="tw:text-muted tw:whitespace-nowrap">{fmtTime(v as number)}</span>,
          }}
          empty={<EmptyState icon={<Activity />} title={mode === 'logs' ? 'No log messages in this window' : 'No events in this window'}>
            {dayList.length ? 'Try another day above.' : running ? 'Rows appear here as the artifact emits them.' : 'This artifact is not running. Use Configure to start it.'}
          </EmptyState>}
        />
      </div>
      {rows.length > 0 && (
        <div className="tw:px-4 tw:h-8 tw:flex tw:items-center tw:text-2xs tw:text-faint tw:border-t tw:border-line tw:bg-panel">
          {fmtNum(rows.length)} {mode === 'logs' ? 'messages' : 'rows'}{q.hasNextPage ? ' loaded, more available' : ''}
          {q.isFetching && <Spinner className="tw:size-3 tw:ml-2" />}
        </div>
      )}
    </>
  );
}

// ---------------------------------------------------------- Configuration

interface Scope { artifacts: string[]; params: Record<string, Record<string, string>>; specs: Record<string, MonitoringSpec> }

function fromTable(t?: MonitoringTable): Scope {
  const params: Scope['params'] = {};
  const specs: Scope['specs'] = {};
  for (const s of t?.specs ?? []) {
    specs[s.artifact] = s;
    params[s.artifact] = Object.fromEntries((s.parameters?.env ?? []).map(e => [e.key, e.value]));
  }
  return { artifacts: [...(t?.artifacts ?? [])], params, specs };
}

function toTable(s: Scope, orig?: MonitoringTable): MonitoringTable {
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  const { compiled_collector_args: _c, ...rest } = (orig ?? {}) as MonitoringTable & { compiled_collector_args?: unknown };
  return {
    ...rest,
    artifacts: s.artifacts,
    specs: s.artifacts.map(a => ({
      ...(s.specs[a] ?? {}),
      artifact: a,
      parameters: { env: Object.entries(s.params[a] ?? {}).map(([key, value]) => ({ key, value })) },
    })),
  };
}

function MonitoringDialog({ server, onClose }: { server: boolean; onClose: () => void }) {
  const { data: state, isLoading } = useMonitoringState(server);
  const save = useSetMonitoringState(server);
  const [scopes, setScopes] = useState<Record<string, Scope> | null>(null);
  const [active, setActive] = useState('All');
  const [expanded, setExpanded] = useState<string | null>(null);
  const [term, setTerm] = useState('');
  const [newLabel, setNewLabel] = useState('');

  useEffect(() => {
    if (!state || scopes) return;
    const s: Record<string, Scope> = { All: fromTable(state.artifacts) };
    for (const l of state.label_events ?? []) s[l.label] = fromTable(l.artifacts);
    setScopes(s);
  }, [state, scopes]);

  const cur = scopes?.[active];
  const { data: defs } = useArtifactDefinitions(cur?.artifacts ?? []);
  const { data: hits = [], isFetching } = useArtifactSearch(term, server ? 'SERVER_EVENT' : 'CLIENT_EVENT', true);
  const addable = hits.filter(h => !cur?.artifacts.includes(h.name)).slice(0, 40);

  const update = (fn: (s: Scope) => Scope) => setScopes(p => p && ({ ...p, [active]: fn(p[active]) }));

  const submit = () => {
    if (!scopes) return;
    const out: ClientMonitoringState = { ...(state ?? {}) };
    out.artifacts = toTable(scopes.All, state?.artifacts);
    if (!server) {
      out.label_events = Object.entries(scopes).filter(([k, v]) => k !== 'All' && v.artifacts.length)
        .map(([label, v]) => ({ label, artifacts: toTable(v, state?.label_events?.find(l => l.label === label)?.artifacts) }));
    }
    delete (out as Record<string, unknown>).version;
    save.mutate(out, {
      onSuccess: () => { toast.success('Monitoring updated. Clients pick up the change within a minute.'); onClose(); },
      onError: e => toast.error(`Could not save: ${(e as Error).message}`),
    });
  };

  const scopeNames = scopes ? ['All', ...Object.keys(scopes).filter(k => k !== 'All').sort()] : [];

  return (
    <Dialog open onOpenChange={o => !o && onClose()} width={980} className="tw:h-[80vh]"
      title={server ? 'Server monitoring' : 'Client monitoring'}
      description={server
        ? 'Artifacts that run continuously on the server.'
        : 'Artifacts that run continuously on clients. Add a label to run extra artifacts only on clients with that label.'}
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancel</Button>
        <Button variant="primary" loading={save.isPending} disabled={!scopes} onClick={submit}>Save and apply</Button>
      </>}>
      {isLoading || !scopes ? <Skeleton className="tw:h-64" /> : (
        <div className="tw:flex tw:gap-4 tw:h-full tw:min-h-[420px]">
          {!server && (
            <div className="tw:w-48 tw:shrink-0 tw:flex tw:flex-col tw:gap-1">
              {scopeNames.map(k => (
                <button key={k} onClick={() => { setActive(k); setExpanded(null); }}
                  className={cn('tw:flex tw:items-center tw:gap-2 tw:h-8 tw:px-2.5 tw:rounded-md tw:text-[13px] tw:text-left',
                    k === active ? 'tw:bg-accent-soft tw:text-accent tw:font-medium' : 'tw:hover:bg-hover')}>
                  {k === 'All' ? <Monitor className="tw:size-3.5" /> : <Tag className="tw:size-3.5" />}
                  <span className="tw:flex-1 tw:truncate">{k === 'All' ? 'All clients' : k}</span>
                  <span className="tw:text-2xs tw:text-faint">{scopes[k].artifacts.length}</span>
                </button>
              ))}
              <form className="tw:flex tw:gap-1 tw:mt-2" onSubmit={e => {
                e.preventDefault();
                const l = newLabel.trim();
                if (!l) return;
                setScopes(p => p && (p[l] ? p : { ...p, [l]: { artifacts: [], params: {}, specs: {} } }));
                setActive(l);
                setNewLabel('');
              }}>
                <Input value={newLabel} onChange={e => setNewLabel(e.target.value)} placeholder="Add label…" className="tw:h-7 tw:text-xs" />
              </form>
            </div>
          )}
          <div className="tw:flex-1 tw:min-w-0 tw:flex tw:flex-col tw:gap-2 tw:overflow-y-auto">
            <div className="tw:text-xs tw:font-semibold tw:text-muted">
              Running {active === 'All' ? (server ? 'on the server' : 'on every client') : <>on clients labelled <Badge tone="accent">{active}</Badge></>}
            </div>
            {cur?.artifacts.length === 0 && <div className="tw:text-xs tw:text-faint tw:py-6 tw:text-center tw:border tw:border-dashed tw:border-line tw:rounded-lg">Nothing yet. Add artifacts from the right.</div>}
            {cur?.artifacts.map(a => (
              <ArtifactParams key={a} name={a} def={defs?.get(a)} values={cur.params[a] ?? {}} expanded={expanded === a}
                onToggle={() => setExpanded(e => e === a ? null : a)}
                onChange={(k, v) => update(s => ({ ...s, params: { ...s.params, [a]: { ...(s.params[a] ?? {}), [k]: v } } }))}
                onReset={() => update(s => ({ ...s, params: { ...s.params, [a]: {} } }))}
                onRemove={() => update(s => ({ ...s, artifacts: s.artifacts.filter(x => x !== a) }))} />
            ))}
          </div>
          <div className="tw:w-72 tw:shrink-0 tw:flex tw:flex-col tw:gap-2 tw:border-l tw:border-line tw:pl-4 tw:min-h-0">
            <div className="tw:relative">
              <Search className="tw:absolute tw:left-2.5 tw:top-1/2 tw:-translate-y-1/2 tw:size-3.5 tw:text-faint" />
              <Input value={term} onChange={e => setTerm(e.target.value)} placeholder="Find event artifacts" className="tw:pl-8" />
              {isFetching && <Spinner className="tw:absolute tw:right-2.5 tw:top-1/2 tw:-translate-y-1/2 tw:size-3" />}
            </div>
            <div className="tw:flex-1 tw:overflow-y-auto tw:-mx-1">
              {addable.map(h => (
                <button key={h.name} onClick={() => update(s => ({ ...s, artifacts: [...s.artifacts, h.name] }))}
                  className="tw:w-full tw:text-left tw:px-2 tw:py-1.5 tw:rounded-md tw:hover:bg-hover tw:group">
                  <div className="tw:flex tw:items-center tw:gap-1.5">
                    <Plus className="tw:size-3 tw:text-faint tw:group-hover:text-accent" />
                    <span className="tw:font-mono tw:text-[11.5px] tw:truncate">{h.name}</span>
                  </div>
                  {h.description && <div className="tw:text-2xs tw:text-faint tw:line-clamp-2 tw:pl-4.5">{h.description}</div>}
                </button>
              ))}
              {!addable.length && !isFetching && <div className="tw:text-xs tw:text-faint tw:p-2">No matching artifacts</div>}
            </div>
          </div>
        </div>
      )}
    </Dialog>
  );
}

