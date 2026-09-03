import { APIClientError, createIdempotencyKey } from './api';
import type { APIErrorPayload } from './types';

export type ChapterVersionType = 'draft' | 'continuity_fix' | 'editor_revision' | 'human_revision' | 'final' | 'rejected';
export type ChapterAuthorType = 'writer' | 'librarian' | 'editor' | 'human' | 'restore' | 'system';
export type ChapterDiffMode = 'inline' | 'side_by_side';

export interface ChapterVersion {
  id: string;
  project_id: string;
  chapter: number;
  version_number: number;
  type: ChapterVersionType;
  status: string;
  content?: string;
  content_sha: string;
  parent_version?: string;
  author_type: ChapterAuthorType;
  provider?: string;
  model?: string;
  prompt_hash?: string;
  review?: Record<string, unknown>;
  continuity?: Record<string, unknown>;
  provenance?: Record<string, unknown>;
  created_at: string;
  accepted: boolean;
  rejected: boolean;
  active_final: boolean;
  authority?: string;
  rejection_reason?: string;
}

export interface ChapterVersionPage {
  versions: ChapterVersion[];
  total: number;
  limit: number;
  offset: number;
  next_offset?: number;
}

export interface ChapterSyncStatus {
  project_id: string;
  chapter: number;
  active_version_id?: string;
  expected_sha?: string;
  observed_sha?: string;
  observed_at?: string;
  sync_required: boolean;
}

export interface ChapterVersionView {
  project_id: string;
  chapter: number;
  active_final?: ChapterVersion | null;
  latest?: ChapterVersion | null;
  version_count: number;
  sync: ChapterSyncStatus;
  derived_state: string;
}

export interface DiffLine {
  kind?: string;
  old_line?: number;
  new_line?: number;
  old_text?: string;
  new_text?: string;
}

export interface DiffHunk {
  old_start: number;
  old_lines: number;
  new_start: number;
  new_lines: number;
  additions: number;
  deletions: number;
  unchanged: number;
  lines: DiffLine[];
}

export interface ChapterDiff {
  from_version: string;
  to_version: string;
  from_sha: string;
  to_sha: string;
  mode: ChapterDiffMode;
  hunks: DiffHunk[];
  additions: number;
  deletions: number;
  unchanged: number;
  truncated: boolean;
  next_cursor?: string;
}

export interface DerivedStateRebuild {
  operation_id?: string;
  project_id?: string;
  boundary_chapter: number;
  source_version?: string;
  status: string;
  current_step?: string;
  affected?: Record<string, unknown>;
  before_digest?: string;
  after_digest?: string;
  started_at?: string;
  completed_at?: string;
  error_code?: string;
}

export interface ChapterPlanImpact {
  id: string;
  source_version: string;
  plan_id: string;
  chapter: number;
  severity: string;
  affected_fact: string;
  previous_assumption: string;
  new_truth: string;
  action_required: string;
  reason: string;
  created_at: string;
}

export interface ChapterPlanImpactPage {
  impacts: ChapterPlanImpact[];
  total: number;
  limit: number;
  offset: number;
  next_offset?: number;
}

export interface ChapterEvaluation {
  version_id?: string;
  proposal?: Record<string, unknown>;
  review?: Record<string, unknown>;
  continuity?: { status?: string; blocking?: boolean; issues?: unknown[] };
  conflicts?: Array<Record<string, unknown>>;
  evaluated_at?: string;
}

export interface ChapterFinalizeResult {
  version: ChapterVersion;
  active_final: ChapterVersion;
  operation_id: string;
  truth_events: number;
  rebuild_status: string;
}

export interface ChapterSyncResult {
  version: ChapterVersion;
  proposal?: Record<string, unknown>;
  continuity?: { status?: string; blocking?: boolean; issues?: unknown[] };
  review?: Record<string, unknown>;
  conflicts: number;
  sync_required: boolean;
}

type FetchLike = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>;

export class ChapterVersionAPI {
  constructor(
    private readonly baseURL = '/api',
    private readonly fetcher?: FetchLike
  ) {}

  state(project: string, chapter: number): Promise<ChapterVersionView> {
    return this.request(this.chapterPath(project, chapter));
  }

  list(project: string, chapter: number, offset = 0): Promise<ChapterVersionPage> {
    return this.request(`${this.chapterPath(project, chapter)}/versions?limit=100&offset=${offset}&include_content=false`);
  }

