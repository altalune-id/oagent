// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { defineComponent, h } from 'vue';
import { fakeKnowledgeClient, projectId } from '~~/test/fixtures/knowledge';
import { pollIntervalMs, useKnowledgeStore } from '~/stores/knowledge';
import { useKnowledgePolling } from './useKnowledgePolling';

const uploadSource = 'Ks1Upl0adQ8sWx3YcLm7D';
let client: ReturnType<typeof fakeKnowledgeClient>;

function owner(sourceId = '') {
  let api!: ReturnType<typeof useKnowledgePolling>;
  const w = mount(defineComponent({
    setup() {
      api = useKnowledgePolling(projectId, sourceId);
      return () => h('div');
    },
  }));
  return { w, api };
}

beforeEach(async () => {
  setActivePinia(createPinia());
  client = fakeKnowledgeClient();
  vi.useFakeTimers();
  await useKnowledgeStore().load(projectId);
});

afterEach(() => {
  vi.useRealTimers();
});

describe('useKnowledgePolling', () => {
  it('keeps the next page polling when the previous page unmounts after it set up', async () => {
    const a = owner();
    const b = owner(uploadSource);
    a.w.unmount();
    await vi.advanceTimersByTimeAsync(pollIntervalMs);
    expect(client.listKnowledgeDocuments).toHaveBeenCalledWith({ sourceId: uploadSource });
    b.w.unmount();
  });

  it('restarts on demand while mounted, and never after unmount', async () => {
    const a = owner();
    a.api.start();
    await vi.advanceTimersByTimeAsync(pollIntervalMs);
    expect(client.listKnowledgeSources).toHaveBeenCalledTimes(2);
    a.w.unmount();
    a.api.start();
    await vi.advanceTimersByTimeAsync(pollIntervalMs * 3);
    expect(client.listKnowledgeSources).toHaveBeenCalledTimes(2);
  });
});
