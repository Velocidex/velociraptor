// User administration: who has access, to which orgs, with which roles.
//
// Replaces the legacy two-pane inspector where roles were a wall of
// checkboxes with no explanation. Here every role says what it allows,
// changes apply immediately (optimistically) and removing the last role,
// which silently drops the user from the org, asks first.
import { useEffect, useMemo, useState } from 'react';
import { useHistory, useLocation, useParams } from 'react-router-dom';
import { toast } from 'sonner';
import {
  Building2, Check, ChevronDown, ChevronRight, Clock, KeyRound, MoreHorizontal, Plus, ShieldCheck, UserMinus,
  UserPlus, Users as UsersIcon, Globe,
} from 'lucide-react';
import {
  useAddUserOrgs, useCreateUser, useGlobalUsers, useSetPassword, useSetUserRoles, useTraits, useUserRoles,
  type GlobalUser,
} from '../../lib/queries';
import { fmtTime, timeAgo } from '../../lib/format';
import {
  Badge, Button, Card, CardHeader, Checkbox, ConfirmDialog, Dialog, EmptyState, Field, Input, Menu, MenuContent,
  MenuItem, MenuLabel, MenuTrigger, SearchInput, Skeleton, Tooltip, cn,
} from '../../ui/primitives';
import { MobileBack } from '../../ui/MobileBack';

export const ROLE_INFO: Record<string, { label: string; blurb: string; danger?: boolean }> = {
  reader: {
    label: 'Read-only',
    blurb: 'Can look at collected results, hunts and notebooks. Cannot change anything.',
  },
  analyst: {
    label: 'Analyst',
    blurb: 'Reads results and post-processes them with notebooks and server-side VQL. Cannot start collections or hunts.',
  },
  investigator: {
    label: 'Investigator',
    blurb: 'Everything an analyst can do, plus starting collections on clients and launching hunts.',
  },
  artifact_writer: {
    label: 'Artifact writer',
    blurb: 'Can create and edit client artifacts. Effectively code execution on every endpoint: grant sparingly.',
    danger: true,
  },
  org_admin: {
    label: 'Org admin',
    blurb: 'Manages organizations and their users. Usually combined with another role.',
  },
  administrator: {
    label: 'Server administrator',
    blurb: 'All powerful: runs arbitrary VQL on the server and reconfigures it.',
    danger: true,
  },
  api: {
    label: 'API client',
    blurb: 'Allows connecting over the API port.',
  },
};

const ROLE_ORDER = ['reader', 'analyst', 'investigator', 'artifact_writer', 'org_admin', 'administrator', 'api'];

export const PERM_INFO: Record<string, string> = {
  ALL_QUERY: 'Issue any query at all',
  ANY_QUERY: 'Issue any query at all',
  PUBLISH: 'Publish events to server side queues',
  READ_RESULTS: 'Read results from hunts, flows and notebooks',
  LABEL_CLIENT: 'Change client labels and metadata',
  COLLECT_CLIENT: 'Schedule or cancel collections on clients',
  COLLECT_BASIC: 'Schedule basic collections on clients',
  START_HUNT: 'Start a new hunt',
  COLLECT_SERVER: 'Collect artifacts on the server',
  ARTIFACT_WRITER: 'Add or edit custom client artifacts',
  SERVER_ARTIFACT_WRITER: 'Add or edit custom server artifacts',
  EXECVE: 'Execute arbitrary commands on clients',
  NOTEBOOK_EDITOR: 'Change notebooks and cells',
  SERVER_ADMIN: 'Manage server configuration',
  ORG_ADMIN: 'Manage organizations',
  IMPERSONATION: 'Run query() as another user',
  FILESYSTEM_READ: 'Read arbitrary files on the server',
  FILESYSTEM_WRITE: 'Create files on the server',
  NETWORK: 'Run plugins that make network connections',
  MACHINE_STATE: 'Collect machine state (e.g. pslist)',
  PREPARE_RESULTS: 'Create export zip files',
  DELETE_RESULTS: 'Delete clients, flows and other data',
  DATASTORE_ACCESS: 'Raw datastore access',
};

