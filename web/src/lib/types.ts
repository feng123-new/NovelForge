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

export interface ChapterPlan {
  chapter: number;
  title: string;
  pov: string;
  location: string;
  objective: string;
  conflict: string;
  required_beats: string[];
  forbidden_outcomes: string[];
  knowledge_boundary: string[];
  inventory_constraints: string[];
  foreshadow_obligations: string[];
  ending_hook: string;
}

export type QualityState =
  | 'planned' | 'drafting' | 'draft_ready' | 'librarian_pending' | 'facts_proposed'
  | 'continuity_pending' | 'continuity_pass' | 'continuity_warn' | 'continuity_fail'
  | 'editor_pending' | 'reviewed' | 'rewrite_pending' | 'final_candidate'
  | 'truth_commit_pending' | 'checkpoint_pending' | 'completed' | 'hold' | 'failed';

export interface QualityTransaction {
  transaction_id?: string;
  project_id?: string;
  chapter?: number;
  state?: QualityState;
  attempt?: number;
  max_rewrites?: number;
  quality_threshold?: number;
  final_candidate_id?: string;
  hold_reason?: string;
  last_reason?: string;
  created_at?: string;
  updated_at?: string;
}

export interface QualityCandidate {
  id: string;
  transaction_id: string;
  chapter: number;
  attempt: number;
  text_sha: string;
  source_version: string;
  continuity_status: '' | 'PASS' | 'WARN' | 'FAIL';
  editor_score?: number | null;
  selected: boolean;
  selection_reason: string;
  created_at: string;
}

export interface ContinuityIssue {
  issue_code: string;
  severity: 'INFO' | 'WARNING' | 'BLOCKING';
  entity: string;
  predicate: string;
  expected: unknown;
  actual: unknown;
  evidence: string;
  source_chapter: number;
  source_version: string;
  suggested_action: string;
}

export interface ContinuityResult {
  status: 'PASS' | 'WARN' | 'FAIL';
  blocking: boolean;
  issues: ContinuityIssue[];
}

export interface EditorReview {
  score: number;
  strengths: string[];
  weaknesses: string[];
  line_level_issues: string[];
  pacing: string;
  characterization: string;
  prose: string;
  dialogue: string;
  ending: string;
  rewrite_recommended: boolean;
  summary: string;
}

export interface QualitySnapshot {
  transaction: QualityTransaction;
  candidates: QualityCandidate[];
  proposal?: Record<string, unknown>;
  continuity?: ContinuityResult;
  editor?: EditorReview;
  state_changes: Array<Record<string, unknown>>;
}

export interface QualityActions {
  generate: boolean;
  check: boolean;
  rewrite: boolean;
  finalize: boolean;
}

export interface QualityView {
  snapshot: QualitySnapshot;
  actions: QualityActions;
}

export interface QualityCandidateList {
  candidates: QualityCandidate[];
  total: number;
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
