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
