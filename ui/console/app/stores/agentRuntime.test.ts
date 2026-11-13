import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it } from 'vitest';
import { sampleAgentRuntime } from '~/preview/samples';
import { emptyRuntime } from '~/stores/agentRuntime';
import { useAgentRuntimeStore } from './agentRuntime';

describe('useAgentRuntimeStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('seeds the sample agent with the sample config and every other agent empty, once', () => {
    const store = useAgentRuntimeStore();
    expect(store.draftFor('a1', 'yasaku-keuangan')).toEqual(sampleAgentRuntime());
    expect(store.draftFor('a2', 'support')).toEqual(emptyRuntime());
    store.drafts.a2!.webFetch = true;
    expect(store.draftFor('a2', 'support').webFetch).toBe(true);
    expect(Object.keys(emptyRuntime())).toEqual(['webFetch', 'allowedDomains', 'webSearch']);
    expect(store.usesSample('a1')).toBe(true);
    expect(store.usesSample('a2')).toBe(false);
    expect(store.draftFor('a1', 'yasaku-keuangan')).toBe(store.drafts.a1);
    expect(store.sampled).toEqual(['a1']);
  });
});
