// All server data access for the new UI goes through these hooks.
//
// TanStack Query gives us request de-duplication, caching between
// screens, background refresh only while the tab is visible, and
// automatic retries - replacing the per-component setInterval polling
// of the legacy UI.
import { useContext } from 'react';
import {
  QueryClient, useInfiniteQuery, useMutation, useQuery, useQueryClient,
  keepPreviousData,
} from '@tanstack/react-query';
import UserConfig from '../../components/core/user.jsx';
import * as api from './api';
import { parseTable, type ParsedTable, type Row } from './table';
import { toDate, toNum } from './format';
import { archiveState } from './downloads';
import type {
  ApiClient, ArtifactCollectorArgs, FlowDetails, ArtifactDescriptor, GetArtifactsResponse,
  Hunt, HuntCondition, HuntStats, ListHuntsResponse, SearchClientsResponse, Num,
  TableResponse, UserTraits,
} from './types';

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 2_000,
      retry: (count, err) => {
        const status = (err as api.ApiError).status;
        if (status && status >= 400 && status < 500) return false;
        return count < 2;
      },
      refetchOnWindowFocus: true,
    },
  },
});

export const POLL = {
  fast: 5_000,
  normal: 10_000,
  slow: 30_000,
};

// ---------------------------------------------------------------- User

interface UserConfigValue {
  traits: UserTraits;
  updateTraits: () => void;
  messages?: number;
}

export function useTraits(): UserConfigValue {
  return useContext(UserConfig as unknown as React.Context<UserConfigValue>);
}

export function usePermission(name: string): boolean {
  const { traits } = useTraits();
  const perms = traits?.Permissions;
  // Before traits load, assume allowed - the server enforces ACLs anyway.
  if (!perms) return true;
  return !!perms[name];
}

// --------------------------------------------------------------- Hunts

export function useHunts(includeArchived: boolean) {
  return useQuery({
    queryKey: ['hunts', { includeArchived }],
    queryFn: ({ signal }) => api.get<ListHuntsResponse>('v1/ListHunts', {
      count: 2000,
      offset: 0,
      include_archived: includeArchived,
    }, signal),
    select: d => d.items ?? [],
    refetchInterval: POLL.fast,
    placeholderData: keepPreviousData,
  });
}

export function useHunt(huntId: string | undefined) {
  return useQuery({
    queryKey: ['hunt', huntId],
    enabled: !!huntId,
    queryFn: ({ signal }) => api.get<Hunt>('v1/GetHunt', {
      hunt_id: huntId,
      include_request: true,
    }, signal),
    // Poll quickly while an export is being prepared so progress is live.
    refetchInterval: q => (q.state.data?.stats?.available_downloads?.files ?? [])
      .some(f => archiveState(f) === 'preparing') ? 1_500 : POLL.fast,
  });
}

export function useHuntTags() {
  return useQuery({
    queryKey: ['hunt-tags'],
    queryFn: ({ signal }) => api.get<{ tags?: string[] }>('v1/GetHuntTags', {}, signal),
    select: d => d.tags ?? [],
    staleTime: 60_000,
  });
}

// The server answers "Not found" for tables that have no rows yet (e.g. a
// freshly created hunt). Treat that as an empty table rather than an error.
async function emptyIfNotFound(p: Promise<TableResponse>): Promise<TableResponse> {
  try {
    return await p;
  } catch (e) {
    if (e instanceof Error && /not found/i.test(e.message)) return { columns: [], rows: [] } as unknown as TableResponse;
    throw e;
  }
}

export function useHuntFlows(huntId: string | undefined) {
  return useQuery({
    queryKey: ['hunt-flows', huntId],
    enabled: !!huntId,
    queryFn: ({ signal }) => emptyIfNotFound(api.get<TableResponse>('v1/GetHuntFlows', {
      hunt_id: huntId,
      start_row: 0,
      rows: 10000,
    }, signal)),
    select: parseTable,
    refetchInterval: POLL.normal,
    placeholderData: keepPreviousData,
  });
}

export function useHuntErrors(huntId: string | undefined) {
  return useQuery({
    queryKey: ['hunt-errors', huntId],
    enabled: !!huntId,
    queryFn: ({ signal }) => emptyIfNotFound(api.get<TableResponse>('v1/GetTable', {
      hunt_id: huntId,
      type: 'hunt_status',
      start_row: 0,
      rows: 10000,
    }, signal)),
    select: parseTable,
    refetchInterval: POLL.normal,
    retry: false,
  });
}

const RESULTS_PAGE = 2000;

// Hunt results are streamed in pages. The table renders whatever has
// arrived so far and offers to load more.
export function useHuntResults(huntId: string | undefined, artifact: string | undefined) {
  return useInfiniteQuery({
    queryKey: ['hunt-results', huntId, artifact],
    enabled: !!huntId && !!artifact,
    initialPageParam: 0,
    queryFn: async ({ pageParam, signal }) => {
      const resp = await emptyIfNotFound(api.get<TableResponse>('v1/GetHuntResults', {
        hunt_id: huntId,
        artifact,
        offset: pageParam,
        count: RESULTS_PAGE,
      }, signal));
      return { ...parseTable(resp), offset: pageParam as number };
    },
    getNextPageParam: (last: ParsedTable & { offset: number }) => {
      if (last.rows.length < RESULTS_PAGE) return undefined;
      return last.offset + last.rows.length;
    },
    staleTime: 30_000,
  });
}

