import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { readFileSync } from 'node:fs';
import { applyLocale, preferredLocale, initializeLocale, locale, storageKey, zhCN, en, message, renderMessage, renderErrorOrMessage, errorMessage, translate, labelArgument, type MessageKey } from '../lib/i18n';
import { buildWizardRequests, initialWizardState, validateWizardStep } from '../lib/wizard';

function keyFor(value: string): MessageKey { const key = Object.keys(zhCN).find(key => zhCN[key as MessageKey] === value); if (!key) throw Error(value); return key as MessageKey; }
afterEach(() => { vi.unstubAllGlobals(); localStorage.clear(); applyLocale('zh-CN'); });
beforeEach(() => { localStorage.clear(); applyLocale('zh-CN'); });
describe('bilingual display-only locale', () => {
  it('has complete paired dictionaries and identical interpolation parameters', () => {
    expect(Object.keys(en).sort()).toEqual(Object.keys(zhCN).sort());
    expect(Object.keys(en).length).toBeGreaterThan(400);
    for (const key of Object.keys(zhCN) as MessageKey[]) {
      expect(zhCN[key].trim(), key).not.toBe(''); expect(en[key].trim(), key).not.toBe('');
      const parameters = (s: string) => [...new Set(s.match(/\{p\d+\}/g) ?? [])].sort();
      expect(parameters(en[key]), key).toEqual(parameters(zhCN[key]));
      expect(en[key], key).not.toMatch(/[\u3400-\u9fff]/);
    }
  });
  it('defaults to Chinese, rejects invalid preferences and restores explicit English', () => {
    expect(preferredLocale({ getItem: () => null })).toBe('zh-CN');
    expect(preferredLocale({ getItem: () => 'unsupported' })).toBe('zh-CN');
    expect(preferredLocale({ getItem: () => 'en' })).toBe('en');
    applyLocale('en'); expect(localStorage.getItem(storageKey)).toBe('en');
    locale.set('zh-CN'); expect(initializeLocale()).toBe('en'); expect(document.documentElement.lang).toBe('en');
  });
  it('switches safely when browser storage is blocked', () => {
    vi.stubGlobal('localStorage', { getItem() { throw Error('blocked'); }, setItem() { throw Error('blocked'); } });
    expect(preferredLocale()).toBe('zh-CN'); expect(() => applyLocale('en')).not.toThrow(); expect(get(locale)).toBe('en');
  });
  it('retranslates notices and errors that were created before a switch', () => {
    const pendingMessage = message(keyFor('任务启动失败'));
    expect(renderMessage(pendingMessage, 'zh-CN')).toBe('任务启动失败');
    expect(renderMessage(pendingMessage, 'en')).toBe('Failed to start job');
    const cause = { message: 'sensitive original is not shown', status: 409, payload: { code: 'PROJECT_BUSY' } };
    const before = JSON.stringify(cause), error = errorMessage(cause);
    expect(renderErrorOrMessage(error, 'zh-CN')).toContain('先暂停任务');
    expect(renderErrorOrMessage(error, 'en')).toContain('Pause'); expect(JSON.stringify(cause)).toBe(before);
  });
  it('keeps interpolation values literal, and explicitly localizes enum arguments', () => {
    const key = keyFor('已请求 {p0}，当前步骤尚未退出。');
    expect(translate(key, { p0: labelArgument('pause') }, 'en')).toContain('Pause');
    expect(translate(key, { p0: '<script>中文正文</script>' }, 'en')).toContain('<script>中文正文</script>');
  });
  it('does not alter manuscript language, wizard payload or original content', () => {
    const state = { ...initialWizardState, language: 'zh-TW', title: '原稿 Original', idea: '不翻译 this manuscript', style: '原样保留' };
    const before = JSON.stringify(buildWizardRequests(state)); applyLocale('en'); applyLocale('zh-CN');
    expect(JSON.stringify(buildWizardRequests(state))).toBe(before); expect(state.language).toBe('zh-TW');
  });
  it('retranslates already-created validation failures', () => {
    const errors = validateWizardStep(1, { ...initialWizardState, title: '' });
    expect(errors.map(e => renderMessage(e, 'zh-CN'))).toContain('标题不能为空');
    expect(errors.map(e => renderMessage(e, 'en'))).toContain('Title is required');
  });
  it('has no network, model, router or project dependency in the locale module', () => {
    const source = readFileSync(new URL('../lib/i18n/index.ts', import.meta.url), 'utf8');
    expect(source).not.toMatch(/import .*from ['"].*(?:api|router|autopilot|project|wizard)/);
    expect(source).not.toMatch(/\bfetch\s*\(|location\.(?:reload|hash)\s*\(|new EventSource/);
  });
});
