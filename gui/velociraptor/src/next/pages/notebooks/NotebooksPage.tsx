// Notebooks: shared, re-runnable investigation write-ups mixing Markdown
// notes with live VQL. A searchable list, then a focused single-column
// document with an outline on the side.
import { useEffect, useMemo, useRef, useState } from 'react';
import { useHistory, useLocation, useParams } from 'react-router-dom';
import { toast } from 'sonner';
import {
  ArrowLeft, BookOpen, Code2, Download, FileArchive, FileText, Globe, Lock, MoreHorizontal, Pencil, Plus,
  Trash2, Type, Users as UsersIcon, ExternalLink, Lightbulb,
} from 'lucide-react';
import {
  useDeleteNotebook, useGlobalUsers, useNewNotebook, useNewNotebookCell, useNotebook, useNotebookList,
  useUpdateNotebook, type Notebook,
} from '../../lib/queries';
import * as api from '../../lib/api';
import { fmtTime, timeAgo } from '../../lib/format';
import { setItem, schema } from '../../../components/core/storage.jsx';
import type { Row } from '../../lib/table';
import {
  Badge, Button, Card, ConfirmDialog, Dialog, EmptyState, Field, Input, Menu, MenuContent, MenuItem, MenuLabel,
  MenuSeparator, MenuTrigger, SearchInput, Skeleton, Switch, Textarea, Tooltip, cn,
} from '../../ui/primitives';
import { TagInput } from '../hunts/TagInput';
import { ArchiveList, useArchiveReadyToast } from '../evidence/ArchiveList';
import NotebookCellView, { CELL_TYPES } from './NotebookCell';

export default function NotebooksPage() {
  const { notebook_id: id } = useParams<{ notebook_id?: string }>();
  if (id && id !== 'new') return <NotebookView key={id} id={id} />;
  return <NotebookList />;
}

// ---------------------------------------------------------------- List

function NotebookList() {
  const history = useHistory();
  const location = useLocation();
  const { data: rows, isLoading } = useNotebookList();
  const [filter, setFilter] = useState('');
  const [creating, setCreating] = useState(false);

  useEffect(() => {
    if (new URLSearchParams(location.search).get('new') === '1') {
      setCreating(true);
      history.replace('/notebooks');
    }
  }, [location.search]); // eslint-disable-line react-hooks/exhaustive-deps

  const shown = useMemo(() => {
    const f = filter.toLowerCase();
    return [...(rows ?? [])]
      .filter(r => !f || `${r.Name} ${r.Description} ${r.Creator} ${r.NotebookId}`.toLowerCase().includes(f))
      .sort((a, b) => String(b['Modified Time']).localeCompare(String(a['Modified Time'])));
  }, [rows, filter]);

  const open = (r: Row) => history.push(`/notebooks/${r.NotebookId}`);

  return (
    <div className="tw:flex-1 tw:overflow-y-auto">
      <div className="tw:max-w-5xl tw:mx-auto tw:p-6 tw:flex tw:flex-col tw:gap-4">
        <header className="tw:flex tw:items-center tw:gap-3">
          <div className="tw:flex-1">
            <h1 className="tw:text-xl tw:font-semibold">Notebooks</h1>
            <p className="tw:text-[13px] tw:text-muted">Write up an investigation with notes and live VQL, and share it with your team.</p>
          </div>
          <Button variant="primary" icon={<Plus />} onClick={() => setCreating(true)}>New notebook</Button>
        </header>
        <SearchInput value={filter} onChange={setFilter} placeholder="Search notebooks" className="tw:max-w-md" />
        {isLoading && <div className="tw:grid tw:grid-cols-1 tw:md:grid-cols-2 tw:gap-3">{Array.from({ length: 4 }, (_, i) => <Skeleton key={i} className="tw:h-28" />)}</div>}
        {!isLoading && shown.length === 0 && (
          <Card className="tw:p-10">
            <EmptyState icon={<BookOpen />} title={filter ? 'No notebooks match' : 'No notebooks yet'}
              action={!filter && <Button icon={<Plus />} onClick={() => setCreating(true)}>Create the first one</Button>}>
              {filter ? `Nothing matched “${filter}”.` : 'Hunt and collection notebooks live with their hunt or collection. Global notebooks live here.'}
            </EmptyState>
          </Card>
        )}
        <div className="tw:grid tw:grid-cols-1 tw:md:grid-cols-2 tw:gap-3">
          {shown.map(r => {
            const collab = (r.Collaborators as string[] | null) ?? [];
            return (
              <button key={String(r.NotebookId)} onClick={() => open(r)}
                className="tw:text-left tw:rounded-xl tw:border tw:border-line tw:bg-panel tw:p-4 tw:hover:border-accent/60 tw:hover:shadow-sm tw:transition tw:flex tw:flex-col tw:gap-2">
                <div className="tw:flex tw:items-start tw:gap-3">
                  <div className="tw:size-9 tw:rounded-lg tw:bg-accent-soft tw:text-accent tw:flex tw:items-center tw:justify-center tw:shrink-0">
                    <BookOpen className="tw:size-4" />
                  </div>
                  <div className="tw:flex-1 tw:min-w-0">
                    <div className="tw:font-semibold tw:text-[14px] tw:truncate">{String(r.Name || r.NotebookId)}</div>
                    <div className="tw:text-xs tw:text-muted tw:line-clamp-2">{String(r.Description || '') || <span className="tw:text-faint">No description</span>}</div>
                  </div>
                </div>
                <div className="tw:flex tw:items-center tw:gap-3 tw:text-2xs tw:text-faint tw:mt-auto">
                  <span>by {String(r.Creator)}</span>
                  <Tooltip content={String(r['Modified Time'])}><span>edited {timeAgo(new Date(String(r['Modified Time'])))}</span></Tooltip>
                  {collab.length > 0 && <span className="tw:flex tw:items-center tw:gap-1"><UsersIcon className="tw:size-3" />{collab.length}</span>}
                  <span className="tw:ml-auto tw:font-mono">{String(r.NotebookId)}</span>
                </div>
              </button>
            );
          })}
        </div>
      </div>
      {creating && <NotebookSettingsDialog onClose={() => setCreating(false)}
        onSaved={nb => history.push(`/notebooks/${nb.notebook_id}`)} />}
    </div>
  );
}

