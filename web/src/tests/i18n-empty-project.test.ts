import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { tick } from 'svelte';
import Chapters from '../pages/Chapters.svelte';
import { applyLocale, type Locale } from '../lib/i18n';

// Exact empty-snapshot shape observed from the real Go server: nil slices are
// JSON null, not arrays. Do not hide this production case with friendlier mocks.
const emptyQuality = {
  snapshot: { transaction: { transaction_id: '', project_id: '', chapter: 0, state: '', attempt: 0, max_rewrites: 0, quality_threshold: 0, created_at: '0001-01-01T00:00:00Z', updated_at: '0001-01-01T00:00:00Z' }, candidates: null, state_changes: null },
  actions: { generate: false, check: false, rewrite: false, finalize: false }
};
afterEach(() => { vi.unstubAllGlobals(); applyLocale('zh-CN'); location.hash = '#/dashboard'; });
describe.each(['zh-CN', 'en'] as Locale[])('native empty project in %s', language => {
  it('renders without exceptions and preserves an unsaved plan across a language switch', async () => {
    applyLocale(language); location.hash = '#/chapters?project=p1';
    const fetcher = vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input); let body: unknown;
      if (path.startsWith('/api/projects?')) body = { projects: [{ id: 'p1', title: '原稿 Original', completed_chapters: 0 }], total: 1 };
      else if (path.includes('/chapters?')) body = { chapters: [], total: 0 };
      else if (path.endsWith('/chapters/1/quality')) body = emptyQuality;
      else throw Error('Unexpected request: ' + path);
      return new Response(JSON.stringify(body), { headers: { 'Content-Type': 'application/json' } });
    });
    vi.stubGlobal('fetch', fetcher); const view = render(Chapters);
    await waitFor(() => expect(view.container.querySelector('.loading-spinner')).toBeNull());
    expect(screen.queryByRole('alert')).toBeNull();
    expect(screen.getByRole('button', { name: language === 'en' ? 'Generate' : '生成' })).toBeDisabled();
    const input = screen.getByLabelText(language === 'en' ? 'Title' : '标题') as HTMLInputElement;
    await fireEvent.input(input, { target: { value: '未保存计划 Unsaved plan' } });
    const calls = fetcher.mock.calls.length; applyLocale(language === 'en' ? 'zh-CN' : 'en'); await tick();
    expect(input.isConnected).toBe(true); expect(input.value).toBe('未保存计划 Unsaved plan');
    expect(fetcher).toHaveBeenCalledTimes(calls); expect(emptyQuality.snapshot.candidates).toBeNull();
  });
});
