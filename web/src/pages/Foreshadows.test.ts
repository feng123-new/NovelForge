
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';

const projectPage = {
  projects: [{
    id: 'p1', title: 'Sky Road', status: 'active', archived: false,
    current_chapter: 135, completed_chapters: 134, total_chapters: 300,
    total_words: 470000, updated_at: '2026-09-02T00:00:00Z'
  }],
  total: 1, limit: 100, offset: 0
};

const foreshadow = {
  id: 'sealed-gate', project_id: 'p1', title: 'The sealed gate',
  description: 'The gate must reopen.', importance: 'critical',
  planted_chapter: 20, expected_payoff_min: 100, expected_payoff_max: 130,
  actual_payoff: null, status: 'planted', related_entities: ['hero'],
  related_arcs: ['arc-2'], last_progress_chapter: 20, urgency: 'high',
  source_version: 'chapter-v1', authority: 'human_final', overdue: true,
  overdue_by_chapters: 5, created_at: '2026-09-02T00:00:00Z',
  updated_at: '2026-09-02T00:00:00Z'
} as const;

const dashboard = {
  chapter: 135, active_foreshadows: 1, overdue_count: 1,
  critical_overdue: 1, upcoming_payoffs: 0, unrevealed_secrets: 0,
  knowledge_boundary_warnings: 0
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' }
  });
}

function readFetcher(items: unknown[] = [foreshadow]) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url.startsWith('/api/projects?')) return json(projectPage);
    if (url.startsWith('/api/projects/p1/foreshadows?')) {
      return json({ foreshadows: items, total: items.length, limit: 100, offset: 0 });
    }
    if (url.includes('/ledger/dashboard')) return json(dashboard);
    if (url.includes('/ledger/diagnostics')) return json({ diagnostics: [], total: 0 });
    throw new Error(`unexpected request ${init?.method ?? 'GET'} ${url}`);
  });
}

describe('Foreshadows page', () => {
  afterEach(() => {
    window.location.hash = '';
    vi.unstubAllGlobals();
  });

  it('renders the authoritative empty state', async () => {
    vi.stubGlobal('fetch', readFetcher([]));
    const { default: Foreshadows } = await import('./Foreshadows.svelte');
    render(Foreshadows);
    expect((await screen.findAllByText('当前筛选没有伏笔')).length).toBeGreaterThan(0);
  });

  it('renders a structured server error', async () => {
    const fetcher = vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).startsWith('/api/projects?')) return json(projectPage);
      return json({ error: {
        code: 'LEDGER_UNAVAILABLE', message: 'ledger unavailable', details: {},
        retryable: true, trace_id: 'trace-ledger-page'
      } }, 503);
    });
    vi.stubGlobal('fetch', fetcher);
    const { default: Foreshadows } = await import('./Foreshadows.svelte');
    render(Foreshadows);
    expect(await screen.findByText('ledger unavailable')).toBeInTheDocument();
    expect(screen.getByText('LEDGER_UNAVAILABLE')).toBeInTheDocument();
    expect(screen.getByText('trace trace-ledger-page')).toBeInTheDocument();
  });

  it('creates a planted foreshadow through the typed API client', async () => {
    const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.startsWith('/api/projects?')) return json(projectPage);
      if (url === '/api/projects/p1/foreshadows' && init?.method === 'POST') return json(foreshadow, 201);
      if (url.startsWith('/api/projects/p1/foreshadows?')) return json({ foreshadows: [], total: 0, limit: 100, offset: 0 });
      if (url.includes('/ledger/dashboard')) return json({ ...dashboard, active_foreshadows: 0, overdue_count: 0, critical_overdue: 0 });
      if (url.includes('/ledger/diagnostics')) return json({ diagnostics: [], total: 0 });
      throw new Error(`unexpected request ${init?.method ?? 'GET'} ${url}`);
    });
    vi.stubGlobal('fetch', fetcher);
    const { default: Foreshadows } = await import('./Foreshadows.svelte');
    render(Foreshadows);
    await screen.findByText('当前筛选没有伏笔');
    await fireEvent.input(screen.getByLabelText('标题'), { target: { value: 'The sealed gate' } });
    await fireEvent.input(screen.getByLabelText('Description'), { target: { value: 'The gate must reopen.' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Create' }));
    expect(await screen.findByText('伏笔已写入权威 Narrative Ledger')).toBeInTheDocument();

    const call = fetcher.mock.calls.find(([input, init]) => String(input) === '/api/projects/p1/foreshadows' && init?.method === 'POST');
    expect(call).toBeTruthy();
    expect(JSON.parse(String(call?.[1]?.body))).toMatchObject({
      title: 'The sealed gate', status: 'planted', planted_chapter: 135,
      expected_payoff_min: 136, expected_payoff_max: 138,
      source_version: 'human-v1'
    });
    expect(new Headers(call?.[1]?.headers).get('Idempotency-Key')).toMatch(/^web-/);
  });

  it('disables lifecycle writes while pending and reloads after resolve', async () => {
    let releasePatch!: (response: Response) => void;
    const patchResponse = new Promise<Response>((resolve) => { releasePatch = resolve; });
    const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.startsWith('/api/projects?')) return json(projectPage);
      if (url.includes('/ledger/dashboard')) return json(dashboard);
      if (url.includes('/ledger/diagnostics')) return json({ diagnostics: [], total: 0 });
      if (url.startsWith('/api/projects/p1/foreshadows?')) return json({ foreshadows: [foreshadow], total: 1, limit: 100, offset: 0 });
      if (url === '/api/projects/p1/foreshadows/sealed-gate' && init?.method === 'PATCH') return patchResponse;
      throw new Error(`unexpected request ${init?.method ?? 'GET'} ${url}`);
    });
    vi.stubGlobal('fetch', fetcher);
    const { default: Foreshadows } = await import('./Foreshadows.svelte');
    render(Foreshadows);
    expect(await screen.findByText('OVERDUE +5')).toBeInTheDocument();
    const resolveButton = screen.getByRole('button', { name: 'Resolve' });
    await fireEvent.click(resolveButton);
    await waitFor(() => expect(resolveButton).toBeDisabled());

    const call = fetcher.mock.calls.find(([input, init]) => String(input).endsWith('/foreshadows/sealed-gate') && init?.method === 'PATCH');
    expect(JSON.parse(String(call?.[1]?.body))).toMatchObject({
      status: 'resolved', actual_payoff: 135, chapter: 135,
      reason: 'human resolved'
    });
    releasePatch(json({ ...foreshadow, status: 'resolved', actual_payoff: 135, overdue: false, overdue_by_chapters: 0 }));
    expect(await screen.findByText('The sealed gate → resolved')).toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole('button', { name: 'Resolve' })).not.toBeDisabled());
  });
});