const permLabel = (p: string) => p.split('_').map(w => w[0] + w.slice(1).toLowerCase()).join(' ');

function Avatar({ name, className }: { name: string; className?: string }) {
  let h = 0;
  for (const c of name) h = (h * 31 + c.charCodeAt(0)) % 360;
  return (
    <span className={cn('tw:inline-flex tw:items-center tw:justify-center tw:rounded-full tw:text-white tw:font-semibold tw:shrink-0 tw:uppercase', className)}
      style={{ background: `hsl(${h} 45% 48%)` }}>
      {name.slice(0, 2)}
    </span>
  );
}

export default function UsersPage() {
  const { user: selected } = useParams<{ user?: string }>();
  const history = useHistory();
  const { data: users, isLoading } = useGlobalUsers();
  const [filter, setFilter] = useState('');
  const [adding, setAdding] = useState(false);
  const location = useLocation();
  useEffect(() => {
    if (new URLSearchParams(location.search).get('add') === '1') {
      setAdding(true);
      history.replace(location.pathname);
    }
  }, [location.search]); // eslint-disable-line react-hooks/exhaustive-deps

  const shown = useMemo(() => {
    const f = filter.toLowerCase();
    return (users ?? []).filter(u => !f || u.name.toLowerCase().includes(f)
      || (u.orgs ?? []).some(o => o.name.toLowerCase().includes(f)));
  }, [users, filter]);

  const current = users?.find(u => u.name === selected);

  return (
    <div className="rx-split tw:flex tw:flex-1 tw:min-h-0" data-detail={selected ? 1 : 0}>
      <aside className="rx-split-list tw:w-80 tw:shrink-0 tw:border-r tw:border-line tw:flex tw:flex-col tw:bg-panel">
        <div className="tw:p-3 tw:flex tw:flex-col tw:gap-2 tw:border-b tw:border-line">
          <div className="tw:flex tw:items-center tw:gap-2">
            <h1 className="tw:text-[15px] tw:font-semibold tw:flex-1">Users</h1>
            <span className="tw:text-xs tw:text-faint">{users?.length ?? ''}</span>
            <Button size="sm" variant="primary" icon={<UserPlus />} onClick={() => setAdding(true)}>Add</Button>
          </div>
          <SearchInput value={filter} onChange={setFilter} placeholder="Filter users or orgs" />
        </div>
        <div className="tw:flex-1 tw:overflow-y-auto tw:py-1">
          {isLoading && Array.from({ length: 4 }, (_, i) => <Skeleton key={i} className="tw:h-12 tw:mx-2 tw:my-1" />)}
          {shown.map(u => (
            <button key={u.name} onClick={() => history.push(`/users/${encodeURIComponent(u.name)}`)}
              className={cn('tw:w-full tw:flex tw:items-center tw:gap-3 tw:px-3 tw:py-2 tw:text-left tw:hover:bg-hover',
                u.name === selected && 'tw:bg-accent-soft tw:hover:bg-accent-soft')}>
              <Avatar name={u.name} className="tw:size-8 tw:text-xs" />
              <div className="tw:flex-1 tw:min-w-0">
                <div className="tw:text-[13px] tw:font-medium tw:truncate">{u.name}</div>
                <div className="tw:text-2xs tw:text-faint tw:truncate">
                  {(u.orgs ?? []).map(o => o.name).join(', ') || 'No orgs'}
                </div>
              </div>
              <span className="tw:text-2xs tw:text-faint tw:shrink-0">
                {u.stats?.last_active_time ? timeAgo(u.stats.last_active_time) : 'never'}
              </span>
            </button>
          ))}
          {!isLoading && shown.length === 0 && (
            <div className="tw:px-4 tw:py-8 tw:text-center tw:text-xs tw:text-faint">No users match</div>
          )}
        </div>
      </aside>
      <main className="rx-split-detail tw:flex-1 tw:min-w-0 tw:overflow-y-auto">
        <MobileBack to="/users" label="All users" />
        {current ? <UserDetail key={current.name} user={current} /> : (
          <EmptyState className="tw:mt-24" icon={<UsersIcon />} title={selected ? `No user named ${selected}` : 'Select a user'}
            action={<Button icon={<UserPlus />} onClick={() => setAdding(true)}>Add a user</Button>}>
            Pick someone on the left to see and change what they can do in each organization.
          </EmptyState>
        )}
      </main>
      <AddUserDialog open={adding} onOpenChange={setAdding}
        onCreated={name => history.push(`/users/${encodeURIComponent(name)}`)} />
    </div>
  );
}

