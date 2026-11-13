import { describe, expect, it } from 'vitest';
import en from '~~/i18n/locales/en-US.json';
import id from '~~/i18n/locales/id-ID.json';
import { clearCopy, clip } from './memories';

describe('memories', () => {
  it('names the scope of a clear: one agent, or every agent', () => {
    expect(clearCopy('Budi', 3, 'People', 'Support')).toEqual({
      title: { key: 'endusers.memories_clear_agent_title', params: { n: 3, agent: 'Support', name: 'Budi', store: 'People' }, count: 3 },
      body: { key: 'endusers.memories_clear_agent_body', params: { agent: 'Support' } },
    });
    expect(clearCopy('Budi', 7, 'People')).toEqual({
      title: { key: 'endusers.memories_clear_title', params: { n: 7, name: 'Budi', store: 'People' }, count: 7 },
      body: { key: 'endusers.memories_clear_body', params: {} },
    });
  });

  it('has a singular and a plural clear title, and says a clear can\'t be undone for one agent too, in every locale', () => {
    for (const locale of [en, id] as Record<string, string>[]) {
      for (const key of ['endusers.memories_clear_title', 'endusers.memories_clear_agent_title']) {
        expect(locale[key]!.split(' | ')).toHaveLength(2);
      }
      expect(locale['endusers.memories_clear_agent_body']).toContain(locale['endusers.memories_clear_body']!.split('. ').at(-1));
    }
  });

  it('clips a long memory for the delete confirm without cutting a word in half', () => {
    expect(clip('Prefers replies in Indonesian', 120)).toBe('Prefers replies in Indonesian');
    expect(clip('Account number ends in 4421 and the card is blue', 30)).toBe('Account number ends in 4421…');
    expect(clip('Supercalifragilisticexpialidocious', 10)).toBe('Supercali…');
  });
});
