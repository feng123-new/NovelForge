import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';

const projectPage = {
  projects: [{
    id: 'p1', title: 'Sky Road', status: 'active', archived: false,
    current_chapter: 2, completed_chapters: 1, total_chapters: 10,
    total_words: 3500, updated_at: '2026-09-01T00:00:00Z'
  }],
  total: 1, limit: 100, offset: 0
};

describe('Projects page', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('loads real projects and sends archive writes through the API client', async () => {
    const fetcher = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'POST') {
        return new Response(JSON.stringify({ ...projectPage.projects[0], archived: true, status: 'archived' }), { status: 200 });
      }
      return new Response(JSON.stringify(projectPage), { status: 200 });
    });
    vi.stubGlobal('fetch', fetcher);
    const { default: Projects } = await import('./Projects.svelte');
    render(Projects);
    expect(await screen.findByText('Sky Road')).toBeInTheDocument();
    await fireEvent.click(screen.getByRole('button', { name: '归档' }));
    await waitFor(() => expect(fetcher).toHaveBeenCalledWith(
      '/api/projects/p1/archive',
      expect.objectContaining({ method: 'POST' })
    ));
  });

  it('requires a confirmation value and uses reversible delete', async () => {
    const fetcher = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'DELETE') {
        expect(JSON.parse(String(init.body))).toEqual({ confirm: 'Sky Road', permanent: false });
        return new Response(JSON.stringify({ id: 'p1', deleted: true, permanent: false }), { status: 200 });
      }
      return new Response(JSON.stringify(projectPage), { status: 200 });
    });
    vi.stubGlobal('fetch', fetcher);
    vi.stubGlobal('prompt', vi.fn(() => 'Sky Road'));
    const { default: Projects } = await import('./Projects.svelte');
    render(Projects);
    expect(await screen.findByText('Sky Road')).toBeInTheDocument();
    await fireEvent.click(screen.getByRole('button', { name: '移入回收站' }));
    await waitFor(() => expect(fetcher).toHaveBeenCalledWith(
      '/api/projects/p1',
      expect.objectContaining({ method: 'DELETE' })
    ));
  });
});
