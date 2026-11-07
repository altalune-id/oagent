import { describe, expect, it } from 'vitest';
import { shownName } from './forget';

const t = (key: string) => key;

describe('shownName', () => {
  it('shows "Deleted person" in place of the name once deleted', () => {
    expect(shownName('', true, t)).toBe('endusers.deleted_person');
    expect(shownName('Sari', false, t)).toBe('Sari');
    expect(shownName(undefined, undefined, t)).toBe('');
  });
});
