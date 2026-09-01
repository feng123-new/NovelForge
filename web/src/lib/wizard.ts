import type { CreateProjectInput, FoundationRequestInput } from './types';

export interface WizardState {
  title: string;
  genre: string;
  language: string;
  targetWords: number;
  targetChapters: number;
  wordsPerChapter: number;
  idea: string;
  style: string;
  architectModel: string;
  writerModel: string;
  automationMode: 'copilot' | 'autopilot';
  reviewPolicy: 'every_chapter' | 'every_n' | 'full_automatic';
  reviewEveryN: number;
}

export const initialWizardState: WizardState = {
  title: '',
  genre: '',
  language: 'zh-CN',
  targetWords: 1_000_000,
  targetChapters: 300,
  wordsPerChapter: 3500,
  idea: '',
  style: '',
  architectModel: '',
  writerModel: '',
  automationMode: 'copilot',
  reviewPolicy: 'every_chapter',
  reviewEveryN: 5
};

export function validateWizardStep(step: number, state: WizardState): string[] {
  const errors: string[] = [];
  if (step === 1) {
    if (!state.title.trim()) errors.push('标题不能为空');
    if (state.targetWords < 1_000) errors.push('目标字数至少为 1000');
    if (state.targetChapters < 1) errors.push('目标章节至少为 1');
    if (state.wordsPerChapter < 100) errors.push('每章字数至少为 100');
  }
  if (step === 2 && !state.idea.trim()) errors.push('请填写核心创意');
  if (step === 4 && (!state.architectModel.trim() || !state.writerModel.trim())) {
    errors.push('请选择 Architect 和 Writer 模型');
  }
  if (step === 5 && state.reviewPolicy === 'every_n' && (state.reviewEveryN < 1 || state.reviewEveryN > 100)) {
    errors.push('审阅间隔必须为 1 到 100 章');
  }
  return errors;
}

export function buildWizardRequests(state: WizardState): {
  project: CreateProjectInput;
  foundation: FoundationRequestInput;
} {
  return {
    project: {
      title: state.title.trim(),
      genre: state.genre.trim(),
      language: state.language.trim(),
      target_words: state.targetWords,
      target_chapters: state.targetChapters,
      words_per_chapter: state.wordsPerChapter
    },
    foundation: {
      idea: state.idea.trim(),
      style: state.style.trim(),
      model_profile: {
        architect: state.architectModel.trim(),
        writer: state.writerModel.trim()
      },
      automation: {
        mode: state.automationMode,
        review_policy: state.reviewPolicy,
        review_every_n: state.reviewPolicy === 'every_n' ? state.reviewEveryN : undefined,
        max_rewrites: 2
      }
    }
  };
}
