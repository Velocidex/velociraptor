import { useEffect } from 'react';
import * as api from '../lib/api';
import { useTraits } from '../lib/queries';

// Legacy themes are still selectable for the legacy screens; the new
// UI renders light or dark depending on which family they belong to.
const DARK_THEMES = new Set([
  'veloci-dark', 'github-dimmed-dark', 'ncurses-dark', 'coolgray-dark', 'midnight', 'vscode-dark',
]);

const CACHE_KEY = 'rx-theme-dark';

export function isDarkTheme(theme: string | undefined): boolean {
  if (!theme) {
    try {
      return localStorage.getItem(CACHE_KEY) === '1';
    } catch {
      return false;
    }
  }
  return DARK_THEMES.has(theme);
}

export function applyThemeClass(dark: boolean) {
  const el = document.documentElement;
  el.classList.toggle('rx-dark', dark);
  el.classList.toggle('rx-light', !dark);
  try {
    localStorage.setItem(CACHE_KEY, dark ? '1' : '0');
  } catch {
    // ignore
  }
}

// Apply the cached theme as early as possible to avoid a flash.
applyThemeClass(isDarkTheme(undefined));

export function useThemeSync(): boolean {
  const { traits } = useTraits();
  const dark = isDarkTheme(traits?.theme);
  useEffect(() => {
    applyThemeClass(dark);
  }, [dark]);
  return dark;
}

export async function setTheme(dark: boolean, uiSettings: string | undefined, onDone: () => void) {
  let options: Record<string, unknown> = {};
  try {
    options = uiSettings ? JSON.parse(uiSettings) : {};
  } catch {
    options = {};
  }
  options.theme = dark ? 'ace/theme/vibrant_ink' : 'ace/theme/xcode';
  options.fontFamily = 'Iosevka Term';
  applyThemeClass(dark);
  await api.post('v1/SetGUIOptions', {
    theme: dark ? 'veloci-dark' : 'veloci-light',
    options: JSON.stringify(options),
  });
  onDone();
}