function useAllOrgs() {
  const { traits } = useTraits();
  const { data: users } = useGlobalUsers();
  return useMemo(() => {
    const m = new Map<string, string>();
    for (const o of traits?.orgs ?? []) m.set(o.id, o.name);
    for (const u of users ?? []) for (const o of u.orgs ?? []) m.set(o.id, o.name);
    if (!m.size) m.set('root', '<root>');
    return [...m].map(([id, name]) => ({ id, name })).sort((a, b) => a.name.localeCompare(b.name));
  }, [traits, users]);
}

function UserDetail({ user }: { user: GlobalUser }) {
  const orgs = user.orgs ?? [];
  const allOrgs = useAllOrgs();
  const [orgId, setOrgId] = useState(orgs.find(o => o.id === 'root')?.id ?? orgs[0]?.id);
  const [pwOpen, setPwOpen] = useState(false);
  const addOrgs = useAddUserOrgs();
  const missing = allOrgs.filter(o => !orgs.some(x => x.id === o.id));
  const activeOrg = orgs.find(o => o.id === orgId) ?? orgs[0];

  const addTo = (org: { id: string; name: string }) => addOrgs.mutate({ name: user.name, orgs: [org.id], roles: ['reader'] }, {
    onSuccess: () => { toast.success(`${user.name} added to ${org.name} as read-only`); setOrgId(org.id); },
    onError: e => toast.error(`Could not add to org: ${(e as Error).message}`),
  });

  return (
    <div className="tw:max-w-5xl tw:mx-auto tw:p-6 tw:flex tw:flex-col tw:gap-5">
      <header className="tw:flex tw:items-center tw:gap-4">
        <Avatar name={user.name} className="tw:size-12 tw:text-base" />
        <div className="tw:flex-1 tw:min-w-0">
          <h2 className="tw:text-xl tw:font-semibold tw:truncate">{user.name}</h2>
          <div className="tw:flex tw:flex-wrap tw:items-center tw:gap-x-4 tw:gap-y-1 tw:text-xs tw:text-muted tw:mt-1">
            <span className="tw:flex tw:items-center tw:gap-1"><Clock className="tw:size-3.5" />
              {user.stats?.last_active_time
                ? <Tooltip content={fmtTime(user.stats.last_active_time)}><span>Active {timeAgo(user.stats.last_active_time)}</span></Tooltip>
                : 'Never signed in'}
            </span>
            {user.stats?.last_ip_address && (
              <span className="tw:flex tw:items-center tw:gap-1"><Globe className="tw:size-3.5" />{user.stats.last_ip_address}</span>
            )}
            <span className="tw:flex tw:items-center tw:gap-1"><Building2 className="tw:size-3.5" />
              {orgs.length} {orgs.length === 1 ? 'organization' : 'organizations'}
            </span>
          </div>
        </div>
        <Button icon={<KeyRound />} onClick={() => setPwOpen(true)}>Set password</Button>
      </header>

      <div className="tw:flex tw:items-center tw:gap-1.5 tw:flex-wrap">
        {orgs.map(o => (
          <button key={o.id} onClick={() => setOrgId(o.id)}
            className={cn('tw:h-8 tw:px-3 tw:rounded-full tw:border tw:text-[13px] tw:flex tw:items-center tw:gap-1.5',
              o.id === activeOrg?.id ? 'tw:border-accent tw:bg-accent-soft tw:text-accent tw:font-medium' : 'tw:border-line tw:hover:bg-hover')}>
            <Building2 className="tw:size-3.5" />{o.name}
          </button>
        ))}
        {missing.length > 0 && (
          <Menu>
            <MenuTrigger asChild>
              <Button size="sm" variant="ghost" icon={<Plus />} loading={addOrgs.isPending}>Add to org</Button>
            </MenuTrigger>
            <MenuContent align="start">
              <MenuLabel>Grant read-only access to</MenuLabel>
              {missing.map(o => <MenuItem key={o.id} icon={<Building2 />} onSelect={() => addTo(o)}>{o.name}</MenuItem>)}
            </MenuContent>
          </Menu>
        )}
      </div>

      {activeOrg
        ? <OrgRoles key={activeOrg.id} user={user.name} org={activeOrg} />
        : <Card className="tw:p-6 tw:text-sm tw:text-muted">This user has no access to any organization. Use “Add to org” to grant some.</Card>}

      <PasswordDialog open={pwOpen} onOpenChange={setPwOpen} user={user.name} />
    </div>
  );
}

