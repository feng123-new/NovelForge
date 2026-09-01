import type {
  APIErrorPayload,
  ChapterList,
  CreateProjectInput,
  DeleteProjectResult,
  FoundationRequest,
  FoundationRequestInput,
  Health,
  ModelList,
  ProjectDetail,
  ProjectList,
  WorkspaceSettings
} from './types';

export class APIClientError extends Error {
  readonly status: number;
  readonly payload?: APIErrorPayload;

  constructor(message: string, status: number, payload?: APIErrorPayload) {
    super(message);
    this.name = 'APIClientError';
    this.status = status;
    this.payload = payload;
  }
}

type FetchLike = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>;

export class APIClient {
  constructor(
    private readonly baseURL = '/api',
    private readonly fetcher: FetchLike = fetch
  ) {}

  health(): Promise<Health> {
    return this.request('/health');
  }

  listProjects(query = ''): Promise<ProjectList> {
    const params = new URLSearchParams({ limit: '100', archived: 'all' });
    if (query.trim()) params.set('query', query.trim());
    return this.request(`/projects?${params.toString()}`);
  }

  getProject(id: string): Promise<ProjectDetail> {
    return this.request(`/projects/${encodeURIComponent(id)}`);
  }

  createProject(input: CreateProjectInput): Promise<ProjectDetail> {
    return this.write('/projects', 'POST', input);
  }

  archiveProject(id: string, archived: boolean): Promise<ProjectDetail> {
    const operation = archived ? 'archive' : 'unarchive';
    return this.write(`/projects/${encodeURIComponent(id)}/${operation}`, 'POST', {});
  }

  duplicateProject(id: string, title?: string): Promise<ProjectDetail> {
    return this.write(`/projects/${encodeURIComponent(id)}/duplicate`, 'POST', title ? { title } : {});
  }

  deleteProject(id: string, confirm: string): Promise<DeleteProjectResult> {
    return this.write(`/projects/${encodeURIComponent(id)}`, 'DELETE', { confirm, permanent: false });
  }

  listChapters(id: string): Promise<ChapterList> {
    return this.request(`/projects/${encodeURIComponent(id)}/chapters?limit=100`);
  }

  listModels(query = ''): Promise<ModelList> {
    const params = new URLSearchParams({ limit: '100' });
    if (query.trim()) params.set('query', query.trim());
    return this.request(`/models?${params.toString()}`);
  }

  settings(): Promise<WorkspaceSettings> {
    return this.request('/settings');
  }

  requestFoundation(id: string, input: FoundationRequestInput): Promise<FoundationRequest> {
    return this.write(`/projects/${encodeURIComponent(id)}/foundation`, 'POST', input);
  }

  getFoundation(id: string): Promise<FoundationRequest> {
    return this.request(`/projects/${encodeURIComponent(id)}/foundation`);
  }

  private async write<T>(path: string, method: string, body: unknown): Promise<T> {
    return this.request<T>(path, {
      method,
      headers: {
        'Content-Type': 'application/json',
        'Idempotency-Key': createIdempotencyKey()
      },
      body: JSON.stringify(body)
    });
  }

  private async request<T>(path: string, init?: RequestInit): Promise<T> {
    const response = await this.fetcher(`${this.baseURL}${path}`, {
      credentials: 'same-origin',
      ...init,
      headers: {
        Accept: 'application/json',
        ...(init?.headers ?? {})
      }
    });
    const text = await response.text();
    let data: unknown = undefined;
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

export function createIdempotencyKey(): string {
  const cryptoObject = globalThis.crypto;
  if (cryptoObject && typeof cryptoObject.randomUUID === 'function') {
    return `web-${cryptoObject.randomUUID()}`;
  }
  return `web-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 14)}`;
}

export const api = new APIClient();
