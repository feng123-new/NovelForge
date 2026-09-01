import { describe, expect, it } from 'vitest';
import { preferredTheme } from './theme';

describe('theme preference', () => {
  it('uses a valid stored theme', () => {
    expect(preferredTheme({ getItem: () => 'novelforge-light' })).toBe('novelforge-light');
  });

  it('ignores invalid stored values', () => {
    const original = globalThis.matchMedia;
    globalThis.matchMedia = (() => ({ matches: false })) as unknown as typeof matchMedia;
    expect(preferredTheme({ getItem: () => 'credential-looking-garbage' })).toBe('novelforge-dark');
    globalThis.matchMedia = original;
  });
});