export interface HuntMutation {
  hunt_id: string;
  state?: 'RUNNING' | 'PAUSED' | 'STOPPED' | 'ARCHIVED';
  description?: string;
  tags?: string[];
  expires?: number;
  recalculate?: boolean;
}

export function useModifyHunt() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (m: HuntMutation) => api.post('v1/ModifyHunt', m),
    onSettled: (_d, _e, m) => {
      qc.invalidateQueries({ queryKey: ['hunts'] });
      qc.invalidateQueries({ queryKey: ['hunt', m.hunt_id] });
    },
  });
}

function collectServerArtifact(artifact: string, env: { key: string; value: string }[]) {
  return api.post<{ flow_id?: string }>('v1/CollectArtifact', {
    client_id: 'server',
    artifacts: [artifact],
    specs: [{ artifact, parameters: { env } }],
  });
}

// Stopping a hunt pauses scheduling and then cancels in-flight
// collections in the background, exactly like the legacy UI.
export function useStopHunts() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (huntIds: string[]) => {
      await Promise.all(huntIds.map(id => api.post('v1/ModifyHunt', { hunt_id: id, state: 'PAUSED' })));
      await collectServerArtifact('Server.Utils.CancelHunt', [
        { key: 'Hunts', value: JSON.stringify(huntIds) },
      ]);
    },
    onSettled: () => qc.invalidateQueries({ queryKey: ['hunts'] }),
  });
}

export function useStartHunts() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (huntIds: string[]) => Promise.all(
      huntIds.map(id => api.post('v1/ModifyHunt', { hunt_id: id, state: 'RUNNING' }))),
    onSettled: () => qc.invalidateQueries({ queryKey: ['hunts'] }),
  });
}

export function useArchiveHunts() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (huntIds: string[]) => Promise.all(
      huntIds.map(id => api.post('v1/ModifyHunt', { hunt_id: id, state: 'ARCHIVED' }))),
    onSettled: () => qc.invalidateQueries({ queryKey: ['hunts'] }),
  });
}

export function useDeleteHunts() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (huntIds: string[]) => collectServerArtifact('Server.Hunts.CancelAndDelete', [
      { key: 'Hunts', value: JSON.stringify(huntIds) },
      { key: 'DeleteAllFiles', value: 'Y' },
    ]),
    onSettled: () => {
      // Deletion runs as a server artifact; refresh a little later too.
      qc.invalidateQueries({ queryKey: ['hunts'] });
      setTimeout(() => qc.invalidateQueries({ queryKey: ['hunts'] }), 3000);
    },
  });
}

export interface CreateHuntRequest {
  hunt_description?: string;
  tags?: string[];
  expires?: number;
  start_request: ArtifactCollectorArgs;
  condition?: HuntCondition;
  org_ids?: string[];
  state?: number;
}

export function useCreateHunt() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: CreateHuntRequest) => api.post<{ flow_id?: string }>('v1/CreateHunt', req),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['hunts'] });
      qc.invalidateQueries({ queryKey: ['hunt-tags'] });
    },
  });
}

export function useEstimateHunt(condition: HuntCondition, lastActive: string, enabled = true) {
  return useQuery({
    queryKey: ['estimate', condition, lastActive],
    enabled,
    queryFn: ({ signal }) => api.post<HuntStats>('v1/EstimateHunt', {
      condition,
      last_active: lastActive === 'all' ? undefined : lastActive,
    }, signal),
    select: d => toNum(d.total_clients_scheduled),
    staleTime: 15_000,
    placeholderData: keepPreviousData,
  });
}

export function usePrepareHuntDownload() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (p: { hunt_id: string; kind: 'all' | 'summary-json' | 'summary-csv'; password?: string }) => {
      const params: Record<string, unknown> = { hunt_id: p.hunt_id, password: p.password || '' };
      if (p.kind === 'all') {
        params.json_format = true;
        params.csv_format = true;
      } else if (p.kind === 'summary-json') {
        params.only_combined_hunt = true;
        params.json_format = true;
      } else {
        params.only_combined_hunt = true;
        params.csv_format = true;
      }
      return api.post('v1/CreateDownload', params);
    },
    onSettled: (_d, _e, p) => qc.invalidateQueries({ queryKey: ['hunt', p.hunt_id] }),
  });
}

// ----------------------------------------------------------- Artifacts

export function useArtifactSearch(
  term: string, type: '' | 'CLIENT' | 'SERVER' | 'CLIENT_EVENT' | 'SERVER_EVENT' = 'CLIENT', enabled = true) {
  return useQuery({
    queryKey: ['artifact-search', type, term],
    enabled,
    queryFn: ({ signal }) => api.post<GetArtifactsResponse>('v1/GetArtifacts', {
      type: type || undefined,
      search_term: term,
      number_of_results: 100,
      fields: { name: true, description: true },
    }, signal),
    select: d => d.items ?? [],
    staleTime: 60_000,
    placeholderData: keepPreviousData,
  });
}

