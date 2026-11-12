import { MessageSchema, UserMessageSchema } from '@ag-ui/core/schemas';
import { z } from 'zod';

export const CONTRACT_VERSION = 3;
export const IMAGE_CONTRACT_VERSION = 2;
export const APPROVAL_CONTRACT_VERSION = 3;

export const ModelProvider = {
  OPENAI_COMPATIBLE: 'openai_compatible',
  ANTHROPIC: 'anthropic',
} as const;

export const MCPAuthKind = {
  NONE: 'none',
  BEARER: 'bearer',
  HEADER: 'header',
} as const;

export const ToolResultStatus = {
  SUCCESS: 'success',
  ERROR: 'error',
} as const;

const ms = z.number().int().positive();
const micros = z.number().int().nonnegative();

const ModelSpec = z.strictObject({
  provider: z.enum(ModelProvider).default(ModelProvider.OPENAI_COMPATIBLE),
  baseUrl: z.string().min(1),
  apiKey: z.string().min(1),
  modelId: z.string().min(1),
  maxTokens: z.number().int().positive().optional(),
});

const MCPAuth = z.strictObject({
  kind: z.enum(MCPAuthKind),
  header: z.string().min(1).optional(),
  value: z.string().min(1).optional(),
});

const FreeCondition = z.strictObject({
  arg: z.string().regex(/^[A-Z_]\w{0,63}$/i),
  values: z.array(z.unknown()).max(16),
  absent: z.boolean().optional(),
});

export const ApprovalSpecSchema = z.discriminatedUnion('mode', [
  z.strictObject({ mode: z.literal('never') }),
  z.strictObject({ mode: z.literal('always') }),
  z.strictObject({ mode: z.literal('conditional'), freeWhen: z.array(FreeCondition).min(1).max(8) }),
]);

export type ApprovalSpec = z.infer<typeof ApprovalSpecSchema>;

const MCPServerSpec = z.strictObject({
  name: z.string().min(1),
  url: z.string().min(1),
  auth: MCPAuth,
  allowedTools: z.array(z.string().min(1)).optional(),
  tools: z.array(z.strictObject({ name: z.string().min(1), approval: ApprovalSpecSchema })).optional(),
  connectTimeoutMs: ms,
  callTimeoutMs: ms,
});

const ToolResult = z.strictObject({
  status: z.enum(ToolResultStatus),
  content: z.array(z.record(z.string(), z.unknown())),
});

const TurnInput = z.discriminatedUnion('kind', [
  z.strictObject({ kind: z.literal('message'), message: UserMessageSchema }),
  z.strictObject({
    kind: z.literal('resume'),
    responses: z.array(z.strictObject({ interruptId: z.string().min(1), response: z.unknown() })).min(1),
  }),
  z.strictObject({ kind: z.literal('continue') }),
]);

const Budget = z.strictObject({
  remainingMicros: z.number().int(),
  prices: z.strictObject({ input: micros, output: micros, cacheRead: micros, cacheWrite: micros }),
});

const TurnRequestShape = z.strictObject({
  contractVersion: z.union([z.literal(1), z.literal(IMAGE_CONTRACT_VERSION), z.literal(APPROVAL_CONTRACT_VERSION)]),
  ids: z.strictObject({
    run: z.string().min(1),
    thread: z.string().min(1),
    agent: z.string().min(1),
    agentVersion: z.number().int().nonnegative(),
    attempt: z.number().int().positive(),
    project: z.string().min(1),
  }),
  agent: z.strictObject({
    instructions: z.string(),
    temperature: z.number().min(0).max(2),
    maxSteps: z.number().int().nonnegative(),
    model: ModelSpec,
  }),
  mcpServers: z.array(MCPServerSpec),
  snapshot: z.record(z.string(), z.unknown()).nullable(),
  history: z.array(MessageSchema),
  input: TurnInput,
  storedResults: z.array(z.strictObject({ toolUseId: z.string().min(1), result: ToolResult })),
  limits: z.strictObject({ deadlineMs: ms, budget: Budget.nullable() }),
});

function isJsonObjectText(text: string): boolean {
  if (text === '') {
    return true;
  }
  try {
    const v: unknown = JSON.parse(text);
    return v !== null && typeof v === 'object' && !Array.isArray(v);
  } catch {
    return false;
  }
}

export const TurnRequestSchema = TurnRequestShape.superRefine((req, ctx) => {
  req.mcpServers.forEach((server, i) => {
    const v3 = req.contractVersion >= APPROVAL_CONTRACT_VERSION;
    if (v3 ? server.tools === undefined || server.allowedTools !== undefined : server.allowedTools === undefined || server.tools !== undefined) {
      ctx.addIssue({ code: 'custom', path: ['mcpServers', i], message: v3 ? 'v3 takes tools with approval specs' : 'v1 and v2 take allowedTools' });
    }
  });
  if (req.input.kind === 'resume' && req.snapshot === null) {
    ctx.addIssue({ code: 'custom', path: ['snapshot'], message: 'resume needs the snapshot it resumes' });
  }
  if (req.input.kind === 'continue' && (req.snapshot !== null || req.history.at(-1)?.role !== 'tool')) {
    ctx.addIssue({ code: 'custom', path: ['input', 'kind'], message: 'continue needs no snapshot and a history ending in a tool result' });
  }
  req.history.forEach((m, i) => {
    if (m.role !== 'assistant') {
      return;
    }
    (m.toolCalls ?? []).forEach((call, j) => {
      if (!isJsonObjectText(call.function.arguments)) {
        ctx.addIssue({ code: 'custom', path: ['history', i, 'toolCalls', j, 'function', 'arguments'], message: 'not a JSON object' });
      }
    });
  });
});

export type TurnRequest = z.infer<typeof TurnRequestSchema>;

export type Validation = { ok: true, request: TurnRequest } | { ok: false, fields: string[] };

export function validateRequest(raw: unknown): Validation {
  const parsed = TurnRequestSchema.safeParse(raw);
  if (parsed.success) {
    return { ok: true, request: parsed.data };
  }
  const fields = new Set<string>();
  for (const issue of parsed.error.issues) {
    const base = issue.path.map(String);
    if (issue.code === 'unrecognized_keys') {
      for (const key of issue.keys) {
        fields.add([...base, key].join('.'));
      }
      continue;
    }
    fields.add(base.join('.'));
  }
  return { ok: false, fields: [...fields] };
}
