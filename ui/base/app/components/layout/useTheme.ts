export type ThemeMode = 'light' | 'dark';

export interface ThemeFamily {
  id: string;
  label: string;
  themes: Partial<Record<ThemeMode, string>>;
  default: ThemeMode;
}

// Keep in sync with the @plugin "daisyui/theme" blocks in app/assets/css/main.css.
export const themeFamilies: ThemeFamily[] = [
  { id: 'oagent', label: 'oagent', default: 'light', themes: { light: 'oagent-light', dark: 'oagent-dark' } },
  { id: 'bumblebee', label: 'Bumblebee', default: 'light', themes: { light: 'bumblebee-light', dark: 'bumblebee-dark' } },
  { id: 'retro', label: 'Retro', default: 'light', themes: { light: 'retro-light', dark: 'retro-dark' } },
  { id: 'pastel', label: 'Pastel', default: 'light', themes: { light: 'pastel-light', dark: 'pastel-dark' } },
  { id: 'caramellatte', label: 'Caramellatte', default: 'light', themes: { light: 'caramellatte-light', dark: 'caramellatte-dark' } },
  { id: 'coffee', label: 'Coffee', default: 'dark', themes: { dark: 'coffee-dark' } },
  { id: 'luxury', label: 'Luxury', default: 'dark', themes: { dark: 'luxury-dark' } },
  { id: 'night', label: 'Night', default: 'dark', themes: { dark: 'night-dark' } },
  { id: 'halloween', label: 'Halloween', default: 'dark', themes: { dark: 'halloween-dark' } },
];

const FALLBACK: ThemeFamily = themeFamilies[0]!;
const STORAGE_KEY = 'oagent_theme';

function themeName(family: ThemeFamily, mode: ThemeMode): string {
  return family.themes[mode] ?? family.themes[family.default]!;
}

export function useTheme() {
  const familyId = useState<string>('theme.family', () => FALLBACK.id);
  const mode = useState<ThemeMode>('theme.mode', () => FALLBACK.default);

  const family = computed<ThemeFamily>(() => themeFamilies.find(f => f.id === familyId.value) ?? FALLBACK);
  const supportsMode = computed(() => Object.keys(family.value.themes).length > 1);

  function apply() {
    if (!import.meta.client) {
      return;
    }
    document.documentElement.setAttribute('data-theme', themeName(family.value, mode.value));
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ family: familyId.value, mode: mode.value }));
  }

  function setFamily(id: string) {
    const next = themeFamilies.find(f => f.id === id);
    if (!next) {
      return;
    }
    familyId.value = next.id;
    mode.value = next.default;
    apply();
  }

  function setMode(next: ThemeMode) {
    if (!family.value.themes[next]) {
      return;
    }
    mode.value = next;
    apply();
  }

  function toggle() {
    if (!supportsMode.value) {
      return;
    }
    setMode(mode.value === 'dark' ? 'light' : 'dark');
  }

  function init() {
    if (!import.meta.client) {
      return;
    }
    try {
      const saved = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}') as { family?: string, mode?: ThemeMode };
      const next = themeFamilies.find(f => f.id === saved.family);
      if (next) {
        familyId.value = next.id;
        mode.value = saved.mode && next.themes[saved.mode] ? saved.mode : next.default;
      }
    } catch {
    }
    apply();
  }

  return { family, mode, supportsMode, families: themeFamilies, setFamily, setMode, toggle, init };
}
