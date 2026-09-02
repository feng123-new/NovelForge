
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';

const projectPage = {
  projects: [{
    id: 'p1', title: 'Sky Road', status: 'active', archived: false,
    current_chapter: 50, completed_chapters: 49, total_chapters: 300,
    total_words: 170000, updated_at: '2026-09-02T00:00:00Z'
  }],
  total: 1, limit: 100, offset: 0
};

const secret = {
  id: 'heir-origin', project_id: 'p1', description: "The heir's origin",
  truth: 'The heir is from the old capital', created_chapter: 1,
  revealed_chapter: null, public_status: 'private', source_version: 'v1',
  authority: 'human_final', holders: [], public_at_chapter: false,
  created_at: '2026-09-02T00:00:00Z', updated_at: '2026-09-02T00:00:00Z'
} as const;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' }
  });
}

describe('Secrets page', () => {
  afterEach(() => {
    window.location.hash = '';
    vi.unstubAllGlobals();
  });

  it('renders the authoritative empty state', async () => {
    const fetcher = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.startsWith('/api/projects?')) return json(projectPage);
      if (url.startsWith('/api/projects/p1/secrets?')) return json({ secrets: [], total: 0, limit: 100, offset: 0 });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal('fetch', fetcher);
    const { default: Secrets } = await import('./Secrets.svelte');
    render(Secrets);
    expect((await screen.findAllByText('尚无 Secret')).length).toBeGreaterThan(0);
  });

  it('renders a structured server error', async () => {
    const fetcher = vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).startsWith('/api/projects?')) return json(projectPage);
      return json({ error: {
        code: 'SECRET_STORE_UNAVAILABLE', message: 'secret store unavailable', details: {},
        retryable: true, trace_id: 'trace-secret-page'
      } }, 503);
    });
    vi.stubGlobal('fetch', fetcher);
    const { default: Secrets } = await import('./Secrets.svelte');
    render(Secrets);
    expect(await screen.findByText('secret store unavailable')).toBeInTheDocument();
    expect(screen.getByText('SECRET_STORE_UNAVAILABLE')).toBeInTheDocument();
    expect(screen.getByText('trace trace-secret-page')).toBeInTheDocument();
  });

  it('creates a private Secret through a real idempotent write', async () => {
    const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.startsWith('/api/projects?')) return json(projectPage);
      if (url === '/api/projects/p1/secrets' && init?.method === 'POST') return json(secret, 201);
      if (url.startsWith('/api/projects/p1/secrets?')) return json({ secrets: [], total: 0, limit: 100, offset: 0 });
      throw new Error(`unexpected request ${init?.method ?? 'GET'} ${url}`);
    });
    vi.stubGlobal('fetch', fetcher);
    const { default: Secrets } = await import('./Secrets.svelte');
    render(Secrets);
    await screen.findByText('尚无 Secret');
    await fireEvent.input(screen.getByLabelText('Description'), { target: { value: "The heir's origin" } });
    await fireEvent.input(screen.getByLabelText('Authority truth'), { target: { value: 'The heir is from the old capital' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Create private Secret' }));
    expect(await screen.findByText('Secret 已写入权威管理视图')).toBeInTheDocument();

    const call = fetcher.mock.calls.find(([input, init]) => String(input) === '/api/projects/p1/secrets' && init?.method === 'POST');
    expect(call).toBeTruthy();
    expect(JSON.parse(String(call?.[1]?.body))).toMatchObject({
      description: "The heir's origin", truth: 'The heir is from the old capital',
      created_chapter: 50, public_status: 'private', source_version: 'human-v1'
    });
    expect(new Headers(call?.[1]?.headers).get('Idempotency-Key')).toMatch(/^web-/);
  });

  it('adds a temporal holder and publicly reveals through server writes', async () => {
    const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.startsWith('/api/projects?')) return json(projectPage);
      if (url.startsWith('/api/projects/p1/secrets?')) return json({ secrets: [secret], total: 1, limit: 100, offset: 0 });
      if (url === '/api/projects/p1/secrets/heir-origin/holders' && init?.method === 'POST') return json(secret);
      if (url === '/api/projects/p1/secrets/heir-origin' && init?.method === 'PATCH') return json({ ...secret, public_status: 'public', revealed_chapter: 50, public_at_chapter: true });
      throw new Error(`unexpected request ${init?.method ?? 'GET'} ${url}`);
    });
    vi.stubGlobal('fetch', fetcher);
    const { default: Secrets } = await import('./Secrets.svelte');
    render(Secrets);
    expect((await screen.findAllByText("The heir's origin")).length).toBeGreaterThan(0);

    const comboboxes = screen.getAllByRole('combobox');
    await fireEvent.change(comboboxes[1], { target: { value: 'heir-origin' } });
    await fireEvent.input(screen.getByPlaceholderText('角色 / entity ID'), { target: { value: 'hero' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Add from Chapter 50' }));
    expect(await screen.findByText('Holder 时态范围已添加')).toBeInTheDocument();

    const holderCall = fetcher.mock.calls.find(([input, init]) => String(input).endsWith('/secrets/heir-origin/holders') && init?.method === 'POST');
    expect(JSON.parse(String(holderCall?.[1]?.body))).toMatchObject({
      entity_id: 'hero', valid_from_chapter: 50, source_version: 'human-v1',
      authority: 'human_final', provenance: { chapter: 50, version: 'human-v1' }
    });

    const revealButton = await screen.findByRole('button', { name: 'Public Reveal' });
    await fireEvent.click(revealButton);
    expect(await screen.findByText("The heir's origin 已在 Chapter 50 公开")).toBeInTheDocument();
    const revealCall = fetcher.mock.calls.find(([input, init]) => String(input).endsWith('/secrets/heir-origin') && init?.method === 'PATCH');
    expect(JSON.parse(String(revealCall?.[1]?.body))).toMatchObject({
      public_status: 'public', revealed_chapter: 50, chapter: 50,
      reason: 'human public reveal'
    });
    await waitFor(() => expect(screen.getByRole('button', { name: 'Public Reveal' })).not.toBeDisabled());
  });
});
