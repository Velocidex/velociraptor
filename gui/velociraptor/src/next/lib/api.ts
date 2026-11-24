// Thin, typed HTTP client for the Velociraptor gRPC gateway (/api/v1/*).
//
// URL building, org header and base path handling mirror the legacy
// api-service so both UIs can run side by side against the same server.
import axios, { AxiosError } from 'axios';

// A private instance: the legacy api-service installs axios-retry on the
// global axios, which silently delays every 5xx by several seconds. Retries
// for the new UI are handled by TanStack Query instead.
const http = axios.create();
import qs from 'qs';
import legacyApi from '../../components/core/api-service.jsx';

declare global {
  interface Window {
    globals: {
      OrgId?: string;
      lang?: string;
      theme?: string;
      base_path?: string;
      AuthRedirectTemplate?: string;
      [k: string]: unknown;
    };
    base_path?: string;
    ErrorState?: { Type: string; [k: string]: unknown };
  }
}

export class ApiError extends Error {
  status?: number;
  constructor(message: string, status?: number) {
    super(message);
    this.status = status;
  }
}

function basePath(): string {
  return (legacyApi as { base_path: () => string }).base_path();
}

function apiUrl(path: string): string {
  const parsed = new URL(window.location.href);
  const base = basePath().replace(/\/+$/, '');
  parsed.pathname = `${base}/api/${path.replace(/^\/+/, '')}`;
  parsed.hash = '';
  parsed.search = '';
  return parsed.href;
}

export function currentOrgId(): string {
  let org = window.globals?.OrgId || '';
  if (org.startsWith('{{')) org = '';
  return org || 'root';
}

function headers() {
  return { 'Grpc-Metadata-OrgId': currentOrgId() };
}

function toApiError(err: unknown): never {
  if (axios.isCancel(err)) {
    throw err;
  }
  const e = err as AxiosError<{ message?: string } | string>;
  const status = e.response?.status;

  if (status === 401) {
    const tpl = window.globals?.AuthRedirectTemplate || '';
    if (tpl) {
      window.location.assign(
        tpl.replaceAll('%LOCATION%', encodeURIComponent(window.location.href)));
    }
  }

  const data = e.response?.data;
  let message = e.message;
  if (data && typeof data === 'object' && data.message) {
    message = data.message;
  } else if (typeof data === 'string' && data && !data.trimStart().startsWith('<')) {
    message = data;
  }
  throw new ApiError(message, status);
}

export async function get<T>(
  path: string, params?: Record<string, unknown>, signal?: AbortSignal): Promise<T> {
  try {
    const resp = await http.get<T>(apiUrl(path), {
      params,
      paramsSerializer: { serialize: p => qs.stringify(p, { indices: false }) },
      headers: headers(),
      signal,
    });
    return resp.data;
  } catch (err) {
    return toApiError(err);
  }
}

export async function post<T>(
  path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
  try {
    const resp = await http.post<T>(apiUrl(path), body ?? {}, {
      headers: headers(),
      signal,
    });
    return resp.data;
  } catch (err) {
    return toApiError(err);
  }
}

// Builds an href to a server-side resource (downloads etc).
export function href(path: string, params?: Record<string, unknown>): string {
  return (legacyApi as { href: (u: string, p?: unknown, o?: unknown) => string })
    .href(path, params, { internal: true });
}

export function srcOf(path: string): string {
  return (legacyApi as { src_of: (u: string) => string }).src_of(path);
}
