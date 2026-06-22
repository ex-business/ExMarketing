export type ExmThemeMode = 'light' | 'dark';

export const EXM_THEME_STORAGE_KEY = 'exmarketing.adminFlow.theme';

export function initialExmTheme(): ExmThemeMode {
  if (typeof window === 'undefined') return 'light';
  try {
    const stored = window.localStorage.getItem(EXM_THEME_STORAGE_KEY);
    if (stored === 'dark' || stored === 'light') return stored;
  } catch {
    return 'light';
  }
  return window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

export function applyDocumentTheme(themeMode: ExmThemeMode) {
  document.documentElement.dataset.exmTheme = themeMode;
  document.documentElement.style.colorScheme = themeMode === 'dark' ? 'dark' : 'light';
}
