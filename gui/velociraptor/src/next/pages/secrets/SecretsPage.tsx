// Secrets: credentials that VQL plugins (S3 uploads, SMTP, SSH, HTTP...)
// use by name, so artifacts never carry plaintext keys.
//
// One page with every secret type down the left, what it is for, and
// the stored secrets with who may use them. Values are write-only: the
// server never sends them back, so "Replace values" re-enters them.
import { useEffect, useMemo, useState } from 'react';
import { useHistory, useParams } from 'react-router-dom';
import { toast } from 'sonner';
import { Eye, EyeOff, KeyRound, Lock, Plus, RefreshCw, Trash2, Users as UsersIcon, Globe } from 'lucide-react';
import {
  useAddSecret, useGlobalUsers, useModifySecret, useSecret, useSecretDefinitions, type SecretDefinition,
} from '../../lib/queries';
import { currentOrgId } from '../../lib/api';
import {
  Badge, Button, Card, CardHeader, ConfirmDialog, Dialog, EmptyState, Field, IconButton, Input, Mono, Skeleton,
  Switch, Textarea, Tooltip, cn,
} from '../../ui/primitives';
import { TagInput } from '../hunts/TagInput';

const MULTILINE = /(^|_)(root_ca|private_key|hostkey|env|extra_headers|extra_params|cookies)$/;
const SENSITIVE = /(secret|password|token|private_key|api_key|kms_encryption_key)$/;

const USE_HINT: Record<string, string> = {
  'AWS S3 Creds': "upload_s3(secret='NAME', ...)",
  'SMTP Creds': "mail(secret='NAME', ...)",
  'SSH PrivateKey': "ssh_exec / scp: secret='NAME'",
  'HTTP Secrets': "http_client(secret='NAME', ...)",
  'Elastic Creds': "elastic_upload(secret='NAME', ...)",
  'Splunk Creds': "splunk_upload(secret='NAME', ...)",
  'ADX Creds': "adx_upload(secret='NAME', ...)",
  'Azure Monitor Creds': "azure_monitor_upload(secret='NAME', ...)",
  'Execve Secrets': "execve(secret='NAME', ...)",
};

export default function SecretsPage() {
  const { type: routeType } = useParams<{ type?: string }>();
  const history = useHistory();
  const { data: defs, isLoading } = useSecretDefinitions();
  const type = routeType ? decodeURIComponent(routeType) : undefined;
  const current = defs?.find(d => d.type_name === type);

  useEffect(() => {
    if (!type && defs?.length) {
      const first = defs.find(d => d.secret_names?.length) ?? defs[0];
      history.replace(`/secrets/${encodeURIComponent(first.type_name)}`);
    }
  }, [type, defs, history]);

  const total = (defs ?? []).reduce((n, d) => n + (d.secret_names?.length ?? 0), 0);

  return (
    <div className="tw:flex tw:flex-1 tw:min-h-0">
      <aside className="tw:w-72 tw:shrink-0 tw:border-r tw:border-line tw:flex tw:flex-col tw:bg-panel">
        <div className="tw:p-3 tw:border-b tw:border-line">
          <div className="tw:flex tw:items-center tw:gap-2">
            <h1 className="tw:text-[15px] tw:font-semibold tw:flex-1">Secrets</h1>
            <span className="tw:text-xs tw:text-faint">{total} stored</span>
          </div>
          <p className="tw:text-2xs tw:text-muted tw:mt-1">
            Credentials VQL can use by name. Values are encrypted and never shown again.
          </p>
        </div>
        <nav className="tw:flex-1 tw:overflow-y-auto tw:py-1">
          {isLoading && Array.from({ length: 6 }, (_, i) => <Skeleton key={i} className="tw:h-9 tw:mx-2 tw:my-1" />)}
          {(defs ?? []).map(d => {
            const n = d.secret_names?.length ?? 0;
            return (
              <button key={d.type_name} onClick={() => history.push(`/secrets/${encodeURIComponent(d.type_name)}`)}
                className={cn('tw:w-full tw:flex tw:items-center tw:gap-2.5 tw:px-3 tw:h-9 tw:text-left tw:text-[13px] tw:hover:bg-hover',
                  d.type_name === type && 'tw:bg-accent-soft tw:text-accent tw:font-medium tw:hover:bg-accent-soft')}>
                <KeyRound className="tw:size-4 tw:text-faint tw:shrink-0" />
                <span className="tw:flex-1 tw:truncate">{d.type_name}</span>
                {n > 0 && <Badge tone={d.type_name === type ? 'accent' : 'neutral'}>{n}</Badge>}
              </button>
            );
          })}
        </nav>
      </aside>
      <main className="tw:flex-1 tw:min-w-0 tw:overflow-y-auto">
        {current ? <SecretType key={current.type_name} def={current} /> : !isLoading && (
          <EmptyState className="tw:mt-24" icon={<KeyRound />} title="Pick a secret type" />
        )}
      </main>
    </div>
  );
}

