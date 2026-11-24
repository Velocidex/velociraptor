// Helpers for everything that leaves the server as a file: prepared
// export archives and individual uploaded files.
import { href } from './api';
import type { AvailableDownloadFile } from './types';
import { toNum } from './format';

export type ArchiveState = 'ready' | 'preparing' | 'failed';

export function archiveState(f: AvailableDownloadFile): ArchiveState {
  const err = f.stats?.error;
  if (err && err !== 'Complete') return 'failed';
  if (f.complete || err === 'Complete') return 'ready';
  return 'preparing';
}

export function archiveHref(f: AvailableDownloadFile): string {
  return href('/api/v1/DownloadVFSFile', {
    fs_components: f.stats?.components ?? f.components,
    vfs_path: f.path || f.name,
  });
}

// Seconds since epoch the archive was started.
export function archiveTime(f: AvailableDownloadFile): number {
  return toNum(f.stats?.timestamp);
}

// Newest first.
export function sortArchives(files: AvailableDownloadFile[]): AvailableDownloadFile[] {
  return [...files].sort((a, b) => archiveTime(b) - archiveTime(a));
}

// Filestore path components for an uploaded file, as the server
// reports them in results rows or the uploads table.
export function uploadComponents(
  components: string[] | undefined, clientId: string, flowId: string, accessor = 'auto'): string[] {
  if (!components || components.length === 0) return [accessor];
  switch (components[0]) {
    case 'clients': case 'downloads': case 'notebooks': case 'hunts':
      return components;
    case 'uploads':
      return ['clients', clientId, 'collections', flowId, ...components];
  }
  return ['clients', clientId, 'collections', flowId, 'uploads', accessor, ...components];
}

export function uploadHref(p: {
  clientId: string; flowId: string; components?: string[]; accessor?: string; path: string;
  lock?: boolean;
}): string {
  let path = p.path;
  let padding = false;
  if (path.endsWith('.idx')) {
    path = path.slice(0, -4);
    padding = true;
  }
  return href('/api/v1/DownloadVFSFile', {
    client_id: p.clientId,
    fs_components: uploadComponents(p.components, p.clientId, p.flowId, p.accessor),
    vfs_path: path,
    padding,
    zip: !!p.lock,
  });
}

// An "Upload" object as it appears inside artifact result rows.
export interface UploadValue {
  Path?: string;
  StoredName?: string;
  Size?: number;
  StoredSize?: number;
  sha256?: string;
  md5?: string;
  Components?: string[];
  Accessor?: string;
}

export function isUploadValue(v: unknown): v is UploadValue {
  if (!v || typeof v !== 'object' || Array.isArray(v)) return false;
  const o = v as Record<string, unknown>;
  return Array.isArray(o.Components) && ('StoredSize' in o || 'StoredName' in o);
}

// Starts a browser download without navigating away.
export function triggerDownload(url: string) {
  const a = document.createElement('a');
  a.href = url;
  a.rel = 'noopener';
  a.download = '';
  document.body.appendChild(a);
  a.click();
  a.remove();
}
