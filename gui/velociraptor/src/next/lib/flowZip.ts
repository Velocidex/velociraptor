// App-wide manager for "build this collection's zip, then download it".
// It lives outside React so a download in progress survives re-renders,
// table virtualization and navigating to another page.
import { useSyncExternalStore } from 'react';
import { toast } from 'sonner';
import * as api from './api';
import type { FlowDetails } from './types';
import { archiveHref, archiveState, archiveTime, sortArchives, triggerDownload } from './downloads';
import { fmtBytes } from './format';
import { queryClient } from './queries';

const pending = new Set<string>();
const listeners = new Set<() => void>();
let version = 0;

function emit() {
  version++;
  listeners.forEach(l => l());
}

const keyOf = (clientId: string, flowId: string) => `${clientId}/${flowId}`;

export function useFlowZipBusy(clientId: string, flowId: string): boolean {
  useSyncExternalStore(
    cb => { listeners.add(cb); return () => listeners.delete(cb); },
    () => version);
  return pending.has(keyOf(clientId, flowId));
}

const sleep = (ms: number) => new Promise(r => setTimeout(r, ms));

export async function downloadFlowZip(clientId: string, flowId: string, opts: { password?: string; label?: string } = {}) {
  const key = keyOf(clientId, flowId);
  if (pending.has(key)) return;
  pending.add(key);
  emit();
  const tid = toast.loading(`Building zip for ${opts.label || flowId}…`);
  try {
    const since = Math.floor(Date.now() / 1000) - 2;
    await api.post('v1/CreateDownload', { client_id: clientId, flow_id: flowId, password: opts.password || '' });
    const deadline = Date.now() + 30 * 60 * 1000;
    let delay = 700;
    while (Date.now() < deadline) {
      await sleep(delay);
      delay = Math.min(delay * 1.3, 3000);
      const d = await api.get<FlowDetails>('v1/GetFlowDetails', { client_id: clientId, flow_id: flowId });
      queryClient.setQueryData(['flow', clientId, flowId], d);
      const fresh = sortArchives(d.available_downloads?.files ?? []).find(f => archiveTime(f) >= since);
      if (!fresh) continue;
      const st = archiveState(fresh);
      if (st === 'ready') {
        triggerDownload(archiveHref(fresh));
        toast.success(`Downloading ${fresh.name}.zip`, {
          id: tid, description: fmtBytes(fresh.size ?? fresh.stats?.total_compressed_bytes),
        });
        return;
      }
      if (st === 'failed') throw new Error(fresh.stats?.error || 'Export failed');
    }
    throw new Error('Timed out waiting for the zip');
  } catch (e) {
    toast.error(`Could not build the zip: ${(e as Error).message}`, { id: tid });
  } finally {
    pending.delete(key);
    emit();
  }
}