export function useArtifactDefinitions(names: string[]) {
  return useQuery({
    queryKey: ['artifact-defs', [...names].sort()],
    enabled: names.length > 0,
    queryFn: ({ signal }) => api.post<GetArtifactsResponse>('v1/GetArtifacts', { names }, signal),
    select: d => {
      const m = new Map<string, ArtifactDescriptor>();
      for (const a of d.items ?? []) m.set(a.name, a);
      return m;
    },
    staleTime: 60_000,
    placeholderData: keepPreviousData,
  });
}

// ------------------------------------------------------------- Clients

export type ClientSort = 'UNSORTED' | 'SORT_UP' | 'SORT_DOWN';

export function useClientSearch(query: string, opts: { limit?: number; onlineOnly?: boolean; sort?: ClientSort } = {}) {
  const { limit = 1000, onlineOnly = false, sort = 'UNSORTED' } = opts;
  return useQuery({
    queryKey: ['clients', query, limit, onlineOnly, sort],
    queryFn: ({ signal }) => api.get<SearchClientsResponse>('v1/SearchClients', {
      query: query || 'all',
      limit,
      offset: 0,
      sort,
      filter: onlineOnly ? 'ONLINE' : 'UNFILTERED',
    }, signal),
    refetchInterval: POLL.normal,
    placeholderData: keepPreviousData,
  });
}

export function useClientNameSuggest(query: string) {
  return useQuery({
    queryKey: ['client-suggest', query],
    enabled: query.length > 0,
    queryFn: ({ signal }) => api.get<SearchClientsResponse>('v1/SearchClients', {
      query: query.includes(':') || query.endsWith('*') ? query : `${query}*`,
      limit: 8,
      name_only: false,
    }, signal),
    select: d => d.items ?? [],
    staleTime: 10_000,
    placeholderData: keepPreviousData,
  });
}

export function useClient(clientId: string | undefined) {
  return useQuery({
    queryKey: ['client', clientId],
    enabled: !!clientId && clientId !== 'server',
    queryFn: ({ signal }) => api.get<ApiClient>(`v1/GetClient/${clientId}`, {}, signal),
    refetchInterval: POLL.normal,
  });
}

export function useClientCounts() {
  return useQuery({
    queryKey: ['client-counts'],
    queryFn: async ({ signal }) => {
      // SearchClients reports the total number of matches regardless
      // of the limit, so limit=1 is enough to count.
      const [all, online] = await Promise.all([
        api.get<SearchClientsResponse>('v1/SearchClients', { query: 'all', limit: 1 }, signal),
        api.get<SearchClientsResponse>('v1/SearchClients', {
          query: 'all', limit: 1, filter: 'ONLINE',
        }, signal),
      ]);
      return { total: toNum(all.total), online: toNum(online.total) };
    },
    refetchInterval: POLL.slow,
  });
}

// Most recently seen clients. The API has no "sort by last seen", so
// we fetch online clients and sort locally.
export function useRecentClients(limit = 8) {
  return useQuery({
    queryKey: ['recent-clients', limit],
    queryFn: ({ signal }) => api.get<SearchClientsResponse>('v1/SearchClients', {
      query: 'all', limit: 500, filter: 'ONLINE',
    }, signal),
    select: d => [...(d.items ?? [])]
      .sort((a, b) => toNum(b.last_seen_at) - toNum(a.last_seen_at))
      .slice(0, limit),
    refetchInterval: POLL.slow,
  });
}

export function useClientFlows(clientId: string, count = 20) {
  return useQuery({
    queryKey: ['client-flows', clientId, count],
    queryFn: ({ signal }) => api.get<TableResponse>('v1/GetClientFlows', {
      client_id: clientId, count, offset: 0,
    }, signal),
    select: parseTable,
    refetchInterval: POLL.normal,
    placeholderData: keepPreviousData,
  });
}

export function useLabelClients() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (p: { client_ids: string[]; labels: string[]; operation: 'set' | 'remove' }) =>
      api.post('v1/LabelClients', p),
    onSettled: () => {
      qc.invalidateQueries({ queryKey: ['clients'] });
      qc.invalidateQueries({ queryKey: ['client'] });
    },
  });
}

// ------------------------------------------------------------- Server

export function useServerHealth(rangeSec: number) {
  return useQuery({
    queryKey: ['server-health', rangeSec],
    queryFn: async ({ signal }) => {
      const now = Math.floor(Date.now() / 1000);
      const resp = await api.get<TableResponse>('v1/GetTable', {
        type: 'SERVER_EVENT',
        artifact: 'Server.Monitor.Health/Prometheus',
        start_time: now - rangeSec,
        end_time: now,
        rows: 5000,
      }, signal);
      return parseTable(resp);
    },
    refetchInterval: POLL.normal,
    placeholderData: keepPreviousData,
    retry: false,
  });
}


// ----------------------------------------------------------- Exports

export interface HuntExportOptions {
  hunt_id: string;
  only_combined_hunt?: boolean;
  json_format?: boolean;
  csv_format?: boolean;
  password?: string;
}

export function useExportHunt() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (p: HuntExportOptions) => api.post('v1/CreateDownload', { password: '', ...p }),
    onSettled: (_d, _e, p) => qc.invalidateQueries({ queryKey: ['hunt', p.hunt_id] }),
  });
}

