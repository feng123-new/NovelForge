import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { diagnosticMessage, labelFor, serverMessage, renderMessage, renderErrorOrMessage, errorMessage } from '../lib/i18n';
import { diagnostics } from '../lib/i18n/diagnostics';
import { labels } from '../lib/i18n/system';

describe('diagnostic and enum localization', () => {
  it('localizes known findings by code while retaining original payload and rule text', () => {
    for (const code of Object.keys(diagnostics)) {
      const finding = { code, message: '原始消息保持不变', action: 'original action', count: 1 };
      const before = JSON.stringify(finding);
      for (const field of ['title', 'message', 'action'] as const) {
        expect(diagnosticMessage(finding, field, 'zh-CN')).toMatch(/[\u3400-\u9fff]/);
        expect(diagnosticMessage(finding, field, 'en')).not.toMatch(/[\u3400-\u9fff]/);
      }
      expect(JSON.stringify(finding)).toBe(before);
    }
    const rule = { code: 'PHRASE_OVERUSE', phrase: '原文 Original', count: 4, limit: 2 };
    expect(diagnosticMessage(rule, 'message', 'en')).toContain('“原文 Original” occurred 4 times (limit 2)');
    expect(diagnosticMessage(rule, 'message', 'zh-CN')).toContain('“原文 Original”出现 4 次（上限 2 次）');
    expect(labelFor(2, 'zh-CN')).toBe('2'); expect(labelFor('', 'en')).toBe('');
    expect(serverMessage('continuity blocks finalization', 'zh-CN')).toBe('一致性检查阻止定稿');
  });
  it('covers production capability names and all current diagnostic codes', () => {
    const root = process.cwd() + '/../';
    const workspace = readFileSync(root + 'internal/server/workspace.go', 'utf8');
    const block = workspace.split('Capabilities: map[string]any{')[1].split('},')[0];
    for (const match of block.matchAll(/"([a-z_]+)":/g)) expect(Object.hasOwn(labels, match[1]), match[1]).toBe(true);
    const code = readFileSync(root + 'internal/observability/store.go', 'utf8');
    const explain = code.split('func Explain(')[1].split('func (s *Store) Findings')[0];
    for (const line of explain.split('\n').filter(line => line.trim().startsWith('case '))) {
      for (const match of line.matchAll(/"([A-Z_]+)"/g)) expect(Object.hasOwn(diagnostics, match[1]), match[1]).toBe(true);
    }
  });
  it('does not confuse unknown prototype keys or raw content with translated enums', () => {
    expect(labelFor('__proto__', 'en')).toBe('Other (__proto__)');
    expect(diagnosticMessage({ code: 'constructor' }, 'title', 'zh-CN')).toBe('诊断项 (constructor)');
    expect(renderErrorOrMessage(errorMessage({ payload: { code: '__proto__' } }), 'en')).toContain('(__proto__)');
    expect(renderMessage('原稿 <script>untouched</script>', 'en')).toBe('原稿 <script>untouched</script>');
  });
});
