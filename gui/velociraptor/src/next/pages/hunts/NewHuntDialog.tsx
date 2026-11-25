// One-page "new hunt" form: pick artifacts on the left, configure
// parameters, targeting and limits on the right, with a live estimate
// of how many clients the hunt will reach.
import React, { useEffect, useMemo, useState } from 'react';
import { toast } from 'sonner';
import { Dialog as RDialog } from 'radix-ui';
import {
  Check, ChevronDown, ChevronRight, Package, Plus, RotateCcw, Search, Target, Users, X, Settings2, Play, Pause,
  Monitor, Server,
} from 'lucide-react';
import {
  useArtifactDefinitions, useArtifactSearch, useCreateHunt, useEstimateHunt, useHuntTags, useTraits,
  useCollectArtifact, type CreateHuntRequest,
} from '../../lib/queries';
import type { ArtifactCollectorArgs, ArtifactDescriptor, ArtifactParameterDef, Hunt, HuntCondition } from '../../lib/types';
import { fmtNum, toNum } from '../../lib/format';
import {
  Badge, Button, cn, Field, Input, Kbd, Segmented, Select, Spinner, Switch, Textarea, Tooltip,
} from '../../ui/primitives';
import { TagInput } from './TagInput';
import { fromLocalInput, toLocalInput } from './time';

type Params = Record<string, Record<string, string>>;
type TargetMode = 'all' | 'labels' | 'os';

function useDebounced<T>(v: T, ms: number): T {
  const [d, setD] = useState(v);
  useEffect(() => {
    const t = setTimeout(() => setD(v), ms);
    return () => clearTimeout(t);
  }, [v, ms]);
  return d;
}

function firstLine(s: string | undefined): string {
  return (s ?? '').trim().split('\n')[0] ?? '';
}