  get(project: string, chapter: number, version: string): Promise<ChapterVersion> {
    return this.request(`${this.chapterPath(project, chapter)}/versions/${encodeURIComponent(version)}`);
  }

  saveHuman(project: string, chapter: number, content: string): Promise<ChapterVersion> {
    return this.write(`${this.chapterPath(project, chapter)}/versions`, { content });
  }

  check(project: string, chapter: number, version: string): Promise<{ evaluation: ChapterEvaluation }> {
    return this.write(`${this.versionPath(project, chapter, version)}/check`, {});
  }

  restore(project: string, chapter: number, version: string): Promise<{ version: ChapterVersion }> {
    return this.write(`${this.versionPath(project, chapter, version)}/restore`, {});
  }

  accept(project: string, chapter: number, version: string, reason = ''): Promise<{ version: ChapterVersion }> {
    return this.write(`${this.versionPath(project, chapter, version)}/accept`, reason ? { reason } : {});
  }

  reject(project: string, chapter: number, version: string, reason: string): Promise<{ version: ChapterVersion }> {
    return this.write(`${this.versionPath(project, chapter, version)}/reject`, { reason });
  }

  finalize(project: string, chapter: number, version: string): Promise<ChapterFinalizeResult> {
    return this.write(`${this.versionPath(project, chapter, version)}/finalize`, {});
  }

  syncStatus(project: string, chapter: number): Promise<ChapterSyncStatus> {
    return this.request(`${this.chapterPath(project, chapter)}/sync-status`);
  }

  sync(project: string, chapter: number, observedSHA: string): Promise<ChapterSyncResult> {
    return this.write(`${this.chapterPath(project, chapter)}/sync`, observedSHA ? { observed_sha: observedSHA } : {});
  }

  rebuild(project: string, chapter: number): Promise<DerivedStateRebuild> {
    return this.request(`${this.chapterPath(project, chapter)}/rebuild`);
  }

  planImpact(project: string, chapter: number): Promise<ChapterPlanImpactPage> {
    return this.request(`${this.chapterPath(project, chapter)}/plan-impact?limit=100&offset=0`);
  }

  diff(project: string, chapter: number, fromVersion: string, toVersion: string, mode: ChapterDiffMode, cursor = ''): Promise<ChapterDiff> {
    const params = new URLSearchParams({
      from_version: fromVersion,
      to_version: toVersion,
      mode,
      limit: '200'
    });
    if (cursor) params.set('cursor', cursor);
    return this.request(`${this.chapterPath(project, chapter)}/diff?${params.toString()}`);
  }

  private chapterPath(project: string, chapter: number): string {
    return `/projects/${encodeURIComponent(project)}/chapters/${chapter}`;
  }

  private versionPath(project: string, chapter: number, version: string): string {
    return `${this.chapterPath(project, chapter)}/versions/${encodeURIComponent(version)}`;
  }

  private write<T>(path: string, body: unknown): Promise<T> {
    return this.request(path, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Idempotency-Key': createIdempotencyKey()
      },
      body: JSON.stringify(body)
    });
  }

  private async request<T>(path: string, init?: RequestInit): Promise<T> {
    const fetcher = this.fetcher ?? globalThis.fetch.bind(globalThis);
    const response = await fetcher(`${this.baseURL}${path}`, {
      credentials: 'same-origin',
      ...init,
      headers: {
        Accept: 'application/json',
        ...(init?.headers ?? {})
      }
    });
    const text = await response.text();
    let data: unknown;
    if (text) {
      try {
        data = JSON.parse(text);
      } catch {
        throw new APIClientError('Server returned invalid JSON', response.status);
      }
    }
    if (!response.ok) {
      const envelope = data as { error?: APIErrorPayload } | undefined;
      const payload = envelope?.error;
      throw new APIClientError(payload?.message ?? `Request failed with status ${response.status}`, response.status, payload);
    }
    return data as T;
  }
}

export const chapterVersions = new ChapterVersionAPI();

export function shortSHA(value?: string): string {
  return value ? value.slice(0, 12) : '—';
}

export function continuityStatus(version?: ChapterVersion | null): string {
  const value = version?.continuity as { status?: string } | undefined;
  return value?.status ?? 'PENDING';
}
