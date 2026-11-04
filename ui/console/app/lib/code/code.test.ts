import { describe, expect, it } from 'vitest';
import { splitCode } from './code';

describe('splitCode', () => {
  it('marks the scopes, events and headers named in copy', () => {
    expect(splitCode('Use a key with run:create. Done.')).toEqual([
      { text: 'Use a key with ', code: false },
      { text: 'run:create', code: true },
      { text: '. Done.', code: false },
    ]);
    expect(splitCode('messages:write and messages:read').map(p => p.code)).toEqual([true, false, true]);
  });

  it('keeps text without tokens whole and treats tokens literally', () => {
    expect(splitCode('No tokens here')).toEqual([{ text: 'No tokens here', code: false }]);
    expect(splitCode('a.b and axb', ['a.b'])).toEqual([{ text: 'a.b', code: true }, { text: ' and axb', code: false }]);
    expect(splitCode('plain', [])).toEqual([{ text: 'plain', code: false }]);
  });
});
