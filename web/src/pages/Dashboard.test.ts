import { render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';

const health = {
  product: 'NovelForge', status: 'ok', version: 'test', api_version: 'v1alpha1',
  workspace: 'library', started_at: '2026-09-01T00:00:00Z', uptime_seconds: 1
};

describe('Dashboard page states', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.resetModules();
  });

  it('renders the real empty project state', async () => {
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      const body = url.endsWith('/health')
        ? health
        : { projects: [], total: 0, limit: 100, offset: 0 };
      return new Response(JSON.stringify(body), { status: 200 });
    }));
    const { default: Dashboard } = await import('./Dashboard.svelte');
    render(Dashboard);
    expect(await screen.findByText('还没有项目')).toBeInTheDocument();
  });

  it('renders a structured server error', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({
      error: {
        code: 'WORKSPACE_UNAVAILABLE',
        message: 'workspace unavailable',
        details: {},
        retryable: true,
        trace_id: 'trace-dashboard'
      }
    }), { status: 503 })));
    const { default: Dashboard } = await import('./Dashboard.svelte');
    render(Dashboard);
    expect(await screen.findByText('workspace unavailable')).toBeInTheDocument();
  });
});
