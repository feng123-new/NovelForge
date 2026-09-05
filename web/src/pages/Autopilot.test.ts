import { render, screen, waitFor, fireEvent } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Autopilot from './Autopilot.svelte';

afterEach(() => vi.unstubAllGlobals());
describe('Autopilot controls', () => {
 it('shows persisted review state and routes approval through an idempotent write', async () => {
  const requests: { url: string; init?: RequestInit }[] = [];
  const job = { id: 'job_test', project_id: 'p1', state: 'paused', stage: 'finalize', chapter: 1, completed_through: 0, target_chapter: 2, error_code: 'REVIEW_REQUIRED', actions: { pause: false, resume: true, stop: true } };
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
   const url = String(input); requests.push({ url, init });
   let data: unknown;
   if (url.includes('/autopilot/job_test/resume')) data = { job };
   else if (url.includes('/autopilot/job_test')) data = { job, candidate_text: 'A reviewed chapter.', chapter_plan: {}, foundation: {} };
   else if (url.includes('/autopilot')) data = { jobs: [job], worker_available: true, model_available: true };
   else if (url.includes('/projects/p1')) data = { id: 'p1', title: 'Story', completed_chapters: 0, total_chapters: 2 };
   else data = { projects: [{ id: 'p1', title: 'Story', archived: false }] };
   return new Response(JSON.stringify(data), { status: 200, headers: { 'Content-Type': 'application/json' } });
  }));
  const view = render(Autopilot);
  await waitFor(() => expect(screen.getByText('批准本章并继续')).toBeTruthy());
  expect((screen.getByText('暂停') as HTMLButtonElement).disabled).toBe(true);
  await fireEvent.click(screen.getByText('查看候选与计划'));
  await waitFor(() => expect(screen.getByText('A reviewed chapter.')).toBeTruthy());
  await fireEvent.click(screen.getByText('批准本章并继续'));
  await waitFor(() => expect(requests.some((r) => r.url.endsWith('/resume') && r.init?.method === 'POST')).toBe(true));
  const write = requests.find((r) => r.url.endsWith('/resume'))!;
  expect(new Headers(write.init?.headers).get('Idempotency-Key')).toBeTruthy();
  view.unmount();
 });
});
