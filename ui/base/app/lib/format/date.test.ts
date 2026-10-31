import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { describe, expect, it } from 'vitest';
import { formatDate } from './date';

const day = new Date(2026, 9, 27, 14, 5);

describe('formatDate', () => {
  it('formats a Date in the locale\'s medium style', () => {
    expect(formatDate(day, 'en-US')).toBe('Oct 27, 2026');
    expect(formatDate(day, 'id-ID')).toBe('27 Okt 2026');
  });

  it('formats a protobuf Timestamp the same way', () => {
    expect(formatDate(timestampFromDate(day), 'en-US')).toBe('Oct 27, 2026');
  });

  it('adds the time for a moment, or shows only the time', () => {
    expect(formatDate(day, 'en-US', { style: 'moment' })).toBe('Oct 27, 2026, 2:05 PM');
    expect(formatDate(day, 'en-US', { style: 'time' })).toBe('2:05 PM');
  });

  it('shows the empty text when there is no date', () => {
    expect(formatDate(undefined, 'en-US')).toBe('');
    expect(formatDate(null, 'en-US', { empty: '—' })).toBe('—');
  });
});
