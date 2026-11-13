import type { KnowledgeTuning } from './agentConfig';
import type { RetrievalDefaults } from '~/stores/knowledge';
import { describe, expect, it } from 'vitest';
import { effectiveTuning, presetLabel, presetOf, recommendedTuning, tuningErrors, withField, withPreset } from './tuning';

const defaults: RetrievalDefaults = {
  topK: 6,
  minScore: 0.25,
  maxTokens: 3000,
  ranges: { topK: [1, 10], minScore: [0, 1], maxTokens: [500, 8000] },
  presets: [{ name: 'precise', topK: 3, minScore: 0.35 }, { name: 'balanced', topK: 6, minScore: 0.25 }, { name: 'broad', topK: 8, minScore: 0.1 }],
};
const unset: KnowledgeTuning = { topK: 0, maxTokens: 0 };

describe('knowledge tuning', () => {
  it('fills unset values from the server\'s recommended ones, keeping 0 as a real minimum relevance', () => {
    expect(effectiveTuning(unset, defaults)).toEqual({ topK: 6, minScore: 0.25, maxTokens: 3000 });
    expect(effectiveTuning({ topK: 2, minScore: 0, maxTokens: 900 }, defaults)).toEqual({ topK: 2, minScore: 0, maxTokens: 900 });
  });

  it('names the preset the effective values match, and custom otherwise', () => {
    expect(presetOf(unset, defaults)).toBe('balanced');
    expect(presetOf({ topK: 3, minScore: 0.35, maxTokens: 0 }, defaults)).toBe('precise');
    expect(presetOf({ topK: 3, minScore: 0.35, maxTokens: 6000 }, defaults), 'the budget is not part of a preset').toBe('precise');
    expect(presetOf({ topK: 7, maxTokens: 0 }, defaults), 'no preset matches').toBeNull();
    expect(presetOf({ topK: 3, minScore: 0.35000001, maxTokens: 0 }, defaults), 'float tolerance').toBe('precise');
    const odd = { ...defaults, presets: [{ name: 'balanced', topK: 4, minScore: 0.3 }, { name: 'custom', topK: 9, minScore: 0.05 }] };
    expect(presetOf(unset, odd), 'unset is Balanced even when Balanced differs from recommended').toBe('balanced');
    expect(presetOf({ topK: 9, minScore: 0.05, maxTokens: 0 }, odd), 'a server preset named custom is still a preset').toBe('custom');
  });

  it('stores a preset\'s values, leaving the ones that equal the recommended ones unset, and keeps the budget', () => {
    expect(withPreset({ topK: 7, minScore: 0.5, maxTokens: 900 }, defaults.presets[1]!, defaults)).toEqual({ topK: 0, maxTokens: 900 });
    expect(withPreset(unset, defaults.presets[0]!, defaults)).toEqual({ topK: 3, minScore: 0.35, maxTokens: 0 });
    expect(withPreset(unset, { name: 'b', topK: 6, minScore: 0.25000001 }, defaults), 'float tolerance').toEqual({ topK: 0, maxTokens: 0 });
  });

  it('resets to recommended as 0, 0 and no minimum relevance', () => {
    expect(recommendedTuning()).toEqual({ topK: 0, maxTokens: 0 });
    expect(recommendedTuning()).not.toHaveProperty('minScore');
  });

  it('flags values outside the server\'s ranges, and lets unset ones pass', () => {
    expect(tuningErrors(unset, defaults)).toEqual({});
    expect(tuningErrors({ topK: 11, minScore: 1.5, maxTokens: 100 }, defaults)).toEqual({ topK: [1, 10], minScore: [0, 1], maxTokens: [500, 8000] });
    expect(tuningErrors({ topK: 10, minScore: 0, maxTokens: 8000 }, defaults)).toEqual({});
  });

  it('commits a typed value only when it is a whole, in-range number, and empty means recommended', () => {
    const set: KnowledgeTuning = { topK: 4, minScore: 0.3, maxTokens: 900 };
    expect(withField(set, 'topK', '', defaults)).toEqual({ tuning: { ...set, topK: 0 } });
    expect(withField(set, 'minScore', ' ', defaults)).toEqual({ tuning: { topK: 4, maxTokens: 900 } });
    expect(withField(set, 'minScore', '0', defaults)).toEqual({ tuning: { ...set, minScore: 0 } });
    expect(withField(set, 'maxTokens', '2500', defaults)).toEqual({ tuning: { ...set, maxTokens: 2500 } });
    expect(withField(set, 'topK', '11', defaults)).toEqual({ error: [1, 10] });
    expect(withField(set, 'topK', '2.5', defaults)).toEqual({ error: [1, 10] });
    expect(withField(set, 'minScore', 'abc', defaults)).toEqual({ error: [0, 1] });
    expect(withField(set, 'topK', '0', defaults), '0 is not the recommended value when typed').toEqual({ error: [1, 10] });
    expect(withField(set, 'maxTokens', '0', defaults)).toEqual({ error: [500, 8000] });
    expect(withField(set, 'minScore', '0.05', defaults)).toEqual({ tuning: { ...set, minScore: 0.05 } });
  });

  it('labels a preset from its locale key, or the server\'s name capitalized when there is none', () => {
    const t = (k: string) => `t:${k}`;
    const te = (k: string) => k === 'configure.preset_precise';
    expect(presetLabel('precise', t, te)).toBe('t:configure.preset_precise');
    expect(presetLabel('deep', t, te)).toBe('Deep');
    expect(presetLabel('', t, te)).toBe('');
  });
});
