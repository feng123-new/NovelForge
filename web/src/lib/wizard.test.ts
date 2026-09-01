import { describe, expect, it } from 'vitest';
import { buildWizardRequests, initialWizardState, validateWizardStep } from './wizard';

describe('new novel wizard', () => {
  it('validates required basic and idea fields', () => {
    expect(validateWizardStep(1, { ...initialWizardState, title: '' })).toContain('标题不能为空');
    expect(validateWizardStep(2, { ...initialWizardState, idea: '' })).toContain('请填写核心创意');
  });

  it('builds separate project and secret-free foundation requests', () => {
    const requests = buildWizardRequests({
      ...initialWizardState,
      title: '天路',
      idea: '一名信使穿越破碎帝国。',
      architectModel: 'openai/architect',
      writerModel: 'openai/writer'
    });
    expect(requests.project.title).toBe('天路');
    expect(requests.foundation.model_profile).toEqual({
      architect: 'openai/architect',
      writer: 'openai/writer'
    });
    expect(JSON.stringify(requests)).not.toMatch(/secret|password|credential/i);
  });
});
