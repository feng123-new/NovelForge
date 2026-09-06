import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { tick } from 'svelte';
import LanguageSwitcher from '../components/LanguageSwitcher.svelte';
import NewNovelWizard from '../pages/NewNovelWizard.svelte';
import ChapterVersions from '../pages/ChapterVersions.svelte';
import Authoring from '../pages/Authoring.svelte';
import Lifecycle from '../pages/Lifecycle.svelte';
import Observability from '../pages/Observability.svelte';
import Autopilot from '../pages/Autopilot.svelte';
import { applyLocale } from '../lib/i18n';

const project = { id: 'p1', title: '原稿 Original 保留', language: 'zh-TW', archived: false, current_chapter: 1, completed_chapters: 0, total_chapters: 3, total_words: 0, updated_at: '2026-09-06T00:00:00Z' };
const finalVersion = { id: 'cv1', project_id: 'p1', chapter: 1, version_number: 1, type: 'final', status: 'final', content: '原稿 Original 保留', content_sha: 'a'.repeat(64), author_type: 'human', created_at: '2026-09-06T00:00:00Z', accepted: true, rejected: false, active_final: true, authority: 'human_final' };
const rules = { enabled: true, phrases: ['原始规则'], max_phrase_occurrences: 3, max_sentence_repeats: 2, min_sentence_runes: 8, previous_chapters: 1 };
const totals = { known_cost_micros: 0, unknown_cost: 0, pending: 0, calls: 0, completed: 0, failed: 0, input_tokens: 0, output_tokens: 0, unknown_usage: 0 };
const policy = { currency: 'USD', project_budget_micros: 10000000, task_budget_micros: 5000000, project_max_calls: 100, task_max_calls: 10, max_output_tokens: 2048, max_input_estimate: 8192, failure_threshold: 3, cooldown_seconds: 60, paused_providers: [], require_known_price: true, block_unknown_cost: true, prices: [] };
function response(body: unknown) { return new Response(JSON.stringify(body), { headers: { 'Content-Type': 'application/json' } }); }
function fetcher() { return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
  const url = String(input); if (init?.method && init.method !== 'GET') throw Error('Unexpected write while switching: ' + url);
  if (url.startsWith('/api/projects?')) return response({ projects: [project], total: 1 });
  if (url.startsWith('/api/models')) return response({ models: [], total: 0 });
  if (url === '/api/projects/p1') return response(project);
  if (url.endsWith('/rebuild')) return response({ state: 'ready' });
  if (url.includes('/plan-impact')) return response({ impacts: [] });
  if (url.includes('/versions?')) return response({ versions: [finalVersion] });
  if (url.endsWith('/versions/cv1')) return response(finalVersion);
  if (url.endsWith('/chapters/1')) return response({ project_id: 'p1', chapter: 1, active_final: finalVersion, latest: finalVersion, version_count: 1, sync: { sync_required: false }, derived_state: 'ready' });
  if (url.includes('/authoring?')) return response({ revision: 1, total: 0, entries: [], builtins: [], rules });
  if (url.includes('/lifecycle/imports')) return response({ imports: [], total: 0, model_available: false });
  if (url.includes('/observability/diagnostics')) return response({ findings: [] });
  if (url.includes('/observability?')) return response({ settings: { revision: 1, policy }, totals, replays: 0, legacy_untracked_calls: 0, health: [], groups: [], attempts: [], total: 0 });
  if (url.includes('/autopilot?')) return response({ worker_available: true, model_available: true, next_chapter: 1, jobs: [{ id: 'job1', state: 'running', stage: 'writing', chapter: 1, completed_through: 0, target_chapter: 3, revision: 2, actions: { pause: true, stop: true, resume: false } }] });
  throw Error('Unexpected read: ' + url);
}); }
async function switchWithoutRequests(mock: ReturnType<typeof fetcher>, before: number) {
  applyLocale('en'); await tick(); await Promise.resolve(); expect(mock).toHaveBeenCalledTimes(before);
  expect(document.documentElement.lang).toBe('en');
  applyLocale('zh-CN'); await tick(); await Promise.resolve(); expect(mock).toHaveBeenCalledTimes(before);
}
beforeEach(() => { applyLocale('zh-CN'); location.hash = '#/dashboard?project=p1&chapter=1'; });
afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.unstubAllGlobals(); applyLocale('zh-CN'); location.hash = '#/dashboard'; });
describe('language switching preserves working state', () => {
  it('preserves wizard fields and the separate manuscript language without requests', async () => {
    const mock = fetcher(); vi.stubGlobal('fetch', mock); const view = render(NewNovelWizard);
    await waitFor(() => expect(mock).toHaveBeenCalledTimes(1));
    const title = view.container.querySelector('input[maxlength="200"]') as HTMLInputElement;
    await fireEvent.input(title, { target: { value: '未保存 Unsaved title' } });
    const language = screen.getByLabelText('创作语言') as HTMLSelectElement;
    await fireEvent.change(language, { target: { value: 'zh-TW' } }); const count = mock.mock.calls.length;
    await switchWithoutRequests(mock, count);
    expect(title).toBe(view.container.querySelector('input[maxlength="200"]')); expect(title.value).toBe('未保存 Unsaved title'); expect(language.value).toBe('zh-TW');
    applyLocale('en'); await tick(); expect(screen.getByLabelText('Manuscript language')).toBe(language);
  });
  it('preserves the chapter editor DOM, unsaved content, cursor and selected version', async () => {
    const mock = fetcher(); vi.stubGlobal('fetch', mock); location.hash = '#/versions?project=p1&chapter=1'; const view = render(ChapterVersions);
    const editor = await screen.findByLabelText('章节 Markdown 编辑器') as HTMLTextAreaElement;
    await fireEvent.input(editor, { target: { value: '未保存正文 Unsaved manuscript' } }); editor.focus(); editor.setSelectionRange(2, 7);
    const count = mock.mock.calls.length; await switchWithoutRequests(mock, count);
    expect(view.container.querySelector('textarea')).toBe(editor); expect(editor.value).toBe('未保存正文 Unsaved manuscript'); expect(editor.selectionStart).toBe(2); expect(editor.selectionEnd).toBe(7);
    expect(screen.getByTestId('version-history')).toHaveTextContent('版本 1'); expect(finalVersion.content).toBe('原稿 Original 保留');
  });
  it('preserves resource Markdown and rule edits', async () => {
    const mock = fetcher(); vi.stubGlobal('fetch', mock); const view = render(Authoring);
    const title = await screen.findByLabelText('标题') as HTMLInputElement;
    await fireEvent.input(title, { target: { value: '自定义资源 Custom resource' } });
    const markdown = view.container.querySelector('textarea') as HTMLTextAreaElement; await fireEvent.input(markdown, { target: { value: '# 原始 Markdown\nNever translate this content.' } });
    const count = mock.mock.calls.length; await switchWithoutRequests(mock, count);
    expect(title.value).toBe('自定义资源 Custom resource'); expect(markdown.value).toBe('# 原始 Markdown\nNever translate this content.'); expect(view.container.querySelector('textarea')).toBe(markdown);
  });
  it('preserves an import file selection without uploading or calling a model', async () => {
    const mock = fetcher(); vi.stubGlobal('fetch', mock); render(Lifecycle);
    const input = await screen.findByLabelText('正文文件') as HTMLInputElement;
    const file = new File(['原文 Original'], 'manuscript.md', { type: 'text/markdown' });
    Object.defineProperty(input, 'files', { configurable: true, value: [file] }); await fireEvent.change(input);
    const count = mock.mock.calls.length; await switchWithoutRequests(mock, count); expect(input.files?.[0]).toBe(file);
    expect(screen.getByRole('button', { name: '暂存并检查章节' })).toBeEnabled();
  });
  it('preserves unsaved budget values and the expanded policy panel', async () => {
    const mock = fetcher(); vi.stubGlobal('fetch', mock); const view = render(Observability);
    const budget = await screen.findByLabelText('项目预算（币种单位）') as HTMLInputElement;
    await fireEvent.input(budget, { target: { value: '12.345678' } }); const details = view.container.querySelector('details')!; details.open = true;
    const count = mock.mock.calls.length; await switchWithoutRequests(mock, count); expect(budget.value).toBe('12.345678'); expect(details.open).toBe(true); expect(policy.project_budget_micros).toBe(10000000);
  });
  it('keeps a running job and its controls unchanged; language changes issue no commands', async () => {
    vi.spyOn(globalThis, 'setInterval').mockReturnValue(123 as unknown as ReturnType<typeof setInterval>);
    const mock = fetcher(); vi.stubGlobal('fetch', mock); const view = render(Autopilot);
    await screen.findByText('运行中 · 写作'); const stop = screen.getByRole('button', { name: '停止' }); const count = mock.mock.calls.length;
    await switchWithoutRequests(mock, count); expect(stop).toBe(screen.getByRole('button', { name: '停止' })); expect(stop).toBeEnabled();
    applyLocale('en'); await tick(); expect(view.container).toHaveTextContent('Running · Writing'); expect(mock).toHaveBeenCalledTimes(count);
  });
  it('synchronizes two selectors without remounting either', async () => {
    const one = render(LanguageSwitcher, { id: 'one' }), two = render(LanguageSwitcher, { id: 'two' });
    const first = one.container.querySelector('select')!, second = two.container.querySelector('select')!;
    await fireEvent.change(first, { target: { value: 'en' } }); await tick(); expect(second.value).toBe('en'); expect(first).toBe(one.container.querySelector('select'));
    await fireEvent.change(second, { target: { value: 'zh-CN' } }); await tick(); expect(first.value).toBe('zh-CN');
  });
});