export function useExportFlow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (p: { client_id: string; flow_id: string; password?: string; expand_sparse?: boolean }) =>
      api.post('v1/CreateDownload', { password: '', ...p }),
    onSettled: (_d, _e, p) => qc.invalidateQueries({ queryKey: ['flow', p.client_id, p.flow_id] }),
  });
}

// ------------------------------------------------------------- Flows

export function useFlow(clientId: string | undefined, flowId: string | undefined, opts?: { fast?: boolean }) {
  return useQuery({
    queryKey: ['flow', clientId, flowId],
    enabled: !!clientId && !!flowId,
    queryFn: ({ signal }) => api.get<FlowDetails>('v1/GetFlowDetails', { client_id: clientId, flow_id: flowId }, signal),
    refetchInterval: q => {
      const d = q.state.data;
      const preparing = (d?.available_downloads?.files ?? []).some(f => archiveState(f) === 'preparing');
      if (preparing || opts?.fast) return 1_500;
      const st = d?.context?.state;
      return st === 'RUNNING' || st === 'IN_PROGRESS' || st === 'WAITING' ? 2_500 : POLL.slow;
    },
  });
}

export function useClientFlowList(clientId: string | undefined, rows = 500) {
  return useQuery({
    queryKey: ['client-flow-list', clientId, rows],
    enabled: !!clientId,
    queryFn: ({ signal }) => emptyIfNotFound(api.get<TableResponse>('v1/GetClientFlows', {
      client_id: clientId, start_row: 0, rows, count: rows, offset: 0,
    }, signal)),
    select: parseTable,
    refetchInterval: POLL.fast,
    placeholderData: keepPreviousData,
  });
}

export function useFlowTable(clientId: string | undefined, flowId: string | undefined,
  kind: { type: 'uploads' | 'log' } | { artifact: string | undefined }, live = false) {
  const key = 'type' in kind ? kind.type : `artifact:${kind.artifact}`;
  return useInfiniteQuery({
    queryKey: ['flow-table', clientId, flowId, key],
    enabled: !!clientId && !!flowId && ('type' in kind || !!kind.artifact),
    initialPageParam: 0,
    queryFn: async ({ pageParam, signal }) => {
      const resp = await emptyIfNotFound(api.get<TableResponse>('v1/GetTable', {
        client_id: clientId, flow_id: flowId, ...kind, start_row: pageParam, rows: RESULTS_PAGE,
      }, signal));
      return { ...parseTable(resp), offset: pageParam as number };
    },
    getNextPageParam: (last: ParsedTable & { offset: number }) =>
      last.rows.length < RESULTS_PAGE ? undefined : last.offset + last.rows.length,
    refetchInterval: live ? 3_000 : false,
  });
}

export function useCollectArtifact() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: ArtifactCollectorArgs & { client_id: string }) =>
      api.post<{ flow_id?: string }>('v1/CollectArtifact', req),
    onSettled: (_d, _e, p) => {
      qc.invalidateQueries({ queryKey: ['client-flow-list', p.client_id] });
      qc.invalidateQueries({ queryKey: ['client-flows', p.client_id] });
    },
  });
}

export function useCancelFlow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (p: { client_id: string; flow_id: string }) => api.post('v1/CancelFlow', p),
    onSettled: (_d, _e, p) => {
      qc.invalidateQueries({ queryKey: ['flow', p.client_id, p.flow_id] });
      qc.invalidateQueries({ queryKey: ['client-flow-list', p.client_id] });
    },
  });
}

// All files uploaded across a hunt. The server has no hunt-wide uploads
// table, so we read each collection's uploads table (only collections that
// uploaded something) with bounded concurrency.
export const HUNT_UPLOADS_MAX_FLOWS = 500;

export function useHuntUploads(huntId: string | undefined, flows: Row[] | undefined, enabled: boolean) {
  const withFiles = (flows ?? []).filter(f => toNum(f.TotalBytes as number) > 0);
  const sig = withFiles.map(f => `${f.FlowId}:${f.TotalBytes}`).join(',');
  return useQuery({
    queryKey: ['hunt-uploads', huntId, sig],
    enabled: enabled && !!huntId && flows !== undefined,
    placeholderData: keepPreviousData,
    staleTime: 15_000,
    queryFn: async ({ signal }) => {
      const todo = withFiles.slice(0, HUNT_UPLOADS_MAX_FLOWS);
      const out: Row[] = [];
      let i = 0;
      const worker = async () => {
        while (i < todo.length) {
          const f = todo[i++];
          const clientId = String(f.ClientId ?? '');
          const flowId = String(f.FlowId ?? '');
          try {
            const t = parseTable(await emptyIfNotFound(api.get<TableResponse>('v1/GetTable', {
              client_id: clientId, flow_id: flowId, type: 'uploads', start_row: 0, rows: 10000,
            }, signal)));
            for (const r of t.rows) {
              out.push({ Hostname: f.Hostname, ClientId: clientId, FlowId: flowId, ...r });
            }
          } catch (e) {
            if (signal.aborted) throw e;
          }
        }
      };
      await Promise.all(Array.from({ length: 6 }, worker));
      return { rows: out, truncated: withFiles.length > todo.length, flowsWithFiles: withFiles.length };
    },
  });
}

// ---------------------------------------------------------- Client info

