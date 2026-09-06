import { message as uiMessage } from './i18n';
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
    if (!state.title.trim()) errors.push(uiMessage("ui_015edb6a8f1a"));
    if (state.targetWords < 1_000) errors.push(uiMessage("ui_80d44788785d"));
    if (state.targetChapters < 1) errors.push(uiMessage("ui_098ca67fa50a"));
    if (state.wordsPerChapter < 100) errors.push(uiMessage("ui_0dbe529fce81"));
  }
  if (step === 2 && !state.idea.trim()) errors.push(uiMessage("ui_3de3e150fe40"));
  // Empty role selections inherit the configured project providers.
  if (step === 5 && state.reviewPolicy === 'every_n' && (state.reviewEveryN < 1 || state.reviewEveryN > 100)) {
    errors.push(uiMessage("ui_44c5a045fcb3"));
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
      model_profile: Object.fromEntries(Object.entries({ architect: state.architectModel.trim(), writer: state.writerModel.trim() }).filter(([, value]) => value !== '')),
      automation: {
        mode: state.automationMode,
        review_policy: state.reviewPolicy,
        review_every_n: state.reviewPolicy === 'every_n' ? state.reviewEveryN : undefined,
        max_rewrites: 2
      }
    }
  };
}
