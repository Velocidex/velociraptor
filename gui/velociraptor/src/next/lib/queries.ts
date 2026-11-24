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
import { parseTable, type ParsedTable } from './table';
import { toNum } from './format';
import type {
  ApiClient, ArtifactCollectorArgs, ArtifactDescriptor, GetArtifactsResponse,
  Hunt, HuntCondition, HuntStats, ListHuntsResponse, SearchClientsResponse,
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
    refetchInterval: POLL.fast,
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
  term: string, type: '' | 'CLIENT' | 'SERVER' | 'CLIENT_EVENT' = 'CLIENT', enabled = true) {
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