export function useClientMetadata(clientId: string | undefined) {
  return useQuery({
    queryKey: ['client-metadata', clientId],
    enabled: !!clientId && clientId !== 'server',
    queryFn: ({ signal }) => api.get<{ items?: { key: string; value: string }[] }>(`v1/GetClientMetadata/${clientId}`, {}, signal),
    select: d => d.items ?? [],
  });
}

export function useSetClientMetadata() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (p: { client_id: string; add: { key: string; value: string }[]; remove: string[] }) =>
      api.post('v1/SetClientMetadata', p),
    onSettled: (_d, _e, p) => qc.invalidateQueries({ queryKey: ['client-metadata', p.client_id] }),
  });
}

// Which of these artifact names exist on this server.
export function useExistingArtifacts(names: string[]) {
  return useQuery({
    queryKey: ['artifact-exists', [...names].sort()],
    enabled: names.length > 0,
    queryFn: ({ signal }) => api.post<GetArtifactsResponse>('v1/GetArtifacts', {
      names, number_of_results: names.length, fields: { name: true, description: true },
    }, signal),
    select: d => new Map((d.items ?? []).map(a => [a.name, a])),
    staleTime: 5 * 60_000,
  });
}

// Collect one artifact with simple parameters.
export async function runArtifact(clientId: string, artifact: string, params: Record<string, string> = {}, urgent = false) {
  return runArtifacts(clientId, [artifact], { [artifact]: params }, urgent);
}

// Collect several artifacts in one flow. `params` maps an artifact name to
// its parameter values; artifacts without an entry use their defaults.
export async function runArtifacts(
  clientId: string, artifacts: string[], params: Record<string, Record<string, string>> = {}, urgent = false,
) {
  return api.post<{ flow_id?: string }>('v1/CollectArtifact', {
    client_id: clientId,
    artifacts,
    specs: artifacts.map(artifact => ({
      artifact,
      parameters: { env: Object.entries(params[artifact] ?? {}).map(([key, value]) => ({ key, value })) },
    })),
    urgent,
  });
}

// ----------------------------------------------------------- Artifacts

export type ArtifactKind = '' | 'client' | 'server' | 'client_event' | 'server_event' | 'notebook';

export function useArtifactCatalog() {
  return useQuery({
    queryKey: ['artifact-catalog'],
    queryFn: ({ signal }) => api.post<GetArtifactsResponse>('v1/GetArtifacts', {
      search_term: '', number_of_results: 5000,
      fields: { name: true, type: true, built_in: true, description: true },
    }, signal),
    select: d => d.items ?? [],
    staleTime: 60_000,
  });
}

export function useArtifactFile(name: string | undefined) {
  return useQuery({
    queryKey: ['artifact-file', name],
    enabled: !!name,
    queryFn: ({ signal }) => api.get<{ artifact?: string }>('v1/GetArtifactFile', { name }, signal),
    select: d => d.artifact ?? '',
  });
}

export interface SetArtifactResult { errors?: string[]; warnings?: string[]; error?: boolean; error_message?: string }

export function useSetArtifactFile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (p: { artifact: string; op: 'CHECK_AND_SET' | 'SET' | 'DELETE' | 'CHECK' }) =>
      api.post<SetArtifactResult>('v1/SetArtifactFile', p),
    onSettled: () => {
      qc.invalidateQueries({ queryKey: ['artifact-catalog'] });
      qc.invalidateQueries({ queryKey: ['artifact-defs'] });
      qc.invalidateQueries({ queryKey: ['artifact-file'] });
      qc.invalidateQueries({ queryKey: ['artifact-search'] });
    },
  });
}

// ----------------------------------------------------------------- VFS

export interface VfsEntry {
  Name: string;
  Mode?: string;
  Size?: number;
  _OSPath?: string;
  _Components?: string[];
  _Accessor?: string;
}

export function useVfsDir(clientId: string | undefined, components: string[]) {
  return useQuery({
    queryKey: ['vfs-dir', clientId, components],
    enabled: !!clientId,
    queryFn: async ({ signal }) => {
      const r = await api.get<{ Response?: string; flow_id?: string }>(`v1/VFSListDirectory/${clientId}`, { vfs_components: components }, signal);
      let entries: VfsEntry[] = [];
      try { entries = JSON.parse(r.Response || '[]') ?? []; } catch { entries = []; }
      return { entries, flowId: r.flow_id };
    },
    staleTime: 10_000,
  });
}

export function useVfsStat(clientId: string | undefined, components: string[]) {
  return useQuery({
    queryKey: ['vfs-stat', clientId, components],
    enabled: !!clientId && components.length > 0,
    queryFn: ({ signal }) => api.get<{ timestamp?: string; download_version?: string; flow_id?: string; total_rows?: string; artifact?: string }>(
      'v1/VFSStatDirectory', { client_id: clientId, vfs_components: components }, signal).catch(() => ({} as { timestamp?: string; download_version?: string; flow_id?: string; total_rows?: string; artifact?: string })),
  });
}

