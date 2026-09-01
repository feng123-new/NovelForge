export interface APIErrorPayload {
  code: string;
  message: string;
  details: Record<string, unknown>;
  retryable: boolean;
  trace_id: string;
}

export interface Health {
  product: string;
  status: string;
  version: string;
  api_version: string;
  workspace: string;
  started_at: string;
  uptime_seconds: number;
}

export interface ProjectSummary {
  id: string;
  title: string;
  path?: string;
  status: 'active' | 'archived';
  archived: boolean;
  phase?: string;
  current_chapter: number;
  completed_chapters: number;
  total_chapters: number;
  total_words: number;
  current_volume?: number;
  current_arc?: number;
  format_version?: number;
  updated_at: string;
  warnings?: string[];
}

export interface ProjectDetail extends ProjectSummary {
  synopsis?: string;
  genre?: string;
  language?: string;
  target_words?: number;
  words_per_chapter?: number;
  created_at?: string;
  archived_at?: string | null;
  source_format?: string;
}

export interface DeleteProjectResult {
  id: string;
  deleted: boolean;
  permanent: boolean;
}

export interface ProjectList {
  projects: ProjectSummary[];
  total: number;
  limit: number;
  offset: number;
  next_offset?: number;
}

export interface CreateProjectInput {
  title: string;
  slug?: string;
  synopsis?: string;
  genre?: string;
  language?: string;
  target_words?: number;
  target_chapters?: number;
  words_per_chapter?: number;
}

export interface ChapterSummary {
  chapter: number;
  title: string;
  status: string;
  character_count: number;
  updated_at: string;
  truncated?: boolean;
}

export interface ChapterList {
  chapters: ChapterSummary[];
  total: number;
  limit: number;
  offset: number;
  next_offset?: number;
}

export interface ModelEntry {
  provider: string;
  id: string;
  name: string;
  context_window: number;
  max_tokens: number;
  input_cost_per_1m: number;
  output_cost_per_1m: number;
}

export interface ModelList {
  models: ModelEntry[];
  total: number;
  limit: number;
  offset: number;
  next_offset?: number;
}

export interface WorkspaceSettings {
  product: string;
  version: string;
  api_version: string;
  workspace: string;
  listen_host: string;
  listen_port: number;
  loopback_only: boolean;
  theme_storage: string;
  request_limits: Record<string, number>;
  capabilities: Record<string, boolean | string | number>;
}

export interface AutomationSettings {
  mode: 'copilot' | 'autopilot';
  review_policy: 'every_chapter' | 'every_n' | 'full_automatic';
  review_every_n?: number;
  max_rewrites: number;
  worker_available?: boolean;
}

export interface FoundationRequestInput {
  idea: string;
  style?: string;
  model_profile?: Record<string, string>;
  automation: AutomationSettings;
}

export interface FoundationRequest extends FoundationRequestInput {
  id: string;
  project_id: string;
  status: 'requested';
  created_at: string;
}

export interface WorkspaceEvent {
  id?: number;
  type: string;
  time: string;
  project?: string;
  data?: unknown;
}
