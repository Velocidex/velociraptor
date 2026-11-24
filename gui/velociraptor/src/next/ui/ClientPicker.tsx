// Small dialog to choose which client to collect from.
import { useEffect, useState } from 'react';
import { Monitor, Search, Server } from 'lucide-react';
import { useClientSearch } from '../lib/queries';
import type { ApiClient } from '../lib/types';
import { clientStatus, timeAgo } from '../lib/format';
import { Dialog, Input, Spinner, StatusDot, cn } from './primitives';

export default function ClientPicker({ open, onOpenChange, onPick, allowServer, title = 'Collect from which client?' }: {
  open: boolean; onOpenChange: (v: boolean) => void; onPick: (clientId: string, label: string) => void;
  allowServer?: boolean; title?: string;
}) {
  const [q, setQ] = useState('');
  const [dq, setDq] = useState('');
  useEffect(() => {
    const t = setTimeout(() => setDq(q.trim()), 200);
    return () => clearTimeout(t);
  }, [q]);
  const query = !dq ? 'all' : dq.includes(':') || dq.endsWith('*') ? dq : `${dq}*`;
  const { data, isFetching } = useClientSearch(query, { limit: 50, sort: 'SORT_DOWN' });
  const items: ApiClient[] = data?.items ?? [];
  return (
    <Dialog open={open} onOpenChange={onOpenChange} title={title} width={520}>
      <div className="tw:relative tw:mb-3">
        <Search className="tw:absolute tw:left-2.5 tw:top-1/2 tw:-translate-y-1/2 tw:size-3.5 tw:text-faint" />
        <Input autoFocus value={q} onChange={e => setQ(e.target.value)} placeholder="Hostname, label:foo, client id…" className="tw:pl-8" />
        {isFetching && <Spinner className="tw:absolute tw:right-2.5 tw:top-1/2 tw:-translate-y-1/2 tw:size-3.5" />}
      </div>
      <div className="tw:max-h-[360px] tw:overflow-y-auto tw:-mx-1">
        {allowServer && (
          <Item onClick={() => onPick('server', 'the server')} icon={<Server className="tw:size-4" />} title="Server" sub="Run on the Velociraptor server itself" />
        )}
        {items.map(c => (
          <Item key={c.client_id} onClick={() => onPick(c.client_id, c.os_info?.hostname || c.client_id)}
            icon={<StatusDot status={clientStatus(c.last_seen_at)} />}
            title={c.os_info?.hostname || c.client_id}
            sub={<><span className="tw:font-mono">{c.client_id}</span> · {c.os_info?.system} · seen {timeAgo(c.last_seen_at)}</>} />
        ))}
        {!isFetching && items.length === 0 && <div className="tw:py-8 tw:text-center tw:text-xs tw:text-faint">No clients match.</div>}
      </div>
    </Dialog>
  );
}

function Item({ onClick, icon, title, sub }: { onClick: () => void; icon: React.ReactNode; title: React.ReactNode; sub: React.ReactNode }) {
  return (
    <button type="button" onClick={onClick}
      className={cn('tw:flex tw:w-full tw:items-center tw:gap-3 tw:rounded-md tw:px-2 tw:py-2 tw:text-left tw:hover:bg-hover')}>
      <span className="tw:flex tw:size-7 tw:items-center tw:justify-center tw:text-faint">{icon ?? <Monitor className="tw:size-4" />}</span>
      <span className="tw:min-w-0">
        <span className="tw:block tw:text-[13px] tw:font-medium tw:truncate">{title}</span>
        <span className="tw:block tw:text-2xs tw:text-muted tw:truncate">{sub}</span>
      </span>
    </button>
  );
}
