import type { InterventionHandler } from '@strands-agents/sdk';
import type { TurnRequest } from '../contract/index.ts';
import type { ToolGate } from './approval.ts';

import { TurnRefusedError } from '../contract/index.ts';
import { APPROVAL_HANDLER, ApprovalCheck } from './approval.ts';

export function approvalSpecs(request: TurnRequest): Map<string, ToolGate> {
  const specs = new Map<string, ToolGate>();
  for (const server of request.mcpServers) {
    for (const tool of server.tools ?? []) {
      specs.set(`${server.name}__${tool.name}`, { server: server.name, tool: tool.name, approval: tool.approval });
    }
    for (const name of server.allowedTools ?? []) {
      specs.set(`${server.name}__${name}`, { server: server.name, tool: name, approval: { mode: 'never' } });
    }
  }
  return specs;
}

export function interventionsFor(specs: Map<string, ToolGate>): InterventionHandler[] {
  return [new ApprovalCheck(specs)];
}

export function checkApprovalReady(request: TurnRequest, handlers: InterventionHandler[]): void {
  const gated = [...approvalSpecs(request).values()].some(g => g.approval.mode !== 'never');
  if (gated && !handlers.some(h => h.name === APPROVAL_HANDLER)) {
    throw new TurnRefusedError(422, 'approval_unavailable');
  }
}
