import type { ApprovalSpec, TurnRequest } from '../contract/index.ts';
import type { ToolGate } from './approval.ts';

export type WebToolName = 'web_search' | 'web_fetch' | 'http_request';

export const WEB_SERVER = 'web_tools';

export const WEB_APPROVAL: Record<WebToolName, ApprovalSpec> = {
  web_search: { mode: 'never' },
  web_fetch: { mode: 'never' },
  http_request: { mode: 'conditional', freeWhen: [{ arg: 'method', values: ['GET'] }] },
};

export function webApprovalSpecs(request: TurnRequest): Map<string, ToolGate> {
  const web = request.web;
  const present: WebToolName[] = [
    ...(web?.search ? ['web_search' as const] : []),
    ...(web?.fetch ? ['web_fetch' as const] : []),
    ...(web?.http ? ['http_request' as const] : []),
  ];
  return new Map(present.map(tool => [tool, { server: WEB_SERVER, tool, approval: WEB_APPROVAL[tool] }]));
}
