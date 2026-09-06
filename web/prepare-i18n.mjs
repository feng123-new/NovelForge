import fs from 'node:fs';
let s=fs.readFileSync('web/localize.mjs','utf8');
s=s.replace("if(n.type==='LogicalExpression')return `(${expression(n.left)} ${n.operator} ${expression(n.right)})`;", "if(n.type==='LogicalExpression'){ const value=n.left.type==='ChainExpression'?n.left.expression:n.left; if(value.type==='MemberExpression'&&machineProps.has(value.property?.name))return `$label(${raw})`; return `(${slice(n.left)} ${n.operator} ${expression(n.right)})`; }");
fs.writeFileSync('web/localize.mjs',s);
let runtime=fs.readFileSync('web/src/lib/i18n/index.ts','utf8');
if(!runtime.includes('export function labelArgument')){
 runtime=runtime.replace("String(params[name] ?? '')", "formatParameter(params[name], language)");
 runtime=runtime.replace("const code = e.payload?.code", "if (typeof e.message === 'string' && e.message.startsWith(marker)) return e.message;\n  const code = e.payload?.code");
 runtime+=`\nexport function labelArgument(value: unknown): { uiLabel: string } { return { uiLabel: String(value ?? 'unknown') }; }\nfunction formatParameter(value: unknown, language: Locale): string {\n  if (value && typeof value === 'object' && 'uiLabel' in value) return labelFor((value as { uiLabel: string }).uiLabel, language);\n  return String(value ?? '');\n}\n`;
 fs.writeFileSync('web/src/lib/i18n/index.ts',runtime);
}
// Native Go nil slices serialize to JSON null for a project without a quality transaction.
const chapterPath='web/src/pages/Chapters.svelte';
fs.writeFileSync(chapterPath,fs.readFileSync(chapterPath,'utf8').replaceAll('quality?.snapshot.candidates.length','quality?.snapshot.candidates?.length').replaceAll("quality?.snapshot.transaction.state ?? 'not_started'","quality?.snapshot.transaction.state || 'not_started'"));
const typesPath='web/src/lib/types.ts';
fs.writeFileSync(typesPath,fs.readFileSync(typesPath,'utf8').replace('  candidates: QualityCandidate[];','  candidates: QualityCandidate[] | null;').replace('  state_changes: Array<Record<string, unknown>>;','  state_changes: Array<Record<string, unknown>> | null;'));
const finishPath='web/finish-i18n.mjs';
fs.writeFileSync(finishPath,fs.readFileSync(finishPath,'utf8').replaceAll('18 bilingual tests','20 bilingual tests').replaceAll('18 focused bilingual tests','20 focused bilingual tests'));
