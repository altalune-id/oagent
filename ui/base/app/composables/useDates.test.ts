import { describe, expect, it, vi } from 'vitest';
import { ref } from 'vue';
import { useDates } from './useDates';

describe('useDates', () => {
  it('formats in the current locale and follows a locale switch', () => {
    const locale = ref('en-US');
    vi.stubGlobal('useI18n', () => ({ locale }));
    const { date } = useDates('—');
    const day = new Date(Date.UTC(2026, 9, 27, 12));
    expect(date(day)).toBe('Oct 27, 2026');
    locale.value = 'id-ID';
    expect(date(day)).toBe('27 Okt 2026');
    expect(date(undefined)).toBe('—');
    expect(useDates().moment(new Date(2026, 9, 27, 14, 5))).toBe('27 Okt 2026, 14.05');
    vi.unstubAllGlobals();
  });

  it('says how long ago from when it was called', () => {
    vi.stubGlobal('useI18n', () => ({ locale: ref('en-US') }));
    const { when } = useDates();
    expect(when(new Date(Date.now() - 3 * 86_400_000))).toBe('3 days ago');
    expect(when(undefined)).toBe('');
    vi.unstubAllGlobals();
  });
});
