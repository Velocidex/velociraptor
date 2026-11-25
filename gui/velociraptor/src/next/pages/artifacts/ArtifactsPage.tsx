// Artifact catalog: find an artifact, understand it (description,
// parameters, VQL), then collect or hunt with it in one click. Custom
// artifacts can be created, edited and deleted here.
import { lazy, Suspense, useEffect, useMemo, useRef, useState } from 'react';
import { useHistory, useLocation, useParams } from 'react-router-dom';
import { toast } from 'sonner';
import { useVirtualizer } from '@tanstack/react-virtual';
import {
  BookOpen, Code2, Copy, Crosshair, ExternalLink, FilePlus2, Package, Pencil, Play, Search, Trash2, Wrench, Link2,
} from 'lucide-react';
import {
  useArtifactCatalog, useArtifactDefinitions, useSetArtifactFile, usePermission, type ArtifactKind,
} from '../../lib/queries';
import type { ArtifactCollectorArgs, ArtifactDescriptor } from '../../lib/types';
import {
  Badge, Button, Card, CardHeader, ConfirmDialog, CopyButton, EmptyState, Input, Mono, Segmented, Skeleton, Switch, cn,
} from '../../ui/primitives';
import { useSelectedClient } from '../../shell/client-context';
import Markdown from '../../ui/Markdown';

const NewHuntDialog = lazy(() => import('../hunts/NewHuntDialog'));
const ClientPicker = lazy(() => import('../../ui/ClientPicker'));
const CodeEditor = lazy(() => import('../../ui/CodeEditor.jsx'));
const ArtifactEditor = lazy(() => import('./ArtifactEditor'));

const KINDS: { value: ArtifactKind; label: string }[] = [
  { value: '', label: 'All' },
  { value: 'client', label: 'Client' },
  { value: 'server', label: 'Server' },
  { value: 'client_event', label: 'Events' },
  { value: 'notebook', label: 'Notebook' },
];

export function typeLabel(t?: string) {
  switch ((t ?? '').toLowerCase()) {
    case 'client': return 'Client';
    case 'server': return 'Server';
    case 'client_event': return 'Client event';
    case 'server_event': return 'Server event';
    case 'notebook': return 'Notebook';
    case 'internal': return 'Internal';
    default: return t || 'Unknown';
  }
}

function firstLine(s?: string) {
  return (s ?? '').trim().split('\n')[0] ?? '';
}

