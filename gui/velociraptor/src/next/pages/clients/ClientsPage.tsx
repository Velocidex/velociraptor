import { useEffect, useMemo, useState } from 'react';
import { useHistory, useLocation, useParams } from 'react-router-dom';
import { toast } from 'sonner';
import { Monitor, Tag, TagsIcon, X, ExternalLink, CircleHelp, Zap, FolderTree, History } from 'lucide-react';
import { useClientSearch, useLabelClients, usePermission } from '../../lib/queries';
import type { ApiClient } from '../../lib/types';
import { clientStatus, fmtNum, fmtTime, timeAgo, toNum } from '../../lib/format';
import type { Row } from '../../lib/table';
import {
  Badge, Button, Checkbox, Dialog, EmptyState, Field, IconButton, Mono, SearchInput, StatusDot, Switch, Tooltip,
} from '../../ui/primitives';
import { DataTable } from '../../ui/DataTable';
import { TagInput } from '../hunts/TagInput';
import { useSelectedClient } from '../../shell/client-context';

const OS_LABEL: Record<string, string> = { windows: 'Windows', linux: 'Linux', darwin: 'macOS' };

const SEARCH_HELP = [
  ['host:web*', 'Hostname (wildcards allowed)'],
  ['label:servers', 'Clients with a label'],
  ['ip:10.0.0.5', 'Last seen IP address'],
  ['mac:00:11:22', 'MAC address'],
  ['client:C.1234', 'Client ID'],
  ['recent:', 'Clients you viewed recently'],
  ['all', 'Every client'],
];

