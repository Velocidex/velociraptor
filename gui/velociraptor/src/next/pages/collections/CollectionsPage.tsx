// Collections on one client (or the server): list on the left, the selected
// collection with its results, files, logs and exports on the right.
import { lazy, Suspense, useEffect, useMemo, useState } from 'react';
import { useHistory, useLocation, useParams } from 'react-router-dom';
import { History, Monitor, Plus, Server, Search } from 'lucide-react';
import { useClient, useClientFlowList } from '../../lib/queries';
import type { ArtifactCollectorArgs, Flow } from '../../lib/types';
import type { Row } from '../../lib/table';
import { clientStatus, fmtBytes, fmtCompact, timeAgo, toNum } from '../../lib/format';
import { Button, cn, EmptyState, Input, Skeleton, StatusDot } from '../../ui/primitives';
import { useSelectedClient } from '../../shell/client-context';
import FlowDetail, { FlowStateBadge } from './FlowDetail';
import { MobileBack } from '../../ui/MobileBack';

const NewHuntDialog = lazy(() => import('../hunts/NewHuntDialog'));

export default function CollectionsPage() {
  const { client_id: clientId = 'server', flow_id: flowId, tab } = useParams<{ client_id?: string; flow_id?: string; tab?: string }>();
  const history = useHistory();
  const location = useLocation();
  const isServer = clientId === 'server';
  const { data: client } = useClient(clientId);
  const { client: selected, setClient } = useSelectedClient();
  const { data, isLoading } = useClientFlowList(clientId);
  const [filter, setFilter] = useState('');
  const [newOpen, setNewOpen] = useState(() => new URLSearchParams(location.search).get('new') === '1');
  const [baseRequest, setBaseRequest] = useState<ArtifactCollectorArgs | undefined>();

  // Keep the sidebar's client in sync with the page.
  useEffect(() => {
    if (client?.client_id && selected?.client_id !== client.client_id) setClient(client);
  }, [client, selected?.client_id, setClient]);

  const flows = useMemo(() => {
    const rows = data?.rows ?? [];
    const f = filter.trim().toLowerCase();
    if (!f) return rows;
    return rows.filter(r => `${r.FlowId} ${(r.Artifacts as string[] | undefined ?? []).join(' ')} ${r.Creator} ${r.State}`
      .toLowerCase().includes(f));
  }, [data, filter]);

  // Select the newest collection when none is selected.
  useEffect(() => {
    if (!flowId && flows.length && !filter) {
      history.replace(`/collected/${clientId}/${flows[0].FlowId}`);
    }
  }, [flowId, flows, clientId, history, filter]);

  const hostname = isServer ? 'Server' : client?.os_info?.hostname || clientId;
  const status = client ? clientStatus(client.last_seen_at) : 'offline';
  const label = isServer ? 'the server' : hostname;

  const openNew = (req?: ArtifactCollectorArgs) => {
    setBaseRequest(req);
    setNewOpen(true);
  };

  return (
    <div className="rx-split tw:flex tw:flex-1 tw:min-h-0" data-detail={flowId ? 1 : 0}>
      {/* ---------------------------------------------- List */}
      <div className="rx-split-list tw:w-[380px] tw:shrink-0 tw:border-r tw:border-line tw:flex tw:flex-col tw:min-h-0 tw:bg-panel">
        <div className="tw:px-4 tw:pt-4 tw:pb-3 tw:space-y-3 tw:border-b tw:border-line">
          <div className="tw:flex tw:items-center tw:gap-2.5">
            <span className="tw:flex tw:size-8 tw:items-center tw:justify-center tw:rounded-lg tw:bg-hover tw:text-muted">
              {isServer ? <Server className="tw:size-4" /> : <Monitor className="tw:size-4" />}
            </span>
            <div className="tw:min-w-0 tw:flex-1">
              <div className="tw:flex tw:items-center tw:gap-1.5">
                {!isServer && <StatusDot status={status} />}
                <h2 className="tw:text-[15px] tw:font-semibold tw:truncate">{hostname}</h2>
              </div>
              <div className="tw:text-2xs tw:text-faint tw:truncate">
                {isServer ? 'Server collections' : <>
                  <span className="tw:font-mono">{clientId}</span>
                  {client?.last_seen_at ? <> · seen {timeAgo(client.last_seen_at)}</> : null}
                </>}
              </div>
            </div>
            <Button variant="primary" size="sm" icon={<Plus className="tw:size-3.5" />} onClick={() => openNew()}>Collect</Button>
          </div>
          <div className="tw:relative">
            <Search className="tw:absolute tw:left-2.5 tw:top-1/2 tw:-translate-y-1/2 tw:size-3.5 tw:text-faint" />
            <Input value={filter} onChange={e => setFilter(e.target.value)} placeholder="Filter collections…" className="tw:pl-8" />
          </div>
        </div>
        <div className="tw:flex-1 tw:overflow-y-auto">
          {isLoading && Array.from({ length: 6 }).map((_, i) => (
            <div key={i} className="tw:px-4 tw:py-3 tw:border-b tw:border-line tw:space-y-2"><Skeleton className="tw:w-2/3" /><Skeleton className="tw:w-1/3" /></div>
          ))}
          {!isLoading && flows.length === 0 && (
            <EmptyState icon={<History />} title={filter ? 'No collections match' : 'No collections yet'}
              action={!filter && <Button variant="primary" icon={<Plus className="tw:size-3.5" />} onClick={() => openNew()}>Collect from {label}</Button>}>
              {!filter && `Collect artifacts and files from ${label}. Results and files show up here.`}
            </EmptyState>
          )}
          {flows.map(r => (
            <FlowRow key={String(r.FlowId)} row={r} selected={r.FlowId === flowId}
              onClick={() => history.push(`/collected/${clientId}/${r.FlowId}${tab ? `/${tab}` : ''}`)} />
          ))}
        </div>
      </div>

      {/* -------------------------------------------- Detail */}
      <div className="rx-split-detail tw:flex-1 tw:min-w-0 tw:flex tw:flex-col tw:min-h-0">
        <MobileBack to={`/collected/${clientId}`} label="All collections" />
        {flowId
          ? <FlowDetail key={flowId} clientId={clientId} flowId={flowId} tab={tab} hostname={hostname}
              onCollectAgain={(f: Flow) => openNew(f.request)} />
          : !isLoading && flows.length > 0 && (
            <EmptyState icon={<History />} title="Select a collection" className="tw:flex-1">
              Pick a collection on the left, or start a new one.
            </EmptyState>
          )}
      </div>

      {newOpen && (
        <Suspense fallback={null}>
          <NewHuntDialog open={newOpen} onOpenChange={setNewOpen}
            target={{ clientId, label: hostname }} baseRequest={baseRequest}
            onCreated={id => {
              setNewOpen(false);
              if (id) history.push(`/collected/${clientId}/${id}`);
            }} />
        </Suspense>
      )}
    </div>
  );
}

