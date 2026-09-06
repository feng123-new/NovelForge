import {createRequire} from 'node:module';
import fs from 'node:fs';
const {parse}=createRequire(import.meta.url)('svelte/compiler');
const paths=['web/src/App.svelte',...fs.readdirSync('web/src/pages').filter(x=>x.endsWith('.svelte')).map(x=>'web/src/pages/'+x)];
for(const path of paths){
 const source=fs.readFileSync(path,'utf8'),ast=parse(source),out=new Set();
 function phrase(nodes){let text='',i=0;for(const n of nodes){if(n.type==='Text')text+=n.data;else text+='{p'+(i++)+'}';}text=text.replace(/\s+/g,' ').trim();if(/[a-zA-Z\u3400-\u9fff]/.test(text.replace(/\{p\d+\}/g,'')))out.add(text);}
 function walk(n){if(!n||typeof n!=='object')return;
  if(n.type==='Attribute'){if(['aria-label','title','placeholder','alt'].includes(n.name)&&Array.isArray(n.value)){phrase(n.value);n.value.forEach(x=>{if(x.expression)walk(x.expression);});}return;}
  if(n.type==='Literal'&&typeof n.value==='string'&&/[\u3400-\u9fff]/.test(n.value))out.add(n.value);
  if(n.type==='TemplateLiteral'&&n.quasis.some(q=>/[\u3400-\u9fff]/.test(q.value.raw)))out.add(n.quasis.map((q,i)=>q.value.cooked+(i<n.expressions.length?'{p'+i+'}':'')).join(''));
  if(Array.isArray(n.children)){let run=[];for(const c of n.children){if(['Text','MustacheTag'].includes(c.type))run.push(c);else{phrase(run);run=[];}}phrase(run);}
  for(const [k,v]of Object.entries(n)){if(['start','end','loc'].includes(k))continue;if(Array.isArray(v))v.forEach(walk);else walk(v);}
 }walk(ast);console.log('INVENTORY '+path+' '+JSON.stringify([...out]));
}
console.log('Inventory only; migration and test suite have not been applied.');
process.exit(1);