export function useVfsFiles(clientId: string | undefined, components: string[], flowId: string | undefined, version: string) {
  return useQuery({
    queryKey: ['vfs-files', clientId, components, flowId, version],
    enabled: !!clientId && !!flowId,
    queryFn: async ({ signal }) => parseTable(await emptyIfNotFound(api.get<TableResponse>('v1/VFSListDirectoryFiles', {
      client_id: clientId, flow_id: flowId, artifact: 'System.VFS.ListDirectory/Listing',
      vfs_components: components, start_row: 0, rows: 20000,
    }, signal))),
    placeholderData: keepPreviousData,
  });
}

export async function vfsRefresh(clientId: string, components: string[], depth = 0) {
  return api.post<{ flow_id?: string }>('v1/VFSRefreshDirectory', { client_id: clientId, vfs_components: components, depth });
}

export async function vfsDownload(clientId: string, accessor: string, components: string[]) {
  return api.post<{ flow_id?: string }>('v1/VFSDownloadFile', { client_id: clientId, accessor, components });
}

// --------------------------------------------------------------- Users

export interface GlobalUser {
  name: string;
  orgs?: { id: string; name: string }[];
  stats?: { last_active_time?: string; last_ip_address?: string };
  locked?: boolean;
}

export interface UserRoles {
  name: string;
  org: string;
  org_name?: string;
  roles?: string[];
  permissions?: string[];
  effective_permissions?: string[];
  all_roles?: string[];
  all_permissions?: string[];
}

export function useGlobalUsers() {
  return useQuery({
    queryKey: ['global-users'],
    queryFn: ({ signal }) => api.get<{ users?: GlobalUser[] }>('v1/GetGlobalUsers', {}, signal),
    select: d => [...(d.users ?? [])].sort((a, b) => a.name.localeCompare(b.name)),
  });
}

export function useUserRoles(name: string | undefined, org: string | undefined) {
  return useQuery({
    queryKey: ['user-roles', name, org],
    enabled: !!name && !!org,
    queryFn: ({ signal }) => api.get<UserRoles>('v1/GetUserRoles', { name, org }, signal),
  });
}

export function useSetUserRoles() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (p: { name: string; org: string; roles: string[]; permissions: string[] }) =>
      api.post('v1/SetUserRoles', p),
    onMutate: async p => {
      const key = ['user-roles', p.name, p.org];
      const prev = qc.getQueryData<UserRoles>(key);
      if (prev) qc.setQueryData(key, { ...prev, roles: p.roles, permissions: p.permissions });
      return { prev, key };
    },
    onError: (_e, _p, ctx) => { if (ctx?.prev) qc.setQueryData(ctx.key, ctx.prev); },
    onSettled: (_d, _e, p) => {
      qc.invalidateQueries({ queryKey: ['user-roles', p.name] });
      qc.invalidateQueries({ queryKey: ['global-users'] });
    },
  });
}

export function useCreateUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (p: { name: string; orgs: string[]; roles: string[] }) =>
      api.post('v1/CreateUser', { ...p, add_new_user: true }),
    onSettled: () => qc.invalidateQueries({ queryKey: ['global-users'] }),
  });
}

// Grant an existing user access to more orgs.
export function useAddUserOrgs() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (p: { name: string; orgs: string[]; roles: string[] }) =>
      api.post('v1/CreateUser', { ...p, add_new_user: false }),
    onSettled: () => qc.invalidateQueries({ queryKey: ['global-users'] }),
  });
}

export function useSetPassword() {
  return useMutation({
    mutationFn: (p: { username: string; password: string }) => api.post('v1/SetPassword', p),
  });
}

// ------------------------------------------------------------- Secrets

export interface SecretDefinition {
  type_name: string;
  description?: string;
  verifier?: string;
  fields?: string[];
  template?: Record<string, string>;
  secret_names?: string[];
  built_in?: boolean;
}

export interface SecretInfo {
  name: string;
  type_name: string;
  users?: string[];
  orgs?: string[];
  visible_to_all_orgs?: boolean;
}

export function useSecretDefinitions() {
  return useQuery({
    queryKey: ['secret-defs'],
    queryFn: ({ signal }) => api.get<{ items?: SecretDefinition[] }>('v1/GetSecretDefinitions', {}, signal),
    select: d => d.items ?? [],
  });
}

export function useSecret(typeName: string | undefined, name: string | undefined) {
  return useQuery({
    queryKey: ['secret', typeName, name],
    enabled: !!typeName && !!name,
    queryFn: ({ signal }) => api.get<SecretInfo>('v1/GetSecret', { type_name: typeName, name }, signal),
  });
}

export function useAddSecret() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (p: { name: string; type_name: string; secret: Record<string, string> }) => api.post('v1/AddSecret', p),
    onSettled: () => qc.invalidateQueries({ queryKey: ['secret-defs'] }),
  });
}

export function useModifySecret() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (p: {
      type_name: string; name: string; delete?: boolean; add_users?: string[]; remove_users?: string[];
      visible_to_all_orgs?: boolean;
    }) => api.post('v1/ModifySecret', p),
    onSettled: (_d, _e, p) => {
      qc.invalidateQueries({ queryKey: ['secret-defs'] });
      qc.invalidateQueries({ queryKey: ['secret', p.type_name, p.name] });
    },
  });
}

// ------------------------------------------------------ Event monitoring

export interface EventLogInfo { artifact: string; row_timestamps?: Num[]; log_timestamps?: Num[] }

