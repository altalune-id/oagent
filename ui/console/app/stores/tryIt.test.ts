import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it } from 'vitest';
import { useTryItStore } from './tryIt';

describe('useTryItStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('offers prompts, plays a script and remembers which agents were tried', () => {
    const store = useTryItStore();
    expect(store.prompts.length).toBeGreaterThan(0);
    expect(store.script(store.prompts[0]!).steps.length).toBeGreaterThan(0);
    store.markTried('a1');
    store.markTried('a1');
    expect(store.tried).toEqual(['a1']);
  });
});
