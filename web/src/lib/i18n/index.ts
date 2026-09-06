import { derived, get, writable } from 'svelte/store';
import { zhCN, type MessageKey } from './zh-CN';
import { en } from './en';
import { labels, errorCodes, serverMessages, type Pair } from './system';
import { diagnostics, type DiagnosticField } from './diagnostics';

export type Locale = 'zh-CN' | 'en';
export type Parameters = Record<string, unknown>;
export const storageKey = 'novelforge-locale';
export const locale = writable<Locale>('zh-CN');
const marker = '\u001eNovelForge:message:';
const dictionaries: Record<Locale, Record<MessageKey, string>> = { 'zh-CN': zhCN, en };

export function isLocale(value: unknown): value is Locale { return value === 'zh-CN' || value === 'en'; }
function storage(): Storage | undefined { try { return globalThis.localStorage; } catch { return undefined; } }
export function preferredLocale(store?: Pick<Storage, 'getItem'>): Locale {
  try { const value = (store ?? storage())?.getItem(storageKey); return isLocale(value) ? value : 'zh-CN'; }
  catch { return 'zh-CN'; }
}
/** Browser display preference only. Never imports an API, router, project or task service. */
export function applyLocale(value: Locale): void {
  if (!isLocale(value)) return;
  try { storage()?.setItem(storageKey, value); } catch { /* Storage is optional. */ }
  if (typeof document !== 'undefined') document.documentElement.lang = value;
  locale.set(value);
}
export function initializeLocale(): Locale { const value = preferredLocale(); applyLocale(value); return value; }

export function translate(key: MessageKey, params: Parameters = {}, language: Locale = get(locale)): string {
  const template = Object.hasOwn(dictionaries[language], key) ? dictionaries[language][key] : undefined;
  if (typeof template !== 'string') return language === 'en' ? 'Text unavailable' : '文本暂不可用';
  return template.replace(/\{(p\d+)\}/g, (_match, name: string) => formatParameter(params[name], language));
}
/** Locale-neutral UI message. Store the key/arguments, not a translated snapshot. Never persist in project data. */
export function message(key: MessageKey, params: Parameters = {}): string { return marker + JSON.stringify([key, params]); }
export function renderMessage(value: unknown, language: Locale = get(locale)): string {
  if (value === null || value === undefined) return '';
  const raw = String(value);
  if (!raw.startsWith(marker)) return raw;
  try {
    const [key, params] = JSON.parse(raw.slice(marker.length));
    if (Object.hasOwn(zhCN, key) && params && typeof params === 'object' && !Array.isArray(params)) return translate(key, params, language);
  } catch { /* A malformed UI reference cannot break rendering. */ }
  return language === 'en' ? 'Text unavailable' : '文本暂不可用';
}
function pair(value: readonly [string, string], language: Locale): string { return value[language === 'en' ? 1 : 0]; }
export function labelFor(value: unknown, language: Locale = get(locale)): string {
  if (typeof value === 'number') return String(value);
  const raw = String(value ?? 'unknown');
  if (raw === '') return '';
  const found = own(labels, raw) ?? own(labels, raw.toLowerCase());
  return found ? pair(found, language) : `${language === 'en' ? 'Other' : '其他'} (${raw})`;
}
export function serverMessage(value: unknown, language: Locale = get(locale)): string {
  const raw = String(value ?? '');
  if (!raw) return '';
  const known = own(serverMessages, raw);
  const chapterImpact = /^Accepted Human Final at Chapter (\d+) changed a fact consumed by downstream planning$/.exec(raw);
  if (chapterImpact) return language === 'en' ? raw : `第 ${chapterImpact[1]} 章的已接受人工定稿改变了后续规划使用的事实`;
  const persisted = /^(rewrite )?draft persisted: (.+)$/.exec(raw);
  if (persisted) return language === 'en' ? raw : `${persisted[1] ? '重写草稿' : '草稿'}已保存：${persisted[2]}`;
  const continuity = /^continuity result (PASS|WARN|FAIL)$/.exec(raw);
  if (continuity) return language === 'en' ? raw : `一致性结果：${labelFor(continuity[1], language)}`;
  return known ? pair(known, language) : `${language === 'en' ? 'Technical detail' : '技术详情'}: ${raw}`;
}
/** Read only the safe public error envelope; do not mutate Error.message or the API contract. */
export function errorMessage(cause: unknown): string {
  const e = cause && typeof cause === 'object' ? cause as { status?: number; message?: string; payload?: { code?: string; message?: string; trace_id?: string } } : {};
  if (typeof e.message === 'string' && e.message.startsWith(marker)) return e.message;
  const code = e.payload?.code ?? (e.message === 'Server returned invalid JSON' ? 'INVALID_RESPONSE' : e.status ? `HTTP_${e.status}` : 'CLIENT_ERROR');
  return '\u001eNovelForge:error:' + JSON.stringify({ code, status: e.status ?? 0 });
}
export function renderErrorOrMessage(value: unknown, language: Locale = get(locale)): string {
  const raw = String(value ?? '');
  const prefix = '\u001eNovelForge:error:';
  if (!raw.startsWith(prefix)) return renderMessage(value, language);
  try {
    const { code, status } = JSON.parse(raw.slice(prefix.length));
    const category = status === 401 || status === 403 ? 'NOT_ALLOWED' : status === 413 ? 'REQUEST_BODY_TOO_LARGE' : status === 404 ? 'NOT_FOUND' : status === 409 ? 'CONFLICT' : status === 400 || status === 422 ? 'INVALID_INPUT' : status === 503 ? 'UNAVAILABLE' : 'UNKNOWN';
    const advice = own(diagnostics, String(code));
    return (advice ? pair(advice.message, language) + ' ' + pair(advice.action, language) : pair(own(errorCodes, String(code)) ?? errorCodes[category] ?? errorCodes.UNKNOWN, language)) + ` (${code})`;
  } catch { return pair(errorCodes.UNKNOWN, language); }
}
export const t = derived(locale, language => (key: MessageKey, params: Parameters = {}) => translate(key, params, language));
export const text = derived(locale, language => (value: unknown) => renderErrorOrMessage(value, language));
export const label = derived(locale, language => (value: unknown) => labelFor(value, language));
export const serverText = derived(locale, language => (value: unknown) => serverMessage(value, language));
export { zhCN, en };
export type { MessageKey };