export default function ClientsPage() {
  const { query: routeQuery } = useParams<{ query?: string }>();
  const location = useLocation();
  const history = useHistory();
  const { setClient } = useSelectedClient();
  const canLabel = usePermission('label_clients');

  const initial = decodeURIComponent(routeQuery || 'all');
  const [input, setInput] = useState(initial === 'all' ? '' : initial);
  const [onlineOnly, setOnlineOnly] = useState(new URLSearchParams(location.search).get('online') === '1');
  const [checked, setChecked] = useState<Set<string>>(new Set());
  const [labelDialog, setLabelDialog] = useState<null | 'set' | 'remove'>(null);

  useEffect(() => {
    setInput(initial === 'all' ? '' : initial);
  }, [initial]);

  const query = initial || 'all';
  const { data, isLoading, error } = useClientSearch(query, { limit: 2000, onlineOnly });
  const clients = data?.items ?? [];
  const total = toNum(data?.total);

  const rows: Row[] = useMemo(() => clients.map((c: ApiClient) => ({
    Select: c.client_id,
    Status: clientStatus(c.last_seen_at),
    Hostname: c.os_info?.hostname || c.os_info?.fqdn || '',
    ClientId: c.client_id,
    OS: OS_LABEL[c.os_info?.system ?? ''] ?? c.os_info?.system ?? '',
    Release: c.os_info?.release ?? '',
    Labels: c.labels ?? [],
    LastSeen: toNum(c.last_seen_at),
    LastIP: (c.last_ip ?? '').replace(/:\d+$/, ''),
    Version: c.agent_information?.version ?? '',
    FirstSeen: toNum(c.first_seen_at),
    _client: c,
  })), [clients]);

  const submit = (q: string) => {
    history.push(`/search/${encodeURIComponent(q.trim() || 'all')}${onlineOnly ? '?online=1' : ''}`);
  };

  const allChecked = rows.length > 0 && rows.every(r => checked.has(String(r.ClientId)));
  const toggleOne = (id: string, v: boolean) => setChecked(prev => {
    const n = new Set(prev);
    if (v) n.add(id);
    else n.delete(id);
    return n;
  });

  const open = (r: Row) => {
    const c = r._client as ApiClient;
    setClient(c);
    history.push(`/host/${c.client_id}`);
  };

  const counts = useMemo(() => {
    let on = 0;
    for (const r of rows) if (r.Status === 'online') on++;
    return { on };
  }, [rows]);

  return (
    <div className="tw:flex tw:flex-col tw:flex-1 tw:min-h-0 tw:bg-panel">
      <div className="tw:px-5 tw:pt-4 tw:pb-3 tw:border-b tw:border-line tw:space-y-3">
        <div className="tw:flex tw:items-center tw:gap-3">
          <h1 className="tw:text-[15px] tw:font-semibold">Clients</h1>
          <span className="tw:text-xs tw:text-muted rx-num">
            {isLoading ? 'Loading…' : `${fmtNum(total)} match${total === 1 ? '' : 'es'} · ${fmtNum(counts.on)} online`}
          </span>
          <div className="tw:ml-auto">
            <Button size="sm" variant="ghost" icon={<ExternalLink className="tw:size-3.5" />}
              onClick={() => history.push(`/classic/search/${encodeURIComponent(query)}`)}>Classic view</Button>
          </div>
        </div>
        <form className="tw:flex tw:items-center tw:gap-2" onSubmit={e => {
          e.preventDefault();
          submit(input);
        }}>
          <SearchInput value={input} onChange={setInput} placeholder="host:web*  label:servers  ip:10.0.0.5  …" className="tw:w-[420px]" />
          <Button type="submit" variant="secondary">Search</Button>
          <Tooltip content={
            <div className="tw:space-y-1 tw:py-1">
              {SEARCH_HELP.map(([k, v]) => (
                <div key={k} className="tw:flex tw:gap-3"><span className="tw:font-mono tw:w-28">{k}</span><span className="tw:opacity-70">{v}</span></div>
              ))}
            </div>}>
            <span className="tw:text-faint tw:p-1"><CircleHelp className="tw:size-4" /></span>
          </Tooltip>
          <div className="tw:ml-3">
            <Switch checked={onlineOnly} onChange={setOnlineOnly} label="Online only" />
          </div>
        </form>
      </div>

      <div className="tw:flex-1 tw:min-h-0">
        {error ? (
          <EmptyState title="Search failed">{(error as Error).message}</EmptyState>
        ) : (
          <DataTable
            rows={rows}
            loading={isLoading}
            total={total}
            storageKey="clients"
            exportName="clients"
            inspector={false}
            onRowClick={open}
            columns={['Select', 'Status', 'Hostname', 'Actions', 'ClientId', 'OS', 'Release', 'Labels', 'LastSeen', 'LastIP', 'Version', 'FirstSeen']}
            columnLabels={{ Actions: '', Select: '', ClientId: 'Client ID', LastSeen: 'Last seen', LastIP: 'Last IP', FirstSeen: 'First seen', Status: '' }}
            columnWidths={{ Actions: 96, Select: 36, Status: 36, Hostname: 220, ClientId: 170, OS: 90, Release: 160, Labels: 220, LastSeen: 120, LastIP: 130, Version: 90, FirstSeen: 120 }}
            headerRenderers={{
              Select: () => (
                <Checkbox checked={allChecked} indeterminate={checked.size > 0 && !allChecked} label="Select all"
                  onChange={v => setChecked(v ? new Set(rows.map(r => String(r.ClientId))) : new Set())} />
              ),
            }}
            toolbar={checked.size > 0 && (
              <div className="tw:flex tw:items-center tw:gap-1 tw:pl-2 tw:border-l tw:border-line">
                <span className="tw:text-xs tw:font-medium tw:mr-1">{checked.size} selected</span>
                <Button size="sm" variant="secondary" disabled={!canLabel} icon={<Tag className="tw:size-3.5" />}
                  onClick={() => setLabelDialog('set')}>Add label</Button>
                <Button size="sm" variant="secondary" disabled={!canLabel} icon={<TagsIcon className="tw:size-3.5" />}
                  onClick={() => setLabelDialog('remove')}>Remove label</Button>
                <IconButton size="sm" label="Clear selection" icon={<X className="tw:size-3.5" />} onClick={() => setChecked(new Set())} />
              </div>
            )}
            renderers={{
              Select: (_v, r) => (
                <span onClick={e => e.stopPropagation()} className="tw:flex">
                  <Checkbox checked={checked.has(String(r.ClientId))} onChange={v => toggleOne(String(r.ClientId), v)} label="Select" />
                </span>
              ),
              Status: v => (
                <Tooltip content={v === 'online' ? 'Online' : v === 'recent' ? 'Seen in the last day' : 'Offline'}>
                  <span className="tw:flex"><StatusDot status={v as 'online'} /></span>
                </Tooltip>
              ),
              Hostname: v => <span className="tw:font-medium">{String(v) || <span className="tw:text-faint">unknown</span>}</span>,
              Actions: (_v, r) => {
                const c = r._client as ApiClient;
                const go = (path: string) => (e: { stopPropagation: () => void }) => {
                  e.stopPropagation();
                  setClient(c);
                  history.push(path);
                };
                return (
                  <span className="tw:flex tw:gap-0.5">
                    <IconButton size="xs" label="Collect an artifact" icon={<Zap className="tw:size-3.5" />}
                      onClick={go(`/collected/${c.client_id}?new=1`)} />
                    <IconButton size="xs" label="Collections" icon={<History className="tw:size-3.5" />}
                      onClick={go(`/collected/${c.client_id}`)} />
                    <IconButton size="xs" label="Browse files" icon={<FolderTree className="tw:size-3.5" />}
                      onClick={go(`/vfs/${c.client_id}/`)} />
                  </span>
                );
              },
              ClientId: v => <Mono className="tw:text-muted">{String(v)}</Mono>,
              Labels: v => (
                <span className="tw:flex tw:gap-1">
                  {(v as string[]).map(l => (
                    <button key={l} type="button" onClick={e => {
                      e.stopPropagation();
                      history.push(`/search/${encodeURIComponent(`label:${l}`)}`);
                    }}>
                      <Badge tone="accent">{l}</Badge>
                    </button>
                  ))}
                </span>
              ),
              LastSeen: v => <Tooltip content={fmtTime(v as number)}><span>{timeAgo(v as number)}</span></Tooltip>,
              FirstSeen: v => <Tooltip content={fmtTime(v as number)}><span className="tw:text-muted">{timeAgo(v as number)}</span></Tooltip>,
              LastIP: v => <Mono>{String(v)}</Mono>,
              Version: v => <span className="tw:text-muted">{String(v)}</span>,
            }}
            empty={<EmptyState icon={<Monitor />} title={query === 'all' ? 'No clients enrolled yet' : 'No clients match'}>
              {query === 'all' ? 'Deploy the client to endpoints to see them here.' : `Nothing matched “${query}”.`}
            </EmptyState>}
          />
        )}
      </div>

      {labelDialog && (
        <LabelDialog mode={labelDialog} clientIds={[...checked]}
          onClose={ok => {
            setLabelDialog(null);
            if (ok) setChecked(new Set());
          }} />
      )}
    </div>
  );
}

