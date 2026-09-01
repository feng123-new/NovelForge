import { writable } from 'svelte/store';

export type WorkspaceTheme = 'novelforge-light' | 'novelforge-dark';
const storageKey = 'novelforge-theme';

export function preferredTheme(storage?: Pick<Storage, 'getItem'>): WorkspaceTheme {
  let stored: string | null = null;
  try {
    stored = storage?.getItem(storageKey) ?? browserStorage()?.getItem(storageKey) ?? null;
  } catch {
    stored = null;
  }
  if (stored === 'novelforge-light' || stored === 'novelforge-dark') return stored;
  return globalThis.matchMedia?.('(prefers-color-scheme: light)').matches
    ? 'novelforge-light'
    : 'novelforge-dark';
}

export const theme = writable<WorkspaceTheme>('novelforge-dark');

export function applyTheme(value: WorkspaceTheme): void {
  document.documentElement.dataset.theme = value;
  try {
    browserStorage()?.setItem(storageKey, value);
  } catch {
    // Theme persistence is optional; project authority never lives here.
  }
  theme.set(value);
}

export function initializeTheme(): WorkspaceTheme {
  const value = preferredTheme();
  applyTheme(value);
  return value;
}

function browserStorage(): Storage | undefined {
  try {
    return globalThis.localStorage;
  } catch {
    return undefined;
  }
}
