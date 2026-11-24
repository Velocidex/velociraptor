// Shapes of the API objects the new UI consumes. The gRPC gateway
// serialises 64 bit integers as strings, so numeric fields are typed
// as Num and converted with toNum().
export type Num = string | number | undefined;

export type HuntState = 'UNSET' | 'PAUSED' | 'RUNNING' | 'STOPPED' | 'ARCHIVED' | 'DELETED';

export interface HuntStats {
  total_clients_scheduled?: Num;
  total_clients_with_results?: Num;
  total_clients_without_results?: Num;
  total_clients_with_errors?: Num;
  total_uploaded_bytes?: Num;
  total_collected_rows?: Num;
  total_finished_clients?: Num;
  stopped?: boolean;
  available_downloads?: { files?: AvailableDownloadFile[] };
}

export interface AvailableDownloadFile {
  name: string;
  type?: string;
  path?: string;
  size?: Num;
  date?: string;
  complete?: boolean;
  stats?: { total_compressed_bytes?: Num; total_uncompressed_bytes?: Num; error?: string };
  components?: string[];
}

export interface ArtifactParameter {
  key: string;
  value: string;
}

export interface ArtifactSpec {
  artifact: string;
  parameters?: { env?: ArtifactParameter[] };
}

export interface ArtifactCollectorArgs {
  creator?: string;
  artifacts?: string[];
  specs?: ArtifactSpec[];
  timeout?: Num;
  max_rows?: Num;
  max_upload_bytes?: Num;
  cpu_limit?: number;
  progress_timeout?: number;
  urgent?: boolean;
  trace_freq_sec?: Num;
  compiled_collector_args?: unknown[];
}

export interface HuntCondition {
  labels?: { label?: string[] };
  excluded_labels?: { label?: string[] };
  os?: { os?: string };
}

export interface Hunt {
  hunt_id: string;
  version?: Num;
  create_time?: Num;
  start_time?: Num;
  expires?: Num;
  creator?: string;
  hunt_description?: string;
  tags?: string[];
  state?: HuntState;
  stats?: HuntStats;
  artifacts?: string[];
  artifact_sources?: string[];
  start_request?: ArtifactCollectorArgs;
  condition?: HuntCondition;
  client_limit?: Num;
  org_ids?: string[];
}

export interface ListHuntsResponse {
  total?: Num;
  items?: Hunt[];
}

export interface TableResponse {
  columns?: string[];
  rows?: { json?: string; cell?: string[] }[];
  total_rows?: Num;
  column_types?: { name: string; type: string }[];
}

export interface ApiClient {
  client_id: string;
  agent_information?: { version?: string; name?: string; build_time?: string };
  os_info?: {
    system?: string;
    hostname?: string;
    release?: string;
    machine?: string;
    fqdn?: string;
    mac_addresses?: string[];
  };
  first_seen_at?: Num;
  last_seen_at?: Num;
  last_ip?: string;
  labels?: string[];
  in_flight_flows?: Record<string, Num>;
  last_interrogate_flow_id?: string;
}

export interface SearchClientsResponse {
  items?: ApiClient[];
  names?: string[];
  total?: Num;
}

export interface ArtifactParameterDef {
  name: string;
  default?: string;
  description?: string;
  type?: string;
  friendly_name?: string;
  choices?: string[];
  validating_regex?: string;
}

export interface ArtifactDescriptor {
  name: string;
  description?: string;
  type?: string;
  author?: string;
  parameters?: ArtifactParameterDef[];
  sources?: { name?: string; query?: string }[];
  built_in?: boolean;
  required_permissions?: string[];
  resources?: { timeout?: Num; max_rows?: Num; max_upload_bytes?: Num; cpu_limit?: number };
}

export interface GetArtifactsResponse {
  items?: ArtifactDescriptor[];
}

export interface Flow {
  session_id: string;
  client_id?: string;
  state?: string;
  create_time?: Num;
  start_time?: Num;
  active_time?: Num;
  total_collected_rows?: Num;
  total_uploaded_bytes?: Num;
  total_uploaded_files?: Num;
  request?: ArtifactCollectorArgs;
  artifacts_with_results?: string[];
  status?: string;
  backtrace?: string;
}

export interface UserTraits {
  username?: string;
  theme?: string;
  timezone?: string;
  lang?: string;
  org?: string;
  org_name?: string;
  orgs?: { id: string; name: string }[];
  picture?: string;
  ui_settings?: string;
  default_password?: string;
  Permissions?: Record<string, boolean>;
  customizations?: Record<string, string>;
  links?: { text?: string; url?: string; icon_url?: string; type?: string }[];
}
