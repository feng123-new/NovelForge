import { describe, expect, it, vi } from 'vitest';
import { APIClientError } from './api';
import { ChapterVersionAPI } from './chapterVersions';

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

describe('ChapterVersionAPI', () => {
  it('creates human revisions with a fresh Idempotency-Key and never targets the active final resource', async () => {
    const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      expect(String(input)).toBe('/api/projects/p1/chapters/50/versions');
      expect(init?.method).toBe('POST');
      expect(new Headers(init?.headers).get('Idempotency-Key')).toMatch(/^web-/);
      expect(JSON.parse(String(init?.body))).toEqual({ content: 'Character A escaped.' });
      return jsonResponse({
        id: 'cv-human', project_id: 'p1', chapter: 50, version_number: 3,
        type: 'human_revision', status: 'human_revision', content: 'Character A escaped.',
        content_sha: 'a'.repeat(64), parent_version_id: 'cv-final', author_type: 'human',
        created_at: '2026-09-03T00:00:00Z', accepted: false, rejected: false, active_final: false
      }, 201);
    });
    const client = new ChapterVersionAPI('/api', fetcher);
    const version = await client.saveHuman('p1', 50, 'Character A escaped.');
    expect(version.type).toBe('human_revision');
    expect(version.active_final).toBe(false);
    expect(fetcher).toHaveBeenCalledTimes(1);
  });

  it('uses bounded structured diff parameters', async () => {
    const fetcher = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      expect(url).toContain('/api/projects/p1/chapters/50/diff?');
      expect(url).toContain('from_version=cv-old');
      expect(url).toContain('to_version=cv-new');
      expect(url).toContain('mode=side_by_side');
      expect(url).toContain('limit=200');
      return jsonResponse({
        from_version: 'cv-old', to_version: 'cv-new', from_sha: 'a'.repeat(64), to_sha: 'b'.repeat(64),
        mode: 'side_by_side', hunks: [], additions: 1, deletions: 1, unchanged: 2, truncated: true,
        next_cursor: 'cursor-2'
      });
    });
    const client = new ChapterVersionAPI('/api', fetcher);
    const result = await client.diff('p1', 50, 'cv-old', 'cv-new', 'side_by_side');
    expect(result.truncated).toBe(true);
    expect(result.next_cursor).toBe('cursor-2');
  });

  it('surfaces Safe Error envelopes without manufacturing filesystem details', async () => {
    const fetcher = vi.fn(async () => jsonResponse({
      error: { code: 'DIFF_TOO_LARGE', message: 'diff exceeds bounded input size', details: {}, retryable: false, trace_id: 'trace-1' }
    }, 413));
    const client = new ChapterVersionAPI('/api', fetcher);
    await expect(client.diff('p1', 50, 'a', 'b', 'inline')).rejects.toMatchObject<Partial<APIClientError>>({
      status: 413,
      payload: expect.objectContaining({ code: 'DIFF_TOO_LARGE', trace_id: 'trace-1' })
    });
  });
});
