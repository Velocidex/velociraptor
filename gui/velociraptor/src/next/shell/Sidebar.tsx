import React from 'react';
import { NavLink, useLocation } from 'react-router-dom';
import {
  LayoutDashboard, Crosshair, Monitor, BookOpen, Package, Radio, Server, Users, KeyRound,
  FolderTree, History, Activity, PanelLeftClose, PanelLeftOpen, ExternalLink, Laptop, X,
} from 'lucide-react';
import { getItem, schema } from '../../components/core/storage.jsx';
import { EncodePathInURL } from '../../components/utils/paths.jsx';
import logo from '../../components/sidebar/velo.svg';
import { srcOf } from '../lib/api';
import { useTraits } from '../lib/queries';
import { cn, StatusDot, Tooltip } from '../ui/primitives';
import { useSelectedClient } from './client-context';
import { clientStatus } from '../lib/format';

interface Item {
  to: string;
  label: string;
  icon: React.ReactNode;
  match?: RegExp;
  exact?: boolean;
}

function remembered(key: string, prefix: string): string {
  const v = getItem(key);
  return v ? `${prefix}/${encodeURIComponent(v)}` : prefix;
}

function NavItem({ item, collapsed }: { item: Item; collapsed: boolean }) {
  const loc = useLocation();
  const active = item.match ? item.match.test(loc.pathname) : loc.pathname.startsWith(item.to);
  const link = (
    <NavLink to={item.to} exact={item.exact}
      className={cn(
        'tw:flex tw:items-center tw:gap-2.5 tw:h-8 tw:rounded-md tw:text-[13px] tw:font-medium tw:transition-colors',
        collapsed ? 'tw:justify-center tw:w-8 tw:mx-auto' : 'tw:px-2.5',
        active ? 'tw:bg-active tw:text-fg' : 'tw:text-muted tw:hover:bg-hover tw:hover:text-fg')}>
      <span className={cn('tw:flex tw:[&>svg]:size-4', active && 'tw:text-accent')}>{item.icon}</span>
      {!collapsed && <span className="tw:truncate">{item.label}</span>}
    </NavLink>
  );
  return collapsed ? <Tooltip content={item.label} side="right">{link}</Tooltip> : link;
}

function Section({ title, children, collapsed }: { title: string; children: React.ReactNode; collapsed: boolean }) {
  return (
    <div className="tw:space-y-0.5">
      {!collapsed
        ? <div className="tw:px-2.5 tw:pt-4 tw:pb-1 tw:text-2xs tw:font-semibold tw:uppercase tw:tracking-wider tw:text-faint">{title}</div>
        : <div className="tw:my-3 tw:mx-3 tw:h-px tw:bg-line" />}
      {children}
    </div>
  );
}

