import { useEffect, useState } from 'react';
import { Command } from 'cmdk';
import { Dialog as RDialog } from 'radix-ui';
import { useHistory } from 'react-router-dom';
import {
  Crosshair, LayoutDashboard, Monitor, BookOpen, Package, Server, Radio, Users, KeyRound, Plus,
  Moon, Sun, Search, CornerDownLeft, Zap, FolderTree, History, UserPlus, FilePlus2,
} from 'lucide-react';
import { useArtifactSearch, useClientNameSuggest, useHunts, useTraits } from '../lib/queries';
import { clientStatus, timeAgo } from '../lib/format';
import { Kbd, StatusDot } from '../ui/primitives';
import { HuntStateBadge } from '../pages/hunts/HuntBits';
import { setTheme } from './theme';
import { useSelectedClient } from './client-context';

function useDebounced<T>(v: T, ms: number): T {
  const [d, setD] = useState(v);
  useEffect(() => {
    const t = setTimeout(() => setD(v), ms);
    return () => clearTimeout(t);
  }, [v, ms]);
  return d;
}

const itemCls = 'tw:flex tw:items-center tw:gap-2.5 tw:h-9 tw:px-3 tw:mx-1 tw:rounded-md tw:text-[13px] tw:cursor-pointer tw:[&>svg]:size-4 tw:[&>svg]:text-faint';