// With `target` set the same form collects from a single client (or the
// server) instead of creating a hunt.
export default function NewHuntDialog({ open, onOpenChange, onCreated, baseHunt, target, baseRequest }: {
  open: boolean; onOpenChange: (v: boolean) => void; onCreated: (id?: string) => void; baseHunt?: Hunt;
  target?: { clientId: string; label?: string }; baseRequest?: ArtifactCollectorArgs;
}) {
  const isCollection = !!target;
  const isServer = target?.clientId === 'server';
  const { traits } = useTraits();
  const expiryHours = toNum(traits?.customizations?.hunt_expiry_hours) || 24 * 7;

  // -------------------------------------------------------- State
  const base = baseHunt?.start_request ?? baseRequest;
  const [selected, setSelected] = useState<string[]>(base?.artifacts ?? baseHunt?.artifacts ?? []);
  const [params, setParams] = useState<Params>(() => {
    const p: Params = {};
    for (const spec of base?.specs ?? []) {
      p[spec.artifact] = {};
      for (const e of spec.parameters?.env ?? []) p[spec.artifact][e.key] = e.value;
    }
    return p;
  });
  const [description, setDescription] = useState(baseHunt?.hunt_description ? `${baseHunt.hunt_description}` : '');
  const [tags, setTags] = useState<string[]>(baseHunt?.tags ?? []);
  const [expires, setExpires] = useState(() => toLocalInput(new Date(Date.now() + expiryHours * 3600 * 1000)));
  const cond = baseHunt?.condition;
  const [mode, setMode] = useState<TargetMode>(
    cond?.labels?.label?.length ? 'labels' : cond?.os?.os ? 'os' : 'all');
  const [includeLabels, setIncludeLabels] = useState<string[]>(cond?.labels?.label ?? []);
  const [excludeLabels, setExcludeLabels] = useState<string[]>(cond?.excluded_labels?.label ?? []);
  const [os, setOs] = useState(cond?.os?.os ?? 'WINDOWS');
  const [lastActive, setLastActive] = useState('all');
  const [timeout, setTimeoutS] = useState(base?.timeout ? String(toNum(base.timeout)) : '');
  const [maxRows, setMaxRows] = useState(base?.max_rows ? String(toNum(base.max_rows)) : '');
  const [maxMb, setMaxMb] = useState(base?.max_upload_bytes ? String(Math.round(toNum(base.max_upload_bytes) / 1048576)) : '');
  const [cpuLimit, setCpuLimit] = useState(base?.cpu_limit ? String(base.cpu_limit) : '');
  const [urgent, setUrgent] = useState(!!base?.urgent);
  const [orgIds, setOrgIds] = useState<string[]>(baseHunt?.org_ids ?? []);
  const [startNow, setStartNow] = useState(false);
  const [showAdvanced, setShowAdvanced] = useState(isCollection);
  const [expanded, setExpanded] = useState<string | null>(null);

  // ------------------------------------------------------ Queries
  const [search, setSearch] = useState('');
  const dsearch = useDebounced(search.trim(), 150);
  const { data: matches = [], isFetching: searching } = useArtifactSearch(dsearch, isServer ? 'SERVER' : 'CLIENT', open);
  const { data: defs } = useArtifactDefinitions(selected);
  const { data: allTags = [] } = useHuntTags();
  const create = useCreateHunt();
  const collect = useCollectArtifact();

  const condition: HuntCondition = useMemo(() => {
    const c: HuntCondition = {};
    if (mode === 'labels' && includeLabels.length) c.labels = { label: includeLabels };
    if (mode === 'os') c.os = { os };
    if (excludeLabels.length) c.excluded_labels = { label: excludeLabels };
    return c;
  }, [mode, includeLabels, excludeLabels, os]);
  const estimate = useEstimateHunt(condition, lastActive, open && !isCollection);

  // Expand the first artifact with parameters by default.
  useEffect(() => {
    if (!expanded && selected.length) setExpanded(selected[0]);
  }, [selected, expanded]);

  const toggle = (name: string) => {
    setSelected(s => {
      if (s.includes(name)) return s.filter(x => x !== name);
      setExpanded(name);
      return [...s, name];
    });
  };

  const setParam = (artifact: string, key: string, value: string) => {
    setParams(p => ({ ...p, [artifact]: { ...(p[artifact] ?? {}), [key]: value } }));
  };

  const resetParams = (artifact: string) => {
    setParams(p => {
      const n = { ...p };
      delete n[artifact];
      return n;
    });
  };

  const autoDescription = selected.length ? selected.join(', ') : '';

  const buildRequest = (): CreateHuntRequest => {
    const specs = selected.map(name => {
      const def = defs?.get(name);
      const env = Object.entries(params[name] ?? {})
        .filter(([k, v]) => {
          const pd = def?.parameters?.find(p => p.name === k);
          return !pd || (pd.default ?? '') !== v;
        })
        .map(([key, value]) => ({ key, value }));
      return { artifact: name, parameters: { env } };
    });
    const req: CreateHuntRequest = {
      hunt_description: description.trim() || autoDescription,
      tags: tags.length ? tags : undefined,
      start_request: { artifacts: selected, specs },
      condition,
    };
    const exp = fromLocalInput(expires);
    if (exp) req.expires = exp.getTime() * 1000;
    if (toNum(timeout)) req.start_request.timeout = toNum(timeout);
    if (toNum(maxRows)) req.start_request.max_rows = toNum(maxRows);
    if (toNum(maxMb)) req.start_request.max_upload_bytes = toNum(maxMb) * 1024 * 1024;
    if (toNum(cpuLimit)) req.start_request.cpu_limit = toNum(cpuLimit);
    if (urgent) req.start_request.urgent = true;
    if (orgIds.length) req.org_ids = orgIds;
    if (startNow) req.state = 2;
    return req;
  };

  const expDate = fromLocalInput(expires);
  const expInPast = !!expDate && expDate.getTime() < Date.now();
  const missingLabels = mode === 'labels' && includeLabels.length === 0;
  const canSubmit = isCollection ? selected.length > 0 : selected.length > 0 && !expInPast && !missingLabels;

  const submit = () => {
    if (!canSubmit) return;
    if (target) {
      const { start_request } = buildRequest();
      collect.mutate({ ...start_request, client_id: target.clientId }, {
        onSuccess: r => {
          toast.success(`Collecting from ${target.label || target.clientId}`);
          onCreated(r?.flow_id);
        },
        onError: e => toast.error((e as Error).message),
      });
      return;
    }
    create.mutate(buildRequest(), {
      onSuccess: r => {
        toast.success(startNow ? 'Hunt created and started' : 'Hunt created (paused). Start it when ready.');
        onCreated(r?.flow_id);
      },
      onError: e => toast.error((e as Error).message),
    });
  };

  const orgs = traits?.orgs ?? [];
  const isOrgAdmin = !!traits?.Permissions?.org_admin;

  return (
    <RDialog.Root open={open} onOpenChange={onOpenChange}>
      <RDialog.Portal>
        <RDialog.Overlay className="rx tw:fixed tw:inset-0 tw:z-[1500] tw:bg-black/40 tw:animate-fade-in" />
        <RDialog.Content aria-describedby={undefined}
          onKeyDown={e => {
            if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
              e.preventDefault();
              submit();
            }
          }}
          className="rx tw:fixed tw:left-1/2 tw:top-1/2 tw:z-[1501] tw:-translate-x-1/2 tw:-translate-y-1/2 tw:w-[1180px] tw:max-w-[calc(100vw-32px)] tw:h-[min(860px,calc(100vh-48px))] tw:flex tw:flex-col tw:bg-panel tw:rounded-xl tw:border tw:border-line rx-shadow-lg tw:animate-pop-in tw:outline-none">
          {/* Header */}
          <div className="tw:flex tw:items-center tw:gap-3 tw:px-5 tw:h-14 tw:border-b tw:border-line tw:shrink-0">
            <span className="tw:flex tw:size-8 tw:items-center tw:justify-center tw:rounded-lg tw:bg-accent-soft tw:text-accent">
              {isServer ? <Server className="tw:size-4" /> : isCollection ? <Monitor className="tw:size-4" /> : <Target className="tw:size-4" />}
            </span>
            <div className="tw:flex-1">
              <RDialog.Title className="tw:text-[15px] tw:font-semibold">
                {isCollection
                  ? <>{baseRequest ? 'Collect again' : 'New collection'} on <span className="tw:text-accent">{isServer ? 'the server' : target?.label || target?.clientId}</span></>
                  : baseHunt ? 'Copy hunt' : 'New hunt'}
              </RDialog.Title>
              <div className="tw:text-2xs tw:text-faint">
                {isServer ? 'Run server artifacts' : isCollection ? 'Collect artifacts and files from this one client' : 'Collect artifacts from many clients at once'}
              </div>
            </div>
            <RDialog.Close className="tw:p-1.5 tw:rounded tw:text-faint tw:hover:bg-hover tw:hover:text-fg" aria-label="Close">
              <X className="tw:size-4" />
            </RDialog.Close>
          </div>

          <div className="tw:flex tw:flex-1 tw:min-h-0">
            {/* ---------------------------------------- Artifact picker */}
            <div className="tw:w-[380px] tw:shrink-0 tw:border-r tw:border-line tw:flex tw:flex-col tw:min-h-0 tw:bg-canvas/50">
              <div className="tw:p-3 tw:border-b tw:border-line">
                <div className="tw:relative">
                  <Search className="tw:absolute tw:left-2.5 tw:top-1/2 tw:-translate-y-1/2 tw:size-3.5 tw:text-faint" />
                  <Input autoFocus value={search} onChange={e => setSearch(e.target.value)}
                    placeholder={isServer ? 'Search server artifacts…' : 'Search client artifacts…'} className="tw:pl-8" />
                  {searching && <Spinner className="tw:absolute tw:right-2.5 tw:top-1/2 tw:-translate-y-1/2 tw:size-3.5" />}
                </div>
              </div>
              <div className="tw:flex-1 tw:overflow-y-auto">
                {matches.length === 0 && !searching && (
                  <div className="tw:p-6 tw:text-center tw:text-xs tw:text-faint">No artifacts match “{search}”</div>
                )}
                {matches.map(a => {
                  const on = selected.includes(a.name);
                  return (
                    <button key={a.name} type="button" onClick={() => toggle(a.name)}
                      className={cn('tw:flex tw:w-full tw:items-start tw:gap-2.5 tw:px-3 tw:py-2 tw:text-left tw:border-b tw:border-line/60',
                        on ? 'tw:bg-accent-soft' : 'tw:hover:bg-hover')}>
                      <span className={cn('tw:mt-0.5 tw:flex tw:size-4 tw:shrink-0 tw:items-center tw:justify-center tw:rounded tw:border',
                        on ? 'tw:bg-accent tw:border-accent tw:text-accent-fg' : 'tw:border-line-strong')}>
                        {on && <Check className="tw:size-3" />}
                      </span>
                      <span className="tw:min-w-0">
                        <span className="tw:block tw:font-mono tw:text-[12px] tw:font-medium tw:break-all">{a.name}</span>
                        {a.description && (
                          <span className="tw:block tw:text-2xs tw:text-muted tw:line-clamp-2 tw:mt-0.5">{firstLine(a.description)}</span>
                        )}
                      </span>
                    </button>
                  );
                })}
              </div>
            </div>

            {/* ------------------------------------------- Configure */}
            <div className="tw:flex-1 tw:overflow-y-auto tw:min-w-0">
              <div className="tw:p-5 tw:space-y-6 tw:max-w-[760px]">
                {/* Artifacts */}
                <section className="tw:space-y-2">
                  <SectionTitle icon={<Package />} title="Artifacts" hint={selected.length ? `${selected.length} selected` : 'Pick at least one artifact on the left'} />
                  {selected.length === 0 && (
                    <div className="tw:rounded-lg tw:border tw:border-dashed tw:border-line-strong tw:p-6 tw:text-center tw:text-xs tw:text-faint">
                      {isServer ? 'Search and select server artifacts to run.' : isCollection ? 'Search and select artifacts to collect from this client.' : 'Search and select artifacts to collect from each client.'}
                    </div>
                  )}
                  {selected.map(name => (
                    <ArtifactParams key={name} name={name} def={defs?.get(name)}
                      values={params[name] ?? {}}
                      expanded={expanded === name}
                      onToggle={() => setExpanded(expanded === name ? null : name)}
                      onChange={(k, v) => setParam(name, k, v)}
                      onReset={() => resetParams(name)}
                      onRemove={() => toggle(name)} />
                  ))}
                </section>

                {/* Targeting */}
                {!isCollection && <section className="tw:space-y-3">
                  <SectionTitle icon={<Users />} title="Targeting" />
                  <div className="tw:flex tw:items-center tw:gap-3 tw:flex-wrap">
                    <Segmented size="md" value={mode} onChange={setMode} options={[
                      { value: 'all', label: 'All clients' },
                      { value: 'labels', label: 'By label' },
                      { value: 'os', label: 'By OS' },
                    ]} />
                    {mode === 'os' && (
                      <Select value={os} onChange={e => setOs(e.target.value)}>
                        <option value="WINDOWS">Windows</option>
                        <option value="LINUX">Linux</option>
                        <option value="OSX">macOS</option>
                      </Select>
                    )}
                  </div>
                  {mode === 'labels' && (
                    <Field label="Include clients with any of these labels" required>
                      <TagInput value={includeLabels} onChange={setIncludeLabels} tone="accent" placeholder="e.g. servers, finance" />
                    </Field>
                  )}
                  <Field label="Exclude clients with these labels">
                    <TagInput value={excludeLabels} onChange={setExcludeLabels} tone="bad" placeholder="Optional" />
                  </Field>
                  <div className="tw:flex tw:items-center tw:gap-3 tw:rounded-lg tw:bg-info-soft tw:px-3 tw:py-2.5">
                    <Users className="tw:size-4 tw:text-info" />
                    <div className="tw:flex-1 tw:text-[13px]">
                      Reaches about{' '}
                      <b className="tw:text-[15px] rx-num">{estimate.data === undefined ? '…' : fmtNum(estimate.data)}</b>{' '}
                      known clients
                      {estimate.isFetching && <Spinner className="tw:inline tw:ml-2 tw:size-3" />}
                    </div>
                    <Select value={lastActive} onChange={e => setLastActive(e.target.value)} className="tw:h-7 tw:text-xs">
                      <option value="all">Seen any time</option>
                      <option value="1 Day">Active in last day</option>
                      <option value="1 Week">Active in last week</option>
                      <option value="1 Month">Active in last month</option>
                    </Select>
                  </div>
                  {isOrgAdmin && orgs.length > 1 && (
                    <Field label="Also run in these organizations" hint="Leave empty to run only in the current organization.">
                      <div className="tw:flex tw:flex-wrap tw:gap-1.5">
                        {orgs.map(o => {
                          const on = orgIds.includes(o.id);
                          return (
                            <button key={o.id} type="button"
                              onClick={() => setOrgIds(on ? orgIds.filter(x => x !== o.id) : [...orgIds, o.id])}
                              className={cn('tw:h-6 tw:px-2 tw:rounded-md tw:text-xs tw:border',
                                on ? 'tw:bg-accent-soft tw:border-accent tw:text-accent' : 'tw:border-line-strong tw:text-muted tw:hover:bg-hover')}>
                              {o.name}
                            </button>
                          );
                        })}
                      </div>
                    </Field>
                  )}
                </section>}

                {/* Details */}
                <section className="tw:space-y-3">
                  <SectionTitle icon={<Settings2 />} title={isCollection ? 'Limits' : 'Details'} />
                  {!isCollection && <div className="tw:grid tw:grid-cols-2 tw:gap-3">
                    <Field label="Description" className="tw:col-span-2">
                      <Input value={description} onChange={e => setDescription(e.target.value)}
                        placeholder={autoDescription || 'What is this hunt looking for?'} />
                    </Field>
                    <Field label="Tags">
                      <TagInput value={tags} onChange={setTags} suggestions={allTags} placeholder="Optional" />
                    </Field>
                    <Field label="Expires" hint={expInPast ? <span className="tw:text-bad">Expiry is in the past</span> : `Default is ${expiryHours / 24} days`}>
                      <Input type="datetime-local" value={expires} onChange={e => setExpires(e.target.value)} />
                    </Field>
                  </div>}

                  {!isCollection && <button type="button" onClick={() => setShowAdvanced(!showAdvanced)}
                    className="tw:flex tw:items-center tw:gap-1 tw:text-xs tw:font-medium tw:text-muted tw:hover:text-fg">
                    {showAdvanced ? <ChevronDown className="tw:size-3.5" /> : <ChevronRight className="tw:size-3.5" />}
                    Resource limits
                  </button>}
                  {showAdvanced && (
                    <div className="tw:grid tw:grid-cols-2 tw:md:grid-cols-4 tw:gap-3">
                      <Field label="Timeout (s)"><Input type="number" min={0} value={timeout} onChange={e => setTimeoutS(e.target.value)} placeholder="600" /></Field>
                      <Field label="Max rows"><Input type="number" min={0} value={maxRows} onChange={e => setMaxRows(e.target.value)} placeholder="1000000" /></Field>
                      <Field label="Max upload (MB)"><Input type="number" min={0} value={maxMb} onChange={e => setMaxMb(e.target.value)} placeholder="1024" /></Field>
                      <Field label="CPU limit (%)"><Input type="number" min={0} max={100} value={cpuLimit} onChange={e => setCpuLimit(e.target.value)} placeholder="None" /></Field>
                      <div className="tw:col-span-full">
                        <Switch checked={urgent} onChange={setUrgent} label="Urgent: skip the client's collection queue" />
                      </div>
                    </div>
                  )}
                </section>
              </div>
            </div>
          </div>

          {/* Footer */}
          <div className="tw:flex tw:items-center tw:gap-3 tw:px-5 tw:h-16 tw:border-t tw:border-line tw:shrink-0 tw:bg-canvas/50 tw:rounded-b-xl">
            {!isCollection && <Switch checked={startNow} onChange={setStartNow}
              label={<span className="tw:text-[13px]">Start immediately <span className="tw:text-faint">(otherwise created paused for review)</span></span>} />}
            <div className="tw:ml-auto tw:flex tw:items-center tw:gap-2">
              <span className="tw:text-2xs tw:text-faint tw:hidden tw:lg:flex tw:items-center tw:gap-1"><Kbd>Ctrl</Kbd><Kbd>Enter</Kbd></span>
              <Button variant="ghost" onClick={() => onOpenChange(false)}>Cancel</Button>
              <Tooltip content={!selected.length ? 'Select at least one artifact' : isCollection ? '' : missingLabels ? 'Add at least one label' : expInPast ? 'Expiry is in the past' : ''}>
                <span>
                  <Button variant="primary" disabled={!canSubmit} loading={create.isPending || collect.isPending} onClick={submit}
                    icon={startNow || isCollection ? <Play className="tw:size-3.5" /> : <Pause className="tw:size-3.5" />}>
                    {isCollection ? 'Collect now' : startNow ? 'Create and start hunt' : 'Create hunt'}
                  </Button>
                </span>
              </Tooltip>
            </div>
          </div>
        </RDialog.Content>
      </RDialog.Portal>
    </RDialog.Root>
  );
}