function OrgRoles({ user, org }: { user: string; org: { id: string; name: string } }) {
  const { data: acl, isLoading } = useUserRoles(user, org.id);
  const setRoles = useSetUserRoles();
  const [confirmRemove, setConfirmRemove] = useState<null | { roles: string[]; permissions: string[] }>(null);
  const [showPerms, setShowPerms] = useState(false);
  const { traits } = useTraits();

  const roles = acl?.roles ?? [];
  const perms = acl?.permissions ?? [];
  const effective = new Set(acl?.effective_permissions ?? []);
  const allRoles = [...ROLE_ORDER.filter(r => acl?.all_roles?.includes(r) ?? true),
    ...(acl?.all_roles ?? []).filter(r => !ROLE_ORDER.includes(r))];
  const self = traits?.username === user;

  const apply = (next: { roles: string[]; permissions: string[] }, force = false) => {
    if (!force && next.roles.length === 0 && next.permissions.length === 0) {
      setConfirmRemove(next);
      return;
    }
    setRoles.mutate({ name: user, org: org.id, ...next }, {
      onError: e => toast.error(`Could not update roles: ${(e as Error).message}`),
      onSuccess: () => {
        if (!next.roles.length && !next.permissions.length) toast.success(`${user} removed from ${org.name}`);
      },
    });
  };

  const toggleRole = (r: string) => apply({
    roles: roles.includes(r) ? roles.filter(x => x !== r) : [...roles, r],
    permissions: perms,
  });
  const togglePerm = (p: string) => apply({
    roles,
    permissions: perms.includes(p) ? perms.filter(x => x !== p) : [...perms, p],
  });

  if (isLoading) return <Skeleton className="tw:h-80" />;

  return (
    <>
      <Card>
        <CardHeader icon={<ShieldCheck />} title={`Roles in ${org.name}`}
          subtitle="Changes apply immediately. Roles add up."
          actions={
            <Menu>
              <MenuTrigger asChild>
                <Button size="sm" variant="ghost" icon={<MoreHorizontal />} aria-label="More" />
              </MenuTrigger>
              <MenuContent>
                <MenuItem danger icon={<UserMinus />} disabled={self}
                  onSelect={() => apply({ roles: [], permissions: [] })}>
                  Remove from {org.name}
                </MenuItem>
              </MenuContent>
            </Menu>
          } />
        <div className="tw:grid tw:grid-cols-1 tw:md:grid-cols-2 tw:gap-2 tw:p-4">
          {allRoles.map(r => {
            const info = ROLE_INFO[r] ?? { label: r, blurb: '' };
            const on = roles.includes(r);
            return (
              <button key={r} onClick={() => toggleRole(r)} role="checkbox" aria-checked={on}
                className={cn('tw:flex tw:items-start tw:gap-3 tw:text-left tw:p-3 tw:rounded-lg tw:border tw:transition-colors',
                  on ? 'tw:border-accent tw:bg-accent-soft' : 'tw:border-line tw:hover:bg-hover')}>
                <span className={cn('tw:mt-0.5 tw:size-4 tw:rounded tw:border tw:flex tw:items-center tw:justify-center tw:shrink-0',
                  on ? 'tw:bg-accent tw:border-accent tw:text-white' : 'tw:border-line')}>
                  {on && <Check className="tw:size-3" strokeWidth={3} />}
                </span>
                <span className="tw:flex-1 tw:min-w-0">
                  <span className="tw:flex tw:items-center tw:gap-2 tw:text-[13px] tw:font-medium">
                    {info.label}
                    {info.danger && <Badge tone="warn">powerful</Badge>}
                  </span>
                  <span className="tw:block tw:text-xs tw:text-muted tw:mt-0.5">{info.blurb}</span>
                </span>
              </button>
            );
          })}
        </div>
      </Card>

      <Card>
        <button onClick={() => setShowPerms(s => !s)}
          className="tw:w-full tw:flex tw:items-center tw:gap-2 tw:px-4 tw:py-3 tw:text-left tw:hover:bg-hover tw:rounded-xl">
          {showPerms ? <ChevronDown className="tw:size-4 tw:text-faint" /> : <ChevronRight className="tw:size-4 tw:text-faint" />}
          <span className="tw:text-[13px] tw:font-semibold tw:flex-1">Effective permissions</span>
          <span className="tw:text-xs tw:text-muted">
            {effective.size} granted{perms.length ? `, ${perms.length} added individually` : ''}
          </span>
        </button>
        {!showPerms && effective.size > 0 && (
          <div className="tw:flex tw:flex-wrap tw:gap-1 tw:px-4 tw:pb-4">
            {[...effective].sort().map(p => <Badge key={p} tone={perms.includes(p) ? 'accent' : 'neutral'}>{permLabel(p)}</Badge>)}
          </div>
        )}
        {showPerms && (
          <div className="tw:px-4 tw:pb-4">
            <p className="tw:text-xs tw:text-muted tw:mb-3">
              Permissions granted by a role are locked on. Tick others to grant them on top of the roles.
            </p>
            <div className="tw:grid tw:grid-cols-1 tw:md:grid-cols-2 tw:gap-x-6 tw:gap-y-1">
              {(acl?.all_permissions ?? []).map(p => {
                const extra = perms.includes(p);
                const viaRole = effective.has(p) && !extra;
                return (
                  <div key={p} className="tw:flex tw:items-start tw:gap-2 tw:py-1">
                    <Tooltip content={viaRole ? 'Granted by a role' : undefined}>
                      <span><Checkbox checked={extra || viaRole} onChange={() => !viaRole && togglePerm(p)}
                        className={viaRole ? 'tw:opacity-50 tw:pointer-events-none' : undefined} /></span>
                    </Tooltip>
                    <div className="tw:min-w-0">
                      <div className="tw:text-[13px]">{permLabel(p)}</div>
                      {PERM_INFO[p] && <div className="tw:text-2xs tw:text-faint">{PERM_INFO[p]}</div>}
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        )}
      </Card>

      <ConfirmDialog open={!!confirmRemove} onOpenChange={o => !o && setConfirmRemove(null)}
        title={`Remove ${user} from ${org.name}?`} confirmLabel="Remove access" danger loading={setRoles.isPending}
        onConfirm={() => { if (confirmRemove) apply(confirmRemove, true); setConfirmRemove(null); }}>
        Without any role or permission the user loses all access to this organization. You can add them back later.
      </ConfirmDialog>
    </>
  );
}

function AddUserDialog({ open, onOpenChange, onCreated }: {
  open: boolean; onOpenChange: (v: boolean) => void; onCreated: (name: string) => void;
}) {
  const allOrgs = useAllOrgs();
  const create = useCreateUser();
  const setPw = useSetPassword();
  const [name, setName] = useState('');
  const [password, setPassword] = useState('');
  const [role, setRole] = useState('reader');
  const [orgs, setOrgs] = useState<string[]>([]);
  const chosen = orgs.length ? orgs : [allOrgs.find(o => o.id === 'root')?.id ?? allOrgs[0]?.id].filter(Boolean) as string[];

  const reset = () => { setName(''); setPassword(''); setRole('reader'); setOrgs([]); };
  const submit = async () => {
    const n = name.trim();
    if (!n) return;
    try {
      await create.mutateAsync({ name: n, orgs: chosen, roles: [role] });
      if (password) await setPw.mutateAsync({ username: n, password });
      toast.success(`Created ${n}`);
      onOpenChange(false);
      reset();
      onCreated(n);
    } catch (e) {
      toast.error(`Could not create user: ${(e as Error).message}`);
    }
  };

  return (
    <Dialog open={open} onOpenChange={o => { onOpenChange(o); if (!o) reset(); }} title="Add a user" width={520}
      description="With SSO, the username must match the identity provider's. With basic auth, set a password here."
      footer={<>
        <Button variant="ghost" onClick={() => onOpenChange(false)}>Cancel</Button>
        <Button variant="primary" icon={<UserPlus />} disabled={!name.trim()} loading={create.isPending || setPw.isPending}
          onClick={submit}>Create user</Button>
      </>}>
      <div className="tw:flex tw:flex-col tw:gap-4">
        <Field label="Username" required>
          <Input value={name} onChange={e => setName(e.target.value)} autoFocus placeholder="jane@example.com"
            onKeyDown={e => e.key === 'Enter' && submit()} />
        </Field>
        <Field label="Password" hint="Optional. Leave empty for SSO users.">
          <Input type="password" value={password} onChange={e => setPassword(e.target.value)} autoComplete="new-password" />
        </Field>
        <Field label="Starting role" hint="You can fine-tune roles after creating the user.">
          <div className="tw:grid tw:grid-cols-3 tw:gap-1.5">
            {['reader', 'analyst', 'investigator'].map(r => (
              <button key={r} onClick={() => setRole(r)}
                className={cn('tw:h-9 tw:rounded-md tw:border tw:text-[13px]',
                  role === r ? 'tw:border-accent tw:bg-accent-soft tw:text-accent tw:font-medium' : 'tw:border-line tw:hover:bg-hover')}>
                {ROLE_INFO[r].label}
              </button>
            ))}
          </div>
          <div className="tw:text-2xs tw:text-faint tw:mt-1.5">{ROLE_INFO[role].blurb}</div>
        </Field>
        {allOrgs.length > 1 && (
          <Field label="Organizations">
            <div className="tw:flex tw:flex-wrap tw:gap-1.5">
              {allOrgs.map(o => {
                const on = chosen.includes(o.id);
                return (
                  <button key={o.id} onClick={() => setOrgs(on ? chosen.filter(x => x !== o.id) : [...chosen, o.id])}
                    className={cn('tw:h-7 tw:px-2.5 tw:rounded-full tw:border tw:text-xs tw:flex tw:items-center tw:gap-1',
                      on ? 'tw:border-accent tw:bg-accent-soft tw:text-accent' : 'tw:border-line tw:hover:bg-hover')}>
                    {on && <Check className="tw:size-3" />}{o.name}
                  </button>
                );
              })}
            </div>
          </Field>
        )}
      </div>
    </Dialog>
  );
}

function PasswordDialog({ open, onOpenChange, user }: { open: boolean; onOpenChange: (v: boolean) => void; user: string }) {
  const setPw = useSetPassword();
  const [a, setA] = useState('');
  const [b, setB] = useState('');
  const mismatch = !!b && a !== b;
  const submit = () => {
    if (!a || a !== b) return;
    setPw.mutate({ username: user, password: a }, {
      onSuccess: () => { toast.success(`Password updated for ${user}`); onOpenChange(false); setA(''); setB(''); },
      onError: e => toast.error(`Could not set password: ${(e as Error).message}`),
    });
  };
  return (
    <Dialog open={open} onOpenChange={onOpenChange} title={`Set password for ${user}`} width={420}
      footer={<>
        <Button variant="ghost" onClick={() => onOpenChange(false)}>Cancel</Button>
        <Button variant="primary" disabled={!a || a !== b} loading={setPw.isPending} onClick={submit}>Set password</Button>
      </>}>
      <div className="tw:flex tw:flex-col tw:gap-3">
        <Field label="New password"><Input type="password" value={a} onChange={e => setA(e.target.value)} autoFocus autoComplete="new-password" /></Field>
        <Field label="Repeat" hint={mismatch ? 'Passwords do not match' : undefined}>
          <Input type="password" value={b} onChange={e => setB(e.target.value)} autoComplete="new-password"
            onKeyDown={e => e.key === 'Enter' && submit()} />
        </Field>
      </div>
    </Dialog>
  );
}
