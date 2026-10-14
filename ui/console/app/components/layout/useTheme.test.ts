import { beforeEach, describe, expect, it, vi } from 'vitest';

import { fakeStorage } from '../../../test/nuxt-globals';
import { themeFamilies, useTheme } from './useTheme';

describe('useTheme', () => {
  let storage: ReturnType<typeof fakeStorage>;
  let setAttribute: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    storage = fakeStorage();
    setAttribute = vi.fn();
    vi.stubGlobal('localStorage', storage);
    vi.stubGlobal('document', { documentElement: { setAttribute } });
  });

  function applied() {
    return setAttribute.mock.calls.at(-1)?.[1];
  }

  it('defaults to the first family in its default mode', () => {
    const theme = useTheme();
    theme.init();
    expect(theme.family.value.id).toBe('oagent');
    expect(theme.mode.value).toBe('light');
    expect(theme.supportsMode.value).toBe(true);
    expect(theme.families).toBe(themeFamilies);
    expect(applied()).toBe('oagent-light');
    expect(JSON.parse(storage.data.get('oagent_theme')!)).toEqual({ family: 'oagent', mode: 'light' });
  });

  it('restores a saved family and mode', () => {
    storage.data.set('oagent_theme', JSON.stringify({ family: 'retro', mode: 'dark' }));
    const theme = useTheme();
    theme.init();
    expect(theme.family.value.id).toBe('retro');
    expect(theme.mode.value).toBe('dark');
    expect(applied()).toBe('retro-dark');
  });

  it('uses the family default for a saved mode it lacks or no mode', () => {
    storage.data.set('oagent_theme', JSON.stringify({ family: 'night', mode: 'light' }));
    const theme = useTheme();
    theme.init();
    expect(theme.mode.value).toBe('dark');
    storage.data.set('oagent_theme', JSON.stringify({ family: 'pastel' }));
    theme.init();
    expect(theme.mode.value).toBe('light');
    expect(applied()).toBe('pastel-light');
  });

  it('ignores an unknown saved family and corrupt storage', () => {
    storage.data.set('oagent_theme', JSON.stringify({ family: 'gone', mode: 'dark' }));
    const theme = useTheme();
    theme.init();
    expect(theme.family.value.id).toBe('oagent');
    storage.data.set('oagent_theme', '{bad');
    theme.init();
    expect(theme.mode.value).toBe('light');
    expect(applied()).toBe('oagent-light');
  });

  it('switches family and mode, ignoring unknown values', () => {
    const theme = useTheme();
    theme.setFamily('coffee');
    expect(theme.family.value.id).toBe('coffee');
    expect(theme.mode.value).toBe('dark');
    expect(theme.supportsMode.value).toBe(false);
    theme.setMode('light');
    theme.toggle();
    expect(theme.mode.value).toBe('dark');
    theme.setFamily('nope');
    expect(theme.family.value.id).toBe('coffee');
    theme.setFamily('bumblebee');
    theme.toggle();
    expect(theme.mode.value).toBe('dark');
    expect(applied()).toBe('bumblebee-dark');
    theme.toggle();
    expect(applied()).toBe('bumblebee-light');
  });

  it('shares state between callers', () => {
    useTheme().setFamily('luxury');
    expect(useTheme().family.value.id).toBe('luxury');
  });
});