function SectionTitle({ icon, title, hint }: { icon: React.ReactNode; title: string; hint?: string }) {
  return (
    <div className="tw:flex tw:items-center tw:gap-2">
      <span className="tw:text-faint tw:flex tw:[&>svg]:size-4">{icon}</span>
      <h3 className="tw:text-[13px] tw:font-semibold">{title}</h3>
      {hint && <span className="tw:text-2xs tw:text-faint">{hint}</span>}
    </div>
  );
}

export function ArtifactParams({ name, def, values, expanded, onToggle, onChange, onReset, onRemove }: {
  name: string; def?: ArtifactDescriptor; values: Record<string, string>; expanded: boolean;
  onToggle: () => void; onChange: (k: string, v: string) => void; onReset: () => void; onRemove: () => void;
}) {
  const visible = (def?.parameters ?? []).filter(p => p.type !== 'hidden');
  const changed = Object.entries(values).filter(([k, v]) => {
    const p = def?.parameters?.find(x => x.name === k);
    return !p || (p.default ?? '') !== v;
  }).length;
  return (
    <div className="tw:rounded-lg tw:border tw:border-line tw:bg-panel">
      <div className="tw:flex tw:items-center tw:gap-2 tw:px-3 tw:h-10">
        <button type="button" onClick={onToggle} className="tw:flex tw:items-center tw:gap-2 tw:flex-1 tw:min-w-0 tw:text-left" disabled={!visible.length}>
          {visible.length
            ? (expanded ? <ChevronDown className="tw:size-3.5 tw:text-faint" /> : <ChevronRight className="tw:size-3.5 tw:text-faint" />)
            : <span className="tw:size-3.5" />}
          <span className="tw:font-mono tw:text-[12.5px] tw:font-medium tw:truncate">{name}</span>
          {visible.length > 0 && <span className="tw:text-2xs tw:text-faint">{visible.length} parameter{visible.length === 1 ? "" : "s"}</span>}
          {changed > 0 && <Badge tone="accent">{changed} changed</Badge>}
        </button>
        {changed > 0 && (
          <Tooltip content="Reset parameters to defaults">
            <button type="button" onClick={onReset} className="tw:p-1 tw:rounded tw:text-faint tw:hover:text-fg tw:hover:bg-hover">
              <RotateCcw className="tw:size-3.5" />
            </button>
          </Tooltip>
        )}
        <Tooltip content="Remove">
          <button type="button" onClick={onRemove} className="tw:p-1 tw:rounded tw:text-faint tw:hover:text-bad tw:hover:bg-hover">
            <X className="tw:size-3.5" />
          </button>
        </Tooltip>
      </div>
      {expanded && visible.length > 0 && (
        <div className="tw:border-t tw:border-line tw:p-3 tw:grid tw:grid-cols-1 tw:md:grid-cols-2 tw:gap-3">
          {visible.map(p => (
            <ParamField key={p.name} def={p}
              value={values[p.name] ?? p.default ?? ''}
              changed={p.name in values && values[p.name] !== (p.default ?? '')}
              onChange={v => onChange(p.name, v)} />
          ))}
        </div>
      )}
    </div>
  );
}

