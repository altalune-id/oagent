// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from 'vitest';
import { applyTheme, themeFor } from './theme';

describe('theme', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('follows the host, else the system preference', () => {
    expect(themeFor('dark')).toBe('oagent-dark');
    expect(themeFor('light', true)).toBe('oagent-light');
    expect(themeFor(undefined)).toBe('oagent-light');
    expect(themeFor(undefined, true)).toBe('oagent-dark');
  });

  it('sets data-theme on the document, reading prefers-color-scheme without a host theme', () => {
    vi.stubGlobal('matchMedia', () => ({ matches: true }));
    applyTheme(undefined);
    expect(document.documentElement.dataset.theme).toBe('oagent-dark');
    applyTheme('light');
    expect(document.documentElement.dataset.theme).toBe('oagent-light');
  });
});
