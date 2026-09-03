import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';

const finalVersion = {
  id: 'cv-final', project_id: 'p1', chapter: 50, version_number: 2,
  type: 'final', status: 'final', content: 'Character A died.', content_sha: 'a'.repeat(64),
  parent_version_id: 'cv-draft', author_type: 'system', created_at: '2026-09-02T00:00:00Z',
  accepted: true, rejected: false, active_final: true, authority: 'generated_final'
};
const rejectedVersion = {
  id: 'cv-rejected', project_id: 'p1', chapter: 50, version_number: 1,
  type: 'editor_revision', status: 'rejected', content_sha: 'b'.repeat(64), author_type: 'editor',
  created_at: '2026-09-01T00:00:00Z', accepted: false, rejected: true, active_final: false
};

function response(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

function workspaceFetcher(syncRequired = true) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url.startsWith('/api/projects?')) {
      return response({ projects: [{ id: 'p1', title: 'Scenario B', status: 'active', archived: false, current_chapter: 50, completed_chapters: 50, total_chapters: 100, total_words: 1, updated_at: '2026-09-03T00:00:00Z' }], total: 1, limit: 100, offset: 0 });
    }
    if (url === '/api/projects/p1/chapters/50/rebuild') return response({ boundary_chapter: 50, state: 'ready' });
    if (url.startsWith('/api/projects/p1/chapters/50/plan-impact')) return response({ impacts: [], total: 0, limit: 100, offset: 0 });
    if (url.startsWith('/api/projects/p1/chapters/50/versions?')) return response({ versions: [finalVersion, rejectedVersion], total: 2, limit: 100, offset: 0 });
    if (url === '/api/projects/p1/chapters/50/versions/cv-final') return response(finalVersion);
    if (url === '/api/projects/p1/chapters/50/versions/cv-rejected') return response({ ...rejectedVersion, content: 'Rejected content.' });
    if (url === '/api/projects/p1/chapters/50' && !init?.method) {
      return response({
        project_id: 'p1', chapter: 50, active_final: finalVersion, latest: finalVersion, version_count: 2,
        sync: { project_id: 'p1', chapter: 50, active_version_id: 'cv-final', expected_sha: 'a'.repeat(64), observed_sha: (syncRequired ? 'c' : 'a').repeat(64), sync_required: syncRequired },
        derived_state: syncRequired ? 'stale' : 'ready'
      });
    }
    if (url === '/api/projects/p1/chapters/50/versions' && init?.method === 'POST') {
      const headers = new Headers(init.headers);
      expect(headers.get('Idempotency-Key')).toMatch(/^web-/);
      expect(JSON.parse(String(init.body))).toEqual({ content: 'Character A is severely injured but escaped alive.' });
      return response({ ...finalVersion, id: 'cv-human', version_number: 3, type: 'human_revision', status: 'human_revision', content: 'Character A is severely injured but escaped alive.', content_sha: 'd'.repeat(64), parent_version_id: 'cv-final', author_type: 'human', accepted: false, active_final: false, authority: '' }, 201);
    }
    throw new Error(`unexpected request ${url}`);
  });
}

describe('ChapterVersions page', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    globalThis.location.hash = '#/dashboard';
  });

  it('shows stable history, rejected versions and external SHA warning from server authority', async () => {
    globalThis.location.hash = '#/versions?project=p1&chapter=50';
    vi.stubGlobal('fetch', workspaceFetcher(true));
    const { default: ChapterVersions } = await import('./ChapterVersions.svelte');
    render(ChapterVersions);
    const history = await screen.findByTestId('version-history');
    expect(within(history).getByText('v2 · final')).toBeInTheDocument();
    expect(within(history).getByText('v1 · editor_revision')).toBeInTheDocument();
    expect(within(history).getByText('Rejected')).toBeInTheDocument();
    expect(await screen.findByTestId('sync-warning')).toHaveTextContent('Sync required');
    expect(screen.getByText(/expected aaaaaaaaaaaa/)).toBeInTheDocument();
    expect(screen.getByText(/observed cccccccccccc/)).toBeInTheDocument();
  });

  it('saves editor content as a new human_revision while leaving the displayed Active Final unchanged', async () => {
    globalThis.location.hash = '#/versions?project=p1&chapter=50';
    const fetcher = workspaceFetcher(false);
    vi.stubGlobal('fetch', fetcher);
    const { default: ChapterVersions } = await import('./ChapterVersions.svelte');
    render(ChapterVersions);
    const editor = await screen.findByLabelText('Chapter markdown editor');
    await fireEvent.input(editor, { target: { value: 'Character A is severely injured but escaped alive.' } });
    const save = screen.getByRole('button', { name: 'Save Human Revision' });
    await fireEvent.click(save);
    await waitFor(() => expect(fetcher).toHaveBeenCalledWith(
      '/api/projects/p1/chapters/50/versions',
      expect.objectContaining({ method: 'POST' })
    ));
    expect(await screen.findByText(/Human revision v3 已创建/)).toBeInTheDocument();
    const history = await screen.findByTestId('version-history');
    expect(within(history).getByText('v2 · final')).toBeInTheDocument();
  });
});