export function labelArgument(value: unknown): { uiLabel: string } { return { uiLabel: String(value ?? 'unknown') }; }
function formatParameter(value: unknown, language: Locale): string {
  if (value && typeof value === 'object' && 'uiLabel' in value) return labelFor((value as { uiLabel: string }).uiLabel, language);
  return String(value ?? '');
}

function own<T>(record: Record<string, T>, key: string): T | undefined { return Object.hasOwn(record, key) ? record[key] : undefined; }
export interface DisplayFinding { code: string; message?: string; action?: string; phrase?: string; count?: number; limit?: number }
const ruleTitles: Record<string, Pair> = {
  PHRASE_OVERUSE: ['词组使用过多', 'Phrase overuse'],
  SENTENCE_REPEATED: ['句子重复', 'Sentence repeated'],
  RECENT_SENTENCE_REUSED: ['复用近期句子', 'Recent sentence reused']
};
export function diagnosticMessage(finding: DisplayFinding, field: DiagnosticField = 'message', language: Locale = get(locale)): string {
  const known = own(diagnostics, finding.code);
  if (known) return pair(known[field], language);
  const rule = own(ruleTitles, finding.code);
  if (rule) {
    if (field === 'title') return pair(rule, language);
    return language === 'en'
      ? `${pair(rule, language)}: “${finding.phrase ?? ''}” occurred ${finding.count ?? 0} times (limit ${finding.limit ?? 0}). Review intent before changing the text.`
      : `${pair(rule, language)}：“${finding.phrase ?? ''}”出现 ${finding.count ?? 0} 次（上限 ${finding.limit ?? 0} 次），请核对表达意图后再修改。`;
  }
  if (field === 'title') return `${language === 'en' ? 'Diagnostic' : '诊断项'} (${finding.code})`;
  if (field === 'action') return language === 'en' ? 'Inspect the job and chapter version. Include redacted diagnostics when reporting the issue.' : '请检查任务与章节版本，反馈问题时附上脱敏诊断。';
  return language === 'en' ? 'The server reported an issue. Inspect its original technical details.' : '服务端报告了待处理问题，请查看原始技术详情。';
}
export const findingText = derived(locale, language => (finding: DisplayFinding, field: DiagnosticField = 'message') => diagnosticMessage(finding, field, language));
export const codeText = derived(locale, language => (code: string) => code ? renderErrorOrMessage(errorMessage({ payload: { code } }), language) : '');