export function useEventArtifacts(clientId: string | undefined) {
  return useQuery({
    queryKey: ['event-artifacts', clientId],
    enabled: !!clientId,
    queryFn: ({ signal }) => api.post<{ logs?: EventLogInfo[] }>('v1/ListAvailableEventResults', { client_id: clientId }, signal),
    select: d => d.logs ?? [],
    refetchInterval: POLL.slow,
  });
}

export function useEventDays(clientId: string | undefined, artifact: string | undefined) {
  return useQuery({
    queryKey: ['event-days', clientId, artifact],
    enabled: !!clientId && !!artifact,
    queryFn: ({ signal }) => api.post<{ logs?: EventLogInfo[] }>('v1/ListAvailableEventResults',
      { client_id: clientId, artifact }, signal),
    select: d => ({
      rows: (d.logs?.[0]?.row_timestamps ?? []).map(toNum),
      logs: (d.logs?.[0]?.log_timestamps ?? []).map(toNum),
    }),
    refetchInterval: POLL.slow,
  });
}

const EVENT_PAGE = 2000;

// Event tables are indexed by time, not row number: page forward from the
// last row's timestamp and drop the rows of that second already shown.
export function useEventTable(clientId: string | undefined, artifact: string | undefined,
  range: { start: number; end: number }, logs: boolean, live: boolean) {
  const server = clientId === 'server';
  const type = logs ? (server ? 'SERVER_EVENT_LOGS' : 'CLIENT_EVENT_LOGS') : (server ? 'SERVER_EVENT' : 'CLIENT_EVENT');
  return useInfiniteQuery({
    queryKey: ['event-table', clientId, artifact, type, range.start, range.end],
    enabled: !!clientId && !!artifact,
    initialPageParam: { start: range.start, skip: 0 },
    queryFn: async ({ pageParam, signal }) => {
      const p = pageParam as { start: number; skip: number };
      const resp = await emptyIfNotFound(api.get<TableResponse>('v1/GetTable', {
        client_id: clientId, artifact, type, start_time: p.start, end_time: range.end, rows: EVENT_PAGE + p.skip,
      }, signal));
      const t = parseTable(resp);
      const full = t.rows.length >= EVENT_PAGE + p.skip;
      const rows = t.rows.slice(p.skip);
      const tsOf = (r: Row) => Math.floor((toDate((r._ts ?? r.Timestamp ?? r.client_time) as Num)?.getTime() ?? 0) / 1000);
      let next: { start: number; skip: number } | undefined;
      if (full && rows.length) {
        const last = tsOf(rows[rows.length - 1]);
        const sameSecond = t.rows.filter(r => tsOf(r) === last).length;
        next = last > p.start ? { start: last, skip: sameSecond } : { start: p.start, skip: p.skip + rows.length };
      }
      return { ...t, rows, next };
    },
    getNextPageParam: last => last.next,
    refetchInterval: live ? POLL.normal : false,
  });
}

export interface MonitoringSpec { artifact: string; parameters?: { env?: { key: string; value: string }[] } }
export interface MonitoringTable { artifacts?: string[]; specs?: MonitoringSpec[]; [k: string]: unknown }
export interface ClientMonitoringState {
  artifacts?: MonitoringTable;
  label_events?: { label: string; artifacts?: MonitoringTable }[];
  [k: string]: unknown;
}

export function useMonitoringState(server: boolean) {
  return useQuery({
    queryKey: ['monitoring-state', server ? 'server' : 'client'],
    queryFn: ({ signal }) => server
      ? api.get<MonitoringTable>('v1/GetServerMonitoringState', {}, signal).then(t => ({ artifacts: t } as ClientMonitoringState))
      : api.get<ClientMonitoringState>('v1/GetClientMonitoringState', {}, signal),
  });
}

export function useSetMonitoringState(server: boolean) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (s: ClientMonitoringState) => server
      ? api.post('v1/SetServerMonitoringState', s.artifacts ?? {})
      : api.post('v1/SetClientMonitoringState', s),
    onSettled: () => {
      qc.invalidateQueries({ queryKey: ['monitoring-state'] });
      qc.invalidateQueries({ queryKey: ['event-artifacts'] });
    },
  });
}

// ------------------------------------------------------------ Notebooks

export interface NotebookCellMeta {
  cell_id: string;
  summary?: string;
  type?: string;
  timestamp?: Num;
  current_version?: string;
  available_versions?: string[];
  calculating?: boolean;
}

export interface Notebook {
  notebook_id: string;
  name?: string;
  description?: string;
  creator?: string;
  created_time?: Num;
  modified_time?: Num;
  collaborators?: string[];
  public?: boolean;
  hidden?: boolean;
  artifacts?: string[];
  specs?: unknown[];
  cell_metadata?: NotebookCellMeta[];
  suggestions?: { name?: string; type?: string; input?: string; env?: { key: string; value: string }[] }[];
  available_downloads?: { files?: import('./types').AvailableDownloadFile[] };
  version?: Num;
  [k: string]: unknown;
}

export interface NotebookCell {
  notebook_id?: string;
  cell_id: string;
  input?: string;
  output?: string;
  type?: string;
  messages?: string[];
  calculating?: boolean;
  error?: string;
  env?: { key: string; value: string }[];
  timestamp?: Num;
  duration?: Num;
  current_version?: string;
  available_versions?: string[];
  more_messages?: boolean;
}

