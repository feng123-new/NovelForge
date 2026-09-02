import { describe, expect, it, vi } from 'vitest';
import { APIClient } from './api';

describe('APIClient', () => {
  it('adds an idempotency key to writes', async () => {
    const fetcher = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      expect(init?.headers).toMatchObject({ 'Idempotency-Key': expect.stringMatching(/^web-/) });
      return new Response(JSON.stringify({ id: 'p1', title: 'Book' }), {
        status: 201,
        headers: { 'Content-Type': 'application/json' }
      });
    });
    const client = new APIClient('/api', fetcher);
    await expect(client.createProject({ title: 'Book' })).resolves.toMatchObject({ id: 'p1' });
  });

  it('preserves the structured error envelope', async () => {
    const fetcher = vi.fn(async () => new Response(JSON.stringify({
      error: { code: 'PROJECT_NOT_FOUND', message: 'project not found', details: {}, retryable: false, trace_id: 'trace-1' }
    }), { status: 404 }));
    const client = new APIClient('/api', fetcher);
    await expect(client.getProject('missing')).rejects.toEqual(expect.objectContaining({
      status: 404,
      payload: expect.objectContaining({ code: 'PROJECT_NOT_FOUND', trace_id: 'trace-1' })
    }));
  });

  it('uses real chapter quality routes and idempotency for every write', async () => {
    const calls: Array<{ url: string; init?: RequestInit }> = [];
    const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      calls.push({ url: String(input), init });
      return new Response(JSON.stringify({ snapshot: { transaction: {}, candidates: [], state_changes: [] }, actions: { generate: false, check: false, rewrite: false, finalize: false } }), { status: 202 });
    });
    const client = new APIClient('/api', fetcher);
    const plan = { chapter: 7, title: 'Seven', pov: 'Mira', location: 'Gate', objective: 'enter', conflict: 'guard', required_beats: [], forbidden_outcomes: [], knowledge_boundary: [], inventory_constraints: [], foreshadow_obligations: [], ending_hook: 'bell' };
    await client.generateChapter('p/1', 7, plan);
    await client.checkChapter('p/1', 7);
    await client.rewriteChapter('p/1', 7, plan);
    await client.finalizeChapter('p/1', 7);
    expect(calls.map((call) => call.url)).toEqual([
      '/api/projects/p%2F1/chapters/7/generate',
      '/api/projects/p%2F1/chapters/7/check',
      '/api/projects/p%2F1/chapters/7/rewrite',
      '/api/projects/p%2F1/chapters/7/finalize'
    ]);
    for (const call of calls) {
      expect(call.init?.method).toBe('POST');
      expect(call.init?.headers).toMatchObject({ 'Idempotency-Key': expect.stringMatching(/^web-/) });
    }
  });

  it('reloads quality state with GET and preserves structured errors', async () => {
    const fetcher = vi.fn(async () => new Response(JSON.stringify({
      error: { code: 'QUALITY_STATE_CONFLICT', message: 'conflict', details: {}, retryable: false, trace_id: 'trace-quality' }
    }), { status: 409 }));
    const client = new APIClient('/api', fetcher);
    await expect(client.quality('p1', 2)).rejects.toEqual(expect.objectContaining({
      status: 409,
      payload: expect.objectContaining({ code: 'QUALITY_STATE_CONFLICT', trace_id: 'trace-quality' })
    }));
  });

  it('uses real Narrative Ledger routes and idempotency for writes', async () => {
    const calls: Array<{ url: string; init?: RequestInit }> = [];
    const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      calls.push({ url: String(input), init });
      return new Response(JSON.stringify({ foreshadows: [], secrets: [], total: 0, limit: 100, offset: 0 }), { status: init?.method ? 201 : 200 });
    });
    const client = new APIClient('/api', fetcher);
    await client.listForeshadows('p/1', 135, { overdue: 'true' });
    await client.createForeshadow('p/1', {
      title: 'Gate', description: 'Return', importance: 'critical', planted_chapter: 20,
      expected_payoff_min: 100, expected_payoff_max: 130, status: 'planted',
      related_entities: [], related_arcs: [], last_progress_chapter: 20,
      urgency: 'high', source_version: 'v1'
    });
    await client.listSecrets('p/1', 4, false);
    await client.createSecret('p/1', { description: 'Origin', truth: 'Hidden', created_chapter: 1, public_status: 'private', source_version: 'v1' });
    expect(calls.map((call) => call.url)).toEqual([
      '/api/projects/p%2F1/foreshadows?chapter=135&limit=100&overdue=true',
      '/api/projects/p%2F1/foreshadows',
      '/api/projects/p%2F1/secrets?chapter=4&limit=100&include_truth=false',
      '/api/projects/p%2F1/secrets'
    ]);
    expect(calls[1].init?.headers).toMatchObject({ 'Idempotency-Key': expect.stringMatching(/^web-/) });
    expect(calls[3].init?.headers).toMatchObject({ 'Idempotency-Key': expect.stringMatching(/^web-/) });
  });

});
