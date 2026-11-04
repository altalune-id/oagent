// NOTE: the only home of sample data; replace each export with the runtime and tool registry RPCs as they land.

export interface ToolInfo {
  name: string;
  description: string;
}

export interface SampleAgent {
  slug: string;
  name: string;
  description: string;
  version: number;
  draft: boolean;
  model: string;
  runs7d: number;
  medianSeconds: number;
  costCents7d: number;
  updatedAt: Date;
}

export type RunEventType = 'input' | 'model' | 'tool' | 'approval' | 'error' | 'output';

export interface RunEvent {
  id: string;
  type: RunEventType;
  name: string;
  at: number;
  took: number;
  costCents?: number;
  tokens?: number;
  summary: string;
  input?: string;
  output?: string;
}

export interface SampleRun {
  id: string;
  agentName: string;
  version: number;
  model: string;
  startedBy: string;
  startedAt: Date;
  maxSteps: number;
  approval: { tool: string, title: string, why: string, amountCents: number };
  events: RunEvent[];
  afterApprove: RunEvent[];
  afterReject: RunEvent[];
}

export const sampleModels = ['large-2', 'small-3'];

export const sampleTools: ToolInfo[] = [
  { name: 'orders.lookup', description: 'Read an order by number' },
  { name: 'billing.charges.list', description: 'List charges on an order' },
  { name: 'billing.refunds.create', description: 'Create a refund' },
  { name: 'knowledge.search', description: 'Search support articles' },
];

function daysAgo(days: number): Date {
  return new Date(Date.now() - days * 86_400_000);
}

export function sampleAgents(): SampleAgent[] {
  return [
    { slug: 'refund-triage', name: 'Refund Triage', description: 'Checks duplicate charges and prepares refunds', version: 12, draft: true, model: 'large-2', runs7d: 1842, medianSeconds: 41, costCents7d: 386, updatedAt: daysAgo(0.1) },
    { slug: 'order-status', name: 'Order Status', description: 'Answers where-is-my-order questions', version: 31, draft: false, model: 'small-3', runs7d: 12904, medianSeconds: 3, costCents7d: 1142, updatedAt: daysAgo(1) },
    { slug: 'ticket-router', name: 'Ticket Router', description: 'Sends each ticket to the right queue', version: 8, draft: false, model: 'small-3', runs7d: 6120, medianSeconds: 6, costCents7d: 810, updatedAt: daysAgo(5) },
    { slug: 'knowledge-answers', name: 'Knowledge Answers', description: 'Answers from support articles', version: 19, draft: false, model: 'large-2', runs7d: 3377, medianSeconds: 18, costCents7d: 2475, updatedAt: daysAgo(8) },
  ];
}

export function sampleRun(): SampleRun {
  return {
    id: 'r7Xp2KqLvN9sT4bWzY1cD',
    agentName: 'Refund Triage',
    version: 12,
    model: 'large-2',
    startedBy: 'API',
    startedAt: new Date(Date.now() - 52_000),
    maxSteps: 12,
    approval: { tool: 'billing.refunds.create', title: 'Refund $48.00 on order 2291', why: 'This tool always needs approval.', amountCents: 4800 },
    events: [
      { id: 'e1', type: 'input', name: 'Input', at: 0, took: 0, summary: 'Order 2291 was charged twice for $48.00. Please fix.', input: 'Order 2291 was charged twice for $48.00. Please fix.' },
      { id: 'e2', type: 'model', name: 'Plan', at: 0.2, took: 2.8, costCents: 0.4, tokens: 812, summary: 'Check the order, then its charges.', output: 'Check the order, then its charges.' },
      { id: 'e3', type: 'tool', name: 'orders.lookup', at: 3.1, took: 0.8, summary: 'Order 2291, $48.00, delivered.', input: '{ "order": "2291" }', output: '{ "order": "2291", "total": 4800, "status": "delivered" }' },
      { id: 'e4', type: 'tool', name: 'billing.charges.list', at: 4.0, took: 1.2, summary: 'Two charges, 11 seconds apart.', input: '{ "order": "2291" }', output: '[{ "amount": 4800 }, { "amount": 4800 }]' },
      { id: 'e5', type: 'model', name: 'Reason', at: 5.3, took: 3.8, costCents: 0.6, tokens: 1204, summary: 'Duplicate charge. Preview a refund of the second charge.' },
      { id: 'e6', type: 'error', name: 'billing.refunds.preview', at: 9.2, took: 5, summary: 'Upstream returned 502 after 5.0 s.' },
      { id: 'e7', type: 'tool', name: 'billing.refunds.preview', at: 14.4, took: 0.8, summary: 'Retry worked. Refund of $48.00 is allowed.' },
      { id: 'e8', type: 'model', name: 'Reason', at: 15.3, took: 4.3, costCents: 0.6, tokens: 990, summary: 'Refund needs a person. Request approval.' },
      { id: 'e9', type: 'approval', name: 'billing.refunds.create', at: 19.7, took: 0, summary: 'Refund $48.00 on order 2291' },
    ],
    afterApprove: [
      { id: 'e10', type: 'tool', name: 'billing.refunds.create', at: 71.4, took: 1.2, summary: 'Refund created.' },
      { id: 'e11', type: 'output', name: 'Final reply', at: 72.7, took: 4.7, costCents: 0.8, tokens: 640, summary: 'Tells the customer the duplicate charge is refunded.' },
    ],
    afterReject: [
      { id: 'e10', type: 'output', name: 'Rejected', at: 66.0, took: 0.4, summary: 'The refund was rejected. The agent stopped.' },
    ],
  };
}
