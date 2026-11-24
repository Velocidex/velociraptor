import { lazy, Suspense, useEffect, useRef, useState } from 'react';
import { useHistory } from 'react-router-dom';
import { Popover } from 'radix-ui';
import {
  Bell, CircleHelp, Command, LogOut, Moon, Settings, Sun, Building2, Keyboard, Check, Monitor,
} from 'lucide-react';
import { useClientNameSuggest, useTraits } from '../lib/queries';
import { href } from '../lib/api';
import { clientStatus, timeAgo } from '../lib/format';
import {
  cn, IconButton, Kbd, Menu, MenuContent, MenuItem, MenuLabel, MenuSeparator, MenuTrigger,
  SearchInput, StatusDot, Tooltip,
} from '../ui/primitives';
import { setTheme } from './theme';
import { useSelectedClient } from './client-context';

const UserSettingsModal = lazy(() => import('./LegacyDialogs.jsx').then(m => ({ default: m.UserSettingsModal })));
const ShowGlobalMessages = lazy(() => import('./LegacyDialogs.jsx').then(m => ({ default: m.ShowGlobalMessages })));
const HelpDialog = lazy(() => import('./LegacyDialogs.jsx').then(m => ({ default: m.HelpDialog })));

function useDebounced<T>(v: T, ms: number): T {
  const [d, setD] = useState(v);
  useEffect(() => {
    const t = setTimeout(() => setD(v), ms);
    return () => clearTimeout(t);
  }, [v, ms]);
  return d;
}