function SecretType({ def }: { def: SecretDefinition }) {
  const [adding, setAdding] = useState<null | { name?: string }>(null);
  const names = def.secret_names ?? [];
  return (
    <div className="tw:max-w-4xl tw:mx-auto tw:p-6 tw:flex tw:flex-col tw:gap-5">
      <header className="tw:flex tw:items-start tw:gap-4">
        <div className="tw:size-11 tw:rounded-xl tw:bg-accent-soft tw:text-accent tw:flex tw:items-center tw:justify-center tw:shrink-0">
          <KeyRound className="tw:size-5" />
        </div>
        <div className="tw:flex-1 tw:min-w-0">
          <h2 className="tw:text-xl tw:font-semibold">{def.type_name}</h2>
          <p className="tw:text-[13px] tw:text-muted tw:mt-0.5">{def.description}</p>
          {USE_HINT[def.type_name] && (
            <p className="tw:text-xs tw:text-faint tw:mt-1.5">Use it in VQL as <Mono className="tw:text-fg">{USE_HINT[def.type_name]}</Mono></p>
          )}
        </div>
        <Button variant="primary" icon={<Plus />} onClick={() => setAdding({})}>Add secret</Button>
      </header>

      {names.length === 0 ? (
        <Card className="tw:p-10">
          <EmptyState icon={<Lock />} title={`No ${def.type_name} yet`}
            action={<Button icon={<Plus />} onClick={() => setAdding({})}>Add the first one</Button>}>
            Store credentials here once and refer to them by name from artifacts and notebooks.
          </EmptyState>
        </Card>
      ) : names.map(n => <SecretCard key={n} def={def} name={n} onReplace={() => setAdding({ name: n })} />)}

      <Card className="tw:p-4">
        <div className="tw:text-xs tw:font-semibold tw:mb-2 tw:text-muted">Fields for this type</div>
        <div className="tw:flex tw:flex-wrap tw:gap-1">
          {(def.fields ?? []).map(f => <Badge key={f} tone="neutral"><span className="tw:font-mono">{f}</span></Badge>)}
        </div>
        {def.verifier && (
          <div className="tw:text-2xs tw:text-faint tw:mt-2">Required: <Mono>{def.verifier.replace(/\s+/g, ' ').replace(/^x=>/, '')}</Mono></div>
        )}
      </Card>

      {adding && <SecretDialog def={def} replaceName={adding.name} onClose={() => setAdding(null)} />}
    </div>
  );
}

function SecretCard({ def, name, onReplace }: { def: SecretDefinition; name: string; onReplace: () => void }) {
  const { data: s, isLoading } = useSecret(def.type_name, name);
  const modify = useModifySecret();
  const { data: users } = useGlobalUsers();
  const [confirmDelete, setConfirmDelete] = useState(false);
  const root = currentOrgId() === 'root';
  const shared = s?.users ?? [];

  const setUsers = (next: string[]) => modify.mutate({
    type_name: def.type_name, name,
    add_users: next.filter(u => !shared.includes(u)),
    remove_users: shared.filter(u => !next.includes(u)),
    visible_to_all_orgs: s?.visible_to_all_orgs,
  }, { onError: e => toast.error(`Could not update sharing: ${(e as Error).message}`) });

  return (
    <Card>
      <CardHeader icon={<Lock />} title={<span className="tw:font-mono">{name}</span>}
        actions={<div className="tw:flex tw:gap-1">
          <Button size="sm" variant="ghost" icon={<RefreshCw className="tw:size-3.5" />} onClick={onReplace}>Replace values</Button>
          <IconButton size="sm" label="Delete secret" icon={<Trash2 className="tw:size-3.5" />} onClick={() => setConfirmDelete(true)} />
        </div>} />
      <div className="tw:p-4 tw:flex tw:flex-col tw:gap-4">
        {isLoading ? <Skeleton className="tw:h-10" /> : <>
          <Field label={<span className="tw:flex tw:items-center tw:gap-1.5"><UsersIcon className="tw:size-3.5" />Shared with</span>}
            hint="Only these users can use the secret in their queries. Server administrators always can.">
            <TagInput value={shared} onChange={setUsers} suggestions={(users ?? []).map(u => u.name)}
              placeholder={shared.length ? 'Add user…' : 'Nobody yet, type a username'} tone="accent" />
          </Field>
          {root && (
            <Switch checked={!!s?.visible_to_all_orgs} label={<span className="tw:flex tw:items-center tw:gap-1.5"><Globe className="tw:size-3.5" />Visible to all organizations</span>}
              onChange={v => modify.mutate({ type_name: def.type_name, name, visible_to_all_orgs: v, add_users: shared, remove_users: shared })} />
          )}
        </>}
      </div>
      <ConfirmDialog open={confirmDelete} onOpenChange={setConfirmDelete} danger title={`Delete ${name}?`} confirmLabel="Delete secret"
        loading={modify.isPending}
        onConfirm={() => modify.mutate({ type_name: def.type_name, name, delete: true }, {
          onSuccess: () => { toast.success(`Deleted ${name}`); setConfirmDelete(false); },
          onError: e => toast.error((e as Error).message),
        })}>
        Any artifact or notebook that uses <Mono>{name}</Mono> will stop working.
      </ConfirmDialog>
    </Card>
  );
}

