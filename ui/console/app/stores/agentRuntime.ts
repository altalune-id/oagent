import { sampleAgentRuntime, sampleAgentSlug } from '~/preview/samples';

export interface AgentRuntimeConfig {
  webFetch: boolean;
  allowedDomains: string[];
  webSearch: boolean;
  memory: boolean;
  knowledgeIds: string[];
}

export function emptyRuntime(): AgentRuntimeConfig {
  return { webFetch: false, allowedDomains: [], webSearch: false, memory: false, knowledgeIds: [] };
}

// TODO: replace with the web, memory and knowledge RPCs; until then this keeps each agent's draft of them.
export const useAgentRuntimeStore = defineStore('agentRuntime', () => {
  const drafts = ref<Record<string, AgentRuntimeConfig>>({});
  const sampled = ref<string[]>([]);

  function draftFor(agentId: string, slug: string): AgentRuntimeConfig {
    if (!drafts.value[agentId]) {
      const sample = slug === sampleAgentSlug;
      drafts.value[agentId] = sample ? sampleAgentRuntime() : emptyRuntime();
      if (sample) {
        sampled.value.push(agentId);
      }
    }
    return drafts.value[agentId];
  }

  function usesSample(agentId: string): boolean {
    return sampled.value.includes(agentId);
  }

  return { drafts, sampled, draftFor, usesSample };
});