export default function Sidebar({ collapsed, onToggle }: { collapsed: boolean; onToggle: () => void }) {
  const { traits } = useTraits();
  const { client, setClient } = useSelectedClient();
  const perms = traits?.Permissions;
  const isAdmin = !perms || perms.org_admin || perms.server_admin;
  const clientId = client?.client_id && client.client_id !== 'server' ? client.client_id : '';

  const main: Item[] = [
    { to: '/dashboard', label: 'Dashboard', icon: <LayoutDashboard />, match: /^\/(dashboard|welcome)?$/ },
    { to: '/hunts', label: 'Hunts', icon: <Crosshair />, match: /^\/hunts/ },
    { to: '/search/all', label: 'Clients', icon: <Monitor />, match: /^\/search/ },
    { to: remembered(schema.CurrentNotebookIdKey, '/notebooks'), label: 'Notebooks', icon: <BookOpen />, match: /^\/notebooks/ },
  ];
  const server: Item[] = [
    { to: remembered(schema.CurrentSelectedArtifactKey, '/artifacts'), label: 'Artifacts', icon: <Package />, match: /^\/artifacts/ },
    { to: remembered(schema.ServerCurrentFlowKey, '/collected/server'), label: 'Server collections', icon: <Server />, match: /^\/collected\/server/ },
    { to: remembered(schema.CurrentServerEventArtifactKey, '/events/server'), label: 'Server events', icon: <Radio />, match: /^\/events\/server/ },
  ];
  const admin: Item[] = isAdmin ? [
    { to: '/users', label: 'Users', icon: <Users />, match: /^\/users/ },
    { to: '/secrets', label: 'Secrets', icon: <KeyRound />, match: /^\/secrets/ },
  ] : [];

  const clientItems: Item[] = clientId ? [
    { to: `/host/${clientId}`, label: 'Overview', icon: <Laptop />, match: /^\/host\/C\./ },
    {
      to: `/vfs/${clientId}${EncodePathInURL(getItem(schema.CurrentVFSPathKey) || '/')}`,
      label: 'Files', icon: <FolderTree />, match: /^\/vfs\//,
    },
    { to: `/collected/${clientId}`, label: 'Collections', icon: <History />, match: /^\/collected\/C\./ },
    { to: `/events/${clientId}`, label: 'Client events', icon: <Activity />, match: /^\/events\/C\./ },
  ] : [];

  const links = (traits?.links ?? []).filter(l => !l.type || l.type === 'sidebar');

  return (
    <aside className={cn(
      'tw:flex tw:flex-col tw:shrink-0 tw:bg-sidebar tw:border-r tw:border-line tw:transition-[width] tw:duration-150',
      collapsed ? 'tw:w-14' : 'tw:w-56')}>
      <div className={cn('tw:flex tw:items-center tw:h-12 tw:border-b tw:border-line', collapsed ? 'tw:justify-center' : 'tw:px-3 tw:gap-2')}>
        <NavLink to="/dashboard" className="tw:flex tw:items-center tw:gap-2 tw:min-w-0">
          <img src={srcOf(logo)} alt="" className="tw:size-7 tw:shrink-0" />
          {!collapsed && (
            <span className="tw:text-[14px] tw:font-semibold tw:tracking-tight tw:truncate">Atlantiraptor</span>
          )}
        </NavLink>
      </div>

      <nav className="tw:flex-1 tw:overflow-y-auto tw:px-2 tw:pb-3">
        <Section title="Investigate" collapsed={collapsed}>
          {main.map(i => <NavItem key={i.label} item={i} collapsed={collapsed} />)}
        </Section>

        {clientId && (
          <Section title="Client" collapsed={collapsed}>
            {!collapsed && (
              <div className="tw:flex tw:items-center tw:gap-2 tw:px-2.5 tw:py-1.5 tw:mb-1 tw:rounded-md tw:bg-panel tw:border tw:border-line">
                <StatusDot status={clientStatus(client.last_seen_at)} />
                <div className="tw:min-w-0 tw:flex-1">
                  <div className="tw:text-xs tw:font-semibold tw:truncate">{client.os_info?.hostname || clientId}</div>
                  <div className="tw:text-2xs tw:text-faint tw:font-mono tw:truncate">{clientId}</div>
                </div>
                <button type="button" aria-label="Clear selected client" onClick={() => setClient({})}
                  className="tw:text-faint tw:hover:text-fg tw:p-0.5 tw:rounded">
                  <X className="tw:size-3.5" />
                </button>
              </div>
            )}
            {clientItems.map(i => <NavItem key={i.label} item={i} collapsed={collapsed} />)}
          </Section>
        )}

        <Section title="Server" collapsed={collapsed}>
          {server.map(i => <NavItem key={i.label} item={i} collapsed={collapsed} />)}
        </Section>

        {admin.length > 0 && (
          <Section title="Admin" collapsed={collapsed}>
            {admin.map(i => <NavItem key={i.label} item={i} collapsed={collapsed} />)}
          </Section>
        )}

        {links.length > 0 && (
          <Section title="Links" collapsed={collapsed}>
            {links.map(l => (
              <a key={l.text} href={l.url} target="_blank" rel="noreferrer"
                className={cn('tw:flex tw:items-center tw:gap-2.5 tw:h-8 tw:rounded-md tw:text-[13px] tw:text-muted tw:hover:bg-hover tw:hover:text-fg',
                  collapsed ? 'tw:justify-center' : 'tw:px-2.5')}>
                {l.icon_url ? <img src={srcOf(l.icon_url)} alt="" className="tw:size-4" /> : <ExternalLink className="tw:size-4" />}
                {!collapsed && <span className="tw:truncate">{l.text}</span>}
              </a>
            ))}
          </Section>
        )}
      </nav>

      <div className="tw:border-t tw:border-line tw:p-2">
        <button type="button" onClick={onToggle}
          className={cn('tw:flex tw:items-center tw:gap-2 tw:h-8 tw:w-full tw:rounded-md tw:text-xs tw:text-faint tw:hover:bg-hover tw:hover:text-fg',
            collapsed ? 'tw:justify-center' : 'tw:px-2.5')}>
          {collapsed ? <PanelLeftOpen className="tw:size-4" /> : <PanelLeftClose className="tw:size-4" />}
          {!collapsed && 'Collapse'}
        </button>
      </div>
    </aside>
  );
}