export default function CommandPalette({ open, onOpenChange, dark }: {
  open: boolean; onOpenChange: (v: boolean) => void; dark: boolean;
}) {
  const history = useHistory();
  const { traits, updateTraits } = useTraits();
  const { client: selClient, setClient } = useSelectedClient();
  const [q, setQ] = useState('');
  const dq = useDebounced(q.trim(), 150);
  const { data: hunts = [] } = useHunts(false);
  const { data: clients = [] } = useClientNameSuggest(open ? dq : '');
  const { data: artifacts = [] } = useArtifactSearch(dq, '', open && dq.length >= 2);

  useEffect(() => {
    if (!open) setQ('');
  }, [open]);

  const go = (path: string) => {
    onOpenChange(false);
    history.push(path);
  };

  const ql = dq.toLowerCase();
  const huntHits = (ql
    ? hunts.filter(h => `${h.hunt_id} ${h.hunt_description ?? ''} ${(h.artifacts ?? []).join(' ')} ${(h.tags ?? []).join(' ')}`
      .toLowerCase().includes(ql))
    : hunts).slice(0, 6);

  const navAll = [
    { label: 'Dashboard', icon: <LayoutDashboard />, to: '/dashboard', keys: 'home' },
    { label: 'Hunts', icon: <Crosshair />, to: '/hunts', keys: 'hunt manager' },
    { label: 'Clients', icon: <Monitor />, to: '/search/all', keys: 'endpoints hosts' },
    { label: 'Notebooks', icon: <BookOpen />, to: '/notebooks', keys: '' },
    { label: 'Artifacts', icon: <Package />, to: '/artifacts', keys: 'vql' },
    { label: 'Server collections', icon: <Server />, to: '/collected/server', keys: 'server artifacts' },
    { label: 'Server events', icon: <Radio />, to: '/events/server', keys: 'monitoring' },
    { label: 'Users', icon: <Users />, to: '/users', keys: 'admin' },
    { label: 'Secrets', icon: <KeyRound />, to: '/secrets', keys: 'credentials' },
  ];
  const navHits = navAll.filter(n => !ql || `${n.label} ${n.keys}`.toLowerCase().includes(ql));

  return (
    <RDialog.Root open={open} onOpenChange={onOpenChange}>
      <RDialog.Portal>
        <RDialog.Overlay className="rx tw:fixed tw:inset-0 tw:z-[1700] tw:bg-black/30 tw:animate-fade-in" />
        <RDialog.Content aria-describedby={undefined}
          className="rx tw:fixed tw:left-1/2 tw:top-[12vh] tw:z-[1701] tw:-translate-x-1/2 tw:w-[640px] tw:max-w-[calc(100vw-32px)] tw:rounded-xl tw:border tw:border-line tw:bg-raised rx-shadow-lg tw:overflow-hidden tw:animate-fade-in">
          <RDialog.Title className="tw:sr-only">Command palette</RDialog.Title>
          <Command shouldFilter={false} loop>
            <div className="tw:flex tw:items-center tw:gap-2 tw:px-4 tw:border-b tw:border-line">
              <Search className="tw:size-4 tw:text-faint" />
              <Command.Input value={q} onValueChange={setQ} autoFocus
                placeholder="Search hunts, clients, artifacts or jump to a page…"
                className="tw:flex-1 tw:h-12 tw:bg-transparent tw:outline-none tw:text-[14px] tw:placeholder:text-faint" />
              <Kbd>Esc</Kbd>
            </div>
            <Command.List className="tw:max-h-[min(60vh,480px)] tw:overflow-y-auto tw:py-1">
              <Command.Empty className="tw:px-4 tw:py-6 tw:text-center tw:text-xs tw:text-faint">No results</Command.Empty>

              {!ql && (
                <Command.Group heading="Actions">
                  <Command.Item value="new-hunt" onSelect={() => go('/hunts?new=1')} className={itemCls}>
                    <Plus /> New hunt
                  </Command.Item>
                  {selClient?.client_id && selClient.client_id !== 'server' && <>
                    <Command.Item value="collect-client" onSelect={() => go(`/collected/${selClient.client_id}?new=1`)} className={itemCls}>
                      <Zap /> Collect from {selClient.os_info?.hostname || selClient.client_id}
                    </Command.Item>
                    <Command.Item value="client-collections" onSelect={() => go(`/collected/${selClient.client_id}`)} className={itemCls}>
                      <History /> Collections on {selClient.os_info?.hostname || selClient.client_id}
                    </Command.Item>
                    <Command.Item value="client-files" onSelect={() => go(`/vfs/${selClient.client_id}/`)} className={itemCls}>
                      <FolderTree /> Browse files on {selClient.os_info?.hostname || selClient.client_id}
                    </Command.Item>
                  </>}
                  <Command.Item value="new-artifact" onSelect={() => go('/artifacts?new=1')} className={itemCls}>
                    <FilePlus2 /> New artifact
                  </Command.Item>
                  <Command.Item value="new-notebook" onSelect={() => go('/notebooks?new=1')} className={itemCls}>
                    <BookOpen /> New notebook
                  </Command.Item>
                  <Command.Item value="new-user" onSelect={() => go('/users?add=1')} className={itemCls}>
                    <UserPlus /> Add a user
                  </Command.Item>
                  <Command.Item value="theme" onSelect={() => {
                    onOpenChange(false);
                    setTheme(!dark, traits?.ui_settings, updateTraits);
                  }} className={itemCls}>
                    {dark ? <Sun /> : <Moon />} Switch to {dark ? 'light' : 'dark'} mode
                  </Command.Item>
                </Command.Group>
              )}

              {navHits.length > 0 && <Command.Group heading="Pages">
                {navHits.map(n => (
                  <Command.Item key={n.to} value={`nav-${n.to}`} onSelect={() => go(n.to)} className={itemCls}>
                    {n.icon} {n.label}
                  </Command.Item>
                ))}
              </Command.Group>}

              {huntHits.length > 0 && (
                <Command.Group heading="Hunts">
                  {huntHits.map(h => (
                    <Command.Item key={h.hunt_id} value={`hunt-${h.hunt_id}`} onSelect={() => go(`/hunts/${h.hunt_id}`)} className={itemCls}>
                      <Crosshair />
                      <span className="tw:truncate tw:flex-1">{h.hunt_description || (h.artifacts ?? []).join(', ')}</span>
                      <span className="tw:font-mono tw:text-2xs tw:text-faint">{h.hunt_id}</span>
                      <HuntStateBadge hunt={h} />
                    </Command.Item>
                  ))}
                </Command.Group>
              )}

              {ql && clients.length > 0 && (
                <Command.Group heading="Clients">
                  {clients.map(c => (
                    <Command.Item key={c.client_id} value={`client-${c.client_id}`} onSelect={() => {
                      setClient(c);
                      go(`/host/${c.client_id}`);
                    }} className={itemCls}>
                      <StatusDot status={clientStatus(c.last_seen_at)} />
                      <span className="tw:truncate tw:flex-1">{c.os_info?.hostname || c.client_id}</span>
                      <span className="tw:font-mono tw:text-2xs tw:text-faint">{c.client_id}</span>
                      <span className="tw:text-2xs tw:text-faint tw:w-20 tw:text-right">{timeAgo(c.last_seen_at)}</span>
                    </Command.Item>
                  ))}
                  <Command.Item value="client-search-all" onSelect={() => go(`/search/${encodeURIComponent(dq)}`)} className={itemCls}>
                    <Monitor /> Search all clients for “{dq}”
                  </Command.Item>
                </Command.Group>
              )}

              {ql.length >= 2 && artifacts.length > 0 && (
                <Command.Group heading="Artifacts">
                  {artifacts.slice(0, 6).map(a => (
                    <Command.Item key={a.name} value={`artifact-${a.name}`} onSelect={() => go(`/artifacts/${a.name}`)} className={itemCls}>
                      <Package />
                      <span className="tw:truncate">{a.name}</span>
                    </Command.Item>
                  ))}
                </Command.Group>
              )}
            </Command.List>
            <div className="tw:flex tw:items-center tw:gap-3 tw:px-4 tw:h-9 tw:border-t tw:border-line tw:text-2xs tw:text-faint">
              <span className="tw:flex tw:items-center tw:gap-1"><Kbd>↑</Kbd><Kbd>↓</Kbd> navigate</span>
              <span className="tw:flex tw:items-center tw:gap-1"><Kbd><CornerDownLeft className="tw:size-2.5" /></Kbd> open</span>
            </div>
          </Command>
        </RDialog.Content>
      </RDialog.Portal>
    </RDialog.Root>
  );
}