export default function ArtifactsPage() {
  const { artifact } = useParams<{ artifact?: string }>();
  const history = useHistory();
  const { data: all = [], isLoading } = useArtifactCatalog();
  const [q, setQ] = useState('');
  const [kind, setKind] = useState<ArtifactKind>('');
  const [customOnly, setCustomOnly] = useState(false);
  const [editor, setEditor] = useState<null | { name?: string; template?: string }>(null);
  const location = useLocation();
  useEffect(() => {
    if (new URLSearchParams(location.search).get('new') === '1') {
      setEditor({});
      history.replace(location.pathname);
    }
  }, [location.search]); // eslint-disable-line react-hooks/exhaustive-deps
  const searchRef = useRef<HTMLInputElement>(null);

  const list = useMemo(() => {
    const terms = q.toLowerCase().split(/\s+/).filter(Boolean);
    return all
      .filter(a => !kind || (kind === 'client_event' ? /event/.test(a.type ?? '') : (a.type ?? '').toLowerCase() === kind))
      .filter(a => !customOnly || !a.built_in)
      .filter(a => {
        if (!terms.length) return true;
        const hay = `${a.name} ${a.description ?? ''}`.toLowerCase();
        return terms.every(t => hay.includes(t));
      })
      // Name matches first.
      .sort((a, b) => {
        if (!terms.length) return a.name.localeCompare(b.name);
        const an = terms.every(t => a.name.toLowerCase().includes(t)) ? 0 : 1;
        const bn = terms.every(t => b.name.toLowerCase().includes(t)) ? 0 : 1;
        return an - bn || a.name.localeCompare(b.name);
      });
  }, [all, q, kind, customOnly]);

  const counts = useMemo(() => {
    const c: Record<string, number> = { '': all.length };
    for (const a of all) {
      const t = (a.type ?? '').toLowerCase();
      const k = /event/.test(t) ? 'client_event' : t;
      c[k] = (c[k] ?? 0) + 1;
    }
    return c;
  }, [all]);

  const scrollRef = useRef<HTMLDivElement>(null);
  const v = useVirtualizer({ count: list.length, getScrollElement: () => scrollRef.current, estimateSize: () => 58, overscan: 10 });

  const select = (name: string) => history.push(`/artifacts/${name}`);
  const idx = list.findIndex(a => a.name === artifact);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      const typing = t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable;
      if (e.key === '/' && !typing) {
        e.preventDefault();
        searchRef.current?.focus();
        return;
      }
      if (typing && t !== searchRef.current) return;
      if (e.key === 'ArrowDown' || (!typing && e.key === 'j')) {
        e.preventDefault();
        const n = list[Math.min(list.length - 1, idx + 1)];
        if (n) { select(n.name); v.scrollToIndex(Math.min(list.length - 1, idx + 1)); }
      } else if (e.key === 'ArrowUp' || (!typing && e.key === 'k')) {
        e.preventDefault();
        const n = list[Math.max(0, idx - 1)];
        if (n) { select(n.name); v.scrollToIndex(Math.max(0, idx - 1)); }
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  return (
    <div className="tw:flex tw:flex-1 tw:min-h-0">
      <div className="tw:w-[400px] tw:shrink-0 tw:border-r tw:border-line tw:flex tw:flex-col tw:min-h-0 tw:bg-panel">
        <div className="tw:px-4 tw:pt-4 tw:pb-3 tw:space-y-3 tw:border-b tw:border-line">
          <div className="tw:flex tw:items-center tw:gap-2">
            <h2 className="tw:text-[15px] tw:font-semibold tw:flex-1">Artifacts</h2>
            <Button size="sm" variant="primary" icon={<FilePlus2 className="tw:size-3.5" />} onClick={() => setEditor({})}>New</Button>
          </div>
          <div className="tw:relative">
            <Search className="tw:absolute tw:left-2.5 tw:top-1/2 tw:-translate-y-1/2 tw:size-3.5 tw:text-faint" />
            <Input ref={searchRef} value={q} onChange={e => setQ(e.target.value)} placeholder="Search name or description…  /" className="tw:pl-8" />
          </div>
          <Segmented value={kind} onChange={setKind} options={KINDS.map(k => ({ ...k, label: <span title={`${counts[k.value] ?? 0} artifacts`}>{k.label}</span> }))} />
          <div className="tw:flex tw:items-center tw:justify-between tw:text-2xs tw:text-muted">
            <span>{list.length} of {all.length}</span>
            <Switch checked={customOnly} onChange={setCustomOnly} label={<span className="tw:text-xs">Custom only</span>} />
          </div>
        </div>
        <div ref={scrollRef} className="tw:flex-1 tw:overflow-y-auto">
          {isLoading && Array.from({ length: 8 }).map((_, i) => <div key={i} className="tw:p-3 tw:border-b tw:border-line"><Skeleton className="tw:w-2/3" /></div>)}
          {!isLoading && list.length === 0 && <EmptyState icon={<Package />} title="No artifacts match" />}
          <div style={{ height: v.getTotalSize(), position: 'relative' }}>
            {v.getVirtualItems().map(it => {
              const a = list[it.index];
              const on = a.name === artifact;
              return (
                <button key={a.name} type="button" onClick={() => select(a.name)}
                  style={{ position: 'absolute', top: 0, left: 0, right: 0, height: it.size, transform: `translateY(${it.start}px)` }}
                  className={cn('tw:text-left tw:px-4 tw:py-2 tw:border-b tw:border-line tw:overflow-hidden',
                    on ? 'tw:bg-accent-soft' : 'tw:hover:bg-hover')}>
                  {on && <span className="tw:absolute tw:left-0 tw:top-0 tw:bottom-0 tw:w-0.5 tw:bg-accent" />}
                  <div className="tw:flex tw:items-center tw:gap-2">
                    <span className="tw:font-mono tw:text-[12.5px] tw:font-medium tw:truncate tw:flex-1">{a.name}</span>
                    {!a.built_in && <Badge tone="accent">custom</Badge>}
                    <span className="tw:text-2xs tw:text-faint tw:shrink-0">{typeLabel(a.type)}</span>
                  </div>
                  <div className="tw:text-2xs tw:text-muted tw:truncate tw:mt-0.5">{firstLine(a.description)}</div>
                </button>
              );
            })}
          </div>
        </div>
      </div>

      <div className="tw:flex-1 tw:min-w-0 tw:flex tw:flex-col tw:min-h-0">
        {artifact
          ? <ArtifactView key={artifact} name={artifact} onEdit={(name, template) => setEditor({ name, template })} />
          : <EmptyState icon={<BookOpen />} title="Pick an artifact" className="tw:flex-1">
              Artifacts are reusable VQL recipes. Pick one to see what it collects, then run it on a client or as a hunt.
            </EmptyState>}
      </div>

      {editor && (
        <Suspense fallback={null}>
          <ArtifactEditor name={editor.name} template={editor.template} onClose={saved => {
            setEditor(null);
            if (saved) history.push(`/artifacts/${saved}`);
          }} />
        </Suspense>
      )}
    </div>
  );
}

function ArtifactView({ name, onEdit }: { name: string; onEdit: (name?: string, template?: string) => void }) {
  const history = useHistory();
  const { data: defs, isLoading } = useArtifactDefinitions([name]);
  const a: ArtifactDescriptor | undefined = defs?.get(name);
  const canHunt = usePermission('start_hunt');
  const canWrite = usePermission('artifact_writer');
  const { client } = useSelectedClient();
  const [picker, setPicker] = useState(false);
  const [dialog, setDialog] = useState<null | { mode: 'hunt' } | { mode: 'collect'; clientId: string; label: string }>(null);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const set = useSetArtifactFile();

  if (isLoading && !a) return <div className="tw:p-6 tw:space-y-3"><Skeleton className="tw:h-7 tw:w-1/2" /><Skeleton className="tw:h-40" /></div>;
  if (!a) return <EmptyState icon={<Package />} title="Artifact not found">{name}</EmptyState>;

  const type = (a.type ?? '').toLowerCase();
  const collectable = type === 'client' || type === 'server';
  const huntable = type === 'client';
  const req: ArtifactCollectorArgs = { artifacts: [a.name], specs: [] };
  const params = (a.parameters ?? []).filter(p => p.type !== 'hidden');

  const startCollect = () => {
    if (type === 'server') setDialog({ mode: 'collect', clientId: 'server', label: 'the server' });
    else if (client?.client_id && client.client_id !== 'server') setDialog({ mode: 'collect', clientId: client.client_id, label: client.os_info?.hostname || client.client_id });
    else setPicker(true);
  };

  return (
    <div className="tw:flex-1 tw:overflow-auto tw:bg-canvas">
      <div className="tw:bg-panel tw:border-b tw:border-line tw:px-6 tw:py-4">
        <div className="tw:flex tw:items-start tw:gap-3">
          <div className="tw:min-w-0 tw:flex-1">
            <div className="tw:flex tw:items-center tw:gap-2 tw:flex-wrap">
              <h1 className="tw:font-mono tw:text-lg tw:font-semibold tw:break-all">{a.name}</h1>
              <CopyButton text={a.name} />
              <Badge tone="info">{typeLabel(a.type)}</Badge>
              {a.built_in ? <Badge>Built-in</Badge> : <Badge tone="accent">Custom</Badge>}
            </div>
            {a.author && <div className="tw:text-xs tw:text-muted tw:mt-1">by {a.author}</div>}
          </div>
          <div className="tw:flex tw:items-center tw:gap-1.5 tw:shrink-0 tw:flex-wrap tw:justify-end">
            {collectable && (
              <Button variant="primary" icon={<Play className="tw:size-3.5" />} onClick={startCollect}>
                {type === 'server' ? 'Run on server' : client?.client_id && client.client_id !== 'server'
                  ? `Collect from ${client.os_info?.hostname || client.client_id}` : 'Collect…'}
              </Button>
            )}
            {huntable && (
              <Button variant="secondary" icon={<Crosshair className="tw:size-3.5" />} disabled={!canHunt} onClick={() => setDialog({ mode: 'hunt' })}>Hunt</Button>
            )}
            {!a.built_in
              ? <Button variant="secondary" icon={<Pencil className="tw:size-3.5" />} disabled={!canWrite} onClick={() => onEdit(a.name)}>Edit</Button>
              : <Button variant="secondary" icon={<Copy className="tw:size-3.5" />} disabled={!canWrite} onClick={() => onEdit(a.name, 'copy')}>Customize</Button>}
            {!a.built_in && (
              <Button variant="ghost" icon={<Trash2 className="tw:size-3.5" />} disabled={!canWrite} onClick={() => setConfirmDelete(true)}>Delete</Button>
            )}
          </div>
        </div>
      </div>

      <div className="tw:p-6 tw:space-y-4">
        {a.description && (
          <Card>
            <Markdown text={a.description.trim()} className="tw:p-4" />
          </Card>
        )}

        {params.length > 0 && (
          <Card>
            <CardHeader title="Parameters" subtitle={`${params.length} you can set when collecting`} />
            <div className="tw:overflow-x-auto">
              <table className="tw:w-full tw:text-xs">
                <thead>
                  <tr className="tw:text-left tw:text-muted tw:border-b tw:border-line">
                    <th className="tw:px-4 tw:py-2 tw:font-medium">Name</th>
                    <th className="tw:px-4 tw:py-2 tw:font-medium">Type</th>
                    <th className="tw:px-4 tw:py-2 tw:font-medium">Default</th>
                    <th className="tw:px-4 tw:py-2 tw:font-medium">Description</th>
                  </tr>
                </thead>
                <tbody className="tw:divide-y tw:divide-line">
                  {params.map(p => (
                    <tr key={p.name} className="tw:align-top">
                      <td className="tw:px-4 tw:py-2 tw:font-mono tw:font-medium tw:whitespace-nowrap">{p.name}</td>
                      <td className="tw:px-4 tw:py-2 tw:text-muted">{p.type || 'string'}</td>
                      <td className="tw:px-4 tw:py-2 tw:font-mono tw:max-w-[260px] tw:break-all tw:whitespace-pre-wrap">{p.default ? (p.default.length > 160 ? `${p.default.slice(0, 160)}…` : p.default) : <span className="tw:text-faint">—</span>}</td>
                      <td className="tw:px-4 tw:py-2 tw:text-muted">{p.description}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </Card>
        )}

        {(a.sources ?? []).map((s, i) => (
          <Card key={i}>
            <CardHeader icon={<Code2 className="tw:size-4" />} title={s.name ? `Source: ${s.name}` : 'Query'}
              subtitle={s.description ? firstLine(s.description) : undefined}
              actions={s.query ? <CopyButton text={s.query} /> : undefined} />
            {(s.precondition || (i === 0 && a.precondition)) && (
              <div className="tw:px-4 tw:pt-3 tw:text-2xs tw:text-muted">Runs only when <Mono className="tw:text-[11px]">{(s.precondition || a.precondition)?.trim()}</Mono></div>
            )}
            <div className="tw:p-3">
              <Suspense fallback={<pre className="tw:text-xs tw:font-mono tw:whitespace-pre-wrap">{s.query}</pre>}>
                <CodeEditor value={(s.query ?? '').trim()} mode="vql" readOnly maxLines={40} minLines={3} />
              </Suspense>
            </div>
          </Card>
        ))}

        {((a.tools?.length ?? 0) > 0 || (a.references?.length ?? 0) > 0) && (
          <div className="tw:grid tw:grid-cols-1 tw:md:grid-cols-2 tw:gap-4">
            {(a.tools?.length ?? 0) > 0 && (
              <Card>
                <CardHeader icon={<Wrench className="tw:size-4" />} title="Tools" subtitle="External binaries this artifact uses" />
                <div className="tw:divide-y tw:divide-line">
                  {a.tools!.map(t => (
                    <div key={t.name} className="tw:px-4 tw:py-2 tw:text-xs">
                      <div className="tw:font-mono tw:font-medium">{t.name}</div>
                      {t.url && <a href={t.url} target="_blank" rel="noreferrer" className="tw:text-2xs tw:text-accent tw:hover:underline tw:break-all">{t.url}</a>}
                    </div>
                  ))}
                </div>
              </Card>
            )}
            {(a.references?.length ?? 0) > 0 && (
              <Card>
                <CardHeader icon={<Link2 className="tw:size-4" />} title="References" />
                <div className="tw:p-4 tw:space-y-1.5">
                  {a.references!.map(r => (
                    <a key={r} href={r} target="_blank" rel="noreferrer" className="tw:flex tw:items-center tw:gap-1.5 tw:text-xs tw:text-accent tw:hover:underline tw:break-all">
                      <ExternalLink className="tw:size-3 tw:shrink-0" />{r}
                    </a>
                  ))}
                </div>
              </Card>
            )}
          </div>
        )}
      </div>

      {picker && (
        <Suspense fallback={null}>
          <ClientPicker open={picker} onOpenChange={setPicker} title={`Collect ${a.name} from…`}
            onPick={(id, label) => { setPicker(false); setDialog({ mode: 'collect', clientId: id, label }); }} />
        </Suspense>
      )}
      {dialog && (
        <Suspense fallback={null}>
          <NewHuntDialog open onOpenChange={o => !o && setDialog(null)} baseRequest={req}
            target={dialog.mode === 'collect' ? { clientId: dialog.clientId, label: dialog.label } : undefined}
            onCreated={id => {
              const d = dialog;
              setDialog(null);
              if (d.mode === 'collect' && id) history.push(`/collected/${d.clientId}/${id}`);
              else if (d.mode === 'hunt' && id) history.push(`/hunts/${id}`);
            }} />
        </Suspense>
      )}
      <ConfirmDialog open={confirmDelete} onOpenChange={setConfirmDelete} danger title={`Delete ${a.name}?`}
        confirmLabel="Delete artifact" loading={set.isPending}
        onConfirm={() => set.mutate({ artifact: `name: ${a.name}`, op: 'DELETE' }, {
          onSuccess: () => { toast.success('Artifact deleted'); setConfirmDelete(false); history.push('/artifacts'); },
          onError: e => toast.error((e as Error).message),
        })}>
        This removes the custom artifact from the server. Past collections that used it are kept.
      </ConfirmDialog>
    </div>
  );
}