function FlowRow({ row, selected, onClick }: { row: Row; selected: boolean; onClick: () => void }) {
  const artifacts = (row.Artifacts as string[] | undefined) ?? [];
  const flow = row._Flow as Flow | undefined;
  const files = toNum(flow?.total_uploaded_files);
  return (
    <button type="button" onClick={onClick}
      className={cn('tw:relative tw:w-full tw:text-left tw:px-4 tw:py-2.5 tw:border-b tw:border-line tw:space-y-1',
        selected ? 'tw:bg-accent-soft' : 'tw:hover:bg-hover')}>
      {selected && <span className="tw:absolute tw:left-0 tw:top-0 tw:bottom-0 tw:w-0.5 tw:bg-accent" />}
      <div className="tw:flex tw:items-center tw:gap-2">
        <span className="tw:font-mono tw:text-[12.5px] tw:font-medium tw:truncate tw:flex-1">
          {artifacts[0] ?? row.FlowId}{artifacts.length > 1 && <span className="tw:text-faint"> +{artifacts.length - 1}</span>}
        </span>
        <FlowStateBadge state={String(row.State ?? '')} />
      </div>
      <div className="tw:flex tw:items-center tw:gap-2 tw:text-2xs tw:text-muted">
        <span className="tw:font-mono tw:text-faint">{String(row.FlowId)}</span>
        <span className="tw:ml-auto tw:truncate">{timeAgo(row.Created as number)} · {String(row.Creator ?? '')}</span>
      </div>
      <div className="tw:flex tw:items-center tw:gap-3 tw:text-2xs tw:text-muted">
        <span className="rx-num">{fmtCompact(toNum(row.Rows as number))} rows</span>
        {files > 0 && <span className="rx-num">{fmtCompact(files)} files · {fmtBytes(toNum(row.Mb as number))}</span>}
      </div>
    </button>
  );
}