export function useNotebookList() {
  return useQuery({
    queryKey: ['notebook-list'],
    queryFn: ({ signal }) => emptyIfNotFound(api.get<TableResponse>('v1/GetTable', { type: 'NOTEBOOKS', start_row: 0, rows: 2000 }, signal)),
    select: d => parseTable(d).rows,
    refetchInterval: POLL.slow,
  });
}

export function useNotebook(id: string | undefined, opts: { uploads?: boolean } = {}) {
  return useQuery({
    queryKey: ['notebook', id, !!opts.uploads],
    enabled: !!id,
    queryFn: ({ signal }) => api.get<{ items?: Notebook[] }>('v1/GetNotebooks',
      { notebook_id: id, include_uploads: opts.uploads || undefined }, signal),
    select: d => d.items?.[0],
    refetchInterval: q => {
      const nb = q.state.data?.items?.[0];
      if (nb?.cell_metadata?.some(c => c.calculating)) return 1_500;
      const files = nb?.available_downloads?.files ?? [];
      if (files.some(f => archiveState(f) === 'preparing')) return 1_500;
      return POLL.fast;
    },
  });
}

export function useNotebookCell(notebookId: string, cellId: string, version: string | undefined) {
  return useQuery({
    queryKey: ['notebook-cell', notebookId, cellId, version],
    queryFn: ({ signal }) => api.get<NotebookCell>('v1/GetNotebookCell', { notebook_id: notebookId, cell_id: cellId, version }, signal),
    refetchInterval: q => (q.state.data?.calculating ? 1_000 : false),
    staleTime: 60_000,
  });
}

function useNotebookInvalidate() {
  const qc = useQueryClient();
  return (id?: string) => {
    qc.invalidateQueries({ queryKey: ['notebook', id] });
    qc.invalidateQueries({ queryKey: ['notebook-list'] });
  };
}

export function useNewNotebook() {
  const inv = useNotebookInvalidate();
  return useMutation({
    mutationFn: (p: Partial<Notebook>) => api.post<Notebook>('v1/NewNotebook', p),
    onSettled: () => inv(),
  });
}

export function useUpdateNotebook() {
  const qc = useQueryClient();
  const inv = useNotebookInvalidate();
  return useMutation({
    mutationFn: (nb: Notebook) => api.post<Notebook>('v1/UpdateNotebook', nb),
    onMutate: nb => {
      // Optimistic for reorder/delete of cells.
      qc.setQueriesData<{ items?: Notebook[] }>({ queryKey: ['notebook', nb.notebook_id] },
        d => (d?.items?.length ? { ...d, items: [{ ...d.items[0], ...nb }] } : d));
    },
    onSettled: (_d, _e, nb) => inv(nb.notebook_id),
  });
}

export function useDeleteNotebook() {
  const inv = useNotebookInvalidate();
  return useMutation({
    mutationFn: (nb: Notebook) => api.post('v1/DeleteNotebook', { ...nb, hidden: true }),
    onSettled: () => inv(),
  });
}

export function useNewNotebookCell() {
  const inv = useNotebookInvalidate();
  return useMutation({
    mutationFn: (p: { notebook_id: string; cell_id: string; type: string; input: string; env?: { key: string; value: string }[] }) =>
      api.post<Notebook>('v1/NewNotebookCell', p),
    onSettled: (_d, _e, p) => inv(p.notebook_id),
  });
}

export function useUpdateNotebookCell() {
  const qc = useQueryClient();
  const inv = useNotebookInvalidate();
  return useMutation({
    mutationFn: (p: { notebook_id: string; cell_id: string; type: string; input: string; env?: { key: string; value: string }[] }) =>
      api.post<NotebookCell>('v1/UpdateNotebookCell', { ...p, currently_editing: false }),
    onSuccess: cell => {
      if (cell?.cell_id) qc.setQueryData(['notebook-cell', cell.notebook_id, cell.cell_id, cell.current_version], cell);
    },
    onSettled: (_d, _e, p) => inv(p.notebook_id),
  });
}

export async function notebookCellAction(action: 'CancelNotebookCell' | 'RevertNotebookCell',
  p: { notebook_id: string; cell_id: string; version?: string }) {
  return api.post<NotebookCell>(`v1/${action}`, p);
}

export async function reformatVQL(vql: string): Promise<string> {
  const r = await api.post<{ vql?: string }>('v1/ReformatVQL', { vql });
  return r.vql || vql;
}

export function useNotebookTable(params: Record<string, unknown> | null) {
  return useInfiniteQuery({
    queryKey: ['notebook-table', params],
    enabled: !!params,
    initialPageParam: 0,
    queryFn: async ({ pageParam, signal }) => {
      const resp = await emptyIfNotFound(api.get<TableResponse>('v1/GetTable', {
        ...params, TableOptions: undefined, start_row: pageParam, rows: RESULTS_PAGE,
      }, signal));
      return { ...parseTable(resp), offset: pageParam as number };
    },
    getNextPageParam: (last: ParsedTable & { offset: number }) =>
      last.rows.length < RESULTS_PAGE ? undefined : last.offset + last.rows.length,
    staleTime: 5 * 60_000,
  });
}
