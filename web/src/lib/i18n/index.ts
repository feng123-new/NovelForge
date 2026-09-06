import { derived, get, writable } from 'svelte/store';
import { zhCN, type MessageKey } from './zh-CN';
import { en } from './en';
import { labels, errorCodes, serverMessages } from './system';

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
  const template = dictionaries[language][key] ?? dictionaries['zh-CN'][key];
  if (typeof template !== 'string') return language === 'en' ? 'Text unavailable' : '文本暂不可用';
  return template.replace(/\{(p\d+)\}/g, (_match, name: string) => String(params[name] ?? ''));
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
  const raw = String(value ?? 'unknown');
  const found = labels[raw] ?? labels[raw.toLowerCase()];
  return found ? pair(found, language) : `${language === 'en' ? 'Other' : '其他'} (${raw})`;
}
export function serverMessage(value: unknown, language: Locale = get(locale)): string {
  const raw = String(value ?? '');
  if (!raw) return '';
  const known = serverMessages[raw];
  return known ? pair(known, language) : `${language === 'en' ? 'Technical detail' : '技术详情'}: ${raw}`;
}
/** Read only the safe public error envelope; do not mutate Error.message or the API contract. */
export function errorMessage(cause: unknown): string {
  const e = cause && typeof cause === 'object' ? cause as { status?: number; message?: string; payload?: { code?: string; message?: string; trace_id?: string } } : {};
  const code = e.payload?.code ?? (e.status ? `HTTP_${e.status}` : 'CLIENT_ERROR');
  return '\u001eNovelForge:error:' + JSON.stringify({ code, status: e.status ?? 0 });
}
export function renderErrorOrMessage(value: unknown, language: Locale = get(locale)): string {
  const raw = String(value ?? '');
  const prefix = '\u001eNovelForge:error:';
  if (!raw.startsWith(prefix)) return renderMessage(value, language);
  try {
    const { code, status } = JSON.parse(raw.slice(prefix.length));
    const category = status === 404 ? 'NOT_FOUND' : status === 409 ? 'CONFLICT' : status === 400 || status === 422 ? 'INVALID_INPUT' : status === 503 ? 'UNAVAILABLE' : 'UNKNOWN';
    return pair(errorCodes[code] ?? errorCodes[category] ?? errorCodes.UNKNOWN, language) + ` (${code})`;
  } catch { return pair(errorCodes.UNKNOWN, language); }
}
export const t = derived(locale, language => (key: MessageKey, params: Parameters = {}) => translate(key, params, language));
export const text = derived(locale, language => (value: unknown) => renderErrorOrMessage(value, language));
export const label = derived(locale, language => (value: unknown) => labelFor(value, language));
export const serverText = derived(locale, language => (value: unknown) => serverMessage(value, language));
export { zhCN, en };
export type { MessageKey };
