import { describe, expect, it } from 'vitest';
import { describedBy, nameTaken } from './names';

describe('nameTaken', () => {
  const items = [{ id: 'a', name: 'OpenRouter' }, { id: 'b', name: 'yasaku' }];

  it('matches names ignoring case and spaces', () => {
    expect(nameTaken(items, ' openrouter ')).toBe(true);
    expect(nameTaken(items, 'YASAKU')).toBe(true);
    expect(nameTaken(items, 'Groq')).toBe(false);
  });

  it('skips the item being edited', () => {
    expect(nameTaken(items, 'OpenRouter', 'a')).toBe(false);
  });
});

describe('describedBy', () => {
  it('keeps only the ids on screen', () => {
    expect(describedBy('hint', false, 'err')).toBe('hint err');
    expect(describedBy(undefined, false)).toBeUndefined();
  });
});
