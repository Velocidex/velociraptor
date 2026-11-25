// A single client at a glance: identity, health, labels, metadata, one-
// click triage collections and recent activity.
import React, { lazy, Suspense, useEffect, useMemo, useState } from 'react';
import { Link, useHistory, useParams } from 'react-router-dom';
import { toast } from 'sonner';
import {
  Activity, Cpu, ExternalLink, FolderTree, History, Info, Monitor, MoreHorizontal, Network, Plus, RefreshCw,
  ShieldAlert, ShieldCheck, Tags, Trash2, Users, Zap, Clock, Globe, KeyRound, ListTree, Terminal, Save,
  Download, HardDrive, PlayCircle,
} from 'lucide-react';
import {
  useClient, useClientFlowList, useClientMetadata, useExistingArtifacts, useLabelClients, useSetClientMetadata,
  runArtifact, runArtifacts, usePermission,
} from '../../lib/queries';
import type { ApiClient } from '../../lib/types';
import { clientStatus, fmtBytes, fmtCompact, fmtTime, timeAgo, toNum } from '../../lib/format';
import {
  Badge, Button, Card, CardHeader, CopyButton, Dialog, EmptyState, IconButton, Input, Menu, MenuContent,
  MenuItem, MenuSeparator, MenuTrigger, Mono, Skeleton, StatusDot, Textarea, Tooltip, cn,
} from '../../ui/primitives';
import { TagInput } from '../hunts/TagInput';
import { FlowStateBadge } from '../collections/FlowDetail';
import { useSelectedClient } from '../../shell/client-context';
import { MftWindowDialog, type MftWindow } from './MftWindowDialog';

const NewHuntDialog = lazy(() => import('../hunts/NewHuntDialog'));

type OS = 'windows' | 'linux' | 'darwin' | 'other';
const osOf = (c?: ApiClient): OS => {
  const s = (c?.os_info?.system ?? '').toLowerCase();
  return s === 'windows' ? 'windows' : s === 'linux' ? 'linux' : s === 'darwin' || s === 'macos' ? 'darwin' : 'other';
};

const QUARANTINE: Record<OS, string> = {
  windows: 'Windows.Remediation.Quarantine',
  linux: 'Linux.Remediation.Quarantine',
  darwin: 'MacOS.Remediation.Quarantine',
  other: '',
};

// A quick triage tile collects one artifact, or several in a single flow
// via `bundle`. `askWindow` opens the MFT time window picker first.
interface Quick {
  artifact: string; label: string; icon: React.ReactNode; hint: string;
  bundle?: string[]; askWindow?: boolean;
}
const artifactsOf = (q: Quick): string[] => q.bundle ?? [q.artifact];

