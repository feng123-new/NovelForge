import { render, screen, waitFor, fireEvent } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Lifecycle from './Lifecycle.svelte';

afterEach(() => vi.unstubAllGlobals());
const project = { id: 'p1', title: '故事', archived: false, format_version: 2 };
const batch = { id: 'imp_test', project_id: 'p1', filename: 'story.txt', source_sha: 'hash', start_chapter: 1, total: 1, saved: 0, analyzed: 0, next_save: 1, next_analysis: 1, created_at: '' };
const json = (data: unknown) => new Response(JSON.stringify(data), { status: 200, headers: { 'Content-Type': 'application/json' } });

describe('Lifecycle workspace', () => {
 it('stages a multipart manuscript with an idempotency key without starting analysis', async () => {
  const requests: {url: string; init?: RequestInit}[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
   const url=String(input); requests.push({url,init});
   if(url.endsWith('/lifecycle/imports') && init?.method==='POST')return json({import:batch});
   if(url.includes('/lifecycle/imports/imp_test'))return json({import:batch,chapters:[{chapter:1,title:'开始',characters:5,version_id:'',state:'pending'}],limit:50,offset:0});
   if(url.includes('/lifecycle/imports'))return json({imports:[],model_available:true,limit:50,offset:0});
   return json({projects:[project]});
  }));
  const view=render(Lifecycle);
  await waitFor(()=>expect(screen.getByText('暂存并检查章节')).toBeTruthy());
  const file=new File(['第1章 开始\n正文。'],'story.txt',{type:'text/plain'});
  await fireEvent.change(screen.getByLabelText('正文文件'),{target:{files:[file]}});
  await fireEvent.click(screen.getByText('暂存并检查章节'));
  await waitFor(()=>expect(requests.some(r=>r.init?.method==='POST')).toBe(true));
  const write=requests.find(r=>r.init?.method==='POST')!;
  expect(write.init?.body).toBeInstanceOf(FormData);
  expect((write.init?.body as FormData).get('start_chapter')).toBe('1');
  expect(new Headers(write.init?.headers).get('Idempotency-Key')).toBeTruthy();
  await waitFor(()=>expect(screen.getByText('继续分析（调用模型）')).toBeTruthy());
  expect((screen.getByText('继续分析（调用模型）') as HTMLButtonElement).disabled).toBe(true);
  expect(requests.filter(r=>r.init?.method==='POST')).toHaveLength(1);
  view.unmount();
 });
 it('resumes the next persisted chapter only after explicit model consent', async () => {
  let analyzed=false; const writes: RequestInit[]=[];
  vi.stubGlobal('fetch',vi.fn(async(input: RequestInfo | URL,init?:RequestInit)=>{
   const url=String(input);
   if(url.endsWith('/step')){writes.push(init!);analyzed=true;return json({chapter:{chapter:1,state:'analyzed',version_id:'cv1'}});}
   const saved={...batch,saved:1,next_save:0,analyzed:analyzed?1:0,next_analysis:analyzed?0:1};
   if(url.includes('/lifecycle/imports/imp_test'))return json({import:saved,chapters:[{chapter:1,title:'开始',characters:5,version_id:'cv1',state:analyzed?'analyzed':'saved'}],limit:50,offset:0});
   if(url.includes('/lifecycle/imports'))return json({imports:[saved],model_available:true,limit:50,offset:0});
   return json({projects:[project]});
  }));
  const view=render(Lifecycle);
  await waitFor(()=>expect(screen.getByText(/story.txt · 1\/1 已导入/)).toBeTruthy());
  await fireEvent.click(screen.getByText(/story.txt · 1\/1 已导入/));
  await waitFor(()=>expect(screen.getByText('继续分析（调用模型）')).toBeTruthy());
  expect(writes).toHaveLength(0);
  await fireEvent.click(screen.getByRole('checkbox'));
  await fireEvent.click(screen.getByText('继续分析（调用模型）'));
  await waitFor(()=>expect(writes).toHaveLength(1));
  expect(JSON.parse(String(writes[0].body))).toEqual({chapter:1,analyze:true});
  expect(new Headers(writes[0].headers).get('Idempotency-Key')).toBeTruthy();
  await waitFor(()=>expect(screen.getByText('本轮处理完成。分析完成不等于已接受或已定稿。')).toBeTruthy());
  view.unmount();
 });
});
