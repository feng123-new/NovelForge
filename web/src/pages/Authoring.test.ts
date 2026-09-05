import { render, screen, waitFor, fireEvent } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Authoring from './Authoring.svelte';

afterEach(()=>vi.unstubAllGlobals());
describe('Authoring workspace',()=>{
 it('writes Markdown with a revision and idempotency key then reloads server state',async()=>{
  const requests:{url:string;init?:RequestInit}[]=[];let revision=1;
  const rules={enabled:true,phrases:[],max_phrase_occurrences:1,max_sentence_repeats:1,min_sentence_runes:12,previous_chapters:3};
  vi.stubGlobal('fetch',vi.fn(async(input:RequestInfo|URL,init?:RequestInit)=>{
   const url=String(input);requests.push({url,init});let data:unknown;
   if(url.includes('/authoring')&&init?.method==='POST'){revision=2;data={revision,entry_id:'entry1',replayed:false};}
   else if(url.includes('/authoring')) data={revision,entries:[],builtins:[],rules,total:0,limit:50,offset:0};
   else data={projects:[{id:'p1',title:'Story',archived:false}]};
   return new Response(JSON.stringify(data),{status:200,headers:{'Content-Type':'application/json'}});
  }));
  const view=render(Authoring);
  await waitFor(()=>expect(screen.getByText('保存资源')).toBeTruthy());
  await fireEvent.input(screen.getByLabelText('标题'),{target:{value:'Custom craft'}});
  await fireEvent.input(screen.getByLabelText('Markdown 内容（最多 16 KiB）'),{target:{value:'# Method\nUse concrete action.'}});
  await fireEvent.submit(screen.getByRole('form',{name:'资源编辑'}));
  await waitFor(()=>expect(requests.some(r=>r.init?.method==='POST')).toBe(true));
  const write=requests.find(r=>r.init?.method==='POST')!;const body=JSON.parse(String(write.init?.body));expect(body.expected_revision).toBe(1);expect(body.entry.role).toBe('writing');expect(body.entry.markdown).toContain('concrete action');expect(new Headers(write.init?.headers).get('Idempotency-Key')).toBeTruthy();
  await waitFor(()=>expect(screen.getByText(/项目资源修订 2/)).toBeTruthy());view.unmount();
 });
 it('renders structured conflicts without pretending the edit succeeded',async()=>{
  vi.stubGlobal('fetch',vi.fn(async(input:RequestInfo|URL,init?:RequestInit)=>{
   const url=String(input);if(init?.method==='POST')return new Response(JSON.stringify({error:{code:'AUTHORING_REVISION_CONFLICT',message:'refresh required',details:{},retryable:false,trace_id:'trace'}}),{status:409,headers:{'Content-Type':'application/json'}});
   const data=url.includes('/authoring')?{revision:1,entries:[],builtins:[],rules:{enabled:true,phrases:[],max_phrase_occurrences:1,max_sentence_repeats:1,min_sentence_runes:12,previous_chapters:3},total:0,limit:50,offset:0}:{projects:[{id:'p1',title:'Story',archived:false}]};return new Response(JSON.stringify(data),{status:200,headers:{'Content-Type':'application/json'}});
  }));
  const view=render(Authoring);await waitFor(()=>expect(screen.getByText('保存规则')).toBeTruthy());await fireEvent.click(screen.getByText('保存规则'));await waitFor(()=>expect(screen.getByRole('alert').textContent).toContain('AUTHORING_REVISION_CONFLICT'));expect(screen.queryByText(/已保存到当前项目/)).toBeNull();view.unmount();
 });
});