function SecretDialog({ def, replaceName, onClose }: { def: SecretDefinition; replaceName?: string; onClose: () => void }) {
  const add = useAddSecret();
  const [name, setName] = useState(replaceName ?? '');
  const [vals, setVals] = useState<Record<string, string>>(() => ({ ...(def.template ?? {}) }));
  const [reveal, setReveal] = useState<Record<string, boolean>>({});
  const fields = def.fields ?? [];
  const filled = useMemo(() => Object.fromEntries(Object.entries(vals).filter(([, v]) => v !== '')), [vals]);

  const submit = () => {
    if (!name.trim()) return;
    add.mutate({ name: name.trim(), type_name: def.type_name, secret: filled }, {
      onSuccess: () => { toast.success(replaceName ? `Updated ${name}` : `Saved ${name}`); onClose(); },
      onError: e => toast.error(`Could not save: ${(e as Error).message}`),
    });
  };

  return (
    <Dialog open onOpenChange={o => !o && onClose()} width={620}
      title={replaceName ? `Replace values of ${replaceName}` : `New ${def.type_name}`}
      description={replaceName ? 'Stored values are never shown. Enter the complete set again.' : def.description}
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancel</Button>
        <Button variant="primary" icon={<Lock />} disabled={!name.trim()} loading={add.isPending} onClick={submit}>
          {replaceName ? 'Save new values' : 'Save secret'}
        </Button>
      </>}>
      <div className="tw:flex tw:flex-col tw:gap-3">
        {!replaceName && (
          <Field label="Name" required hint="How VQL refers to this secret.">
            <Input value={name} onChange={e => setName(e.target.value)} autoFocus placeholder="e.g. evidence-bucket" className="tw:font-mono" />
          </Field>
        )}
        <div className="tw:grid tw:grid-cols-1 tw:md:grid-cols-2 tw:gap-3">
          {fields.map(f => {
            const multi = MULTILINE.test(f);
            const sens = SENSITIVE.test(f);
            const v = vals[f] ?? '';
            const set = (x: string) => setVals(p => ({ ...p, [f]: x }));
            return (
              <div key={f} className={multi ? 'tw:md:col-span-2' : undefined}>
                <div className="tw:flex tw:items-center tw:gap-1 tw:mb-1">
                  <span className="tw:text-xs tw:font-medium tw:font-mono">{f}</span>
                  {sens && (
                    <Tooltip content={reveal[f] ? 'Hide' : 'Show'}>
                      <button type="button" className="tw:text-faint tw:hover:text-fg tw:ml-auto"
                        onClick={() => setReveal(r => ({ ...r, [f]: !r[f] }))}>
                        {reveal[f] ? <EyeOff className="tw:size-3.5" /> : <Eye className="tw:size-3.5" />}
                      </button>
                    </Tooltip>
                  )}
                </div>
                {multi
                  ? <Textarea rows={f === 'root_ca' || f === 'private_key' ? 5 : 3} value={v} onChange={e => set(e.target.value)}
                    className={cn('tw:font-mono tw:text-xs', sens && !reveal[f] && 'rx-masked')} spellCheck={false} />
                  : <Input value={v} onChange={e => set(e.target.value)} type={sens && !reveal[f] ? 'password' : 'text'}
                    autoComplete="off" spellCheck={false} className="tw:font-mono tw:text-xs" />}
              </div>
            );
          })}
        </div>
      </div>
    </Dialog>
  );
}
