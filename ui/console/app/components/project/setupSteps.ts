export interface SetupProgress {
  agentConfigured: boolean;
  tried: boolean;
  published: boolean;
  whatsappConnected: boolean;
}

export type SetupState = 'done' | 'next' | 'todo';

export interface SetupStep {
  key: string;
  labelKey: string;
  to: string;
  state: SetupState;
}

export interface SetupInput extends SetupProgress {
  base: string;
  agentSlug?: string;
  providers: number;
  mcpServers: number;
}

// setupSteps lists the project's first-run journey in order and marks the first unfinished step as next.
export function setupSteps(input: SetupInput): SetupStep[] {
  const agent = input.agentSlug ? `${input.base}/agents/${input.agentSlug}` : `${input.base}/agents`;
  const configured = !!input.agentSlug && input.agentConfigured;
  const steps: [string, string, boolean][] = [
    ['provider', `${input.base}/settings/model-providers`, input.providers > 0],
    ['mcp', `${input.base}/settings/mcp-servers`, input.mcpServers > 0],
    ['agent', agent, configured],
    ['try', input.agentSlug ? `${agent}/try` : agent, configured && input.tried],
    ['publish', agent, configured && input.published],
    ['channel', input.agentSlug ? `${agent}/channels` : agent, configured && input.whatsappConnected],
  ];
  const next = steps.findIndex(([, , done]) => !done);
  return steps.map(([key, to, done], i) => ({ key, labelKey: `setup.step_${key}`, to, state: done ? 'done' : i === next ? 'next' : 'todo' }));
}