const QUICK: Record<OS, Quick[]> = {
  windows: [
    {
      artifact: 'Windows.Applications.Chrome.History', label: 'Browser history', icon: <Globe />,
      hint: 'Visited URLs from Chrome, Edge, Brave and Firefox',
      bundle: ['Windows.Applications.Chrome.History', 'Windows.Applications.Firefox.History'],
    },
    {
      artifact: 'Windows.Applications.Chrome.Downloads', label: 'Downloads history', icon: <Download />,
      hint: 'Browser downloads plus files marked as downloaded (Zone.Identifier)',
      bundle: ['Windows.Applications.Chrome.Downloads', 'Windows.Applications.Firefox.Downloads', 'Windows.Analysis.EvidenceOfDownload'],
    },
    {
      artifact: 'Windows.Forensics.Prefetch', label: 'Recently run programs', icon: <PlayCircle />,
      hint: 'Prefetch, Amcache, ShimCache, BAM and UserAssist',
      bundle: ['Windows.Forensics.Prefetch', 'Windows.Forensics.Amcache', 'Windows.Registry.AppCompatCache', 'Windows.Forensics.Bam', 'Windows.Registry.UserAssist'],
    },
    {
      artifact: 'Windows.NTFS.MFT', label: 'MFT (time window)', icon: <HardDrive />,
      hint: 'File system entries changed inside a time window you pick', askWindow: true,
    },
    { artifact: 'Windows.System.Pslist', label: 'Processes', icon: <Cpu />, hint: 'Running processes with hashes and signers' },
    { artifact: 'Windows.Network.Netstat', label: 'Network connections', icon: <Network />, hint: 'Open sockets and owning processes' },
    { artifact: 'Windows.Sys.Users', label: 'Users', icon: <Users />, hint: 'Local and domain user profiles' },
    { artifact: 'Windows.System.Services', label: 'Services', icon: <ListTree />, hint: 'Installed services and binaries' },
    { artifact: 'Windows.Sys.StartupItems', label: 'Autoruns', icon: <Zap />, hint: 'Programs that start automatically' },
    { artifact: 'Windows.System.TaskScheduler', label: 'Scheduled tasks', icon: <Clock />, hint: 'Task Scheduler entries' },
    { artifact: 'Windows.EventLogs.Evtx', label: 'Event logs', icon: <Activity />, hint: 'Parse EVTX event logs' },
    { artifact: 'Windows.KapeFiles.Targets', label: 'Triage package', icon: <ShieldAlert />, hint: 'Collect a KAPE-style triage file set' },
  ],
  linux: [
    { artifact: 'Linux.Sys.Pslist', label: 'Processes', icon: <Cpu />, hint: 'Running processes and command lines' },
    { artifact: 'Linux.Network.Netstat', label: 'Network connections', icon: <Network />, hint: 'Open sockets and owning processes' },
    { artifact: 'Linux.Sys.Users', label: 'Users', icon: <Users />, hint: 'Accounts from /etc/passwd' },
    { artifact: 'Linux.Sys.LastUserLogin', label: 'Logins', icon: <Clock />, hint: 'Recent logins from wtmp' },
    { artifact: 'Linux.Sys.Crontab', label: 'Cron jobs', icon: <Zap />, hint: 'Scheduled cron entries' },
    { artifact: 'Linux.Ssh.AuthorizedKeys', label: 'SSH keys', icon: <KeyRound />, hint: 'Authorized SSH keys per user' },
    { artifact: 'Linux.Sys.BashHistory', label: 'Shell history', icon: <Terminal />, hint: 'Bash history for all users' },
    { artifact: 'Linux.Collection.UAC', label: 'Triage package', icon: <ShieldAlert />, hint: 'Unix-like Artifacts Collector triage' },
  ],
  darwin: [
    { artifact: 'Generic.System.Pstree', label: 'Processes', icon: <Cpu />, hint: 'Process tree' },
    { artifact: 'MacOS.Network.Netstat', label: 'Network connections', icon: <Network />, hint: 'Open sockets' },
    { artifact: 'MacOS.Sys.Users', label: 'Users', icon: <Users />, hint: 'Local users' },
    { artifact: 'MacOS.System.LaunchAgents', label: 'Launch agents', icon: <Zap />, hint: 'Persistence via launchd' },
    { artifact: 'MacOS.Sys.BashHistory', label: 'Shell history', icon: <Terminal />, hint: 'Shell history for all users' },
  ],
  other: [],
};
const GENERIC: Quick[] = [
  { artifact: 'Generic.Client.Info', label: 'Refresh host info', icon: <Info />, hint: 'Re-interrogate the client' },
  { artifact: 'Generic.System.Pstree', label: 'Process tree', icon: <ListTree />, hint: 'Processes as a tree' },
];

