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

export type ForeshadowStatus = 'planned' | 'planted' | 'progressing' | 'resolved' | 'abandoned' | 'contradicted';
export type LedgerImportance = 'low' | 'medium' | 'high' | 'critical';
export type LedgerUrgency = 'low' | 'normal' | 'high' | 'critical';
export type SecretPublicStatus = 'private' | 'public';

export interface Foreshadow {
  id: string;
  project_id: string;
  title: string;
  description: string;
  importance: LedgerImportance;
  planted_chapter: number;
  expected_payoff_min: number;
  expected_payoff_max: number;
  actual_payoff?: number | null;
  status: ForeshadowStatus;
  related_entities: string[];
  related_arcs: string[];
  last_progress_chapter: number;
  urgency: LedgerUrgency;
  source_version: string;
  authority: string;
  overdue: boolean;
  overdue_by_chapters: number;
  created_at: string;
  updated_at: string;
}

export interface ForeshadowInput {
  id?: string;
  title: string;
  description: string;
  importance: LedgerImportance;
  planted_chapter: number;
  expected_payoff_min: number;
  expected_payoff_max: number;
  actual_payoff?: number | null;
  status: ForeshadowStatus;
  related_entities: string[];
  related_arcs: string[];
  last_progress_chapter: number;
  urgency: LedgerUrgency;
  source_version: string;
}

export interface ForeshadowPatch {
  title?: string;
  description?: string;
  importance?: LedgerImportance;
  expected_payoff_min?: number;
  expected_payoff_max?: number;
  actual_payoff?: number;
  clear_actual_payoff?: boolean;
  status?: ForeshadowStatus;
  last_progress_chapter?: number;
  urgency?: LedgerUrgency;
  source_version?: string;
  chapter: number;
  reason: string;
}

export interface ForeshadowPage {
  foreshadows: Foreshadow[];
  total: number;
  limit: number;
  offset: number;
  next_offset?: number;
}

export interface SecretHolder {
  secret_id: string;
  entity_id: string;
  valid_from_chapter: number;
  valid_to_chapter?: number | null;
  source_version: string;
  authority: string;
  provenance: { type: string; id: string; chapter: number; version: string };
}

export interface SecretRecord {
  id: string;
  project_id: string;
  description: string;
  truth?: string;
  created_chapter: number;
  revealed_chapter?: number | null;
  public_status: SecretPublicStatus;
  related_foreshadow?: string;
  source_version: string;
  authority: string;
  holders: SecretHolder[];
  public_at_chapter: boolean;
  created_at: string;
  updated_at: string;
}

export interface SecretInput {
  id?: string;
  description: string;
  truth: string;
  created_chapter: number;
  revealed_chapter?: number | null;
  public_status: SecretPublicStatus;
  related_foreshadow?: string;
  source_version: string;
  holders?: Array<{
    entity_id: string;
    valid_from_chapter: number;
    valid_to_chapter?: number | null;
    source_version: string;
    authority: string;
    provenance: { type: string; id: string; chapter: number; version: string };
  }>;
}

export interface SecretPatch {
  description?: string;
  truth?: string;
  revealed_chapter?: number;
  clear_revealed_chapter?: boolean;
  public_status?: SecretPublicStatus;
  related_foreshadow?: string;
  source_version?: string;
  chapter: number;
  reason: string;
}

export interface SecretPage {
  secrets: SecretRecord[];
  total: number;
  limit: number;
  offset: number;
  next_offset?: number;
}

export interface LedgerDashboard {
  chapter: number;
  active_foreshadows: number;
  overdue_count: number;
  critical_overdue: number;
  upcoming_payoffs: number;
  unrevealed_secrets: number;
  knowledge_boundary_warnings: number;
}

export interface LedgerDiagnostic {
  id: string;
  code: string;
  severity: string;
  project: string;
  chapter: number;
  entity: string;
  message: string;
  retryable: boolean;
  evidence: Record<string, unknown>;
}

export interface LedgerDiagnosticPage {
  diagnostics: LedgerDiagnostic[];
  total: number;
}

export interface LedgerPlannerItem {
  id: string;
  kind: string;
  title: string;
  summary: string;
  mandatory: boolean;
  importance?: LedgerImportance;
  urgency?: LedgerUrgency;
  source_chapter: number;
  source_version: string;
  authority: string;
  metadata: Record<string, unknown>;
}

export interface LedgerPlannerContext {
  project_id: string;
  chapter: number;
  pov?: string;
  foreshadows: LedgerPlannerItem[];
  known_secrets: LedgerPlannerItem[];
  unknown_secret_boundaries: LedgerPlannerItem[];
}
