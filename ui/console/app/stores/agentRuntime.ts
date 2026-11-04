import { sampleAgentRuntime, sampleAgentSlug } from '~/preview/samples';

export type AskWhen = 'never' | 'always' | 'match';

export interface ToolGrant {
  serverId: string;
  serverName: string;
  tool: string;
  ask: AskWhen;
  matchArg?: string;
  matchValue?: boolean;
}

export interface AgentRuntimeConfig {
  providerId: string;
  model: string;
  helperModel: string;
  grants: ToolGrant[];
  webFetch: boolean;
  allowedDomains: string[];
  webSearch: boolean;
  memory: boolean;
  knowledgeIds: string[];
}

export function emptyRuntime(): AgentRuntimeConfig {
  return { providerId: '', model: '', helperModel: '', grants: [], webFetch: false, allowedDomains: [], webSearch: false, memory: false, knowledgeIds: [] };
}

// TODO: replace with the runtime config RPCs; until then this keeps each agent's provider, tools, web, memory and knowledge draft.
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