export default function ClientOverview() {
  const { client_id: clientId = '' } = useParams<{ client_id: string }>();
  const history = useHistory();
  const { data: client, isLoading, error, refetch } = useClient(clientId);
  const { client: selected, setClient } = useSelectedClient();
  const [collectOpen, setCollectOpen] = useState(false);
  const [quarantineOpen, setQuarantineOpen] = useState(false);
  const [running, setRunning] = useState<string | null>(null);
  const [mftOpen, setMftOpen] = useState(false);
  const label = useLabelClients();
  const canCollect = usePermission('collect_client');

  useEffect(() => {
    if (client?.client_id && selected?.client_id !== client.client_id) setClient(client);
  }, [client, selected?.client_id, setClient]);

  const os = osOf(client);
  const quick = useMemo(() => [...QUICK[os], ...GENERIC], [os]);
  const names = useMemo(() => [...quick.flatMap(artifactsOf), QUARANTINE[os]].filter(Boolean), [quick, os]);
  const { data: existing } = useExistingArtifacts(names);

  if (isLoading) {
    return <div className="tw:p-6 tw:space-y-4"><Skeleton className="tw:h-8 tw:w-1/3" /><Skeleton className="tw:h-32" /></div>;
  }
  if (!client?.client_id) {
    return <EmptyState icon={<Monitor />} title="Client not found">{error ? (error as Error).message : clientId}</EmptyState>;
  }

  const hostname = client.os_info?.hostname || clientId;
  const status = clientStatus(client.last_seen_at);
  const labels = client.labels ?? [];
  const quarantined = labels.includes('Quarantine');
  const qArtifact = QUARANTINE[os];
  const canQuarantine = !!qArtifact && !!existing?.has(qArtifact);

  const collect = async (artifact: string, params?: Record<string, string>, quiet = false) => {
    setRunning(artifact);
    try {
      const r = await runArtifact(clientId, artifact, params, true);
      if (!quiet) toast.success(`Collecting ${artifact}`, {
        action: r.flow_id ? { label: 'Open', onClick: () => history.push(`/collected/${clientId}/${r.flow_id}`) } : undefined,
      });
      return r.flow_id;
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setRunning(null);
    }
  };

  // Runs a quick triage tile. Bundles skip artifacts this server lacks.
  const runQuick = async (q: Quick, params: Record<string, Record<string, string>> = {}) => {
    const arts = artifactsOf(q).filter(a => !existing || existing.has(a));
    if (arts.length === 0) return;
    setRunning(q.artifact);
    try {
      const r = await runArtifacts(clientId, arts, params, true);
      if (r.flow_id) history.push(`/collected/${clientId}/${r.flow_id}`);
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setRunning(null);
    }
  };

  const setLabels = async (next: string[]) => {
    const add = next.filter(l => !labels.includes(l));
    const remove = labels.filter(l => !next.includes(l));
    try {
      if (add.length) await label.mutateAsync({ client_ids: [clientId], labels: add, operation: 'set' });
      if (remove.length) await label.mutateAsync({ client_ids: [clientId], labels: remove, operation: 'remove' });
      refetch();
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  const unquarantine = async () => {
    await label.mutateAsync({ client_ids: [clientId], labels: ['Quarantine'], operation: 'remove' });
    await collect(qArtifact, { RemovePolicy: 'Y' });
    refetch();
  };

  return (
    <div className="tw:flex-1 tw:overflow-auto tw:bg-canvas">
      {/* ---------------------------------------------------------- Header */}
      <div className="tw:bg-panel tw:border-b tw:border-line tw:px-4 tw:md:px-6 tw:pt-5 tw:pb-4">
        <div className="tw:flex tw:items-start tw:gap-4 tw:flex-wrap tw:md:flex-nowrap">
          <div className={cn('tw:hidden tw:sm:flex tw:size-12 tw:shrink-0 tw:items-center tw:justify-center tw:rounded-xl',
            status === 'online' ? 'tw:bg-ok-soft tw:text-ok' : status === 'recent' ? 'tw:bg-warn-soft tw:text-warn' : 'tw:bg-hover tw:text-faint')}>
            <Monitor className="tw:size-6" />
          </div>
          <div className="tw:min-w-0 tw:flex-1 tw:max-sm:basis-full">
            <div className="tw:flex tw:items-center tw:gap-2.5 tw:flex-wrap">
              <h1 className="tw:text-xl tw:font-semibold tw:truncate">{hostname}</h1>
              <Badge tone={status === 'online' ? 'ok' : status === 'recent' ? 'warn' : 'neutral'} dot>
                {status === 'online' ? 'Online' : `Seen ${timeAgo(client.last_seen_at)}`}
              </Badge>
              {quarantined && <Badge tone="bad"><ShieldAlert className="tw:size-3" /> Quarantined</Badge>}
            </div>
            <div className="tw:mt-1 tw:flex tw:items-center tw:gap-2 tw:text-xs tw:text-muted tw:flex-wrap">
              <Mono>{clientId}</Mono><CopyButton text={clientId} />
              <span>·</span>
              <span className="tw:capitalize">{client.os_info?.system}</span>
              <span>{client.os_info?.release}</span>
              <span>·</span>
              <span>{client.last_ip?.replace(/:\d+$/, '')}</span>
            </div>
            <div className="tw:mt-3 tw:max-w-[640px]">
              <TagInput value={labels} onChange={setLabels} tone="accent" placeholder="Add labels…" />
            </div>
          </div>
          <div className="tw:flex tw:items-center tw:gap-1.5 tw:shrink-0 tw:flex-wrap">
            <Button variant="primary" icon={<Plus className="tw:size-3.5" />} disabled={!canCollect} onClick={() => setCollectOpen(true)}>Collect</Button>
            <Button variant="secondary" icon={<History className="tw:size-3.5" />} onClick={() => history.push(`/collected/${clientId}`)}>Collections</Button>
            <Button variant="secondary" icon={<FolderTree className="tw:size-3.5" />} onClick={() => history.push(`/vfs/${clientId}/`)}>Files</Button>
            <Menu>
              <MenuTrigger asChild>
                <IconButton label="More actions" variant="secondary" icon={<MoreHorizontal className="tw:size-4" />} />
              </MenuTrigger>
              <MenuContent>
                <MenuItem icon={<RefreshCw />} onSelect={() => collect('Generic.Client.Info')}>Interrogate</MenuItem>
                {canQuarantine && !quarantined && <MenuItem icon={<ShieldAlert />} danger onSelect={() => setQuarantineOpen(true)}>Quarantine host…</MenuItem>}
                {canQuarantine && quarantined && <MenuItem icon={<ShieldCheck />} onSelect={unquarantine}>Lift quarantine</MenuItem>}
                <MenuSeparator />
                <MenuItem icon={<Terminal />} onSelect={() => history.push(`/classic/host/${clientId}/shell`)}>Shell (classic)</MenuItem>
                <MenuItem icon={<ExternalLink />} onSelect={() => history.push(`/classic/host/${clientId}`)}>Open in classic view</MenuItem>
              </MenuContent>
            </Menu>
          </div>
        </div>
      </div>

      <div className="tw:p-3 tw:md:p-6 tw:grid tw:grid-cols-1 tw:xl:grid-cols-3 tw:gap-4">
        {/* --------------------------------------------------- Quick triage */}
        <Card className="tw:xl:col-span-3">
          <CardHeader icon={<Zap className="tw:size-4" />} title="Quick triage" subtitle="One click collects from this host. Results open in Collections." />
          <div className="tw:grid tw:grid-cols-2 tw:md:grid-cols-3 tw:xl:grid-cols-5 tw:gap-2 tw:p-3">
            {quick.filter(q => !existing || artifactsOf(q).some(a => existing.has(a))).map(q => (
              <button key={q.artifact} type="button" disabled={!canCollect || running === q.artifact}
                title={artifactsOf(q).join('\n')}
                onClick={() => (q.askWindow ? setMftOpen(true) : runQuick(q))}
                className="tw:group tw:flex tw:items-start tw:gap-2.5 tw:rounded-lg tw:border tw:border-line tw:bg-canvas/50 tw:p-3 tw:text-left tw:transition-colors tw:hover:border-accent tw:hover:bg-accent-soft tw:disabled:opacity-50">
                <span className="tw:flex tw:text-faint tw:group-hover:text-accent tw:[&>svg]:size-4 tw:mt-0.5">{q.icon}</span>
                <span className="tw:min-w-0">
                  <span className="tw:block tw:text-[13px] tw:font-medium">{q.label}</span>
                  <span className="tw:block tw:text-2xs tw:text-muted tw:line-clamp-2">{q.hint}</span>
                  <span className="tw:block tw:mt-1 tw:font-mono tw:text-[10.5px] tw:text-faint tw:truncate">
                    {q.artifact}{q.bundle && q.bundle.length > 1 && ` +${q.bundle.length - 1}`}
                  </span>
                </span>
              </button>
            ))}
          </div>
        </Card>

        {/* -------------------------------------------------- Details */}
        <Card>
          <CardHeader icon={<Info className="tw:size-4" />} title="System" />
          <dl className="tw:p-4 tw:space-y-2.5 tw:text-xs">
            <Def label="Hostname">{client.os_info?.hostname}</Def>
            <Def label="FQDN">{client.os_info?.fqdn}</Def>
            <Def label="OS"><span className="tw:capitalize">{client.os_info?.system}</span> {client.os_info?.release}</Def>
            <Def label="Architecture">{client.os_info?.machine}</Def>
            <Def label="MAC addresses">
              <div className="tw:font-mono tw:space-y-0.5">{(client.os_info?.mac_addresses ?? []).map(m => <div key={m}>{m}</div>)}</div>
            </Def>
            <Def label="Agent">{client.agent_information?.name} {client.agent_information?.version}</Def>
            <Def label="Agent built">{client.agent_information?.build_time ? fmtTime(new Date(client.agent_information.build_time)) : ''}</Def>
          </dl>
        </Card>
        <Card>
          <CardHeader icon={<Globe className="tw:size-4" />} title="Connectivity" />
          <dl className="tw:p-4 tw:space-y-2.5 tw:text-xs">
            <Def label="Status"><span className="tw:inline-flex tw:items-center tw:gap-1.5"><StatusDot status={status} />{status}</span></Def>
            <Def label="Last seen"><Tooltip content={fmtTime(client.last_seen_at)}><span>{timeAgo(client.last_seen_at)}</span></Tooltip></Def>
            <Def label="Last IP"><Mono>{client.last_ip}</Mono></Def>
            <Def label="First seen"><Tooltip content={fmtTime(client.first_seen_at)}><span>{timeAgo(client.first_seen_at)}</span></Tooltip></Def>
            <Def label="In flight">{Object.keys(client.in_flight_flows ?? {}).length || 'None'}</Def>
          </dl>
          <Metadata clientId={clientId} />
        </Card>
        <RecentCollections clientId={clientId} />
      </div>

      {collectOpen && (
        <Suspense fallback={null}>
          <NewHuntDialog open={collectOpen} onOpenChange={setCollectOpen} target={{ clientId, label: hostname }}
            onCreated={id => { setCollectOpen(false); if (id) history.push(`/collected/${clientId}/${id}`); }} />
        </Suspense>
      )}
      {mftOpen && (
        <MftWindowDialog hostname={hostname} onClose={() => setMftOpen(false)}
          onConfirm={async (w: MftWindow) => {
            const mft = quick.find(q => q.askWindow);
            if (mft) await runQuick(mft, { [mft.artifact]: { ...w } });
            setMftOpen(false);
          }} />
      )}
      {quarantineOpen && (
        <QuarantineDialog hostname={hostname} onClose={() => setQuarantineOpen(false)}
          onConfirm={async message => {
            await label.mutateAsync({ client_ids: [clientId], labels: ['Quarantine'], operation: 'set' });
            await collect(qArtifact, message ? { MessageBox: message } : {});
            setQuarantineOpen(false);
            refetch();
          }} />
      )}
    </div>
  );
}

function Def({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="tw:grid tw:grid-cols-[110px_1fr] tw:gap-3">
      <dt className="tw:text-muted">{label}</dt>
      <dd className="tw:break-all">{children || <span className="tw:text-faint">—</span>}</dd>
    </div>
  );
}

function RecentCollections({ clientId }: { clientId: string }) {
  const { data, isLoading } = useClientFlowList(clientId, 12);
  const rows = data?.rows ?? [];
  return (
    <Card>
      <CardHeader icon={<History className="tw:size-4" />} title="Recent collections"
        actions={<Link to={`/collected/${clientId}`} className="tw:text-xs tw:font-medium tw:text-accent tw:hover:underline">All</Link>} />
      <div className="tw:divide-y tw:divide-line">
        {isLoading && <div className="tw:p-4"><Skeleton /></div>}
        {!isLoading && rows.length === 0 && <div className="tw:px-4 tw:py-6 tw:text-center tw:text-xs tw:text-faint">Nothing collected yet.</div>}
        {rows.map(r => {
          const arts = (r.Artifacts as string[] | undefined) ?? [];
          return (
            <Link key={String(r.FlowId)} to={`/collected/${clientId}/${r.FlowId}`}
              className="tw:flex tw:items-center tw:gap-2 tw:px-4 tw:py-2 tw:hover:bg-hover">
              <div className="tw:min-w-0 tw:flex-1">
                <div className="tw:font-mono tw:text-[12px] tw:truncate">{arts[0]}{arts.length > 1 && <span className="tw:text-faint"> +{arts.length - 1}</span>}</div>
                <div className="tw:text-2xs tw:text-faint">{timeAgo(r.Created as number)} · {fmtCompact(toNum(r.Rows as number))} rows{toNum(r.Mb as number) > 0 ? ` · ${fmtBytes(toNum(r.Mb as number))}` : ''}</div>
              </div>
              <FlowStateBadge state={String(r.State ?? '')} />
            </Link>
          );
        })}
      </div>
    </Card>
  );
}

function Metadata({ clientId }: { clientId: string }) {
  const { data = [] } = useClientMetadata(clientId);
  const set = useSetClientMetadata();
  const [k, setK] = useState('');
  const [v, setV] = useState('');
  return (
    <div className="tw:border-t tw:border-line">
      <div className="tw:flex tw:items-center tw:gap-2 tw:px-4 tw:pt-3 tw:text-xs tw:font-semibold"><Tags className="tw:size-3.5 tw:text-faint" />Metadata</div>
      <div className="tw:px-4 tw:py-2 tw:space-y-1">
        {data.length === 0 && <div className="tw:text-2xs tw:text-faint">No metadata. Use it for owner, location, asset tag…</div>}
        {data.map(m => (
          <div key={m.key} className="tw:group tw:flex tw:items-center tw:gap-2 tw:text-xs">
            <span className="tw:text-muted tw:w-[110px] tw:shrink-0 tw:truncate">{m.key}</span>
            <span className="tw:flex-1 tw:break-all">{m.value}</span>
            <button type="button" aria-label={`Remove ${m.key}`} onClick={() => set.mutate({ client_id: clientId, add: [], remove: [m.key] })}
              className="tw:opacity-0 tw:group-hover:opacity-100 tw:text-faint tw:hover:text-bad"><Trash2 className="tw:size-3" /></button>
          </div>
        ))}
      </div>
      <form className="tw:flex tw:items-center tw:gap-1.5 tw:px-4 tw:pb-3"
        onSubmit={e => {
          e.preventDefault();
          if (!k.trim()) return;
          set.mutate({ client_id: clientId, add: [{ key: k.trim(), value: v }], remove: [] }, {
            onSuccess: () => { setK(''); setV(''); },
            onError: err => toast.error((err as Error).message),
          });
        }}>
        <Input value={k} onChange={e => setK(e.target.value)} placeholder="Key" className="tw:h-7 tw:text-xs tw:w-[110px]" />
        <Input value={v} onChange={e => setV(e.target.value)} placeholder="Value" className="tw:h-7 tw:text-xs tw:flex-1" />
        <IconButton size="xs" type="submit" label="Add" variant="secondary" icon={<Save className="tw:size-3.5" />} loading={set.isPending} />
      </form>
    </div>
  );
}

function QuarantineDialog({ hostname, onClose, onConfirm }: {
  hostname: string; onClose: () => void; onConfirm: (message: string) => Promise<void>;
}) {
  const [msg, setMsg] = useState('');
  const [busy, setBusy] = useState(false);
  return (
    <Dialog open onOpenChange={v => !v && onClose()} title={`Quarantine ${hostname}?`}
      description="The host is cut off from the network except for this server, so you can keep investigating."
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancel</Button>
        <Button variant="danger" icon={<ShieldAlert className="tw:size-3.5" />} loading={busy}
          onClick={async () => { setBusy(true); try { await onConfirm(msg); } finally { setBusy(false); } }}>Quarantine</Button>
      </>}>
      <label className="tw:block tw:text-xs tw:font-medium tw:mb-1.5">Message shown to the user (optional)</label>
      <Textarea rows={3} value={msg} onChange={e => setMsg(e.target.value)} placeholder="This computer has been isolated by the security team." />
    </Dialog>
  );
}