function ClientSearch() {
  const history = useHistory();
  const { setClient } = useSelectedClient();
  const [q, setQ] = useState('');
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const debounced = useDebounced(q.trim(), 150);
  const { data: hits = [], isFetching } = useClientNameSuggest(debounced);
  const inputRef = useRef<HTMLInputElement>(null);

  const go = (query: string) => {
    setOpen(false);
    history.push(`/search/${encodeURIComponent(query || 'all')}`);
  };

  const pick = (idx: number) => {
    const c = hits[idx];
    if (!c) return go(q);
    setClient(c);
    setOpen(false);
    setQ('');
    history.push(`/host/${c.client_id}`);
  };

  return (
    <Popover.Root open={open && q.length > 0} onOpenChange={setOpen}>
      <Popover.Anchor asChild>
        <div className="tw:w-full tw:max-w-md">
          <SearchInput
            inputRef={inputRef}
            value={q}
            onChange={v => {
              setQ(v);
              setOpen(true);
              setActive(-1);
            }}
            placeholder="Search clients by host, label:, ip:, client id…"
            onKeyDown={e => {
              if (e.key === 'ArrowDown') {
                e.preventDefault();
                setActive(a => Math.min(a + 1, hits.length - 1));
              } else if (e.key === 'ArrowUp') {
                e.preventDefault();
                setActive(a => Math.max(a - 1, -1));
              } else if (e.key === 'Enter') {
                e.preventDefault();
                if (active >= 0) pick(active);
                else go(q.trim());
              } else if (e.key === 'Escape') {
                setOpen(false);
              }
            }}
          />
          {/* Keeps the legacy alt-key "focus search" hotkey working. */}
          <input id="client-search-bar" className="tw:sr-only" tabIndex={-1} aria-hidden
            onFocus={() => inputRef.current?.focus()} />
        </div>
      </Popover.Anchor>
      <Popover.Portal>
        <Popover.Content align="start" sideOffset={6}
          onOpenAutoFocus={e => e.preventDefault()}
          className="rx tw:z-[1600] tw:w-[var(--radix-popover-trigger-width)] tw:min-w-[420px] tw:rounded-lg tw:border tw:border-line tw:bg-raised rx-shadow-lg tw:p-1 tw:animate-fade-in">
          {hits.length === 0 && (
            <div className="tw:px-3 tw:py-2 tw:text-xs tw:text-faint">{isFetching ? 'Searching…' : 'No matching clients'}</div>
          )}
          {hits.map((c, i) => (
            <button key={c.client_id} type="button" onMouseEnter={() => setActive(i)} onClick={() => pick(i)}
              className={cn('tw:flex tw:w-full tw:items-center tw:gap-2.5 tw:rounded-md tw:px-2.5 tw:h-9 tw:text-left',
                i === active ? 'tw:bg-hover' : '')}>
              <StatusDot status={clientStatus(c.last_seen_at)} />
              <span className="tw:font-medium tw:text-[13px] tw:truncate">{c.os_info?.hostname || c.client_id}</span>
              <span className="tw:font-mono tw:text-2xs tw:text-faint">{c.client_id}</span>
              <span className="tw:ml-auto tw:text-2xs tw:text-faint">{timeAgo(c.last_seen_at)}</span>
            </button>
          ))}
          <div className="tw:border-t tw:border-line tw:mt-1 tw:pt-1">
            <button type="button" onClick={() => go(q.trim())}
              className="tw:flex tw:w-full tw:items-center tw:gap-2 tw:rounded-md tw:px-2.5 tw:h-8 tw:text-xs tw:text-muted tw:hover:bg-hover">
              <Monitor className="tw:size-3.5" /> Show all results for “{q}” <Kbd className="tw:ml-auto">Enter</Kbd>
            </button>
          </div>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}

function initials(name: string | undefined) {
  if (!name) return '?';
  return name.slice(0, 2).toUpperCase();
}

export default function TopBar({ onOpenPalette, dark }: { onOpenPalette: () => void; dark: boolean }) {
  const ctx = useTraits();
  const { traits, updateTraits } = ctx;
  const { setClient } = useSelectedClient();
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [messagesOpen, setMessagesOpen] = useState(false);
  const [helpOpen, setHelpOpen] = useState(false);
  const messages = ctx.messages || 0;
  const orgs = traits?.orgs ?? [];
  const currentOrg = window.globals?.OrgId || traits?.org || 'root';
  const orgName = traits?.org_name || orgs.find(o => o.id === currentOrg)?.name;

  const switchOrg = (org: string) => {
    window.location.assign(href('/app/index.html', { org_id: org }) + '#/dashboard');
  };

  const isMac = typeof navigator !== 'undefined' && /Mac/.test(navigator.platform);

  return (
    <header className="tw:flex tw:items-center tw:gap-3 tw:h-12 tw:px-4 tw:border-b tw:border-line tw:bg-panel tw:shrink-0">
      <ClientSearch />

      <button type="button" onClick={onOpenPalette}
        className="tw:hidden tw:md:flex tw:items-center tw:gap-2 tw:h-8 tw:px-2.5 tw:rounded-md tw:border tw:border-line tw:text-xs tw:text-faint tw:hover:bg-hover tw:hover:text-fg">
        <Command className="tw:size-3.5" />
        Jump to…
        <span className="tw:flex tw:gap-0.5"><Kbd>{isMac ? '⌘' : 'Ctrl'}</Kbd><Kbd>K</Kbd></span>
      </button>

      <div className="tw:ml-auto tw:flex tw:items-center tw:gap-1">
        {orgName && orgs.length > 1 && (
          <Menu>
            <MenuTrigger asChild>
              <button type="button" className="tw:flex tw:items-center tw:gap-1.5 tw:h-8 tw:px-2.5 tw:rounded-md tw:text-xs tw:font-medium tw:text-muted tw:hover:bg-hover">
                <Building2 className="tw:size-3.5" /> {orgName}
              </button>
            </MenuTrigger>
            <MenuContent>
              <MenuLabel>Switch organization</MenuLabel>
              {orgs.map(o => (
                <MenuItem key={o.id} onSelect={() => switchOrg(o.id)}
                  icon={o.id === currentOrg ? <Check /> : <span className="tw:size-3.5" />}>{o.name}</MenuItem>
              ))}
            </MenuContent>
          </Menu>
        )}

        <Tooltip content={messages ? `${messages} notifications` : 'No notifications'}>
          <button type="button" onClick={() => messages && setMessagesOpen(true)} aria-label="Notifications"
            className="tw:relative tw:flex tw:items-center tw:justify-center tw:size-8 tw:rounded-md tw:text-muted tw:hover:bg-hover tw:hover:text-fg">
            <Bell className="tw:size-4" />
            {messages > 0 && (
              <span className="tw:absolute tw:top-1 tw:right-1 tw:min-w-[14px] tw:h-[14px] tw:px-0.5 tw:rounded-full tw:bg-bad tw:text-white tw:text-[9px] tw:font-bold tw:flex tw:items-center tw:justify-center">
                {messages > 99 ? '99+' : messages}
              </span>
            )}
          </button>
        </Tooltip>

        <IconButton label="Search documentation" onClick={() => setHelpOpen(true)} icon={<CircleHelp className="tw:size-4" />} />

        <IconButton label={dark ? 'Light mode' : 'Dark mode'}
          onClick={() => setTheme(!dark, traits?.ui_settings, updateTraits)}
          icon={dark ? <Sun className="tw:size-4" /> : <Moon className="tw:size-4" />} />

        <Menu>
          <MenuTrigger asChild>
            <button type="button" className="tw:flex tw:items-center tw:gap-2 tw:h-8 tw:pl-1 tw:pr-2 tw:rounded-md tw:hover:bg-hover tw:ml-1">
              {traits?.picture
                ? <img src={traits.picture} alt="" className="tw:size-6 tw:rounded-full" />
                : <span className="tw:flex tw:items-center tw:justify-center tw:size-6 tw:rounded-full tw:bg-accent tw:text-accent-fg tw:text-2xs tw:font-bold">{initials(traits?.username)}</span>}
              <span className="tw:text-[13px] tw:font-medium tw:hidden tw:lg:inline">{traits?.username}</span>
            </button>
          </MenuTrigger>
          <MenuContent>
            <MenuLabel>{traits?.username}{orgName ? ` · ${orgName}` : ''}</MenuLabel>
            <MenuItem icon={<Settings />} onSelect={() => setSettingsOpen(true)}>Settings…</MenuItem>
            <MenuItem icon={dark ? <Sun /> : <Moon />} onSelect={() => setTheme(!dark, traits?.ui_settings, updateTraits)}>
              {dark ? 'Light mode' : 'Dark mode'}
            </MenuItem>
            <MenuItem icon={<Keyboard />} shortcut="?" onSelect={() => {
              document.dispatchEvent(new KeyboardEvent('keydown', { key: '?', shiftKey: true, bubbles: true }));
            }}>Keyboard shortcuts</MenuItem>
            <MenuSeparator />
            <MenuItem icon={<LogOut />} onSelect={() => {
              window.location.assign(href('/app/logoff.html', { username: traits?.username }));
            }}>Sign out</MenuItem>
          </MenuContent>
        </Menu>
      </div>

      <Suspense fallback={null}>
        {settingsOpen && <UserSettingsModal setClient={setClient} onClose={() => setSettingsOpen(false)} />}
        {messagesOpen && <ShowGlobalMessages onClose={() => setMessagesOpen(false)} />}
        {helpOpen && <HelpDialog onClose={() => setHelpOpen(false)} />}
      </Suspense>
    </header>
  );
}
