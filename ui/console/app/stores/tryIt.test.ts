import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it } from 'vitest';
import { useTryItStore } from './tryIt';

describe('useTryItStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('offers prompts and plays a script', () => {
    const store = useTryItStore();
    expect(store.prompts.length).toBeGreaterThan(0);
    expect(store.script(store.prompts[0]!).steps.length).toBeGreaterThan(0);
  });
});