const WIDE_TYPES = new Set(['csv', 'json', 'json_array', 'yara', 'regex_array', 'artifactset', 'multichoice', 'upload', 'upload_file']);

function ParamField({ def, value, changed, onChange }: {
  def: ArtifactParameterDef; value: string; changed: boolean; onChange: (v: string) => void;
}) {
  const label = (
    <span className="tw:flex tw:items-center tw:gap-1.5">
      <span className={cn('tw:font-mono', changed && 'tw:text-accent')}>{def.friendly_name || def.name}</span>
      {def.type && def.type !== 'string' && <span className="tw:text-2xs tw:text-faint tw:font-normal">{def.type}</span>}
    </span>
  );
  const hint = def.description ? <span className="tw:line-clamp-2" title={def.description}>{def.description}</span> : undefined;
  const wide = WIDE_TYPES.has(def.type ?? '') || value.length > 60 || value.includes('\n');
  const cls = wide ? 'tw:md:col-span-2' : '';

  switch (def.type) {
    case 'bool': {
      const on = value === 'Y' || value === 'true' || value === 'TRUE' || value === 'y';
      return (
        <div className={cn('tw:space-y-1', cls)}>
          <Switch checked={on} onChange={v => onChange(v ? 'Y' : 'N')} label={label} />
          {hint && <div className="tw:text-2xs tw:text-faint tw:pl-9">{hint}</div>}
        </div>
      );
    }
    case 'choices':
      return (
        <Field label={label} hint={hint} className={cls}>
          <Select value={value} onChange={e => onChange(e.target.value)} className="tw:w-full">
            {!def.choices?.includes(value) && <option value={value}>{value || '(empty)'}</option>}
            {(def.choices ?? []).map(c => <option key={c} value={c}>{c}</option>)}
          </Select>
        </Field>
      );
    case 'multichoice': {
      let current: string[] = [];
      try {
        current = value ? JSON.parse(value) : [];
      } catch {
        current = [];
      }
      return (
        <Field label={label} hint={hint} className={cls}>
          <div className="tw:flex tw:flex-wrap tw:gap-1.5">
            {(def.choices ?? []).map(c => {
              const on = current.includes(c);
              return (
                <button key={c} type="button"
                  onClick={() => onChange(JSON.stringify(on ? current.filter(x => x !== c) : [...current, c]))}
                  className={cn('tw:h-6 tw:px-2 tw:rounded-md tw:text-xs tw:border tw:flex tw:items-center tw:gap-1',
                    on ? 'tw:bg-accent-soft tw:border-accent tw:text-accent' : 'tw:border-line-strong tw:text-muted tw:hover:bg-hover')}>
                  {on ? <Check className="tw:size-3" /> : <Plus className="tw:size-3" />}{c}
                </button>
              );
            })}
          </div>
        </Field>
      );
    }
    case 'int':
    case 'int64':
    case 'float':
      return (
        <Field label={label} hint={hint} className={cls}>
          <Input type="number" value={value} onChange={e => onChange(e.target.value)} />
        </Field>
      );
    case 'timestamp':
      return (
        <Field label={label} hint={hint ?? 'RFC3339, e.g. 2024-01-31T00:00:00Z'} className={cls}>
          <Input value={value} onChange={e => onChange(e.target.value)} placeholder="2024-01-31T00:00:00Z" className="tw:font-mono" />
        </Field>
      );
    default:
      if (wide) {
        return (
          <Field label={label} hint={hint} className={cls}>
            <Textarea rows={Math.min(10, Math.max(3, value.split('\n').length + 1))} value={value}
              onChange={e => onChange(e.target.value)} spellCheck={false} />
          </Field>
        );
      }
      return (
        <Field label={label} hint={hint} className={cls}>
          <Input value={value} onChange={e => onChange(e.target.value)}
            className={def.type === 'regex' ? 'tw:font-mono' : undefined} spellCheck={false} />
        </Field>
      );
  }
}
