import { describe, expect, it } from 'vitest';
import { chatTime, dayBreaks } from './chatTime';

describe('chatTime', () => {
  const now = new Date(2026, 10, 4, 15, 0);

  it('shows the time alone for today', () => {
    expect(chatTime(new Date(2026, 10, 4, 10, 42), 'en-US', now)).toBe('10:42 AM');
  });

  it('marks the first message of each day, calling today by name', () => {
    const items = [
      { id: 1, at: new Date(2026, 10, 2, 20, 25) },
      { id: 2 },
      { id: 3, at: new Date(2026, 10, 2, 21, 0) },
      { id: 4, at: new Date(2026, 10, 4, 7, 49) },
      { id: 5, at: new Date(2026, 10, 4, 8, 0) },
    ];
    expect([...dayBreaks(items, 'en-US', 'Today', now)]).toEqual([[1, 'Nov 2, 2026'], [4, 'Today']]);
  });

  it('adds the date for older messages', () => {
    expect(chatTime(new Date(2026, 10, 3, 10, 42), 'id-ID', now)).toContain('3 Nov 2026');
  });
});