// ---------------------------------------------------------- Settings

function NotebookSettingsDialog({ notebook, onClose, onSaved }: {
  notebook?: Notebook; onClose: () => void; onSaved?: (nb: Notebook) => void;
}) {
  const create = useNewNotebook();
  const update = useUpdateNotebook();
  const { data: users } = useGlobalUsers();
  const [name, setName] = useState(notebook?.name ?? '');
  const [description, setDescription] = useState(notebook?.description ?? '');
  const [collaborators, setCollaborators] = useState<string[]>(notebook?.collaborators ?? []);
  const [pub, setPub] = useState(!!notebook?.public);

  const submit = () => {
    if (!name.trim()) return;
    const body = { name: name.trim(), description, collaborators, public: pub };
    const opts = {
      onSuccess: (nb: Notebook) => { toast.success(notebook ? 'Notebook updated' : 'Notebook created'); onClose(); onSaved?.(nb); },
      onError: (e: Error) => toast.error(e.message),
    };
    if (notebook) update.mutate({ ...notebook, ...body }, opts);
    else create.mutate(body, opts);
  };

  return (
    <Dialog open onOpenChange={o => !o && onClose()} width={520} title={notebook ? 'Notebook settings' : 'New notebook'}
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancel</Button>
        <Button variant="primary" disabled={!name.trim()} loading={create.isPending || update.isPending} onClick={submit}>
          {notebook ? 'Save' : 'Create notebook'}
        </Button>
      </>}>
      <div className="tw:flex tw:flex-col tw:gap-4">
        <Field label="Name" required>
          <Input value={name} onChange={e => setName(e.target.value)} autoFocus placeholder="e.g. Lateral movement on FIN-SRV-02"
            onKeyDown={e => e.key === 'Enter' && submit()} />
        </Field>
        <Field label="Description">
          <Textarea rows={3} value={description} onChange={e => setDescription(e.target.value)} placeholder="What is this investigation about?" />
        </Field>
        <Field label="Collaborators" hint="They can see and edit this notebook.">
          <TagInput value={collaborators} onChange={setCollaborators} suggestions={(users ?? []).map(u => u.name)} placeholder="Add a username" tone="accent" />
        </Field>
        <Switch checked={pub} onChange={setPub} label="Visible to everyone in this organization" />
      </div>
    </Dialog>
  );
}

// ---------------------------------------------------------------- View

