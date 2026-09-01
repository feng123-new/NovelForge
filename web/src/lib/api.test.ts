import { describe, expect, it, vi } from 'vitest';
import { APIClient, APIClientError } from './api';

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
});