function LabelDialog({ mode, clientIds, onClose }: {
  mode: 'set' | 'remove'; clientIds: string[]; onClose: (ok: boolean) => void;
}) {
  const [labels, setLabels] = useState<string[]>([]);
  const label = useLabelClients();
  const submit = () => {
    if (!labels.length) return;
    label.mutate({ client_ids: clientIds, labels, operation: mode }, {
      onSuccess: () => {
        toast.success(mode === 'set' ? 'Labels added' : 'Labels removed');
        onClose(true);
      },
      onError: e => toast.error((e as Error).message),
    });
  };
  return (
    <Dialog open onOpenChange={v => !v && onClose(false)} width={460}
      title={mode === 'set' ? `Add labels to ${clientIds.length} client(s)` : `Remove labels from ${clientIds.length} client(s)`}
      footer={<>
        <Button variant="ghost" onClick={() => onClose(false)}>Cancel</Button>
        <Button variant={mode === 'set' ? 'primary' : 'danger'} disabled={!labels.length} loading={label.isPending} onClick={submit}>
          {mode === 'set' ? 'Add labels' : 'Remove labels'}
        </Button>
      </>}>
      <Field label="Labels" hint="Press Enter after each label.">
        <TagInput value={labels} onChange={setLabels} tone={mode === 'set' ? 'accent' : 'bad'} placeholder="e.g. servers" />
      </Field>
    </Dialog>
  );
}