// Hunt and collection notebooks embed the same view; `create` makes the
// notebook on first open, like the classic UI did.
export function NotebookView({ id, embedded, create }: {
  id: string; embedded?: boolean; create?: { name: string; description?: string };
}) {
  const history = useHistory();
  const { data: nb, isLoading, error, isFetched } = useNotebook(id, { uploads: true });
  const createNb = useNewNotebook();
  const triedCreate = useRef(false);
  useEffect(() => {
    if (!create || triedCreate.current || nb || (!isFetched && !error)) return;
    triedCreate.current = true;
    createNb.mutate({ notebook_id: id, name: create.name, description: create.description, public: true });
  }, [create, isFetched, error, nb, id]); // eslint-disable-line react-hooks/exhaustive-deps
  const update = useUpdateNotebook();
  const del = useDeleteNotebook();
  const addCell = useNewNotebookCell();
  const [selected, setSelected] = useState<string | null>(null);
  const [settings, setSettings] = useState(false);
  const [exportOpen, setExportOpen] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const autoDl = useRef<Set<string>>(new Set());

  useEffect(() => {
    if (embedded) return;
    try { setItem(schema.CurrentNotebookIdKey, id); } catch { /* ignore */ }
  }, [id, embedded]);

  const files = nb?.available_downloads?.files ?? [];
  useArchiveReadyToast(files, autoDl);

  const cells = nb?.cell_metadata ?? [];

  const insert = (index: number, where: 'above' | 'below' | 'end', type: string, input = '', env?: { key: string; value: string }[]) => {
    if (!nb) return;
    const before = where === 'above' ? cells[index]?.cell_id : where === 'below' ? cells[index + 1]?.cell_id : undefined;
    addCell.mutate({ notebook_id: nb.notebook_id, cell_id: before ?? '', type, input, env }, {
      onSuccess: r => {
        const newId = (r as Notebook & { latest_cell_id?: string }).latest_cell_id;
        if (newId) {
          setSelected(newId);
          setTimeout(() => document.getElementById(`cell-${newId}`)?.scrollIntoView({ behavior: 'smooth', block: 'center' }), 300);
        }
      },
      onError: e => toast.error((e as Error).message),
    });
  };

  const move = (i: number, dir: -1 | 1) => {
    if (!nb) return;
    const next = [...cells];
    const j = i + dir;
    if (j < 0 || j >= next.length) return;
    [next[i], next[j]] = [next[j], next[i]];
    update.mutate({ ...nb, cell_metadata: next });
  };

  const remove = (i: number) => {
    if (!nb || cells.length <= 1) return;
    update.mutate({ ...nb, cell_metadata: cells.filter((_, k) => k !== i) }, {
      onSuccess: () => toast.success('Cell deleted', { description: 'Undo from the cell history is not possible once deleted.' }),
    });
  };

  const exportAs = async (type: 'html' | 'zip') => {
    try {
      await api.post('v1/CreateNotebookDownloadFile', { notebook_id: id, type });
      autoDl.current.add('*');
      setExportOpen(true);
      toast.loading('Preparing export…', { id: 'nb-export', duration: 4000 });
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  if (error && !embedded) {
    return <EmptyState className="tw:mt-24" icon={<BookOpen />} title="Notebook not found"
      action={<Button icon={<ArrowLeft />} onClick={() => history.push('/notebooks')}>All notebooks</Button>}>
      {(error as Error).message}
    </EmptyState>;
  }

  return (
    <div className="tw:flex tw:flex-1 tw:min-h-0 tw:min-w-0">
      {/* Outline */}
      {!embedded && <aside className="tw:hidden tw:xl:flex tw:w-60 tw:shrink-0 tw:flex-col tw:border-r tw:border-line tw:bg-panel">
        <button onClick={() => history.push('/notebooks')}
          className="tw:flex tw:items-center tw:gap-1.5 tw:px-4 tw:h-11 tw:text-xs tw:text-muted tw:hover:text-fg tw:border-b tw:border-line">
          <ArrowLeft className="tw:size-3.5" /> All notebooks
        </button>
        <div className="tw:px-4 tw:pt-3 tw:pb-1 tw:text-2xs tw:font-semibold tw:uppercase tw:tracking-wide tw:text-faint">Outline</div>
        <nav className="tw:flex-1 tw:overflow-y-auto tw:pb-3">
          {cells.map((c, i) => {
            const t = (c.type ?? 'vql').toLowerCase();
            return (
              <button key={c.cell_id} onClick={() => {
                setSelected(c.cell_id);
                document.getElementById(`cell-${c.cell_id}`)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
              }}
                className={cn('tw:w-full tw:flex tw:items-center tw:gap-2 tw:px-4 tw:py-1.5 tw:text-left tw:text-xs tw:hover:bg-hover',
                  selected === c.cell_id ? 'tw:text-accent tw:font-medium' : 'tw:text-muted')}>
                {t === 'markdown' ? <Type className="tw:size-3 tw:shrink-0" /> : <Code2 className="tw:size-3 tw:shrink-0" />}
                <span className="tw:truncate">{c.summary || (t === 'markdown' ? 'Note' : `Query ${i + 1}`)}</span>
              </button>
            );
          })}
        </nav>
      </aside>}

      <div className="tw:flex-1 tw:min-w-0 tw:overflow-y-auto" onClick={() => setSelected(null)}>
        <div className="tw:max-w-5xl tw:mx-auto tw:px-6 tw:py-5 tw:flex tw:flex-col tw:gap-3">
          {/* Header */}
          {embedded && nb && (
            <div className="tw:flex tw:items-center tw:gap-2" onClick={e => e.stopPropagation()}>
              <span className="tw:text-xs tw:text-muted tw:flex-1">
                Post-process results with VQL and keep notes. {cells.length} cell{cells.length === 1 ? '' : 's'}.
              </span>
              {(nb.suggestions?.length ?? 0) > 0 && (
                <Menu>
                  <MenuTrigger asChild><Button size="sm" variant="ghost" icon={<Lightbulb />}>Suggestions</Button></MenuTrigger>
                  <MenuContent>
                    {nb.suggestions!.map((s, i) => (
                      <MenuItem key={i} icon={<Lightbulb />} onSelect={() => insert(cells.length - 1, 'end', s.type ?? 'vql', s.input ?? '', s.env)}>
                        {s.name || `Suggestion ${i + 1}`}
                      </MenuItem>
                    ))}
                  </MenuContent>
                </Menu>
              )}
              <Menu>
                <MenuTrigger asChild><Button size="sm" icon={<Download />}>Export notebook</Button></MenuTrigger>
                <MenuContent>
                  <MenuItem icon={<FileText />} onSelect={() => exportAs('html')}>As an HTML report</MenuItem>
                  <MenuItem icon={<FileArchive />} onSelect={() => exportAs('zip')}>As a zip with all data</MenuItem>
                  {files.length > 0 && <MenuItem icon={<Download />} onSelect={() => setExportOpen(true)}>Previous exports ({files.length})</MenuItem>}
                </MenuContent>
              </Menu>
            </div>
          )}
          {embedded ? (!nb && <Skeleton className="tw:h-40" />) : isLoading || !nb ? <Skeleton className="tw:h-20" /> : (
            <header className="tw:flex tw:items-start tw:gap-3 tw:mb-2" onClick={e => e.stopPropagation()}>
              <div className="tw:flex-1 tw:min-w-0">
                <div className="tw:flex tw:items-center tw:gap-2">
                  <h1 className="tw:text-xl tw:font-semibold tw:truncate">{nb.name || nb.notebook_id}</h1>
                  {nb.public ? <Badge tone="info"><Globe className="tw:size-3" />Public</Badge> : <Badge tone="neutral"><Lock className="tw:size-3" />Private</Badge>}
                </div>
                {nb.description && <div className="tw:text-[13px] tw:text-muted tw:mt-0.5">{nb.description}</div>}
                <div className="tw:flex tw:flex-wrap tw:items-center tw:gap-x-4 tw:gap-y-1 tw:text-2xs tw:text-faint tw:mt-1.5">
                  <span>Created by {nb.creator}</span>
                  <Tooltip content={fmtTime(nb.modified_time ?? 0)}><span>Edited {timeAgo(nb.modified_time ?? 0)}</span></Tooltip>
                  {(nb.collaborators?.length ?? 0) > 0 && (
                    <span className="tw:flex tw:items-center tw:gap-1"><UsersIcon className="tw:size-3" />{nb.collaborators!.join(', ')}</span>
                  )}
                  <span className="tw:font-mono">{nb.notebook_id}</span>
                </div>
              </div>
              <Button size="sm" icon={<Pencil />} onClick={() => setSettings(true)}>Settings</Button>
              <Menu>
                <MenuTrigger asChild>
                  <Button size="sm" icon={<Download />}>Export</Button>
                </MenuTrigger>
                <MenuContent>
                  <MenuItem icon={<FileText />} onSelect={() => exportAs('html')}>As an HTML report</MenuItem>
                  <MenuItem icon={<FileArchive />} onSelect={() => exportAs('zip')}>As a zip with all data</MenuItem>
                  {files.length > 0 && <>
                    <MenuSeparator />
                    <MenuItem icon={<Download />} onSelect={() => setExportOpen(true)}>Previous exports ({files.length})</MenuItem>
                  </>}
                </MenuContent>
              </Menu>
              <Menu>
                <MenuTrigger asChild>
                  <Button size="sm" variant="ghost" square icon={<MoreHorizontal />} aria-label="More" />
                </MenuTrigger>
                <MenuContent>
                  {(nb.suggestions?.length ?? 0) > 0 && <>
                    <MenuLabel>Add a suggested cell</MenuLabel>
                    {nb.suggestions!.map((s, i) => (
                      <MenuItem key={i} icon={<Lightbulb />} onSelect={() => insert(cells.length - 1, 'end', s.type ?? 'vql', s.input ?? '', s.env)}>
                        {s.name || `Suggestion ${i + 1}`}
                      </MenuItem>
                    ))}
                    <MenuSeparator />
                  </>}
                  <MenuItem icon={<ExternalLink />} onSelect={() => history.push(`/classic/notebooks/${id}`)}>Open in classic view</MenuItem>
                  <MenuItem danger icon={<Trash2 />} onSelect={() => setConfirmDelete(true)}>Delete notebook</MenuItem>
                </MenuContent>
              </Menu>
            </header>
          )}

          {nb && cells.map((c, i) => (
            <div key={c.cell_id} onClick={e => e.stopPropagation()}>
              <NotebookCellView notebook={nb} meta={c} index={i} count={cells.length}
                selected={selected === c.cell_id} onSelect={() => setSelected(c.cell_id)}
                onMove={dir => move(i, dir)} onDelete={() => remove(i)}
                onInsert={(where, type) => insert(i, where, type)} />
              <InsertBar onInsert={type => insert(i, 'below', type)} />
            </div>
          ))}

          {nb && cells.length === 0 && (
            <Card className="tw:p-8"><EmptyState icon={<BookOpen />} title="This notebook is empty">Add a cell to start.</EmptyState></Card>
          )}
          {nb && (
            <div className="tw:flex tw:justify-center tw:gap-2 tw:py-4" onClick={e => e.stopPropagation()}>
              {CELL_TYPES.slice(0, 2).map(t => (
                <Button key={t.value} variant="secondary" icon={t.icon} loading={addCell.isPending}
                  onClick={() => insert(cells.length - 1, 'end', t.value)}>Add {t.label}</Button>
              ))}
            </div>
          )}
        </div>
      </div>

      {settings && nb && <NotebookSettingsDialog notebook={nb} onClose={() => setSettings(false)} />}
      <Dialog open={exportOpen} onOpenChange={setExportOpen} title="Exports" width={640}
        description="Exports are kept with the notebook. HTML is a readable report; the zip contains every table as data.">
        <ArchiveList files={files} empty={<div className="tw:text-xs tw:text-faint tw:p-4">No exports yet.</div>} />
      </Dialog>
      <ConfirmDialog open={confirmDelete} onOpenChange={setConfirmDelete} danger title="Delete this notebook?"
        confirmLabel="Delete notebook" loading={del.isPending}
        onConfirm={() => nb && del.mutate(nb, {
          onSuccess: () => { toast.success('Notebook deleted'); history.push('/notebooks'); },
          onError: e => toast.error((e as Error).message),
        })}>
        {nb?.name} and all its cells will be removed.
      </ConfirmDialog>
    </div>
  );
}

function InsertBar({ onInsert }: { onInsert: (type: string) => void }) {
  return (
    <div className="tw:group/ins tw:relative tw:h-4 tw:flex tw:items-center tw:justify-center">
      <div className="tw:absolute tw:inset-x-6 tw:h-px tw:bg-accent tw:opacity-0 tw:group-hover/ins:opacity-40 tw:transition-opacity" />
      <div className="tw:relative tw:flex tw:gap-1 tw:opacity-0 tw:group-hover/ins:opacity-100 tw:transition-opacity">
        {CELL_TYPES.slice(0, 2).map(t => (
          <button key={t.value} onClick={() => onInsert(t.value)}
            className="tw:flex tw:items-center tw:gap-1 tw:h-5 tw:px-2 tw:rounded-full tw:bg-panel tw:border tw:border-line tw:text-2xs tw:text-muted tw:hover:text-accent tw:hover:border-accent">
            <Plus className="tw:size-3" />{t.label}
          </button>
        ))}
      </div>
    </div>
  );
}

