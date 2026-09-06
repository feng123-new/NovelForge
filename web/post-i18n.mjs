// Reviewed migration follow-ups; deleted once materialized source is verified.
import fs from 'node:fs';
import {createRequire} from 'node:module';
const ts=createRequire(import.meta.url)('typescript');
const paths=['web/src/App.svelte',...fs.readdirSync('web/src/pages').filter(x=>x.endsWith('.svelte')).map(x=>'web/src/pages/'+x)];
for(const path of paths){let s=fs.readFileSync(path,'utf8');s=s.replace('t, text, label, serverText','t, text, label as stateLabel, serverText').replaceAll('$label(', '$stateLabel(');fs.writeFileSync(path,s);}
let main=fs.readFileSync('web/src/main.ts','utf8');if(!main.includes('initializeLocale();'))main=main.replace('mount(App, { target });','initializeLocale();\nmount(App, { target });');fs.writeFileSync('web/src/main.ts',main);
const testPath='web/src/tests/i18n.test.ts';let test=fs.readFileSync(testPath,'utf8');test=test.replace("readFileSync(new URL('../lib/i18n/index.ts', import.meta.url), 'utf8')","readFileSync(process.cwd() + '/src/lib/i18n/index.ts', 'utf8')");fs.writeFileSync(testPath,test);
// Only translate UI query/matcher literals in existing tests. Fixtures, request
// parameters, API assertions and the number of test cases remain unchanged.
const translations=new Map();for(const line of fs.readFileSync('web/i18n-translations.tsv','utf8').trim().split('\n')){const [source,other,corrected]=line.split('\t');translations.set(source,/[\u3400-\u9fff]/.test(source)?corrected||source:other);}
for(const [a,b]of Object.entries({'v2 · final':'版本 2 · 定稿','v1 · editor_revision':'版本 1 · 审稿修订','Sync required':'需要同步','FAIL':'未通过','PASS':'通过','WARN':'警告'}))translations.set(a,b);
for(const name of fs.readdirSync('web/src/pages').filter(x=>x.endsWith('.test.ts'))){const path='web/src/pages/'+name;let source=fs.readFileSync(path,'utf8'),changes=[];const tree=ts.createSourceFile(path,source,ts.ScriptTarget.Latest,true);
 function visit(node){if(ts.isStringLiteral(node)&&translations.has(node.text)){let p=node.parent;while(p&&!ts.isCallExpression(p)&&!ts.isSourceFile(p))p=p.parent;const call=p&&ts.isCallExpression(p)?p.expression.getText(tree):'';if(/(?:get|find|query)(?:All)?By(?:Text|Role|LabelText|PlaceholderText)$|\.toHaveTextContent$/.test(call))changes.push([node.getStart(tree),node.end,JSON.stringify(translations.get(node.text))]);}ts.forEachChild(node,visit);}visit(tree);for(const [a,b,value]of changes.sort((a,b)=>b[0]-a[0]))source=source.slice(0,a)+value+source.slice(b);
 source=source.replace('/expected aaaaaaaaaaaa/','/预期 aaaaaaaaaaaa/').replace('/observed cccccccccccc/','/当前 cccccccccccc/').replace('/Human revision v3 已创建/','/已创建人工修订版本 3/');fs.writeFileSync(path,source);
}
const wizardPath='web/src/lib/wizard.test.ts';let wizard=fs.readFileSync(wizardPath,'utf8');if(!wizard.includes('renderMessage'))wizard="import { renderMessage } from './i18n';\n"+wizard.replace("title: '' })).toContain", "title: '' }).map(value => renderMessage(value, 'zh-CN'))).toContain").replace("idea: '' })).toContain", "idea: '' }).map(value => renderMessage(value, 'zh-CN'))).toContain");fs.writeFileSync(wizardPath,wizard);
// Review inventory for dynamic backend display, without reading configuration.
for(const name of fs.readdirSync('internal/server').filter(x=>/observ|diagnos|workspace/.test(x)&&x.endsWith('.go')&&!x.endsWith('_test.go'))){console.log('SERVER_DISPLAY_SOURCE '+name+'\n'+fs.readFileSync('internal/server/'+name,'utf8'));}
console.log('PAGE_TEST_FILES '+fs.readdirSync('web/src/pages').filter(x=>x.endsWith('.test.ts')).join(', '));
