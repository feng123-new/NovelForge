import fs from 'node:fs';
import {createRequire} from 'node:module';
const {parse}=createRequire(import.meta.url)('svelte/compiler');
const paths=['src/App.svelte',...fs.readdirSync('src/pages').filter(name=>name.endsWith('.svelte')).map(name=>'src/pages/'+name)];
const failures=[];
for(const path of paths){
  const source=fs.readFileSync(path,'utf8'),ast=parse(source);
  function walk(node){
    if(!node||typeof node!=='object')return;
    if(node.type==='Attribute'){
      if(['title','aria-label','placeholder','alt'].includes(node.name)&&Array.isArray(node.value)&&node.value.some(n=>n.type==='Text'&&/[A-Za-z\u3400-\u9fff]/.test(n.data))) failures.push(path+': untranslated '+node.name);
      return;
    }
    if(node.type==='Element'&&node.name==='option'&&!node.attributes.some(a=>a.type==='Attribute'&&a.name==='value'))failures.push(path+': option without stable value');
    if(node.type==='Text'&&/[A-Za-z\u3400-\u9fff]/.test(node.data))failures.push(path+': untranslated text '+node.data.trim());
    for(const [key,value]of Object.entries(node)){
      if(['start','end','loc'].includes(key))continue;
      if(Array.isArray(value))value.forEach(walk);else if(typeof value==='object')walk(value);
    }
  }
  walk(ast.html);
}
if(failures.length)throw Error(failures.join('\n'));
console.log(`I18N static audit passed: ${paths.length} surfaces, explicit option values, no hardcoded interface text/attributes.`);
