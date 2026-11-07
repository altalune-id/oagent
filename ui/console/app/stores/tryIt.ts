import type { ApprovalRequest } from '~/stores/approvals';
import { sampleChatScript, samplePrompts } from '~/preview/samples';

export type ChatStep
  = | { kind: 'tool', name: string, args: Record<string, unknown>, result: unknown, preview?: boolean }
    | { kind: 'text', text: string }
    | { kind: 'approval', approval: ApprovalRequest };

export interface ChatScript {
  steps: ChatStep[];
  afterApprove: ChatStep[];
  afterReject: ChatStep[];
  tokensPerStep: number;
  centsPerStep: number;
}

// TODO: replace with the console test-run RPC; until then Try it plays a scripted reply.
export const useTryItStore = defineStore('tryIt', () => {
  const prompts = ref<string[]>(samplePrompts);

  function script(prompt: string): ChatScript {
    return sampleChatScript(prompt);
  }

  return { prompts, script };
});
